package veneur

import (
	"fmt"
	"sync"
	"time"

	"github.com/DataDog/datadog-go/statsd"
	"github.com/Sirupsen/logrus"
)

// Worker is the doodad that does work.
type Worker struct {
	id         int
	PacketChan chan UDPMetric
	ImportChan chan []JSONMetric
	QuitChan   chan struct{}
	processed  int64
	imported   int64
	mutex      *sync.Mutex
	stats      *statsd.Client
	logger     *logrus.Logger
	wm         WorkerMetrics
}

// just a plain struct bundling together the flushed contents of a worker
type WorkerMetrics struct {
	// we do not want to key on the metric's Digest here, because those could
	// collide, and then we'd have to implement a hashtable on top of go maps,
	// which would be silly
	counters   map[MetricKey]*Counter
	gauges     map[MetricKey]*Gauge
	histograms map[MetricKey]*Histo
	sets       map[MetricKey]*Set
	timers     map[MetricKey]*Histo

	// these are used for metrics that shouldn't be forwarded
	localHistograms map[MetricKey]*Histo
	localSets       map[MetricKey]*Set
	localTimers     map[MetricKey]*Histo
}

func NewWorkerMetrics() WorkerMetrics {
	return WorkerMetrics{
		counters:        make(map[MetricKey]*Counter),
		gauges:          make(map[MetricKey]*Gauge),
		histograms:      make(map[MetricKey]*Histo),
		sets:            make(map[MetricKey]*Set),
		timers:          make(map[MetricKey]*Histo),
		localHistograms: make(map[MetricKey]*Histo),
		localSets:       make(map[MetricKey]*Set),
		localTimers:     make(map[MetricKey]*Histo),
	}
}

// Create an entry in wm for the given metrickey, if it does not already exist.
// Returns true if the metric entry was created and false otherwise.
func (wm WorkerMetrics) Upsert(mk MetricKey, localOnly bool, tags []string) bool {
	present := false
	switch mk.Type {
	case "counter":
		if _, present = wm.counters[mk]; !present {
			wm.counters[mk] = NewCounter(mk.Name, tags)
		}
	case "gauge":
		if _, present = wm.gauges[mk]; !present {
			wm.gauges[mk] = NewGauge(mk.Name, tags)
		}
	case "histogram":
		if localOnly {
			if _, present = wm.localHistograms[mk]; !present {
				wm.localHistograms[mk] = NewHist(mk.Name, tags)
			}
		} else {
			if _, present = wm.histograms[mk]; !present {
				wm.histograms[mk] = NewHist(mk.Name, tags)
			}
		}
	case "set":
		if localOnly {
			if _, present = wm.localSets[mk]; !present {
				wm.localSets[mk] = NewSet(mk.Name, tags)
			}
		} else {
			if _, present = wm.sets[mk]; !present {
				wm.sets[mk] = NewSet(mk.Name, tags)
			}
		}
	case "timer":
		if localOnly {
			if _, present = wm.localTimers[mk]; !present {
				wm.localTimers[mk] = NewHist(mk.Name, tags)
			}
		} else {
			if _, present = wm.timers[mk]; !present {
				wm.timers[mk] = NewHist(mk.Name, tags)
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
		PacketChan: make(chan UDPMetric),
		ImportChan: make(chan []JSONMetric),
		QuitChan:   make(chan struct{}),
		processed:  0,
		imported:   0,
		mutex:      &sync.Mutex{},
		stats:      stats,
		logger:     logger,
		wm:         NewWorkerMetrics(),
	}
}

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
			w.logger.WithField("worker", w.id).Error("Stopping")
			return
		}
	}
}

// ProcessMetric takes a Metric and samples it
//
// This is standalone to facilitate testing
func (w *Worker) ProcessMetric(m *UDPMetric) {
	w.mutex.Lock()
	defer w.mutex.Unlock()

	w.processed++
	w.wm.Upsert(m.MetricKey, m.LocalOnly, m.Tags)

	switch m.Type {
	case "counter":
		w.wm.counters[m.MetricKey].Sample(m.Value.(float64), m.SampleRate)
	case "gauge":
		w.wm.gauges[m.MetricKey].Sample(m.Value.(float64), m.SampleRate)
	case "histogram":
		if m.LocalOnly {
			w.wm.localHistograms[m.MetricKey].Sample(m.Value.(float64), m.SampleRate)
		} else {
			w.wm.histograms[m.MetricKey].Sample(m.Value.(float64), m.SampleRate)
		}
	case "set":
		if m.LocalOnly {
			w.wm.localSets[m.MetricKey].Sample(m.Value.(string), m.SampleRate)
		} else {
			w.wm.sets[m.MetricKey].Sample(m.Value.(string), m.SampleRate)
		}
	case "timer":
		if m.LocalOnly {
			w.wm.localTimers[m.MetricKey].Sample(m.Value.(float64), m.SampleRate)
		} else {
			w.wm.timers[m.MetricKey].Sample(m.Value.(float64), m.SampleRate)
		}
	default:
		w.logger.WithField("type", m.Type).Error("Unknown metric type for processing")
	}
}

func (w *Worker) ImportMetric(other JSONMetric) {
	w.mutex.Lock()
	defer w.mutex.Unlock()

	// we don't increment the processed metric counter here, it was already
	// counted by the original veneur that sent this to us
	w.imported++
	w.wm.Upsert(other.MetricKey, false, other.Tags)

	switch other.Type {
	case "set":
		if err := w.wm.sets[other.MetricKey].Combine(other.Value); err != nil {
			w.logger.WithError(err).Error("Could not merge sets")
		}
	case "histogram":
		if err := w.wm.histograms[other.MetricKey].Combine(other.Value); err != nil {
			w.logger.WithError(err).Error("Could not merge histograms")
		}
	default:
		w.logger.WithField("type", other.Type).Error("Unknown metric type for importing")
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
		float64(time.Now().Sub(start).Nanoseconds()),
		nil,
		1.0,
	)

	w.stats.Count("worker.metrics_processed_total", processed, []string{fmt.Sprintf("worker:%d", w.id)}, 1.0)
	w.stats.Count("worker.metrics_imported_total", imported, []string{fmt.Sprintf("worker:%d", w.id)}, 1.0)

	return ret
}

// Stop tells the worker to stop listening for work requests.
//
// Note that the worker will only stop *after* it has finished its work.
func (w *Worker) Stop() {
	close(w.QuitChan)
}

// A Worker that collects events and service checks instead of metrics.
type EventWorker struct {
	EventChan        chan UDPEvent
	ServiceCheckChan chan UDPServiceCheck
	mutex            *sync.Mutex
	events           []UDPEvent
	checks           []UDPServiceCheck
	stats            *statsd.Client
}

func NewEventWorker(stats *statsd.Client) *EventWorker {
	return &EventWorker{
		EventChan:        make(chan UDPEvent),
		ServiceCheckChan: make(chan UDPServiceCheck),
		mutex:            &sync.Mutex{},
		stats:            stats,
	}
}

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

func (ew *EventWorker) Flush() ([]UDPEvent, []UDPServiceCheck) {
	start := time.Now()
	ew.mutex.Lock()

	retevts := ew.events
	retsvchecks := ew.checks
	// these slices will be allocated again at append time
	ew.events = nil
	ew.checks = nil

	ew.mutex.Unlock()
	ew.stats.TimeInMilliseconds("flush.event_worker_duration_ns", float64(time.Now().Sub(start).Nanoseconds()), nil, 1.0)
	return retevts, retsvchecks
}
