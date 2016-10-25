package veneur

import (
	"bytes"
	"compress/zlib"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/stripe/veneur/samplers"
	"github.com/stripe/veneur/ssf"
)

// Flush takes the slices of metrics, combines then and marshals them to json
// for posting to Datadog.
func (s *Server) Flush(interval time.Duration, metricLimit int) {
	// right now we have only one destination plugin
	// but eventually, this is where we would loop over our supported
	// destinations
	if s.IsLocal() {
		s.FlushLocal(interval, metricLimit)
	} else {
		s.FlushGlobal(interval, metricLimit)
	}
}

func (s *Server) FlushGlobal(interval time.Duration, metricLimit int) {
	go s.flushEventsChecks() // we can do all of this separately

	percentiles := s.HistogramPercentiles

	tempMetrics, ms := s.tallyMetrics(percentiles)

	// the global veneur instance is also responsible for reporting the sets
	// and global counters
	ms.totalLength += ms.totalSets
	ms.totalLength += ms.totalGlobalCounters

	finalMetrics := s.generateDDMetrics(interval, percentiles, tempMetrics, ms)

	s.reportMetricsFlushCounts(ms)

	s.reportGlobalMetricsFlushCounts(ms)

	go func() {
		for _, p := range s.getPlugins() {
			start := time.Now()
			err := p.Flush(finalMetrics, s.Hostname)
			s.statsd.TimeInMilliseconds(fmt.Sprintf("flush.plugins.%s.total_duration_ns", p.Name()), float64(time.Now().Sub(start).Nanoseconds()), []string{"part:post"}, 1.0)
			if err != nil {
				countName := fmt.Sprintf("flush.plugins.%s.error_total", p.Name())
				s.statsd.Count(countName, 1, []string{}, 1.0)
			}
			s.statsd.Gauge(fmt.Sprintf("flush.plugins.%s.post_metrics_total", p.Name()), float64(len(finalMetrics)), nil, 1.0)
		}
	}()

	s.flushRemote(finalMetrics, metricLimit)
}

// FlushLocal takes the slices of metrics, combines then and marshals them to json
// for posting to Datadog.
func (s *Server) FlushLocal(interval time.Duration, metricLimit int) {
	go s.flushEventsChecks() // we can do all of this separately
	go s.flushTraces() // this too!

	// don't publish percentiles if we're a local veneur; that's the global
	// veneur's job
	var percentiles []float64 = nil

	tempMetrics, ms := s.tallyMetrics(percentiles)

	finalMetrics := s.generateDDMetrics(interval, percentiles, tempMetrics, ms)

	s.reportMetricsFlushCounts(ms)

	// we don't report totalHistograms, totalSets, or totalTimers for local veneur instances

	// we cannot do this until we're done using tempMetrics within this function,
	// since not everything in tempMetrics is safe for sharing
	go s.flushForward(tempMetrics)

	go func() {
		for _, p := range s.getPlugins() {
			start := time.Now()
			err := p.Flush(finalMetrics, s.Hostname)
			s.statsd.TimeInMilliseconds(fmt.Sprintf("flush.plugins.%s.total_duration_ns", p.Name()), float64(time.Now().Sub(start).Nanoseconds()), []string{"part:post"}, 1.0)
			if err != nil {
				countName := fmt.Sprintf("flush.plugins.%s.error_total", p.Name())
				s.statsd.Count(countName, 1, []string{}, 1.0)
			}
			s.statsd.Gauge(fmt.Sprintf("flush.plugins.%s.post_metrics_total", p.Name()), float64(len(finalMetrics)), nil, 1.0)
		}
	}()

	s.flushRemote(finalMetrics, metricLimit)
}

type metricsSummary struct {
	totalCounters   int
	totalGauges     int
	totalHistograms int
	totalSets       int
	totalTimers     int

	totalGlobalCounters int

	totalLocalHistograms int
	totalLocalSets       int
	totalLocalTimers     int

	totalLength int
}

