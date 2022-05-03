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

	"github.com/axiomhq/hyperloglog"
	"github.com/sirupsen/logrus"
	"github.com/stripe/veneur/v14/forwardrpc"
	vhttp "github.com/stripe/veneur/v14/http"
	"github.com/stripe/veneur/v14/samplers"
	"github.com/stripe/veneur/v14/samplers/metricpb"
	"github.com/stripe/veneur/v14/sinks"
	"github.com/stripe/veneur/v14/ssf"
	"github.com/stripe/veneur/v14/trace"
	"github.com/stripe/veneur/v14/util/matcher"
	"google.golang.org/grpc/status"
)

// Flush collects sampler's metrics and passes them to sinks.
func (s *Server) Flush(ctx context.Context) {
	span := tracer.StartSpan("flush").(*trace.Span)
	defer span.ClientFinish(s.TraceClient)

	mem := &runtime.MemStats{}
	runtime.ReadMemStats(mem)

	flushTime := time.Now().UnixNano()
	atomic.StoreInt64(&s.lastFlushUnix, flushTime)

	s.Statsd.Gauge("worker.span_chan.total_elements", float64(len(s.SpanChan)), nil, 1.0)
	s.Statsd.Gauge("worker.span_chan.total_capacity", float64(cap(s.SpanChan)), nil, 1.0)
	s.Statsd.Gauge("gc.number", float64(mem.NumGC), nil, 1.0)
	s.Statsd.Gauge("gc.pause_total_ns", float64(mem.PauseTotalNs), nil, 1.0)
	s.Statsd.Gauge("mem.heap_alloc_bytes", float64(mem.HeapAlloc), nil, 1.0)
	s.Statsd.Gauge("flush.flush_timestamp_ns", float64(flushTime), nil, 1.0)

	if s.CountUniqueTimeseries {
		s.Statsd.Count("flush.unique_timeseries_total", s.tallyTimeseries(), []string{fmt.Sprintf("global_veneur:%t", !s.IsLocal())}, 1.0)
	}

	samples := s.EventWorker.Flush()

	// TODO Concurrency
	for _, sink := range s.metricSinks {
		sink.sink.FlushOtherSamples(span.Attach(ctx), samples)
	}

	go s.flushTraces(span.Attach(ctx))

	var finalMetrics []samplers.InterMetric

	// This ensures that mixedscope histograms and timers behave correctly.
	// That is, they should emit aggregates when forwarding, but no percentiles.
	// Similarly, they should emit percentiles when global, but no aggregates.
	//
	// This serves two purposes:
	//   * Percentiles are only accurate when aggregated globally.
	//   * Avoid double counting and breaking existing queries (if count is also
	//     emitted globally, queries that sum over counts double!)
	var percentiles []float64
	aggregates := s.HistogramAggregates
	if !s.IsLocal() {
		percentiles = s.HistogramPercentiles
		aggregates = samplers.HistogramAggregates{}
	}

	tempMetrics, ms := s.tallyMetrics(percentiles)

	finalMetrics = s.generateInterMetrics(span.Attach(ctx), percentiles, aggregates, tempMetrics, ms)

	s.reportMetricsFlushCounts(ms)

	wg := sync.WaitGroup{}
	defer wg.Wait()

	if s.IsLocal() {
		wg.Add(1)
		switch s.proxyProtocol {
		case ProxyProtocolGrpcStream, ProxyProtocolGrpcSingle:
			go func() {
				s.forwardGRPC(span.Attach(ctx), tempMetrics, s.proxyProtocol)
				wg.Done()
			}()
		case ProxyProtocolRest:
			go func() {
				s.flushForward(span.Attach(ctx), tempMetrics)
				wg.Done()
			}()
		}
	} else {
		s.reportGlobalMetricsFlushCounts(ms)
		s.reportGlobalReceivedProtocolMetrics()
	}

	// If there's nothing to flush, don't bother calling the plugins and stuff.
	if len(finalMetrics) == 0 {
		return
	}

	if s.Config.Features.EnableMetricSinkRouting {
		for index := range finalMetrics {
			metric := &finalMetrics[index]
			metric.Sinks = make(samplers.RouteInformation)
			for _, config := range s.Config.MetricSinkRouting {
				var sinks []string
				if matcher.Match(config.Match, metric.Name, metric.Tags) {
					sinks = config.Sinks.Matched
				} else {
					sinks = config.Sinks.NotMatched
				}
				for _, sink := range sinks {
					metric.Sinks[sink] = struct{}{}
				}
			}
		}
	}

	for _, sink := range s.metricSinks {
		wg.Add(1)
		go func(sink internalMetricSink) {
			filteredMetrics := finalMetrics
			if s.Config.Features.EnableMetricSinkRouting {
				sinkName := sink.sink.Name()
				filteredMetrics = []samplers.InterMetric{}
				for _, metric := range finalMetrics {
					_, ok := metric.Sinks[sinkName]
					if !ok {
						continue
					}
					filteredTags := []string{}
					if len(sink.stripTags) == 0 {
						filteredTags = metric.Tags
					} else {
					tagLoop:
						for _, tag := range metric.Tags {
							for _, tagMatcher := range sink.stripTags {
								if tagMatcher.Match(tag) {
									continue tagLoop
								}
							}
							filteredTags = append(filteredTags, tag)
						}
					}
					metric.Tags = filteredTags
					filteredMetrics = append(filteredMetrics, metric)
				}
			}
			flushStart := time.Now()
			err := sink.sink.Flush(span.Attach(ctx), filteredMetrics)
			if err == nil {
				s.logger.WithFields(logrus.Fields{
					"sink":    sink.sink.Name(),
					"success": true,
				}).Info(sinks.FlushCompleteMessage)
			} else {
				s.logger.WithError(err).WithFields(logrus.Fields{
					"sink":    sink.sink.Name(),
					"success": false,
				}).Warn(sinks.FlushCompleteMessage)
			}
			span.Add(ssf.Timing(
				sinks.MetricKeyMetricFlushDuration, time.Since(flushStart),
				time.Nanosecond, map[string]string{"sink": sink.sink.Name()}))
			wg.Done()
		}(sink)
	}
}

