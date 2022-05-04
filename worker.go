package veneur

import (
	"encoding/binary"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/axiomhq/hyperloglog"
	"github.com/sirupsen/logrus"
	"github.com/stripe/veneur/v14/protocol"
	"github.com/stripe/veneur/v14/samplers"
	"github.com/stripe/veneur/v14/samplers/metricpb"
	"github.com/stripe/veneur/v14/scopedstatsd"
	"github.com/stripe/veneur/v14/sinks"
	"github.com/stripe/veneur/v14/ssf"
	"github.com/stripe/veneur/v14/trace"
	"github.com/stripe/veneur/v14/trace/metrics"
	"github.com/stripe/veneur/v14/util/matcher"
)

const (
	CounterTypeName   = "counter"
	GaugeTypeName     = "gauge"
	HistogramTypeName = "histogram"
	SetTypeName       = "set"
	TimerTypeName     = "timer"
	StatusTypeName    = "status"
)

// Worker is the doodad that does work.
type Worker struct {
	id                    int
	isLocal               bool
	countUniqueTimeseries bool
	uniqueMTS             *hyperloglog.Sketch
	uniqueMTSMtx          *sync.RWMutex
	PacketChan            chan samplers.UDPMetric
	ImportChan            chan []samplers.JSONMetric
	ImportMetricChan      chan []*metricpb.Metric
	QuitChan              chan struct{}
	processed             int64
	imported              int64
	mutex                 *sync.Mutex
	traceClient           *trace.Client
	logger                *logrus.Logger
	wm                    WorkerMetrics
	stats                 scopedstatsd.Client
}

// IngestUDP on a Worker feeds the metric into the worker's PacketChan.
func (w *Worker) IngestUDP(metric samplers.UDPMetric) {
	w.PacketChan <- metric
}

func (w *Worker) IngestMetrics(ms []*metricpb.Metric) {
	w.ImportMetricChan <- ms
}

// WorkerMetrics is just a plain struct bundling together the flushed contents of a worker
type WorkerMetrics struct {
	// we do not want to key on the metric's Digest here, because those could
	// collide, and then we'd have to implement a hashtable on top of go maps,
	// which would be silly
	counters   map[samplers.MetricKey]*samplers.Counter
	gauges     map[samplers.MetricKey]*samplers.Gauge
	histograms map[samplers.MetricKey]*samplers.Histo
	sets       map[samplers.MetricKey]*samplers.Set
	timers     map[samplers.MetricKey]*samplers.Histo

	// this is for counters which are globally aggregated
	globalCounters map[samplers.MetricKey]*samplers.Counter
	// and gauges which are global
	globalGauges map[samplers.MetricKey]*samplers.Gauge
	// This means that no histo related stats are emitted locally, not even max min etc.
	// Instead, everything is forwarded.
	globalHistograms map[samplers.MetricKey]*samplers.Histo
	globalTimers     map[samplers.MetricKey]*samplers.Histo

	// these are used for metrics that shouldn't be forwarded
	localHistograms   map[samplers.MetricKey]*samplers.Histo
	localSets         map[samplers.MetricKey]*samplers.Set
	localTimers       map[samplers.MetricKey]*samplers.Histo
	localStatusChecks map[samplers.MetricKey]*samplers.StatusCheck
}

// NewWorkerMetrics initializes a WorkerMetrics struct
func NewWorkerMetrics() WorkerMetrics {
	return WorkerMetrics{
		counters:          map[samplers.MetricKey]*samplers.Counter{},
		globalCounters:    map[samplers.MetricKey]*samplers.Counter{},
		globalGauges:      map[samplers.MetricKey]*samplers.Gauge{},
		globalHistograms:  map[samplers.MetricKey]*samplers.Histo{},
		globalTimers:      map[samplers.MetricKey]*samplers.Histo{},
		gauges:            map[samplers.MetricKey]*samplers.Gauge{},
		histograms:        map[samplers.MetricKey]*samplers.Histo{},
		sets:              map[samplers.MetricKey]*samplers.Set{},
		timers:            map[samplers.MetricKey]*samplers.Histo{},
		localHistograms:   map[samplers.MetricKey]*samplers.Histo{},
		localSets:         map[samplers.MetricKey]*samplers.Set{},
		localTimers:       map[samplers.MetricKey]*samplers.Histo{},
		localStatusChecks: map[samplers.MetricKey]*samplers.StatusCheck{},
	}
}

