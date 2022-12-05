// Package proxysrv proxies metrics over gRPC to global veneurs using
// consistent hashing
//
// The Server provided accepts a hash ring of destinations, and then listens
// for metrics over gRPC.  It hashes each metric to a specific destination,
// and forwards each metric to its appropriate destination Veneur.
package proxysrv

import (
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"context"

	"github.com/golang/protobuf/ptypes/empty"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"
	"stathat.com/c/consistent"

	"github.com/stripe/veneur/v14/forwardrpc"
	"github.com/stripe/veneur/v14/samplers"
	"github.com/stripe/veneur/v14/samplers/metricpb"
	"github.com/stripe/veneur/v14/ssf"
	"github.com/stripe/veneur/v14/trace"
	"github.com/stripe/veneur/v14/trace/metrics"
	"github.com/stripe/veneur/v14/util/matcher"
)

const (
	// By default, cancel forwarding operations after 10 seconds
	defaultForwardTimeout = 10 * time.Second

	// Report server statistics every 10 seconds
	defaultReportStatsInterval = 10 * time.Second
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

	// A simple counter to track the number of goroutines spawned to handle
	// proxying metrics
	activeProxyHandlers *int64
}

// Option modifies an internal options type.
type Option func(*options)

type options struct {
	log            *logrus.Entry
	forwardTimeout time.Duration
	traceClient    *trace.Client
	statsInterval  time.Duration
	ignoredTags    []matcher.TagMatcher
	streaming      bool
}

// New creates a new Server with the provided destinations. The server returned
// is unstarted.
func New(destinations *consistent.Consistent, opts ...Option) (*Server, error) {
	res := &Server{
		Server: grpc.NewServer(),
		opts: &options{
			forwardTimeout: defaultForwardTimeout,
			statsInterval:  defaultReportStatsInterval,
		},
		conns:               newClientConnMap(grpc.WithInsecure()),
		activeProxyHandlers: new(int64),
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
// listening for requests. If listening is interrupted by some means other than
// Stop or GracefulStop being called, it returns a non-nil error.
func (s *Server) Serve(addr string) error {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to bind the proxy server to '%s': %v",
			addr, err)
	}
	defer ln.Close()

	// Start up a goroutine to periodically report statistics about the
	// server.
	done := make(chan struct{})
	ticker := time.NewTicker(s.opts.statsInterval)
	go func() {
		for {
			select {
			case <-done:
				ticker.Stop()
				return
			case <-ticker.C:
				s.reportStats()
			}
		}
	}()

	// Start the server and block.  This explicitly sets err so that the
	// deferred listener close can set an error if this didn't exit with one.
	err = s.Server.Serve(ln)

	// Close the statistics goroutine
	done <- struct{}{}
	return err
}

// Stop stops the gRPC server (if it was started) and closes all gRPC client
// connections.
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
		// Track the number of active goroutines in a counter
		atomic.AddInt64(s.activeProxyHandlers, 1)
		_ = s.sendMetrics(context.Background(), mlist)
		atomic.AddInt64(s.activeProxyHandlers, -1)
	}()
	return &empty.Empty{}, nil
}