func (s *Server) tallyTimeseries() int64 {
	allTimeseries := hyperloglog.New()
	for _, w := range s.Workers {
		w.uniqueMTSMtx.Lock()
		allTimeseries.Merge(w.uniqueMTS)
		w.uniqueMTS = hyperloglog.New()
		w.uniqueMTSMtx.Unlock()
	}
	return int64(allTimeseries.Estimate())
}

type metricsSummary struct {
	totalCounters   int
	totalGauges     int
	totalHistograms int
	totalSets       int
	totalTimers     int

	totalGlobalCounters   int
	totalGlobalGauges     int
	totalGlobalHistograms int
	totalGlobalTimers     int

	totalLocalHistograms   int
	totalLocalSets         int
	totalLocalTimers       int
	totalLocalStatusChecks int

	totalLength int
}

const perProtocolTotalMetricName string = "listen.received_per_protocol_total"

// tallyMetrics gives a slight overestimate of the number
// of metrics we'll be reporting, so that we can pre-allocate
// a slice of the correct length instead of constantly appending
// for performance
func (s *Server) tallyMetrics(percentiles []float64) ([]WorkerMetrics, metricsSummary) {
	// allocating this long array to count up the sizes is cheaper than appending
	// the []WorkerMetrics together one at a time
	tempMetrics := make([]WorkerMetrics, 0, len(s.Workers))

	ms := metricsSummary{}

	for i, w := range s.Workers {
		s.logger.WithField("worker", i).Debug("Flushing")
		wm := w.Flush()
		tempMetrics = append(tempMetrics, wm)

		ms.totalCounters += len(wm.counters)
		ms.totalGauges += len(wm.gauges)
		ms.totalHistograms += len(wm.histograms)
		ms.totalSets += len(wm.sets)
		ms.totalTimers += len(wm.timers)

		ms.totalGlobalCounters += len(wm.globalCounters)
		ms.totalGlobalGauges += len(wm.globalGauges)
		ms.totalGlobalHistograms += len(wm.globalHistograms)
		ms.totalGlobalTimers += len(wm.globalTimers)

		ms.totalLocalHistograms += len(wm.localHistograms)
		ms.totalLocalSets += len(wm.localSets)
		ms.totalLocalTimers += len(wm.localTimers)

		ms.totalLocalStatusChecks += len(wm.localStatusChecks)
	}

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
		ms.totalLength += ms.totalGlobalHistograms * (s.HistogramAggregates.Count + len(s.HistogramPercentiles))
		ms.totalLength += ms.totalGlobalTimers * (s.HistogramAggregates.Count + len(s.HistogramPercentiles))
	}

	return tempMetrics, ms
}