// tallyMetrics gives a slight overestimate of the number
// of metrics we'll be reporting, so that we can pre-allocate
// a slice of the correct length instead of constantly appending
// for performance
func (s *Server) tallyMetrics(percentiles []float64) ([]WorkerMetrics, metricsSummary) {
	// allocating this long array to count up the sizes is cheaper than appending
	// the []DDMetrics together one at a time
	tempMetrics := make([]WorkerMetrics, 0, len(s.Workers))

	gatherStart := time.Now()
	ms := metricsSummary{}

	for i, w := range s.Workers {
		log.WithField("worker", i).Debug("Flushing")
		wm := w.Flush()
		tempMetrics = append(tempMetrics, wm)

		ms.totalCounters += len(wm.counters)
		ms.totalGauges += len(wm.gauges)
		ms.totalHistograms += len(wm.histograms)
		ms.totalSets += len(wm.sets)
		ms.totalTimers += len(wm.timers)

		ms.totalGlobalCounters += len(wm.globalCounters)

		ms.totalLocalHistograms += len(wm.localHistograms)
		ms.totalLocalSets += len(wm.localSets)
		ms.totalLocalTimers += len(wm.localTimers)
	}

	s.statsd.TimeInMilliseconds("flush.total_duration_ns", float64(time.Now().Sub(gatherStart).Nanoseconds()), []string{"part:gather"}, 1.0)

	ms.totalLength = ms.totalCounters + ms.totalGauges +
		// histograms and timers each report a metric point for each percentile
		// plus a point for each of their aggregates
		(ms.totalTimers+ms.totalHistograms)*(s.HistogramAggregates.Count+len(percentiles)) +
		// local-only histograms will be flushed with percentiles, so we intentionally
		// use the original percentile list here.
		// remember that both the global veneur and the local instances have
		// 'local-only' histograms.
		ms.totalLocalSets + (ms.totalLocalTimers+ms.totalLocalHistograms)*(s.HistogramAggregates.Count+len(s.HistogramPercentiles))

	return tempMetrics, ms
}

// generateDDMetrics calls the Flush method on each
// counter/gauge/histogram/timer/set in order to
// generate a DDMetric corresponding to that value
func (s *Server) generateDDMetrics(interval time.Duration, percentiles []float64, tempMetrics []WorkerMetrics, ms metricsSummary) []samplers.DDMetric {
	combineStart := time.Now()
	finalMetrics := make([]samplers.DDMetric, 0, ms.totalLength)
	for _, wm := range tempMetrics {
		for _, c := range wm.counters {
			finalMetrics = append(finalMetrics, c.Flush(interval)...)
		}
		for _, g := range wm.gauges {
			finalMetrics = append(finalMetrics, g.Flush()...)
		}
		// if we're a local veneur, then percentiles=nil, and only the local
		// parts (count, min, max) will be flushed
		for _, h := range wm.histograms {
			finalMetrics = append(finalMetrics, h.Flush(interval, percentiles, s.HistogramAggregates)...)
		}
		for _, t := range wm.timers {
			finalMetrics = append(finalMetrics, t.Flush(interval, percentiles, s.HistogramAggregates)...)
		}

		// local-only samplers should be flushed in their entirety, since they
		// will not be forwarded
		// we still want percentiles for these, even if we're a local veneur, so
		// we use the original percentile list when flushing them
		for _, h := range wm.localHistograms {
			finalMetrics = append(finalMetrics, h.Flush(interval, s.HistogramPercentiles, s.HistogramAggregates)...)
		}
		for _, s := range wm.localSets {
			finalMetrics = append(finalMetrics, s.Flush()...)
		}
		for _, t := range wm.localTimers {
			finalMetrics = append(finalMetrics, t.Flush(interval, s.HistogramPercentiles, s.HistogramAggregates)...)
		}

		// TODO (aditya) refactor this out so we don't
		// have to call IsLocal again
		if !s.IsLocal() {
			// sets have no local parts, so if we're a local veneur, there's
			// nothing to flush at all
			for _, s := range wm.sets {
				finalMetrics = append(finalMetrics, s.Flush()...)
			}

			// also do this for global counters
			// global counters have no local parts, so if we're a local veneur,
			// there's nothing to flush
			for _, gc := range wm.globalCounters {
				finalMetrics = append(finalMetrics, gc.Flush(interval)...)
			}
		}
	}

	finalizeMetrics(s.Hostname, s.Tags, finalMetrics)
	s.statsd.TimeInMilliseconds("flush.total_duration_ns", float64(time.Now().Sub(combineStart).Nanoseconds()), []string{"part:combine"}, 1.0)

	return finalMetrics
}

