package veneur

import (
	"context"
	"fmt"
	"net/http"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/sirupsen/logrus"
	vhttp "github.com/stripe/veneur/http"
	"github.com/stripe/veneur/samplers"
	"github.com/stripe/veneur/sinks"
	"github.com/stripe/veneur/ssf"
	"github.com/stripe/veneur/trace"
	"github.com/stripe/veneur/trace/metrics"
)

// Flush collects sampler's metrics and passes them to sinks.
func (s *Server) Flush(ctx context.Context) {
	span := tracer.StartSpan("flush").(*trace.Span)
	defer span.ClientFinish(s.TraceClient)

	mem := &runtime.MemStats{}
	runtime.ReadMemStats(mem)

	span.Add(ssf.Gauge("mem.heap_alloc_bytes", float32(mem.HeapAlloc), nil),
		ssf.Gauge("gc.number", float32(mem.NumGC), nil),
		ssf.Gauge("gc.pause_total_ns", float32(mem.PauseTotalNs), nil),
		ssf.Gauge("gc.alloc_heap_bytes", float32(mem.HeapAlloc), nil),
		ssf.Gauge("gc.alloc_heap_bytes_total", float32(mem.TotalAlloc), nil),
		ssf.Gauge("gc.mallocs_objects_total", float32(mem.Mallocs), nil),
		ssf.Gauge("gc.GCCPUFraction", float32(mem.GCCPUFraction), nil),
		ssf.Gauge("worker.span_chan.total_elements", float32(len(s.SpanChan)), nil),
		ssf.Gauge("worker.span_chan.total_capacity", float32(cap(s.SpanChan)), nil),
	)

	samples := s.EventWorker.Flush()

	// TODO Concurrency
	for _, sink := range s.metricSinks {
		sink.FlushOtherSamples(span.Attach(ctx), samples)
	}

	go s.flushTraces(span.Attach(ctx))

	// don't publish percentiles if we're a local veneur; that's the global
	// veneur's job
	var percentiles []float64
	var finalMetrics []samplers.InterMetric

	if !s.IsLocal() {
		percentiles = s.HistogramPercentiles
	}

	tempMetrics, ms := s.tallyMetrics(percentiles)

	finalMetrics = s.generateInterMetrics(span.Attach(ctx), percentiles, tempMetrics, ms)

	s.reportMetricsFlushCounts(ms)

	if s.IsLocal() {
		go s.flushForward(span.Attach(ctx), tempMetrics)
	} else {
		s.reportGlobalMetricsFlushCounts(ms)
	}

	// If there's nothing to flush, don't bother calling the plugins and stuff.
	if len(finalMetrics) == 0 {
		return
	}

	wg := sync.WaitGroup{}
	for _, sink := range s.metricSinks {
		wg.Add(1)
		go func(ms sinks.MetricSink) {
			err := ms.Flush(span.Attach(ctx), finalMetrics)
			if err != nil {
				log.WithError(err).WithField("sink", ms.Name()).Warn("Error flushing sink")
			}
			wg.Done()
		}(sink)
	}
	wg.Wait()

	go func() {
		samples := &ssf.Samples{}
		defer metrics.Report(s.TraceClient, samples)

		tags := map[string]string{"part": "post"}
		for _, p := range s.getPlugins() {
			start := time.Now()
			err := p.Flush(span.Attach(ctx), finalMetrics)
			samples.Add(ssf.Timing(fmt.Sprintf("flush.plugins.%s.total_duration_ns", p.Name()), time.Since(start), time.Nanosecond, tags))
			if err != nil {
				samples.Add(ssf.Count(fmt.Sprintf("flush.plugins.%s.error_total", p.Name()), 1, nil))
			}
			samples.Add(ssf.Gauge(fmt.Sprintf("flush.plugins.%s.post_metrics_total", p.Name()), float32(len(finalMetrics)), nil))
		}
	}()
}

type metricsSummary struct {
	totalCounters   int
	totalGauges     int
	totalHistograms int
	totalSets       int
	totalTimers     int

	totalGlobalCounters int
	totalGlobalGauges   int

	totalLocalHistograms   int
	totalLocalSets         int
	totalLocalTimers       int
	totalStatusChecks      int
	totalLocalStatusChecks int

	totalLength int
}

