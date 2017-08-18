package veneur

import (
	"fmt"
	"runtime"
	"time"

	"github.com/DataDog/datadog-go/statsd"
)

const workerMetricsFlushedTotal = "worker.metrics_flushed_total"
const flushDuration = "flush.total_duration_ns"
const flushPostTotal = "flush.post_metrics_total"
const memHeapAlloc = "mem.heap_alloc_bytes"
const memGCNumber = "gc.number"
const memGCPauseTotal = "gc.pause_total_ns"
const sentryErrorsTotal = "sentry.errors_total"
const flushPluginErrorsTotal = "flush.plugins.%s.error_total"
const flushPluginDuration = "flush.plugins.%s.total_duration_ns"

// Recorder is a type-safe interface for emitting metrics.
type Recorder struct {
	Statsd *statsd.Client
}

// NewRecorder makes a new Recorder instance from a Config.
func NewRecorder(conf Config) (*Recorder, error) {
	statsd, err := statsd.NewBuffered(conf.StatsAddress, 1024)
	if err != nil {
		return nil, err
	}
	statsd.Namespace = "veneur."
	statsd.Tags = append(conf.Tags, "veneurlocalonly")

	return &Recorder{
		Statsd: statsd,
	}, nil
}

// NewProxyRecorder makes a new Recorder instance from a ProxyConfig.
func NewProxyRecorder(conf ProxyConfig) (*Recorder, error) {
	statsd, err := statsd.NewBuffered(conf.StatsAddress, 1024)
	if err != nil {
		return nil, err
	}
	statsd.Namespace = "veneur."

	return &Recorder{
		Statsd: statsd,
	}, nil
}

// DurationSinceStart is a convenience method for getting a duration in
// nanoseconds given a start time.
func DurationSinceStart(start time.Time) float64 {
	return float64(time.Since(start).Nanoseconds())
}

// LocalMetricsFlushCounts reports the counts of
// Counters, Gauges, LocalHistograms, LocalSets, and LocalTimers
// as metrics. These are shared by both global and local flush operations.
// It does *not* report the totalHistograms, totalSets, or totalTimers
// because those are only performed by the global veneur instance.
// It also does not report the total metrics posted, because on the local veneur,
// that should happen *after* the flush-forward operation.
func (r *Recorder) LocalMetricsFlushCounts(ms metricsSummary) {
	r.Statsd.Count(workerMetricsFlushedTotal, int64(ms.totalCounters), []string{"metric_type:counter"}, 1.0)
	r.Statsd.Count(workerMetricsFlushedTotal, int64(ms.totalGauges), []string{"metric_type:gauge"}, 1.0)
	r.Statsd.Count(workerMetricsFlushedTotal, int64(ms.totalLocalHistograms), []string{"metric_type:local_histogram"}, 1.0)
	r.Statsd.Count(workerMetricsFlushedTotal, int64(ms.totalLocalSets), []string{"metric_type:local_set"}, 1.0)
	r.Statsd.Count(workerMetricsFlushedTotal, int64(ms.totalLocalTimers), []string{"metric_type:local_timer"}, 1.0)
}

// GlobalMetricsFlushCounts reports the counts of
// globalCounters, totalHistograms, totalSets, and totalTimers,
// which are the three metrics reported *only* by the global
// veneur instance.
func (r *Recorder) GlobalMetricsFlushCounts(ms metricsSummary) {
	// we only report these lengths in FlushGlobal
	// since if we're the global veneur instance responsible for flushing them
	// this avoids double-counting problems where a local veneur reports
	// histograms that it received, and then a global veneur reports them
	// again
	r.Statsd.Count(workerMetricsFlushedTotal, int64(ms.totalGlobalCounters), []string{"metric_type:global_counter"}, 1.0)
	r.Statsd.Count(workerMetricsFlushedTotal, int64(ms.totalHistograms), []string{"metric_type:histogram"}, 1.0)
	r.Statsd.Count(workerMetricsFlushedTotal, int64(ms.totalSets), []string{"metric_type:set"}, 1.0)
	r.Statsd.Count(workerMetricsFlushedTotal, int64(ms.totalTimers), []string{"metric_type:timer"}, 1.0)
}

// FlushDuration tracks the duration of a flush operation, with a part tag.
func (r *Recorder) FlushDuration(start time.Time, part string) {
	r.Statsd.TimeInMilliseconds(flushDuration, DurationSinceStart(start), []string{fmt.Sprintf("part:%s", part)}, 1.0)
}

// FlushMetricCount tracks the number of metrics posted.
func (r *Recorder) FlushMetricCount(count int) {
	r.Statsd.Gauge(flushPostTotal, float64(count), nil, 1.0)
}

// GCStats tracks various Go GC metrics.
func (r *Recorder) GCStats(mem *runtime.MemStats) {
	r.Statsd.Gauge(memHeapAlloc, float64(mem.HeapAlloc), nil, 1.0)
	r.Statsd.Gauge(memGCNumber, float64(mem.NumGC), nil, 1.0)
	r.Statsd.Gauge(memGCPauseTotal, float64(mem.PauseTotalNs), nil, 1.0)
}

// PluginFlushDuration tracks the duration of a plugin's flush, tagged by the
// plugin's name.
func (r *Recorder) PluginFlushDuration(start time.Time, plugin string) {
	r.Statsd.TimeInMilliseconds(fmt.Sprintf(flushPluginDuration, plugin), DurationSinceStart(start), nil, 1.0)
}

// PluginFlushErrorCount tracks the number of errors a plugin encounters,
// tagged by the plugin's name.
func (r *Recorder) PluginFlushErrorCount(plugin string) {
	r.Statsd.Count(fmt.Sprintf(flushPluginErrorsTotal, plugin), 1, nil, 1.0)
}

// SentryErrorCount tracks the number of errors sent to Sentry.
func (r *Recorder) SentryErrorCount() {
	r.Statsd.Count(sentryErrorsTotal, 1, nil, 1.0)
}