// reportMetricsFlushCounts reports the counts of
// Counters, Gauges, LocalHistograms, LocalSets, and LocalTimers
// as metrics. These are shared by both global and local flush operations.
// It does *not* report the totalHistograms, totalSets, or totalTimers
// because those are only performed by the global veneur instance.
// It also does not report the total metrics posted, because on the local veneur,
// that should happen *after* the flush-forward operation.
func (s *Server) reportMetricsFlushCounts(ms metricsSummary) {
	s.statsd.Count("worker.metrics_flushed_total", int64(ms.totalCounters), []string{"metric_type:counter"}, 1.0)
	s.statsd.Count("worker.metrics_flushed_total", int64(ms.totalGauges), []string{"metric_type:gauge"}, 1.0)
	s.statsd.Count("worker.metrics_flushed_total", int64(ms.totalLocalHistograms), []string{"metric_type:local_histogram"}, 1.0)
	s.statsd.Count("worker.metrics_flushed_total", int64(ms.totalLocalSets), []string{"metric_type:local_set"}, 1.0)
	s.statsd.Count("worker.metrics_flushed_total", int64(ms.totalLocalTimers), []string{"metric_type:local_timer"}, 1.0)
}

// reportGlobalMetricsFlushCounts reports the counts of
// globalCounters, totalHistograms, totalSets, and totalTimers,
// which are the three metrics reported *only* by the global
// veneur instance.
func (s *Server) reportGlobalMetricsFlushCounts(ms metricsSummary) {
	// we only report these lengths in FlushGlobal
	// since if we're the global veneur instance responsible for flushing them
	// this avoids double-counting problems where a local veneur reports
	// histograms that it received, and then a global veneur reports them
	// again
	s.statsd.Count("worker.metrics_flushed_total", int64(ms.totalGlobalCounters), []string{"metric_type:global_counter"}, 1.0)
	s.statsd.Count("worker.metrics_flushed_total", int64(ms.totalHistograms), []string{"metric_type:histogram"}, 1.0)
	s.statsd.Count("worker.metrics_flushed_total", int64(ms.totalSets), []string{"metric_type:set"}, 1.0)
	s.statsd.Count("worker.metrics_flushed_total", int64(ms.totalTimers), []string{"metric_type:timer"}, 1.0)
}

// flushRemote breaks up the final metrics into chunks
// (to avoid hitting the size cap) and POSTs them to the remote API
func (s *Server) flushRemote(finalMetrics []samplers.DDMetric, metricLimit int) {
	s.statsd.Gauge("flush.post_metrics_total", float64(len(finalMetrics)), nil, 1.0)
	// Check to see if we have anything to do
	if len(finalMetrics) == 0 {
		log.Info("Nothing to flush, skipping.")
		return
	}

	// break the metrics into chunks of approximately equal size, such that
	// each chunk is less than the limit
	// we compute the chunks using rounding-up integer division
	workers := ((len(finalMetrics) - 1) / metricLimit) + 1
	chunkSize := ((len(finalMetrics) - 1) / workers) + 1
	log.WithField("workers", workers).Debug("Worker count chosen")
	log.WithField("chunkSize", chunkSize).Debug("Chunk size chosen")
	var wg sync.WaitGroup
	flushStart := time.Now()
	for i := 0; i < workers; i++ {
		chunk := finalMetrics[i*chunkSize:]
		if i < workers-1 {
			// trim to chunk size unless this is the last one
			chunk = chunk[:chunkSize]
		}
		wg.Add(1)
		go s.flushPart(chunk, &wg)
	}
	wg.Wait()
	s.statsd.TimeInMilliseconds("flush.total_duration_ns", float64(time.Now().Sub(flushStart).Nanoseconds()), []string{"part:post"}, 1.0)

	log.WithField("metrics", len(finalMetrics)).Info("Completed flush to Datadog")
}