// Upsert creates an entry on the WorkerMetrics struct for the given metrickey (if one does not already exist)
// and updates the existing entry (if one already exists).
// Returns true if the metric entry was created and false otherwise.
func (wm WorkerMetrics) Upsert(mk samplers.MetricKey, Scope samplers.MetricScope, tags []string) bool {
	present := false
	switch mk.Type {
	case CounterTypeName:
		if Scope == samplers.GlobalOnly {
			if _, present = wm.globalCounters[mk]; !present {
				wm.globalCounters[mk] = samplers.NewCounter(mk.Name, tags)
			}
		} else {
			if _, present = wm.counters[mk]; !present {
				wm.counters[mk] = samplers.NewCounter(mk.Name, tags)
			}
		}
	case GaugeTypeName:
		if Scope == samplers.GlobalOnly {
			if _, present = wm.globalGauges[mk]; !present {
				wm.globalGauges[mk] = samplers.NewGauge(mk.Name, tags)
			}
		} else {
			if _, present = wm.gauges[mk]; !present {
				wm.gauges[mk] = samplers.NewGauge(mk.Name, tags)
			}
		}
	case HistogramTypeName:
		if Scope == samplers.LocalOnly {
			if _, present = wm.localHistograms[mk]; !present {
				wm.localHistograms[mk] = samplers.NewHist(mk.Name, tags)
			}
		} else if Scope == samplers.GlobalOnly {
			if _, present = wm.globalHistograms[mk]; !present {
				wm.globalHistograms[mk] = samplers.NewHist(mk.Name, tags)
			}
		} else {
			if _, present = wm.histograms[mk]; !present {
				wm.histograms[mk] = samplers.NewHist(mk.Name, tags)
			}
		}
	case SetTypeName:
		if Scope == samplers.LocalOnly {
			if _, present = wm.localSets[mk]; !present {
				wm.localSets[mk] = samplers.NewSet(mk.Name, tags)
			}
		} else {
			if _, present = wm.sets[mk]; !present {
				wm.sets[mk] = samplers.NewSet(mk.Name, tags)
			}
		}
	case TimerTypeName:
		if Scope == samplers.LocalOnly {
			if _, present = wm.localTimers[mk]; !present {
				wm.localTimers[mk] = samplers.NewHist(mk.Name, tags)
			}
		} else if Scope == samplers.GlobalOnly {
			if _, present = wm.globalTimers[mk]; !present {
				wm.globalTimers[mk] = samplers.NewHist(mk.Name, tags)
			}
		} else {
			if _, present = wm.timers[mk]; !present {
				wm.timers[mk] = samplers.NewHist(mk.Name, tags)
			}
		}
	case StatusTypeName:
		if _, present = wm.localStatusChecks[mk]; !present {
			wm.localStatusChecks[mk] = samplers.NewStatusCheck(mk.Name, tags)
		}
		// no need to raise errors on unknown types
		// the caller will probably end up doing that themselves
	}
	return !present
}

