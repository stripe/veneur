package veneur

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stripe/veneur/protocol"
	"github.com/stripe/veneur/samplers"
	"github.com/stripe/veneur/sinks"
	"github.com/stripe/veneur/ssf"
	"github.com/stripe/veneur/trace"
	"github.com/stripe/veneur/trace/metrics"
)

const counterTypeName = "counter"
const gaugeTypeName = "gauge"
const histogramTypeName = "histogram"
const setTypeName = "set"
const timerTypeName = "timer"

// Worker is the doodad that does work.
type Worker struct {
	id          int
	PacketChan  chan samplers.UDPMetric
	ImportChan  chan []samplers.JSONMetric
	QuitChan    chan struct{}
	processed   int64
	imported    int64
	mutex       *sync.Mutex
	traceClient *trace.Client
	logger      *logrus.Logger
	wm          WorkerMetrics
}

// IngestUDP on a Worker feeds the metric into the worker's PacketChan.
func (w *Worker) IngestUDP(metric samplers.UDPMetric) {
	w.PacketChan <- metric
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

	// these are used for metrics that shouldn't be forwarded
	localHistograms map[samplers.MetricKey]*samplers.Histo
	localSets       map[samplers.MetricKey]*samplers.Set
	localTimers     map[samplers.MetricKey]*samplers.Histo
}

// NewWorkerMetrics initializes a WorkerMetrics struct
func NewWorkerMetrics() WorkerMetrics {
	return WorkerMetrics{
		counters:        make(map[samplers.MetricKey]*samplers.Counter),
		globalCounters:  make(map[samplers.MetricKey]*samplers.Counter),
		globalGauges:    make(map[samplers.MetricKey]*samplers.Gauge),
		gauges:          make(map[samplers.MetricKey]*samplers.Gauge),
		histograms:      make(map[samplers.MetricKey]*samplers.Histo),
		sets:            make(map[samplers.MetricKey]*samplers.Set),
		timers:          make(map[samplers.MetricKey]*samplers.Histo),
		localHistograms: make(map[samplers.MetricKey]*samplers.Histo),
		localSets:       make(map[samplers.MetricKey]*samplers.Set),
		localTimers:     make(map[samplers.MetricKey]*samplers.Histo),
	}
}

// Upsert creates an entry on the WorkerMetrics struct for the given metrickey (if one does not already exist)
// and updates the existing entry (if one already exists).
// Returns true if the metric entry was created and false otherwise.
func (wm WorkerMetrics) Upsert(mk samplers.MetricKey, Scope samplers.MetricScope, tags []string) bool {
	present := false
	switch mk.Type {
	case counterTypeName:
		if Scope == samplers.GlobalOnly {
			if _, present = wm.globalCounters[mk]; !present {
				wm.globalCounters[mk] = samplers.NewCounter(mk.Name, tags)
			}
		} else {
			if _, present = wm.counters[mk]; !present {
				wm.counters[mk] = samplers.NewCounter(mk.Name, tags)
			}
		}
	case gaugeTypeName:
		if Scope == samplers.GlobalOnly {
			if _, present = wm.globalGauges[mk]; !present {
				wm.globalGauges[mk] = samplers.NewGauge(mk.Name, tags)
			}
		} else {
			if _, present = wm.gauges[mk]; !present {
				wm.gauges[mk] = samplers.NewGauge(mk.Name, tags)
			}
		}
	case histogramTypeName:
		if Scope == samplers.LocalOnly {
			if _, present = wm.localHistograms[mk]; !present {
				wm.localHistograms[mk] = samplers.NewHist(mk.Name, tags)
			}
		} else {
			if _, present = wm.histograms[mk]; !present {
				wm.histograms[mk] = samplers.NewHist(mk.Name, tags)
			}
		}
	case setTypeName:
		if Scope == samplers.LocalOnly {
			if _, present = wm.localSets[mk]; !present {
				wm.localSets[mk] = samplers.NewSet(mk.Name, tags)
			}
		} else {
			if _, present = wm.sets[mk]; !present {
				wm.sets[mk] = samplers.NewSet(mk.Name, tags)
			}
		}
	case timerTypeName:
		if Scope == samplers.LocalOnly {
			if _, present = wm.localTimers[mk]; !present {
				wm.localTimers[mk] = samplers.NewHist(mk.Name, tags)
			}
		} else {
			if _, present = wm.timers[mk]; !present {
				wm.timers[mk] = samplers.NewHist(mk.Name, tags)
			}
		}
		// no need to raise errors on unknown types
		// the caller will probably end up doing that themselves
	}
	return !present
}