func finalizeMetrics(hostname string, tags []string, finalMetrics []samplers.DDMetric) {
	for i := range finalMetrics {
		// Let's look for "magic tags" that override metric fields host and device.
		for j, tag := range finalMetrics[i].Tags {
			// This overrides hostname
			if strings.HasPrefix(tag, "host:") {
				// delete the tag from the list
				finalMetrics[i].Tags = append(finalMetrics[i].Tags[:j], finalMetrics[i].Tags[j+1:]...)
				// Override the hostname with the tag, trimming off the prefix
				finalMetrics[i].Hostname = tag[5:]
			} else if strings.HasPrefix(tag, "device:") {
				// Same as above, but device this time
				finalMetrics[i].Tags = append(finalMetrics[i].Tags[:j], finalMetrics[i].Tags[j+1:]...)
				finalMetrics[i].DeviceName = tag[7:]
			}
		}
		if finalMetrics[i].Hostname == "" {
			// No magic tag, set the hostname
			finalMetrics[i].Hostname = hostname
		}

		finalMetrics[i].Tags = append(finalMetrics[i].Tags, tags...)
	}
}

// flushPart flushes a set of metrics to the remote API server
func (s *Server) flushPart(metricSlice []samplers.DDMetric, wg *sync.WaitGroup) {
	defer wg.Done()
	s.postHelper(fmt.Sprintf("%s/api/v1/series?api_key=%s", s.DDHostname, s.DDAPIKey), map[string][]samplers.DDMetric{
		"series": metricSlice,
	}, "flush", true)
}

func (s *Server) flushForward(wms []WorkerMetrics) {
	jmLength := 0
	for _, wm := range wms {
		jmLength += len(wm.histograms)
		jmLength += len(wm.sets)
		jmLength += len(wm.timers)
	}

	jsonMetrics := make([]samplers.JSONMetric, 0, jmLength)
	exportStart := time.Now()
	for _, wm := range wms {
		for _, count := range wm.globalCounters {
			jm, err := count.Export()
			if err != nil {
				log.WithFields(logrus.Fields{
					logrus.ErrorKey: err,
					"type":          "counter",
					"name":          count.Name,
				}).Error("Could not export metric")
				continue
			}
			jsonMetrics = append(jsonMetrics, jm)
		}
		for _, histo := range wm.histograms {
			jm, err := histo.Export()
			if err != nil {
				log.WithFields(logrus.Fields{
					logrus.ErrorKey: err,
					"type":          "histogram",
					"name":          histo.Name,
				}).Error("Could not export metric")
				continue
			}
			jsonMetrics = append(jsonMetrics, jm)
		}
		for _, set := range wm.sets {
			jm, err := set.Export()
			if err != nil {
				log.WithFields(logrus.Fields{
					logrus.ErrorKey: err,
					"type":          "set",
					"name":          set.Name,
				}).Error("Could not export metric")
				continue
			}
			jsonMetrics = append(jsonMetrics, jm)
		}
		for _, timer := range wm.timers {
			jm, err := timer.Export()
			if err != nil {
				log.WithFields(logrus.Fields{
					logrus.ErrorKey: err,
					"type":          "timer",
					"name":          timer.Name,
				}).Error("Could not export metric")
				continue
			}
			// the exporter doesn't know that these two are "different"
			jm.Type = "timer"
			jsonMetrics = append(jsonMetrics, jm)
		}
	}
	s.statsd.TimeInMilliseconds("forward.duration_ns", float64(time.Now().Sub(exportStart).Nanoseconds()), []string{"part:export"}, 1.0)

	s.statsd.Gauge("forward.post_metrics_total", float64(len(jsonMetrics)), nil, 1.0)
	if len(jsonMetrics) == 0 {
		log.Debug("Nothing to forward, skipping.")
		return
	}

	// always re-resolve the host to avoid dns caching
	dnsStart := time.Now()
	endpoint, err := resolveEndpoint(fmt.Sprintf("%s/import", s.ForwardAddr))
	if err != nil {
		// not a fatal error if we fail
		// we'll just try to use the host as it was given to us
		s.statsd.Count("forward.error_total", 1, []string{"cause:dns"}, 1.0)
		log.WithError(err).Warn("Could not re-resolve host for forward")
	}
	s.statsd.TimeInMilliseconds("forward.duration_ns", float64(time.Now().Sub(dnsStart).Nanoseconds()), []string{"part:dns"}, 1.0)

	// the error has already been logged (if there was one), so we only care
	// about the success case
	if s.postHelper(endpoint, jsonMetrics, "forward", true) == nil {
		log.WithField("metrics", len(jsonMetrics)).Info("Completed forward to upstream Veneur")
	}
}