// ForwardableMetrics converts all metrics that should be forwarded to
// metricpb.Metric (protobuf-compatible).
func (wm WorkerMetrics) ForwardableMetrics(
	cl *trace.Client, logger *logrus.Entry,
) []*metricpb.Metric {
	bufLen := len(wm.histograms) + len(wm.sets) + len(wm.timers) +
		len(wm.globalCounters) + len(wm.globalGauges)

	metrics := make([]*metricpb.Metric, 0, bufLen)
	for _, count := range wm.globalCounters {
		metrics = wm.appendExportedMetric(
			metrics, count, metricpb.Type_Counter, cl, samplers.GlobalOnly, logger)
	}
	for _, gauge := range wm.globalGauges {
		metrics = wm.appendExportedMetric(
			metrics, gauge, metricpb.Type_Gauge, cl, samplers.GlobalOnly, logger)
	}
	for _, histo := range wm.histograms {
		metrics = wm.appendExportedMetric(
			metrics, histo, metricpb.Type_Histogram, cl, samplers.MixedScope, logger)
	}
	for _, histo := range wm.globalHistograms {
		metrics = wm.appendExportedMetric(
			metrics, histo, metricpb.Type_Histogram, cl, samplers.GlobalOnly, logger)
	}
	for _, set := range wm.sets {
		metrics = wm.appendExportedMetric(
			metrics, set, metricpb.Type_Set, cl, samplers.MixedScope, logger)
	}
	for _, timer := range wm.timers {
		metrics = wm.appendExportedMetric(
			metrics, timer, metricpb.Type_Timer, cl, samplers.MixedScope, logger)
	}
	for _, histo := range wm.globalTimers {
		metrics = wm.appendExportedMetric(
			metrics, histo, metricpb.Type_Timer, cl, samplers.GlobalOnly, logger)
	}

	return metrics
}

// A type implemented by all valid samplers
type metricExporter interface {
	GetName() string
	Metric() (*metricpb.Metric, error)
}

// appendExportedMetric appends the exported version of the input metric, with
// the inputted type.  If the export fails, the original slice is returned
// and an error is logged.
func (wm WorkerMetrics) appendExportedMetric(
	res []*metricpb.Metric, exp metricExporter, mType metricpb.Type,
	cl *trace.Client, scope samplers.MetricScope, logger *logrus.Entry,
) []*metricpb.Metric {
	m, err := exp.Metric()
	m.Scope = scope.ToPB()
	if err != nil {
		logger.WithFields(logrus.Fields{
			logrus.ErrorKey: err,
			"type":          mType,
			"name":          exp.GetName(),
		}).Error("Could not export metric")
		metrics.ReportOne(cl,
			ssf.Count("worker_metrics.export_metric.errors", 1, map[string]string{
				"type": mType.String(),
			}),
		)
		return res
	}

	m.Type = mType
	return append(res, m)
}

// NewWorker creates, and returns a new Worker object.
func NewWorker(id int, isLocal bool, countUniqueTimeseries bool, cl *trace.Client, logger *logrus.Logger, stats scopedstatsd.Client) *Worker {
	return &Worker{
		id:                    id,
		isLocal:               isLocal,
		countUniqueTimeseries: countUniqueTimeseries,
		uniqueMTS:             hyperloglog.New(),
		uniqueMTSMtx:          &sync.RWMutex{},
		PacketChan:            make(chan samplers.UDPMetric, 32),
		ImportChan:            make(chan []samplers.JSONMetric, 32),
		ImportMetricChan:      make(chan []*metricpb.Metric, 32),
		QuitChan:              make(chan struct{}),
		processed:             0,
		imported:              0,
		mutex:                 &sync.Mutex{},
		traceClient:           cl,
		logger:                logger,
		wm:                    NewWorkerMetrics(),
		stats:                 scopedstatsd.Ensure(stats),
	}
}

// Work will start the worker listening for metrics to process or import.
// It will not return until the worker is sent a message to terminate using Stop()
func (w *Worker) Work() {
	for {
		select {
		case m := <-w.PacketChan:
			if w.countUniqueTimeseries {
				w.SampleTimeseries(&m)
			}
			w.ProcessMetric(&m)
		case m := <-w.ImportChan:
			for _, j := range m {
				w.ImportMetric(j)
			}
		case ms := <-w.ImportMetricChan:
			for _, m := range ms {
				w.ImportMetricGRPC(m)
			}
		case <-w.QuitChan:
			// We have been asked to stop.
			w.logger.WithField("worker", w.id).Error("Stopping")
			return
		}
	}
}

// MetricsProcessedCount is a convenince method for testing
// that allows us to fetch the Worker's processed count
// in a non-racey way.
func (w *Worker) MetricsProcessedCount() int64 {
	w.mutex.Lock()
	defer w.mutex.Unlock()
	return w.processed
}

