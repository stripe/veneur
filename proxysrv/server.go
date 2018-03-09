// Package proxysrv proxies metrics over gRPC to global Veneur's using
// consistent hashing
//
// The Server provided accepts a hash ring of destinations, and then listens
// for metrics over gRPC.  It hashes each metric to a specific destination,
// and forwards each metric to its appropriate destination Veneur.
package proxysrv

import (
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/golang/protobuf/ptypes/empty"
	"github.com/sirupsen/logrus"
	"github.com/stripe/veneur/forwardrpc"
	"github.com/stripe/veneur/samplers"
	"github.com/stripe/veneur/samplers/metricpb"
	"github.com/stripe/veneur/trace/metrics"

	"github.com/stripe/veneur/ssf"
	"github.com/stripe/veneur/trace"
	"google.golang.org/grpc"
	"stathat.com/c/consistent"
)

// Server is a gRPC server that implements the forwardrpc.Forward service.
// It receives metrics and forwards them consistently to a destination, based
// on the metric name, type and tags.
type Server struct {
	*grpc.Server
	destinations *consistent.Consistent
	opts         *options
	conns        *clientConnMap
	updateMtx    sync.Mutex
}

// Option modifies an internal options type.
type Option func(*options)

type options struct {
	log            *logrus.Entry
	forwardTimeout time.Duration
	traceClient    *trace.Client
}

// New creates a new Server with the provided destinations. The server returned
// is unstarted.
func New(destinations *consistent.Consistent, opts ...Option) (*Server, error) {
	res := &Server{
		Server: grpc.NewServer(),
		opts:   &options{},
		conns:  newClientConnMap(grpc.WithInsecure()),
	}

	for _, opt := range opts {
		opt(res.opts)
	}

	if res.opts.log == nil {
		log := logrus.New()
		log.Out = ioutil.Discard
		res.opts.log = logrus.NewEntry(log)
	}

	if err := res.SetDestinations(destinations); err != nil {
		return nil, fmt.Errorf("failed to set the destinations: %v", err)
	}

	forwardrpc.RegisterForwardServer(res.Server, res)

	return res, nil
}

// Serve starts a gRPC listener on the specified address and blocks while
// listening for requests. If listening is iterrupted by some means other than
// Stop or GracefulStop being called, it returns a non-nil error.
func (s *Server) Serve(addr string) (err error) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to bind the proxy server to '%s': %v",
			addr, err)
	}
	defer func() {
		if cerr := ln.Close(); err == nil && cerr != nil {
			err = cerr
		}
	}()

	return s.Server.Serve(ln)
}

// Stop stops the gRPC server (if it was started) and closes all gRPC client
// connections
func (s *Server) Stop() {
	s.Server.Stop()
	s.conns.Clear()
}

// SetDestinations updates the ring of hosts that are forwarded to by
// the server.  If new hosts are being added, a gRPC connection is initialized
// for each.
//
// This also prunes the list of open connections.  If a connection exists to
// a host that wasn't in either the current list or the last one, the
// connection is closed.
func (s *Server) SetDestinations(dests *consistent.Consistent) error {
	s.updateMtx.Lock()
	defer s.updateMtx.Unlock()

	var current []string
	if s.destinations != nil {
		current = s.destinations.Members()
	}
	new := dests.Members()

	// for every connection in the map that isn't in either the current or
	// previous list of destinations, delete it
	for _, k := range s.conns.Keys() {
		if !strInSlice(k, current) && !strInSlice(k, new) {
			s.conns.Delete(k)
		}
	}

	// create a connection for each destination
	for _, dest := range new {
		if err := s.conns.Add(dest); err != nil {
			return fmt.Errorf("failed to setup a connection for the "+
				"destination '%s': %v", dest, err)
		}
	}

	s.destinations = dests
	return nil
}

// SendMetrics spawns a new goroutine that forwards metrics to the destinations
// and exist immediately.
func (s *Server) SendMetrics(ctx context.Context, mlist *forwardrpc.MetricList) (*empty.Empty, error) {
	go func() {
		_ = s.sendMetrics(context.Background(), mlist)
	}()
	return &empty.Empty{}, nil
}