// generateInterMetrics calls the Flush method on each
// counter/gauge/histogram/timer/set in order to
// generate an InterMetric corresponding to that value
func (s *Server) generateInterMetrics(ctx context.Context, percentiles []float64, aggregates samplers.HistogramAggregates, tempMetrics []WorkerMetrics, ms metricsSummary) []samplers.InterMetric {

	span, _ := trace.StartSpanFromContext(ctx, "")
	defer span.ClientFinish(s.TraceClient)

	finalMetrics := make([]samplers.InterMetric, 0, ms.totalLength)
	for _, wm := range tempMetrics {
		for _, c := range wm.counters {
			finalMetrics = append(finalMetrics, c.Flush(s.Interval)...)
		}
		for _, g := range wm.gauges {
			finalMetrics = append(finalMetrics, g.Flush()...)
		}
		// if we're a local veneur, then percentiles=nil, and only the local
		// parts (count, min, max) will be flushed
		//
		// if we're a global veneur, aggregates will be nil.
		for _, h := range wm.histograms {
			finalMetrics = append(finalMetrics, h.Flush(s.Interval, percentiles, s.HistogramAggregates, false)...)
		}
		for _, t := range wm.timers {
			finalMetrics = append(finalMetrics, t.Flush(s.Interval, percentiles, s.HistogramAggregates, false)...)
		}

		// local-only samplers should be flushed in their entirety, since they
		// will not be forwarded
		// we still want percentiles for these, even if we're a local veneur, so
		// we use the original percentile list when flushing them
		for _, h := range wm.localHistograms {
			finalMetrics = append(finalMetrics, h.Flush(s.Interval, s.HistogramPercentiles, s.HistogramAggregates, false)...)
		}
		for _, s := range wm.localSets {
			finalMetrics = append(finalMetrics, s.Flush()...)
		}
		for _, t := range wm.localTimers {
			finalMetrics = append(finalMetrics, t.Flush(s.Interval, s.HistogramPercentiles, s.HistogramAggregates, false)...)
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
				finalMetrics = append(finalMetrics, gc.Flush(s.Interval)...)
			}

			// and global gauges
			for _, gg := range wm.globalGauges {
				finalMetrics = append(finalMetrics, gg.Flush()...)
			}

			for _, h := range wm.globalHistograms {
				finalMetrics = append(finalMetrics, h.Flush(s.Interval, s.HistogramPercentiles, s.HistogramAggregates, true)...)
			}
			for _, h := range wm.globalTimers {
				finalMetrics = append(finalMetrics, h.Flush(s.Interval, s.HistogramPercentiles, s.HistogramAggregates, true)...)
			}
		}
	}

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
	s.Statsd.Count(flushTotalMetric, int64(ms.totalLocalStatusChecks), []string{"metric_type:status"}, 1.0)
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
	s.Statsd.Count(flushTotalMetric, int64(ms.totalGlobalHistograms), []string{"metric_type:global_histogram"}, 1.0)
	s.Statsd.Count(flushTotalMetric, int64(ms.totalGlobalTimers), []string{"metric_type:global_timers"}, 1.0)
	s.Statsd.Count(flushTotalMetric, int64(ms.totalHistograms), []string{"metric_type:histogram"}, 1.0)
	s.Statsd.Count(flushTotalMetric, int64(ms.totalSets), []string{"metric_type:set"}, 1.0)
	s.Statsd.Count(flushTotalMetric, int64(ms.totalTimers), []string{"metric_type:timer"}, 1.0)
}