// SampleTimeseries takes a metric and counts whether the timeseries
// has already been seen by the worker in this flush interval.
func (w *Worker) SampleTimeseries(m *samplers.UDPMetric) {
	digest := make([]byte, 8)
	binary.LittleEndian.PutUint32(digest, m.Digest)

	w.uniqueMTSMtx.RLock()
	defer w.uniqueMTSMtx.RUnlock()

	// Always sample if worker is running in global Veneur instance,
	// as there is nowhere the metric can be forwarded to.
	if !w.isLocal {
		w.uniqueMTS.Insert(digest)
		return
	}
	// Otherwise, sample the timeseries iff the metric will not be
	// forwarded to a global Veneur instance.
	switch m.Type {
	case CounterTypeName:
		if m.Scope != samplers.GlobalOnly {
			w.uniqueMTS.Insert(digest)
		}
	case GaugeTypeName:
		if m.Scope != samplers.GlobalOnly {
			w.uniqueMTS.Insert(digest)
		}
	case HistogramTypeName:
		if m.Scope == samplers.LocalOnly {
			w.uniqueMTS.Insert(digest)
		}
	case SetTypeName:
		if m.Scope == samplers.LocalOnly {
			w.uniqueMTS.Insert(digest)
		}
	case TimerTypeName:
		if m.Scope == samplers.LocalOnly {
			w.uniqueMTS.Insert(digest)
		}
	case StatusTypeName:
		w.uniqueMTS.Insert(digest)
	default:
		w.logger.WithField("type", m.Type).
			Error("Unknown metric type for counting")
	}
}

// ProcessMetric takes a Metric and samples it
func (w *Worker) ProcessMetric(m *samplers.UDPMetric) {
	w.mutex.Lock()
	defer w.mutex.Unlock()
	w.processed++
	w.wm.Upsert(m.MetricKey, m.Scope, m.Tags)

	switch m.Type {
	case CounterTypeName:
		if m.Scope == samplers.GlobalOnly {
			w.wm.globalCounters[m.MetricKey].Sample(m.Value.(float64), m.SampleRate)
		} else {
			w.wm.counters[m.MetricKey].Sample(m.Value.(float64), m.SampleRate)
		}
	case GaugeTypeName:
		if m.Scope == samplers.GlobalOnly {
			w.wm.globalGauges[m.MetricKey].Sample(m.Value.(float64), m.SampleRate)
		} else {
			w.wm.gauges[m.MetricKey].Sample(m.Value.(float64), m.SampleRate)
		}
	case HistogramTypeName:
		if m.Scope == samplers.LocalOnly {
			w.wm.localHistograms[m.MetricKey].Sample(m.Value.(float64), m.SampleRate)
		} else if m.Scope == samplers.GlobalOnly {
			w.wm.globalHistograms[m.MetricKey].Sample(m.Value.(float64), m.SampleRate)
		} else {
			w.wm.histograms[m.MetricKey].Sample(m.Value.(float64), m.SampleRate)
		}
	case SetTypeName:
		if m.Scope == samplers.LocalOnly {
			w.wm.localSets[m.MetricKey].Sample(m.Value.(string))
		} else {
			w.wm.sets[m.MetricKey].Sample(m.Value.(string))
		}
	case TimerTypeName:
		if m.Scope == samplers.LocalOnly {
			w.wm.localTimers[m.MetricKey].Sample(m.Value.(float64), m.SampleRate)
		} else if m.Scope == samplers.GlobalOnly {
			w.wm.globalTimers[m.MetricKey].Sample(m.Value.(float64), m.SampleRate)
		} else {
			w.wm.timers[m.MetricKey].Sample(m.Value.(float64), m.SampleRate)
		}
	case StatusTypeName:
		v := float64(m.Value.(ssf.SSFSample_Status))
		w.wm.localStatusChecks[m.MetricKey].Sample(v, m.SampleRate, m.Message, m.HostName)
	default:
		w.logger.WithField("type", m.Type).
			Error("Unknown metric type for processing")
	}
}