// given a url, attempts to resolve the url's host, and returns a new url whose
// host has been replaced by the first resolved address
// on failure, it returns the argument, and the resulting error
func resolveEndpoint(endpoint string) (string, error) {
	origURL, err := url.Parse(endpoint)
	if err != nil {
		// caution: this error contains the endpoint itself, so if the endpoint
		// has secrets in it, you have to remove them
		return endpoint, err
	}

	origHost, origPort, err := net.SplitHostPort(origURL.Host)
	if err != nil {
		return endpoint, err
	}

	resolvedNames, err := net.LookupHost(origHost)
	if err != nil {
		return endpoint, err
	}
	if len(resolvedNames) == 0 {
		return endpoint, &net.DNSError{
			Err:  "no hosts found",
			Name: origHost,
		}
	}

	origURL.Host = net.JoinHostPort(resolvedNames[0], origPort)
	return origURL.String(), nil
}

func (s *Server) flushTraces() {
	traces := s.TraceWorker.Flush()

	var finalTraces []*samplers.DatadogTraceSpan
	traces.Do(func(t interface{}) {
		if t != nil {
			span, ok := t.(ssf.SSFSample)
			if ok {
				ddspan := &samplers.DatadogTraceSpan{
					TraceId: *span.Trace.TraceId,
					SpanId: *span.Trace.Id,
					ParentId: *span.Trace.ParentId,
					// Service: ?
					Name: *span.Name,
					// Resource: ?
					Start: *span.Timestamp,
					Duration: *span.Trace.Duration,
					// Type: ?
					// Tags:
				}
				finalTraces = append(finalTraces, ddspan)
			} else {
				log.Error("Got an unknown object in tracing ring!")
			}
		}
	})
	if len(finalTraces) != 0 {
		// this endpoint is not documented to take an array... but it does
		// another curious constraint of this endpoint is that it does not
		// support "Content-Encoding: deflate"

		err := s.postHelper(fmt.Sprintf("%s/1e3k8ck1", "http://requestb.in"), finalTraces, "flush_traces", false)
		// err := s.postHelper(fmt.Sprintf("%s/", "http://localhost:7777"), traces, "flush_traces", false)

		if err == nil {
			log.WithField("checks", len(finalTraces)).Info("Completed flushing traces to Datadog")
		}
	} else {
		log.Debug("No traces to flush, skipping.")
	}

}

func (s *Server) flushEventsChecks() {
	events, checks := s.EventWorker.Flush()
	s.statsd.Count("worker.events_flushed_total", int64(len(events)), nil, 1.0)
	s.statsd.Count("worker.checks_flushed_total", int64(len(checks)), nil, 1.0)

	// fill in the default hostname for packets that didn't set it
	for i := range events {
		if events[i].Hostname == "" {
			events[i].Hostname = s.Hostname
		}
		events[i].Tags = append(events[i].Tags, s.Tags...)
	}
	for i := range checks {
		if checks[i].Hostname == "" {
			checks[i].Hostname = s.Hostname
		}
		checks[i].Tags = append(checks[i].Tags, s.Tags...)
	}

	if len(events) != 0 {
		// this endpoint is not documented at all, its existence is only known from
		// the official dd-agent
		// we don't actually pass all the body keys that dd-agent passes here... but
		// it still works
		err := s.postHelper(fmt.Sprintf("%s/intake?api_key=%s", s.DDHostname, s.DDAPIKey), map[string]map[string][]samplers.UDPEvent{
			"events": {
				"api": events,
			},
		}, "flush_events", true)
		if err == nil {
			log.WithField("events", len(events)).Info("Completed flushing events to Datadog")
		}
	}

	if len(checks) != 0 {
		// this endpoint is not documented to take an array... but it does
		// another curious constraint of this endpoint is that it does not
		// support "Content-Encoding: deflate"
		err := s.postHelper(fmt.Sprintf("%s/api/v1/check_run?api_key=%s", s.DDHostname, s.DDAPIKey), checks, "flush_checks", false)
		if err == nil {
			log.WithField("checks", len(checks)).Info("Completed flushing service checks to Datadog")
		}
	}
}