// NewWorker creates, and returns a new Worker object.
func NewWorker(id int, cl *trace.Client, logger *logrus.Logger) *Worker {
	return &Worker{
		id:          id,
		PacketChan:  make(chan samplers.UDPMetric, 32),
		ImportChan:  make(chan []samplers.JSONMetric, 32),
		QuitChan:    make(chan struct{}),
		processed:   0,
		imported:    0,
		mutex:       &sync.Mutex{},
		traceClient: cl,
		logger:      logger,
		wm:          NewWorkerMetrics(),
	}
}

// Work will start the worker listening for metrics to process or import.
// It will not return until the worker is sent a message to terminate using Stop()
func (w *Worker) Work() {
	for {
		select {
		case m := <-w.PacketChan:
			w.ProcessMetric(&m)
		case m := <-w.ImportChan:
			for _, j := range m {
				w.ImportMetric(j)
			}
		case <-w.QuitChan:
			// We have been asked to stop.
			log.WithField("worker", w.id).Error("Stopping")
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

// ProcessMetric takes a Metric and samples it
//
// This is standalone to facilitate testing
func (w *Worker) ProcessMetric(m *samplers.UDPMetric) {
	w.mutex.Lock()
	defer w.mutex.Unlock()
	w.processed++
	w.wm.Upsert(m.MetricKey, m.Scope, m.Tags)

	switch m.Type {
	case counterTypeName:
		if m.Scope == samplers.GlobalOnly {
			w.wm.globalCounters[m.MetricKey].Sample(m.Value.(float64), m.SampleRate)
		} else {
			w.wm.counters[m.MetricKey].Sample(m.Value.(float64), m.SampleRate)
		}
	case gaugeTypeName:
		if m.Scope == samplers.GlobalOnly {
			w.wm.globalGauges[m.MetricKey].Sample(m.Value.(float64), m.SampleRate)
		} else {
			w.wm.gauges[m.MetricKey].Sample(m.Value.(float64), m.SampleRate)
		}
	case histogramTypeName:
		if m.Scope == samplers.LocalOnly {
			w.wm.localHistograms[m.MetricKey].Sample(m.Value.(float64), m.SampleRate)
		} else {
			w.wm.histograms[m.MetricKey].Sample(m.Value.(float64), m.SampleRate)
		}
	case setTypeName:
		if m.Scope == samplers.LocalOnly {
			w.wm.localSets[m.MetricKey].Sample(m.Value.(string), m.SampleRate)
		} else {
			w.wm.sets[m.MetricKey].Sample(m.Value.(string), m.SampleRate)
		}
	case timerTypeName:
		if m.Scope == samplers.LocalOnly {
			w.wm.localTimers[m.MetricKey].Sample(m.Value.(float64), m.SampleRate)
		} else {
			w.wm.timers[m.MetricKey].Sample(m.Value.(float64), m.SampleRate)
		}
	default:
		log.WithField("type", m.Type).Error("Unknown metric type for processing")
	}
}

// ImportMetric receives a metric from another veneur instance
func (w *Worker) ImportMetric(other samplers.JSONMetric) {
	w.mutex.Lock()
	defer w.mutex.Unlock()

	// we don't increment the processed metric counter here, it was already
	// counted by the original veneur that sent this to us
	w.imported++
	if other.Type == counterTypeName || other.Type == gaugeTypeName {
		// this is an odd special case -- counters that are imported are global
		w.wm.Upsert(other.MetricKey, samplers.GlobalOnly, other.Tags)
	} else {
		w.wm.Upsert(other.MetricKey, samplers.MixedScope, other.Tags)
	}

	switch other.Type {
	case counterTypeName:
		if err := w.wm.globalCounters[other.MetricKey].Combine(other.Value); err != nil {
			log.WithError(err).Error("Could not merge counters")
		}
	case gaugeTypeName:
		if err := w.wm.globalGauges[other.MetricKey].Combine(other.Value); err != nil {
			log.WithError(err).Error("Could not merge gauges")
		}
	case setTypeName:
		if err := w.wm.sets[other.MetricKey].Combine(other.Value); err != nil {
			log.WithError(err).Error("Could not merge sets")
		}
	case histogramTypeName:
		if err := w.wm.histograms[other.MetricKey].Combine(other.Value); err != nil {
			log.WithError(err).Error("Could not merge histograms")
		}
	case timerTypeName:
		if err := w.wm.timers[other.MetricKey].Combine(other.Value); err != nil {
			log.WithError(err).Error("Could not merge timers")
		}
	default:
		log.WithField("type", other.Type).Error("Unknown metric type for importing")
	}
}

// Flush resets the worker's internal metrics and returns their contents.
func (w *Worker) Flush() WorkerMetrics {
	start := time.Now()
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

	// Track how much time each worker takes to flush.
	metrics.ReportBatch(w.traceClient, []*ssf.SSFSample{
		ssf.Timing("flush.worker_duration_ns", time.Since(start), time.Millisecond, nil),
		ssf.Count("worker.metrics_processed_total", float32(processed), nil),
		ssf.Count("worker.metrics_imported_total", float32(imported), nil),
	})

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
	EventChan        chan samplers.UDPEvent
	ServiceCheckChan chan samplers.UDPServiceCheck
	mutex            *sync.Mutex
	events           []samplers.UDPEvent
	checks           []samplers.UDPServiceCheck
	traceClient      *trace.Client
}

// NewEventWorker creates an EventWorker ready to collect events and service checks.
func NewEventWorker(cl *trace.Client) *EventWorker {
	return &EventWorker{
		EventChan:        make(chan samplers.UDPEvent),
		ServiceCheckChan: make(chan samplers.UDPServiceCheck),
		mutex:            &sync.Mutex{},
		traceClient:      cl,
	}
}

// Work will start the EventWorker listening for events and service checks.
// This function will never return.
func (ew *EventWorker) Work() {
	for {
		select {
		case evt := <-ew.EventChan:
			ew.mutex.Lock()
			ew.events = append(ew.events, evt)
			ew.mutex.Unlock()
		case svcheck := <-ew.ServiceCheckChan:
			ew.mutex.Lock()
			ew.checks = append(ew.checks, svcheck)
			ew.mutex.Unlock()
		}
	}
}

// Flush returns the EventWorker's stored events and service checks and
// resets the stored contents.
func (ew *EventWorker) Flush() ([]samplers.UDPEvent, []samplers.UDPServiceCheck) {
	start := time.Now()
	ew.mutex.Lock()

	retevts := ew.events
	retsvchecks := ew.checks
	// these slices will be allocated again at append time
	ew.events = nil
	ew.checks = nil

	ew.mutex.Unlock()
	metrics.ReportOne(ew.traceClient, ssf.Timing("flush.event_worker_duration_ns", time.Since(start), time.Nanosecond, nil))
	return retevts, retsvchecks
}

// SpanWorker is similar to a Worker but it collects events and service checks instead of metrics.
type SpanWorker struct {
	SpanChan        <-chan *ssf.SSFSpan
	sinkTags        []map[string]string
	sinks           []sinks.SpanSink
	cumulativeTimes []int64
	traceClient     *trace.Client
	capCount        int64
}

// NewSpanWorker creates a SpanWorker ready to collect events and service checks.
func NewSpanWorker(sinks []sinks.SpanSink, cl *trace.Client, spanChan <-chan *ssf.SSFSpan) *SpanWorker {
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
		cumulativeTimes: make([]int64, len(sinks)),
		traceClient:     cl,
	}
}

// Work will start the SpanWorker listening for spans.
// This function will never return.
func (tw *SpanWorker) Work() {
	capcmp := cap(tw.SpanChan) - 1
	for m := range tw.SpanChan {
		// If we are at or one below cap, increment the counter.
		if len(tw.SpanChan) >= capcmp {
			atomic.AddInt64(&tw.capCount, 1)
		}

		var wg sync.WaitGroup
		for i, s := range tw.sinks {
			tags := tw.sinkTags[i]
			wg.Add(1)
			go func(i int, sink sinks.SpanSink, span *ssf.SSFSpan, wg *sync.WaitGroup) {
				start := time.Now()
				// Give each sink a change to ingest.
				err := sink.Ingest(span)
				if err != nil {
					if _, isNoTrace := err.(*protocol.InvalidTrace); !isNoTrace {
						// If a sink goes wacko and errors a lot, we stand to emit a
						// loooot of metrics towards all span workers here since
						// span ingest rates can be very high. C'est la vie.
						metrics.ReportOne(tw.traceClient,
							ssf.Count("worker.span.ingest_error_total", 1, tags))
					}
				}
				atomic.AddInt64(&tw.cumulativeTimes[i],
					int64(time.Since(start)/time.Nanosecond))
				wg.Done()
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
		tags := tw.sinkTags[i]
		sinkFlushStart := time.Now()
		s.Flush()
		samples.Add(ssf.Timing("worker.span.flush_duration_ns", time.Since(sinkFlushStart), time.Nanosecond, tags))
		cumulative := time.Duration(atomic.SwapInt64(&tw.cumulativeTimes[i], 0)) * time.Nanosecond
		samples.Add(ssf.Timing(sinks.MetricKeySpanIngestDuration, cumulative, time.Nanosecond, tags))
	}

	metrics.Report(tw.traceClient, samples)
	metrics.ReportOne(tw.traceClient,
		ssf.Count("worker.span.hit_chan_cap", float32(atomic.SwapInt64(&tw.capCount, 0)), nil))
}