func (s *Server) reportGlobalReceivedProtocolMetrics() {
	protocolMetrics := s.listeningPerProtocolMetrics

	dogstatsdTcpTotal := atomic.SwapInt64(&protocolMetrics.dogstatsdTcpReceivedTotal, 0)
	dogstatsdUdpTotal := atomic.SwapInt64(&protocolMetrics.dogstatsdUdpReceivedTotal, 0)
	dogstatsdUnixTotal := atomic.SwapInt64(&protocolMetrics.dogstatsdUnixReceivedTotal, 0)
	dogstatsdGrpcTotal := atomic.SwapInt64(&protocolMetrics.dogstatsdGrpcReceivedTotal, 0)

	ssfUdpTotal := atomic.SwapInt64(&protocolMetrics.ssfUdpReceivedTotal, 0)
	ssfUnixTotal := atomic.SwapInt64(&protocolMetrics.ssfUnixReceivedTotal, 0)
	ssfGrpcTotal := atomic.SwapInt64(&protocolMetrics.ssfGrpcReceivedTotal, 0)

	s.Statsd.Count(perProtocolTotalMetricName, dogstatsdTcpTotal, []string{"veneurglobalonly:true", "protocol:" + DOGSTATSD_TCP.String()}, 1.0)
	s.Statsd.Count(perProtocolTotalMetricName, dogstatsdUdpTotal, []string{"veneurglobalonly:true", "protocol:" + DOGSTATSD_UDP.String()}, 1.0)
	s.Statsd.Count(perProtocolTotalMetricName, dogstatsdUnixTotal, []string{"veneurglobalonly:true", "protocol:" + DOGSTATSD_UNIX.String()}, 1.0)
	s.Statsd.Count(perProtocolTotalMetricName, dogstatsdGrpcTotal, []string{"veneurglobalonly:true", "protocol:" + DOGSTATSD_GRPC.String()}, 1.0)

	s.Statsd.Count(perProtocolTotalMetricName, ssfUdpTotal, []string{"veneurglobalonly:true", "protocol:" + SSF_UDP.String()}, 1.0)
	s.Statsd.Count(perProtocolTotalMetricName, ssfUnixTotal, []string{"veneurglobalonly:true", "protocol:" + SSF_UNIX.String()}, 1.0)
	s.Statsd.Count(perProtocolTotalMetricName, ssfGrpcTotal, []string{"veneurglobalonly:true", "protocol:" + SSF_GRPC.String()}, 1.0)
}