// ImportMetric receives a metric from another veneur instance
func (w *Worker) ImportMetric(other samplers.JSONMetric) {
	w.mutex.Lock()
	defer w.mutex.Unlock()

	// we don't increment the processed metric counter here, it was already
	// counted by the original veneur that sent this to us
	w.imported++
	if other.Type == CounterTypeName || other.Type == GaugeTypeName {
		// this is an odd special case -- counters that are imported are global
		w.wm.Upsert(other.MetricKey, samplers.GlobalOnly, other.Tags)
	} else {
		w.wm.Upsert(other.MetricKey, samplers.MixedScope, other.Tags)
	}

	switch other.Type {
	case CounterTypeName:
		if err := w.wm.globalCounters[other.MetricKey].Combine(other.Value); err != nil {
			w.logger.WithError(err).Error("Could not merge counters")
		}
	case GaugeTypeName:
		if err := w.wm.globalGauges[other.MetricKey].Combine(other.Value); err != nil {
			w.logger.WithError(err).Error("Could not merge gauges")
		}
	case SetTypeName:
		if err := w.wm.sets[other.MetricKey].Combine(other.Value); err != nil {
			w.logger.WithError(err).Error("Could not merge sets")
		}
	case HistogramTypeName:
		if err := w.wm.histograms[other.MetricKey].Combine(other.Value); err != nil {
			w.logger.WithError(err).Error("Could not merge histograms")
		}
	case TimerTypeName:
		if err := w.wm.timers[other.MetricKey].Combine(other.Value); err != nil {
			w.logger.WithError(err).Error("Could not merge timers")
		}
	default:
		w.logger.WithField("type", other.Type).
			Error("Unknown metric type for importing")
	}
}

// ImportMetricGRPC receives a metric from another veneur instance over gRPC.
//
// In practice, this is only called when in the aggregation tier, so we don't
// handle LocalOnly scope.
func (w *Worker) ImportMetricGRPC(other *metricpb.Metric) (err error) {
	w.mutex.Lock()
	defer w.mutex.Unlock()

	key := samplers.NewMetricKeyFromMetric(other, []matcher.TagMatcher{})

	scope := samplers.ScopeFromPB(other.Scope)
	if other.Type == metricpb.Type_Counter || other.Type == metricpb.Type_Gauge {
		scope = samplers.GlobalOnly
	}

	if scope == samplers.LocalOnly {
		return fmt.Errorf("gRPC import does not accept local metrics")
	}

	w.wm.Upsert(key, scope, other.Tags)
	w.imported++

	switch v := other.GetValue().(type) {
	case *metricpb.Metric_Counter:
		w.wm.globalCounters[key].Merge(v.Counter)
	case *metricpb.Metric_Gauge:
		w.wm.globalGauges[key].Merge(v.Gauge)
	case *metricpb.Metric_Set:
		if merr := w.wm.sets[key].Merge(v.Set); merr != nil {
			err = fmt.Errorf("could not merge a set: %v", err)
		}
	case *metricpb.Metric_Histogram:
		switch other.Type {
		case metricpb.Type_Histogram:
			if other.Scope == metricpb.Scope_Mixed {
				w.wm.histograms[key].Merge(v.Histogram)
			} else if other.Scope == metricpb.Scope_Global {
				w.wm.globalHistograms[key].Merge(v.Histogram)
			}
		case metricpb.Type_Timer:
			if other.Scope == metricpb.Scope_Mixed {
				w.wm.timers[key].Merge(v.Histogram)
			} else if other.Scope == metricpb.Scope_Global {
				w.wm.globalTimers[key].Merge(v.Histogram)
			}
		}
	case nil:
		err = errors.New("Can't import a metric with a nil value")
	default:
		err = fmt.Errorf("Unknown metric type for importing: %T", v)
	}

	if err != nil {
		w.logger.WithError(err).WithFields(logrus.Fields{
			"type":     other.Type,
			"name":     other.Name,
			"protocol": "grpc",
		}).Error("Failed to import a metric")
	}

	return err
}