func (s *Server) SendMetricsV2(
	server forwardrpc.Forward_SendMetricsV2Server,
) error {
	metrics := []*metricpb.Metric{}
	for {
		metric, err := server.Recv()
		if err == io.EOF {
			break
		} else if err != nil {
			return err
		}
		metrics = append(metrics, metric)
	}
	_, err := s.SendMetrics(context.Background(), &forwardrpc.MetricList{
		Metrics: metrics,
	})
	if err != nil {
		return err
	}
	return server.SendAndClose(&emptypb.Empty{})
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
	span.Add(ssf.Count("proxy.metrics_total", float32(len(metrics)), globalProtocolTags))

	var errs forwardErrors

	dests := make(map[string][]*metricpb.Metric)
	for _, metric := range metrics {
		dest, err := s.destForMetric(metric)
		if err != nil {
			errs = append(errs, forwardError{err: err, cause: "no-destination",
				msg: "failed to get a destination for a metric", numMetrics: 1})
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
	errCh := make(chan forwardError)

	for dest, batch := range dests {
		go func(dest string, batch []*metricpb.Metric) {
			defer wg.Done()
			if err := s.forward(ctx, dest, batch); err != nil {
				msg := fmt.Sprintf("failed to forward to the host '%s'", dest)
				errCh <- forwardError{err: err, cause: "forward", msg: msg,
					numMetrics: len(batch)}
			}
		}(dest, batch)
	}

	go func() {
		wg.Wait() // Wait for all the above goroutines to complete
		close(errCh)
	}()

	// read errors and block until all the forward complete
	for err := range errCh {
		errs = append(errs, err)
	}

	span.Add(ssf.RandomlySample(0.1,
		ssf.Timing("proxy.duration_ns", time.Since(span.Start), time.Nanosecond,
			protocolTags),
		ssf.Count("proxy.proxied_metrics_total", float32(len(metrics)), protocolTags),
	)...)

	var res error
	log := s.opts.log.WithFields(logrus.Fields{
		"protocol": "grpc",
		"duration": time.Since(span.Start),
	})

	if len(errs) > 0 {
		// if there were errors, report stats and log them
		for _, err := range errs {
			err.reportMetrics(span)
		}
		res = errs
		log.WithError(res).Error("Proxying failed")
	} else {
		// otherwise just print a success message
		log.Debug("Completed forwarding to downstream Veneurs")
	}

	return res
}

// destForMetric returns a destination for the input metric.
func (s *Server) destForMetric(m *metricpb.Metric) (string, error) {
	key := samplers.NewMetricKeyFromMetric(m, s.opts.ignoredTags)
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

	if s.opts.streaming {
		forwardStream, err := c.SendMetricsV2(ctx)
		if err != nil {
			return fmt.Errorf("failed to stream %d metrics over gRPC: %v",
				len(ms), err)
		}

		defer forwardStream.CloseAndRecv()

		for i, metric := range ms {
			err := forwardStream.Send(metric)
			if err != nil {
				return fmt.Errorf("failed to stream (%d/%d) metrics over gRPC: %v", len(ms)-i, len(ms), err)
			}
		}
	} else {
		_, err = c.SendMetrics(ctx, &forwardrpc.MetricList{Metrics: ms})
		if err != nil {
			return fmt.Errorf("failed to send %d metrics over gRPC: %v",
				len(ms), err)
		}
	}

	_ = metrics.ReportBatch(s.opts.traceClient, ssf.RandomlySample(0.1,
		ssf.Count("metrics_by_destination", float32(len(ms)),
			map[string]string{"destination": dest, "protocol": "grpc"}),
	))

	return nil
}

// reportStats reports statistics about the server to the internal trace client
func (s *Server) reportStats() {
	_ = metrics.ReportOne(s.opts.traceClient,
		ssf.Gauge("proxy.active_goroutines", float32(atomic.LoadInt64(s.activeProxyHandlers)), globalProtocolTags))
}

func strInSlice(s string, slice []string) bool {
	for _, val := range slice {
		if val == s {
			return true
		}
	}
	return false
}

// forwardError represents an error that caused a number of metrics to not be
// forwarded.  The type records the original error, as well as the number of
// metrics that failed forwarding.
type forwardError struct {
	err        error
	cause      string
	msg        string
	numMetrics int
}

// Error returns a summary of the data in a forwardError.
func (e forwardError) Error() string {
	return fmt.Sprintf("%s (cause=%s, metrics=%d): %v", e.msg, e.cause,
		e.numMetrics, e.err)
}

// reportMetrics adds various metrics to an input span.
func (e forwardError) reportMetrics(span *trace.Span) {
	tags := map[string]string{
		"cause":    e.cause,
		"protocol": "grpc",
	}
	span.Add(
		ssf.Count("proxy.proxied_metrics_failed", float32(e.numMetrics), tags),
		ssf.Count("proxy.forward_errors", 1, tags),
	)
}

// forwardErrors wraps a slice of errors and implements the "error" type.
type forwardErrors []forwardError

// Error prints the first 10 errors.
func (errs forwardErrors) Error() string {
	// Only print 10 errors
	strsLen := len(errs)
	if len(errs) > 10 {
		strsLen = 10
	}

	// convert the errors into a slice of strings
	strs := make([]string, strsLen)
	for i, err := range errs[:len(strs)] {
		strs[i] = err.Error()
	}

	// If there are errors that weren't printed, add a message to the end
	str := strings.Join(strs, "\n * ")
	if len(strs) < len(errs) {
		str += fmt.Sprintf("\nand %d more...", len(errs)-len(strs))
	}

	return str
}

// protocolTags and globalProtocolTags can be used as arguments to various
// ssf metric types.  These are declared at the package-level  here to avoid
// repeated operation allocations per request.
var protocolTags = map[string]string{"protocol": "grpc"}
var globalProtocolTags = map[string]string{
	"veneurglobalonly": "",
	"protocol":         "grpc",
}