func (s *Server) flushForward(ctx context.Context, wms []WorkerMetrics) {
	span, _ := trace.StartSpanFromContext(ctx, "")
	defer span.ClientFinish(s.TraceClient)
	jmLength := 0
	for _, wm := range wms {
		jmLength += len(wm.globalCounters)
		jmLength += len(wm.globalGauges)
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
				s.logger.WithFields(logrus.Fields{
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
				s.logger.WithFields(logrus.Fields{
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
				s.logger.WithFields(logrus.Fields{
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
				s.logger.WithFields(logrus.Fields{
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
				s.logger.WithFields(logrus.Fields{
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
	s.Statsd.Count("forward.post_metrics_total", int64(len(jsonMetrics)), nil, 1.0)
	if len(jsonMetrics) == 0 {
		s.logger.Debug("Nothing to forward, skipping.")
		return
	}

	// the error has already been logged (if there was one), so we only care
	// about the success case
	endpoint := fmt.Sprintf("%s/import", s.ForwardAddr)
	if vhttp.PostHelper(
		span.Attach(ctx), s.HTTPClient, s.TraceClient, http.MethodPost, endpoint,
		jsonMetrics, "forward", true, nil, s.logger) == nil {
		s.logger.WithFields(logrus.Fields{
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
			s.logger.WithFields(logrus.Fields{
				"key":  keyI,
				"type": reflect.TypeOf(keyI),
			}).Error("received non-string key")
			return true
		}

		value, ok := valueI.(*ssfServiceSpanMetrics)
		if !ok {
			s.logger.WithFields(logrus.Fields{
				"value": valueI,
				"type":  reflect.TypeOf(valueI),
			}).Error("received non-struct value")
			return true
		}

		tags := strings.Split(key, ",")
		if len(tags) != 2 {
			s.logger.WithFields(logrus.Fields{
				"key":    key,
				"length": len(tags),
			}).Error("received key of incorrect format")
		}

		spansReceivedTotal := atomic.SwapInt64(&value.ssfSpansReceivedTotal, 0)
		spansRootReceivedTotal := atomic.SwapInt64(&value.ssfRootSpansReceivedTotal, 0)
		s.Statsd.Count("ssf.spans.received_total", spansReceivedTotal, tags, 1.0)
		s.Statsd.Count("ssf.spans.root.received_total", spansRootReceivedTotal, append(tags, "veneurglobalonly:true"), 1.0)
		return true
	})

	s.SpanWorker.Flush()
}

// forwardGRPC forwards all input metrics to a downstream Veneur, over gRPC.
func (s *Server) forwardGRPC(
	ctx context.Context, wms []WorkerMetrics, protocol ProxyProtocol,
) {
	span, _ := trace.StartSpanFromContext(ctx, "")
	span.SetTag("protocol", "grpc")
	defer span.ClientFinish(s.TraceClient)

	exportStart := time.Now()

	// Collect all of the forwardable metrics from the various WorkerMetrics.
	var metrics []*metricpb.Metric
	for _, wm := range wms {
		metrics = append(metrics, wm.ForwardableMetrics(s.TraceClient, s.logger)...)
	}

	span.Add(
		ssf.Timing("forward.duration_ns", time.Since(exportStart),
			time.Nanosecond, map[string]string{"part": "export"}),
		ssf.Gauge("forward.metrics_total", float32(len(metrics)), nil),
		// Maintain compatibility with metrics used in HTTP-based forwarding
		ssf.Count("forward.post_metrics_total", float32(len(metrics)), nil),
	)

	if len(metrics) == 0 {
		s.logger.Debug("Nothing to forward, skipping.")
		return
	}

	entry := s.logger.WithFields(logrus.Fields{
		"metrics":     len(metrics),
		"destination": s.ForwardAddr,
		"protocol":    "grpc",
		"grpcstate":   s.grpcForwardConn.GetState().String(),
	})

	c := forwardrpc.NewForwardClient(s.grpcForwardConn)

	grpcStart := time.Now()
	var err error
	if protocol == ProxyProtocolGrpcSingle {
		err = ForwardGrpcSingle(ctx, c, metrics)
	} else {
		err = ForwardGrpcStream(ctx, c, metrics)
	}
	if err != nil {
		if ctx.Err() != nil {
			// We exceeded the deadline of the flush context.
			span.Add(ssf.Count("forward.error_total", 1, map[string]string{"cause": "deadline_exceeded"}))
		} else if statErr, ok := status.FromError(err); ok &&
			(statErr.Message() == "all SubConns are in TransientFailure" || statErr.Message() == "transport is closing") {
			// We could check statErr.Code() == codes.Unavailable, but we don't know all of the cases that
			// could return that code. These two particular cases are fairly safe and usually associated
			// with connection rebalancing or host replacement, so we don't want them going to sentry.
			span.Add(ssf.Count("forward.error_total", 1, map[string]string{"cause": "transient_unavailable"}))
		} else {
			span.Add(ssf.Count("forward.error_total", 1, map[string]string{"cause": "send"}))
			entry.WithError(err).Error("Failed to forward to an upstream Veneur")
		}
	} else {
		entry.Info("Completed forward to an upstream Veneur")
	}

	span.Add(
		ssf.Timing("forward.duration_ns", time.Since(grpcStart), time.Nanosecond,
			map[string]string{"part": "grpc"}),
		ssf.Count("forward.error_total", 0, nil),
	)
}

func ForwardGrpcSingle(
	ctx context.Context, client forwardrpc.ForwardClient,
	metrics []*metricpb.Metric,
) error {
	_, err := client.SendMetrics(ctx, &forwardrpc.MetricList{Metrics: metrics})
	return err
}

func ForwardGrpcStream(
	ctx context.Context, client forwardrpc.ForwardClient,
	metrics []*metricpb.Metric,
) error {
	sendMetricsClient, err := client.SendMetricsV2(ctx)
	if err != nil {
		return err
	}
	for _, metric := range metrics {
		sendMetricsClient.Send(metric)
	}
	_, err = sendMetricsClient.CloseAndRecv()
	return err
}