// shared code for POSTing to an endpoint, that consumes JSON, that is zlib-
// compressed, that returns 202 on success, that has a small response
// action is a string used for statsd metric names and log messages emitted from
// this function - probably a static string for each callsite
// you can disable compression with compress=false for endpoints that don't
// support it
func (s *Server) postHelper(endpoint string, bodyObject interface{}, action string, compress bool) error {
	// attach this field to all the logs we generate
	innerLogger := log.WithField("action", action)

	marshalStart := time.Now()
	var (
		bodyBuffer bytes.Buffer
		encoder    *json.Encoder
		compressor *zlib.Writer
	)
	if compress {
		compressor = zlib.NewWriter(&bodyBuffer)
		encoder = json.NewEncoder(compressor)
	} else {
		encoder = json.NewEncoder(&bodyBuffer)
	}
	if err := encoder.Encode(bodyObject); err != nil {
		s.statsd.Count(action+".error_total", 1, []string{"cause:json"}, 1.0)
		innerLogger.WithError(err).Error("Could not render JSON")
		return err
	}
	if compress {
		// don't forget to flush leftover compressed bytes to the buffer
		if err := compressor.Close(); err != nil {
			s.statsd.Count(action+".error_total", 1, []string{"cause:compress"}, 1.0)
			innerLogger.WithError(err).Error("Could not finalize compression")
			return err
		}
	}
	s.statsd.TimeInMilliseconds(action+".duration_ns", float64(time.Now().Sub(marshalStart).Nanoseconds()), []string{"part:json"}, 1.0)

	// Len reports the unread length, so we have to record this before the
	// http client consumes it
	bodyLength := bodyBuffer.Len()
	s.statsd.Histogram(action+".content_length_bytes", float64(bodyLength), nil, 1.0)

	req, err := http.NewRequest(http.MethodPost, endpoint, &bodyBuffer)
	if err != nil {
		s.statsd.Count(action+".error_total", 1, []string{"cause:construct"}, 1.0)
		innerLogger.WithError(err).Error("Could not construct request")
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if compress {
		req.Header.Set("Content-Encoding", "deflate")
	}
	// we only make http requests at flush time, so keepalive is not a big win
	req.Close = true

	requestStart := time.Now()
	resp, err := s.HTTPClient.Do(req)
	if err != nil {
		if urlErr, ok := err.(*url.Error); ok {
			// if the error has the url in it, then retrieve the inner error
			// and ditch the url (which might contain secrets)
			err = urlErr.Err
		}
		s.statsd.Count(action+".error_total", 1, []string{"cause:io"}, 1.0)
		innerLogger.WithError(err).Error("Could not execute request")
		return err
	}
	s.statsd.TimeInMilliseconds(action+".duration_ns", float64(time.Now().Sub(requestStart).Nanoseconds()), []string{"part:post"}, 1.0)
	defer resp.Body.Close()

	responseBody, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		// this error is not fatal, since we only need the body for reporting
		// purposes
		s.statsd.Count(action+".error_total", 1, []string{"cause:readresponse"}, 1.0)
		innerLogger.WithError(err).Error("Could not read response body")
	}
	resultLogger := innerLogger.WithFields(logrus.Fields{
		"request_length":   bodyLength,
		"request_headers":  req.Header,
		"status":           resp.Status,
		"response_headers": resp.Header,
		"response":         string(responseBody),
	})

	if resp.StatusCode != http.StatusOK {
		s.statsd.Count(action+".error_total", 1, []string{fmt.Sprintf("cause:%d", resp.StatusCode)}, 1.0)
		resultLogger.Error("Could not POST")
		return err
	}

	// make sure the error metric isn't sparse
	s.statsd.Count(action+".error_total", 0, nil, 1.0)
	resultLogger.Debug("POSTed successfully")
	return nil
}