// tallyMetrics gives a slight overestimate of the number
// of metrics we'll be reporting, so that we can pre-allocate
// a slice of the correct length instead of constantly appending
// for performance
func (s *Server) tallyMetrics(percentiles []float64) ([]WorkerMetrics, metricsSummary) {
	// allocating this long array to count up the sizes is cheaper than appending
	// the []WorkerMetrics together one at a time
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
		ms.totalGlobalGauges += len(wm.globalGauges)

		ms.totalLocalHistograms += len(wm.localHistograms)
		ms.totalLocalSets += len(wm.localSets)
		ms.totalLocalTimers += len(wm.localTimers)

		ms.totalStatusChecks += len(wm.statusChecks)
		ms.totalLocalStatusChecks += len(wm.localStatusChecks)
	}

	metrics.ReportOne(s.TraceClient, ssf.Timing("flush.total_duration_ns", time.Since(gatherStart), time.Nanosecond, map[string]string{"part": "gather"}))

	ms.totalLength = ms.totalCounters + ms.totalGauges +
		// histograms and timers each report a metric point for each percentile
		// plus a point for each of their aggregates
		(ms.totalTimers+ms.totalHistograms)*(s.HistogramAggregates.Count+len(percentiles)) +
		// local-only histograms will be flushed with percentiles, so we intentionally
		// use the original percentile list here.
		// remember that both the global veneur and the local instances have
		// 'local-only' histograms.
		ms.totalLocalSets + (ms.totalLocalTimers+ms.totalLocalHistograms)*(s.HistogramAggregates.Count+len(s.HistogramPercentiles))

	// Global instances also flush sets and global counters, so be sure and add
	// them to the total size
	if !s.IsLocal() {
		ms.totalLength += ms.totalSets
		ms.totalLength += ms.totalGlobalCounters
		ms.totalLength += ms.totalGlobalGauges
	}

	return tempMetrics, ms
}

// generateInterMetrics calls the Flush method on each
// counter/gauge/histogram/timer/set in order to
// generate an InterMetric corresponding to that value
func (s *Server) generateInterMetrics(ctx context.Context, percentiles []float64, tempMetrics []WorkerMetrics, ms metricsSummary) []samplers.InterMetric {

	span, _ := trace.StartSpanFromContext(ctx, "")
	defer span.ClientFinish(s.TraceClient)

	finalMetrics := make([]samplers.InterMetric, 0, ms.totalLength)
	for _, wm := range tempMetrics {
		for _, c := range wm.counters {
			finalMetrics = append(finalMetrics, c.Flush(s.interval)...)
		}
		for _, g := range wm.gauges {
			finalMetrics = append(finalMetrics, g.Flush()...)
		}
		// if we're a local veneur, then percentiles=nil, and only the local
		// parts (count, min, max) will be flushed
		for _, h := range wm.histograms {
			finalMetrics = append(finalMetrics, h.Flush(s.interval, percentiles, s.HistogramAggregates)...)
		}
		for _, t := range wm.timers {
			finalMetrics = append(finalMetrics, t.Flush(s.interval, percentiles, s.HistogramAggregates)...)
		}
		for _, status := range wm.statusChecks {
			finalMetrics = append(finalMetrics, status.Flush()...)
		}

		// local-only samplers should be flushed in their entirety, since they
		// will not be forwarded
		// we still want percentiles for these, even if we're a local veneur, so
		// we use the original percentile list when flushing them
		for _, h := range wm.localHistograms {
			finalMetrics = append(finalMetrics, h.Flush(s.interval, s.HistogramPercentiles, s.HistogramAggregates)...)
		}
		for _, s := range wm.localSets {
			finalMetrics = append(finalMetrics, s.Flush()...)
		}
		for _, t := range wm.localTimers {
			finalMetrics = append(finalMetrics, t.Flush(s.interval, s.HistogramPercentiles, s.HistogramAggregates)...)
		}

		for _, status := range wm.localStatusChecks {
			finalMetrics = append(finalMetrics, status.Flush()...)
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
				finalMetrics = append(finalMetrics, gc.Flush(s.interval)...)
			}

			// and global gauges
			for _, gg := range wm.globalGauges {
				finalMetrics = append(finalMetrics, gg.Flush()...)
			}
		}
	}

	metrics.ReportOne(s.TraceClient, ssf.Timing("flush.total_duration_ns", time.Since(span.Start), time.Nanosecond, map[string]string{"part": "combine"}))
	return finalMetrics
}

const flushTotalMetric = "worker.metrics_flushed_total"