// Flush resets the worker's internal metrics and returns their contents.
func (w *Worker) Flush() WorkerMetrics {
	// This is a critical spot. The worker can't process metrics while this
	// mutex is held! So we try and minimize it by copying the maps of values
	// and assigning new ones.
	wm := NewWorkerMetrics()
	w.mutex.Lock()
	ret := w.wm
	processed := w.processed
	imported := w.imported

	w.wm = wm
	w.processed = 0
	w.imported = 0
	w.mutex.Unlock()

	w.stats.Count("worker.metrics_processed_total", processed, []string{}, 1.0)
	w.stats.Count("worker.metrics_imported_total", imported, []string{}, 1.0)

	return ret
}

// Stop tells the worker to stop listening for work requests.
//
// Note that the worker will only stop *after* it has finished its work.
func (w *Worker) Stop() {
	close(w.QuitChan)
}

// EventWorker is similar to a Worker but it collects events and service checks instead of metrics.
type EventWorker struct {
	sampleChan  chan ssf.SSFSample
	mutex       *sync.Mutex
	samples     []ssf.SSFSample
	traceClient *trace.Client
	stats       scopedstatsd.Client
}

// NewEventWorker creates an EventWorker ready to collect events and service checks.
func NewEventWorker(cl *trace.Client, stats scopedstatsd.Client) *EventWorker {
	return &EventWorker{
		sampleChan:  make(chan ssf.SSFSample),
		mutex:       &sync.Mutex{},
		traceClient: cl,
		stats:       scopedstatsd.Ensure(stats),
	}
}

// Work will start the EventWorker listening for events and service checks.
// This function will never return.
func (ew *EventWorker) Work() {
	for {
		select {
		case s := <-ew.sampleChan:
			ew.mutex.Lock()
			ew.samples = append(ew.samples, s)
			ew.mutex.Unlock()
		}
	}
}

// Flush returns the EventWorker's stored events and service checks and
// resets the stored contents.
func (ew *EventWorker) Flush() []ssf.SSFSample {
	ew.mutex.Lock()

	retsamples := ew.samples
	// these slices will be allocated again at append time
	ew.samples = nil

	ew.mutex.Unlock()
	if len(retsamples) != 0 {
		ew.stats.Count("worker.other_samples_flushed_total", int64(len(retsamples)), nil, 1.0)
	}
	return retsamples
}

// SpanWorker is similar to a Worker but it collects events and service checks instead of metrics.
type SpanWorker struct {
	SpanChan   <-chan *ssf.SSFSpan
	sinkTags   []map[string]string
	commonTags map[string]string
	sinks      []sinks.SpanSink
	logger     *logrus.Entry

	// cumulative time spent per sink, in nanoseconds
	cumulativeTimes []int64
	traceClient     *trace.Client
	statsd          scopedstatsd.Client
	capCount        int64
	emptySSFCount   int64
}

// NewSpanWorker creates a SpanWorker ready to collect events and service checks.
func NewSpanWorker(
	sinks []sinks.SpanSink, cl *trace.Client, statsd scopedstatsd.Client,
	spanChan <-chan *ssf.SSFSpan, commonTags map[string]string,
	logger *logrus.Entry,
) *SpanWorker {
	tags := make([]map[string]string, len(sinks))
	for i, sink := range sinks {
		tags[i] = map[string]string{
			"sink": sink.Name(),
		}
	}

	return &SpanWorker{
		SpanChan:        spanChan,
		sinks:           sinks,
		sinkTags:        tags,
		commonTags:      commonTags,
		cumulativeTimes: make([]int64, len(sinks)),
		traceClient:     cl,
		statsd:          scopedstatsd.Ensure(statsd),
		logger:          logger,
	}
}

