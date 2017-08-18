package veneur

import (
	"fmt"
	"sync"
	"time"

	"github.com/DataDog/datadog-go/statsd"
	"github.com/Sirupsen/logrus"
	"github.com/stripe/veneur/samplers"
	"github.com/stripe/veneur/ssf"
)

// Worker is the doodad that does work.
type Worker struct {
	id         int
	PacketChan chan samplers.UDPMetric
	ImportChan chan []samplers.JSONMetric
	QuitChan   chan struct{}
	processed  int64
	imported   int64
	mutex      *sync.Mutex
	stats      *statsd.Client
	logger     *logrus.Logger
	wm         WorkerMetrics
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
	case "counter":
		if Scope == samplers.GlobalOnly {
			if _, present = wm.globalCounters[mk]; !present {
				wm.globalCounters[mk] = samplers.NewCounter(mk.Name, tags)
			}
		} else {
			if _, present = wm.counters[mk]; !present {
				wm.counters[mk] = samplers.NewCounter(mk.Name, tags)
			}
		}
	case "gauge":
		if _, present = wm.gauges[mk]; !present {
			wm.gauges[mk] = samplers.NewGauge(mk.Name, tags)
		}
	case "histogram":
		if Scope == samplers.LocalOnly {
			if _, present = wm.localHistograms[mk]; !present {
				wm.localHistograms[mk] = samplers.NewHist(mk.Name, tags)
			}
		} else {
			if _, present = wm.histograms[mk]; !present {
				wm.histograms[mk] = samplers.NewHist(mk.Name, tags)
			}
		}
	case "set":
		if Scope == samplers.LocalOnly {
			if _, present = wm.localSets[mk]; !present {
				wm.localSets[mk] = samplers.NewSet(mk.Name, tags)
			}
		} else {
			if _, present = wm.sets[mk]; !present {
				wm.sets[mk] = samplers.NewSet(mk.Name, tags)
			}
		}
	case "timer":
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
func NewWorker(id int, stats *statsd.Client, logger *logrus.Logger) *Worker {
	return &Worker{
		id:         id,
		PacketChan: make(chan samplers.UDPMetric),
		ImportChan: make(chan []samplers.JSONMetric),
		QuitChan:   make(chan struct{}),
		processed:  0,
		imported:   0,
		mutex:      &sync.Mutex{},
		stats:      stats,
		logger:     logger,
		wm:         NewWorkerMetrics(),
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
	case "counter":
		if m.Scope == samplers.GlobalOnly {
			w.wm.globalCounters[m.MetricKey].Sample(m.Value.(float64), m.SampleRate)
		} else {
			w.wm.counters[m.MetricKey].Sample(m.Value.(float64), m.SampleRate)
		}
	case "gauge":
		w.wm.gauges[m.MetricKey].Sample(m.Value.(float64), m.SampleRate)
	case "histogram":
		if m.Scope == samplers.LocalOnly {
			w.wm.localHistograms[m.MetricKey].Sample(m.Value.(float64), m.SampleRate)
		} else {
			w.wm.histograms[m.MetricKey].Sample(m.Value.(float64), m.SampleRate)
		}
	case "set":
		if m.Scope == samplers.LocalOnly {
			w.wm.localSets[m.MetricKey].Sample(m.Value.(string), m.SampleRate)
		} else {
			w.wm.sets[m.MetricKey].Sample(m.Value.(string), m.SampleRate)
		}
	case "timer":
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
	if other.Type == "counter" {
		// this is an odd special case -- counters that are imported are global
		w.wm.Upsert(other.MetricKey, samplers.GlobalOnly, other.Tags)
	} else {
		w.wm.Upsert(other.MetricKey, samplers.MixedScope, other.Tags)
	}

	switch other.Type {
	case "counter":
		if err := w.wm.globalCounters[other.MetricKey].Combine(other.Value); err != nil {
			log.WithError(err).Error("Could not merge counters")
		}
	case "set":
		if err := w.wm.sets[other.MetricKey].Combine(other.Value); err != nil {
			log.WithError(err).Error("Could not merge sets")
		}
	case "histogram":
		if err := w.wm.histograms[other.MetricKey].Combine(other.Value); err != nil {
			log.WithError(err).Error("Could not merge histograms")
		}
	case "timer":
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
	w.mutex.Lock()
	ret := w.wm
	processed := w.processed
	imported := w.imported

	w.wm = NewWorkerMetrics()
	w.processed = 0
	w.imported = 0
	w.mutex.Unlock()

	// Track how much time each worker takes to flush.
	w.stats.TimeInMilliseconds(
		"flush.worker_duration_ns",
		float64(time.Since(start).Nanoseconds()),
		nil,
		1.0,
	)

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
	EventChan        chan samplers.UDPEvent
	ServiceCheckChan chan samplers.UDPServiceCheck
	mutex            *sync.Mutex
	events           []samplers.UDPEvent
	checks           []samplers.UDPServiceCheck
	stats            *statsd.Client
}

// NewEventWorker creates an EventWorker ready to collect events and service checks.
func NewEventWorker(stats *statsd.Client) *EventWorker {
	return &EventWorker{
		EventChan:        make(chan samplers.UDPEvent),
		ServiceCheckChan: make(chan samplers.UDPServiceCheck),
		mutex:            &sync.Mutex{},
		stats:            stats,
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
	ew.stats.TimeInMilliseconds("flush.event_worker_duration_ns", float64(time.Since(start).Nanoseconds()), nil, 1.0)
	return retevts, retsvchecks
}

// SpanWorker is similar to a Worker but it collects events and service checks instead of metrics.
type SpanWorker struct {
	TraceChan chan ssf.SSFSpan
	sinks     []SpanSink
	stats     *statsd.Client
}

// NewSpanWorker creates an TraceWorker ready to collect events and service checks.
func NewSpanWorker(sinks []SpanSink, stats *statsd.Client) *SpanWorker {
	return &SpanWorker{
		TraceChan: make(chan ssf.SSFSpan),
		sinks:     sinks,
		stats:     stats,
	}
}

// Work will start the SpanWorker listening for spans.
// This function will never return.
func (tw *SpanWorker) Work() {
	for m := range tw.TraceChan {
		// Give each sink a change to ingest.
		for _, s := range tw.sinks {
			err := s.Ingest(m)
			if err != nil {
				// If a sink goes wacko and errors a lot, we stand to emit a loooot of metrics
				// here since span ingest rates can be very high. C'est la vie.
				tw.stats.Count("worker.trace.ingest.error_total", 1, []string{fmt.Sprintf("sink:%s", s.Name())}, 1.0)
			}
		}
	}
}

// Flush invokes flush on each sink.
func (tw *SpanWorker) Flush() {

	// Flush and time each sink.
	for _, s := range tw.sinks {
		sinkFlushStart := time.Now()
		s.Flush()
		tw.stats.TimeInMilliseconds("worker.trace.sink.flush_duration_ns", float64(time.Since(sinkFlushStart).Nanoseconds()), []string{fmt.Sprintf("sink:%s", s.Name())}, 1.0)
	}
}