func (s *Server) sendMetrics(ctx context.Context, mlist *forwardrpc.MetricList) error {
	span, _ := trace.StartSpanFromContext(ctx, "veneur.opentracing.proxysrv.send_metrics")
	defer span.ClientFinish(s.opts.traceClient)

	if s.opts.forwardTimeout > 0 {
		var cancel func()
		ctx, cancel = context.WithTimeout(ctx, s.opts.forwardTimeout)
		defer cancel()
	}
	metrics := mlist.Metrics
	span.Add(ssf.Count("import.metrics_total", float32(len(metrics)), map[string]string{
		"veneurglobalonly": "",
		"protocol":         "grpc",
	}))

	var errs []error

	dests := make(map[string][]*metricpb.Metric)
	for _, metric := range metrics {
		dest, err := s.destForMetric(metric)
		if err != nil {
			errs = append(errs, s.recordError(span, err, "no-destination",
				"failed to get a destination for a metric", 1))
		} else {
			// Lazily initialize keys in the map as necessary
			if _, ok := dests[dest]; !ok {
				dests[dest] = make([]*metricpb.Metric, 0, 1)
			}
			dests[dest] = append(dests[dest], metric)
		}
	}

	// Wait for all of the forward to finish
	wg := sync.WaitGroup{}
	wg.Add(len(dests))

	for dest, batch := range dests {
		go func(dest string, batch []*metricpb.Metric) {
			defer wg.Done()
			if err := s.forward(ctx, dest, batch); err != nil {
				errs = append(errs, s.recordError(span, err, "forward",
					fmt.Sprintf("failed to forward %d metrics to the host '%s'",
						len(batch), dest),
					len(batch)))
			}
		}(dest, batch)
	}

	wg.Wait() // Wait for all the above goroutines to complete

	protocolTags := map[string]string{"protocol": "grpc"}
	span.Add(ssf.RandomlySample(0.1,
		ssf.Timing("proxy.duration_ns", time.Since(span.Start), time.Nanosecond,
			protocolTags),
		ssf.Count("proxy.proxied_metrics_total", float32(len(metrics)), protocolTags),
	)...)

	s.opts.log.WithFields(logrus.Fields{
		"protocol": "grpc",
		"duration": time.Since(span.Start),
	}).Info("Completed forwarding to downstream Veneurs")

	return errSliceToErr(errs)
}

// recordError records when an error has resulted in some metrics not being
// forwarded.  It submits diagnostic metrics, logs an error, and then returns
// a wrapped error.
func (s *Server) recordError(
	span *trace.Span,
	err error,
	cause string,
	message string,
	numMetrics int,
) error {
	tags := map[string]string{
		"cause":    cause,
		"protocol": "grpc",
	}
	span.Add(ssf.Count("proxy.proxied_metrics_failed", float32(numMetrics), tags))
	span.Add(ssf.Count("proxy.forward_errors", 1, tags))
	s.opts.log.WithError(err).WithFields(logrus.Fields{
		"cause": cause,
	}).Error(message)

	return fmt.Errorf("%s: %v", message, err)
}

// destForMetric returns a destination for the input metric.
func (s *Server) destForMetric(m *metricpb.Metric) (string, error) {
	key := samplers.NewMetricKeyFromMetric(m)
	dest, err := s.destinations.Get(key.String())
	if err != nil {
		return "", fmt.Errorf("failed to hash the MetricKey '%s' to a "+
			"destination: %v", key.String(), err)
	}

	return dest, nil
}

// forward sends a set of metrics to the destination address, and returns
// an error if necessary.
func (s *Server) forward(ctx context.Context, dest string, ms []*metricpb.Metric) (err error) {
	conn, ok := s.conns.Get(dest)
	if !ok {
		return fmt.Errorf("no connection was found for the host '%s'", dest)
	}

	c := forwardrpc.NewForwardClient(conn)
	_, err = c.SendMetrics(ctx, &forwardrpc.MetricList{Metrics: ms})
	if err != nil {
		return fmt.Errorf("failed to send %d metrics over gRPC: %v",
			len(ms), err)
	}

	_ = metrics.ReportBatch(s.opts.traceClient, ssf.RandomlySample(0.1,
		ssf.Gauge("metrics_by_destination", float32(len(ms)),
			map[string]string{"destination": dest}),
	))

	return nil
}

// errSliceToErr merges a slice of errors into a single error.  If errs is
// empty, it returns nil
func errSliceToErr(errs []error) error {
	if len(errs) == 0 {
		return nil
	}

	// convert the errors into a slice of strings
	strs := make([]string, len(errs))
	for i, err := range errs {
		strs[i] = err.Error()
	}

	return errors.New(strings.Join(strs, "\n * "))
}

func strInSlice(s string, slice []string) bool {
	for _, val := range slice {
		if val == s {
			return true
		}
	}
	return false
}