// reportMetricsFlushCounts reports the counts of
// Counters, Gauges, LocalHistograms, LocalSets, and LocalTimers
// as metrics. These are shared by both global and local flush operations.
// It does *not* report the totalHistograms, totalSets, or totalTimers
// because those are only performed by the global veneur instance.
// It also does not report the total metrics posted, because on the local veneur,
// that should happen *after* the flush-forward operation.
func (s *Server) reportMetricsFlushCounts(ms metricsSummary) {
	s.Statsd.Count(flushTotalMetric, int64(ms.totalCounters), []string{"metric_type:counter"}, 1.0)
	s.Statsd.Count(flushTotalMetric, int64(ms.totalGauges), []string{"metric_type:gauge"}, 1.0)
	s.Statsd.Count(flushTotalMetric, int64(ms.totalLocalHistograms), []string{"metric_type:local_histogram"}, 1.0)
	s.Statsd.Count(flushTotalMetric, int64(ms.totalLocalSets), []string{"metric_type:local_set"}, 1.0)
	s.Statsd.Count(flushTotalMetric, int64(ms.totalLocalTimers), []string{"metric_type:local_timer"}, 1.0)
}

// reportGlobalMetricsFlushCounts reports the counts of
// globalCounters, globalGauges, totalHistograms, totalSets, and totalTimers,
// which are the three metrics reported *only* by the global
// veneur instance.
func (s *Server) reportGlobalMetricsFlushCounts(ms metricsSummary) {
	// we only report these lengths in FlushGlobal
	// since if we're the global veneur instance responsible for flushing them
	// this avoids double-counting problems where a local veneur reports
	// histograms that it received, and then a global veneur reports them
	// again

	s.Statsd.Count(flushTotalMetric, int64(ms.totalGlobalCounters), []string{"metric_type:global_counter"}, 1.0)
	s.Statsd.Count(flushTotalMetric, int64(ms.totalGlobalGauges), []string{"metric_type:global_gauge"}, 1.0)
	s.Statsd.Count(flushTotalMetric, int64(ms.totalHistograms), []string{"metric_type:histogram"}, 1.0)
	s.Statsd.Count(flushTotalMetric, int64(ms.totalSets), []string{"metric_type:set"}, 1.0)
	s.Statsd.Count(flushTotalMetric, int64(ms.totalTimers), []string{"metric_type:timer"}, 1.0)
}

func (s *Server) flushForward(ctx context.Context, wms []WorkerMetrics) {
	span, _ := trace.StartSpanFromContext(ctx, "")
	defer span.ClientFinish(s.TraceClient)
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
		for _, gauge := range wm.globalGauges {
			jm, err := gauge.Export()
			if err != nil {
				log.WithFields(logrus.Fields{
					logrus.ErrorKey: err,
					"type":          "gauge",
					"name":          gauge.Name,
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
	s.Statsd.TimeInMilliseconds("forward.duration_ns", float64(time.Since(exportStart).Nanoseconds()), []string{"part:export"}, 1.0)
	s.Statsd.Gauge("forward.post_metrics_total", float64(len(jsonMetrics)), nil, 1.0)
	if len(jsonMetrics) == 0 {
		log.Debug("Nothing to forward, skipping.")
		return
	}

	// the error has already been logged (if there was one), so we only care
	// about the success case
	endpoint := fmt.Sprintf("%s/import", s.ForwardAddr)
	if vhttp.PostHelper(span.Attach(ctx), s.HTTPClient, s.TraceClient, http.MethodPost, endpoint, jsonMetrics, "forward", true, nil, log) == nil {
		log.WithFields(logrus.Fields{
			"metrics":     len(jsonMetrics),
			"endpoint":    endpoint,
			"forwardAddr": s.ForwardAddr,
		}).Info("Completed forward to upstream Veneur")
	}
}

func (s *Server) flushTraces(ctx context.Context) {
	s.ssfInternalMetrics.Range(func(keyI, valueI interface{}) bool {
		key, ok := keyI.(string)
		if !ok {
			log.WithFields(logrus.Fields{
				"key":  key,
				"type": reflect.TypeOf(key),
			}).Error("received non-string key")
			return true
		}

		value, ok := valueI.(*ssfServiceSpanMetrics)
		if !ok {
			log.WithFields(logrus.Fields{
				"value": value,
				"type":  reflect.TypeOf(value),
			}).Error("received non-struct value")
			return true
		}

		tags := strings.Split(key, ",")
		if len(tags) != 2 {
			log.WithFields(logrus.Fields{
				"key":    key,
				"length": len(tags),
			}).Error("received key of incorrect format")
		}

		spansReceivedTotal := atomic.SwapInt64(&value.ssfSpansReceivedTotal, 0) * internalMetricSampleRate
		s.Statsd.Count("ssf.spans.received_total", spansReceivedTotal, tags, 1.0)
		return true
	})

	s.SpanWorker.Flush()
}