// Work will start the SpanWorker listening for spans.
// This function will never return.
func (tw *SpanWorker) Work() {
	const Timeout = 9 * time.Second
	capcmp := cap(tw.SpanChan) - 1
	for m := range tw.SpanChan {
		// If we are at or one below cap, increment the counter.
		if len(tw.SpanChan) >= capcmp {
			atomic.AddInt64(&tw.capCount, 1)
		}

		if m.Tags == nil && len(tw.commonTags) != 0 {
			m.Tags = make(map[string]string, len(tw.commonTags))
		}

		for k, v := range tw.commonTags {
			if _, has := m.Tags[k]; !has {
				m.Tags[k] = v
			}
		}

		// An SSF packet may contain a valid span, one or more valid metrics,
		// or both (a valid span *and* one or more valid metrics).
		// If it contains neither, it is the result of a client error, and the
		// span does not need to be passed to any sink.
		// If the span is empty but one or more metrics exist, the span still needs
		// to be passed to the sinks for potential metric extraction.
		if err := protocol.ValidateTrace(m); err != nil {
			if len(m.Metrics) == 0 {
				atomic.AddInt64(&tw.emptySSFCount, 1)
				tw.logger.WithError(err).Debug(
					"Invalid SSF packet: packet contains neither valid metrics nor a valid span")
				continue
			}
		}

		var wg sync.WaitGroup
		for i, s := range tw.sinks {
			tags := tw.sinkTags[i]
			wg.Add(1)
			go func(i int, sink sinks.SpanSink, span *ssf.SSFSpan, wg *sync.WaitGroup) {
				defer wg.Done()

				done := make(chan struct{})
				start := time.Now()

				go func() {
					// Give each sink a change to ingest.
					err := sink.Ingest(span)
					if err != nil {
						if _, isNoTrace := err.(*protocol.InvalidTrace); !isNoTrace {
							// If a sink goes wacko and errors a lot, we stand to emit a
							// loooot of metrics towards all span workers here since
							// span ingest rates can be very high. C'est la vie.
							t := make([]string, 0, len(tags)+1)
							for k, v := range tags {
								t = append(t, k+":"+v)
							}

							t = append(t, "sink:"+sink.Name())
							tw.statsd.Incr("worker.span.ingest_error_total", t, 1.0)
						}
					}
					done <- struct{}{}
				}()

				select {
				case _ = <-done:
				case <-time.After(Timeout):
					tw.logger.WithFields(logrus.Fields{
						"sink":  sink.Name(),
						"index": i,
					}).Error("Timed out on sink ingestion")

					t := make([]string, 0, len(tags)+1)
					for k, v := range tags {
						t = append(t, k+":"+v)
					}

					t = append(t, "sink:"+sink.Name())
					tw.statsd.Incr("worker.span.ingest_timeout_total", t, 1.0)
				}
				atomic.AddInt64(&tw.cumulativeTimes[i], int64(time.Since(start)/time.Nanosecond))
			}(i, s, m, &wg)
		}
		wg.Wait()
	}
}

// Flush invokes flush on each sink.
func (tw *SpanWorker) Flush() {
	samples := &ssf.Samples{}

	// Flush and time each sink.
	for i, s := range tw.sinks {
		tags := make([]string, 0, len(tw.sinkTags[i]))
		for k, v := range tw.sinkTags[i] {
			tags = append(tags, fmt.Sprintf("%s:%s", k, v))
		}
		sinkFlushStart := time.Now()
		s.Flush()
		tw.statsd.Timing("worker.span.flush_duration_ns", time.Since(sinkFlushStart), tags, 1.0)

		// cumulative time is measured in nanoseconds
		cumulative := time.Duration(atomic.SwapInt64(&tw.cumulativeTimes[i], 0)) * time.Nanosecond
		tw.statsd.Timing(sinks.MetricKeySpanIngestDuration, cumulative, tags, 1.0)
	}

	metrics.Report(tw.traceClient, samples)
	tw.statsd.Count("worker.span.hit_chan_cap", atomic.SwapInt64(&tw.capCount, 0), nil, 1.0)
	tw.statsd.Count("worker.ssf.empty_total", atomic.SwapInt64(&tw.emptySSFCount, 0), nil, 1.0)
}
