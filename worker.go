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
	id        int
	WorkChan  chan UDPMetric
	QuitChan  chan struct{}
	processed int64
	imported  int64
	mutex     *sync.Mutex
	stats     *statsd.Client
	logger    *logrus.Logger
	wm        WorkerMetrics
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
}

func NewWorkerMetrics() WorkerMetrics {
	return WorkerMetrics{
		counters:   make(map[MetricKey]*Counter),
		gauges:     make(map[MetricKey]*Gauge),
		histograms: make(map[MetricKey]*Histo),
		sets:       make(map[MetricKey]*Set),
		timers:     make(map[MetricKey]*Histo),
	}
}

// Create an entry in wm for the given metrickey, if it does not already exist.
// Returns true if the metric entry was created and false otherwise.
func (wm WorkerMetrics) Upsert(mk MetricKey, tags []string) bool {
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
		if _, present = wm.histograms[mk]; !present {
			wm.histograms[mk] = NewHist(mk.Name, tags)
		}
	case "set":
		if _, present = wm.sets[mk]; !present {
			wm.sets[mk] = NewSet(mk.Name, tags)
		}
	case "timer":
		if _, present = wm.timers[mk]; !present {
			wm.timers[mk] = NewHist(mk.Name, tags)
		}
		// no need to raise errors on unknown types
		// the caller will probably end up doing that themselves
	}
	return !present
}

// NewWorker creates, and returns a new Worker object.
func NewWorker(id int, stats *statsd.Client, logger *logrus.Logger) *Worker {
	return &Worker{
		id:        id,
		WorkChan:  make(chan UDPMetric),
		QuitChan:  make(chan struct{}),
		processed: 0,
		imported:  0,
		mutex:     &sync.Mutex{},
		stats:     stats,
		logger:    logger,
		wm:        NewWorkerMetrics(),
	}
}

func (w *Worker) Work() {
	for {
		select {
		case m := <-w.WorkChan:
			w.ProcessMetric(&m)
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
	w.wm.Upsert(m.MetricKey, m.Tags)

	switch m.Type {
	case "counter":
		w.wm.counters[m.MetricKey].Sample(m.Value.(float64), m.SampleRate)
	case "gauge":
		w.wm.gauges[m.MetricKey].Sample(m.Value.(float64), m.SampleRate)
	case "histogram":
		w.wm.histograms[m.MetricKey].Sample(m.Value.(float64), m.SampleRate)
	case "set":
		w.wm.sets[m.MetricKey].Sample(m.Value.(string), m.SampleRate)
	case "timer":
		w.wm.timers[m.MetricKey].Sample(m.Value.(float64), m.SampleRate)
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
	w.wm.Upsert(other.MetricKey, other.Tags)

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

	w.stats.Count("worker.metrics_flushed_total", int64(len(ret.counters)), []string{"metric_type:counter"}, 1.0)
	w.stats.Count("worker.metrics_flushed_total", int64(len(ret.gauges)), []string{"metric_type:gauge"}, 1.0)
	w.stats.Count("worker.metrics_flushed_total", int64(len(ret.histograms)), []string{"metric_type:histogram"}, 1.0)
	w.stats.Count("worker.metrics_flushed_total", int64(len(ret.sets)), []string{"metric_type:set"}, 1.0)
	w.stats.Count("worker.metrics_flushed_total", int64(len(ret.timers)), []string{"metric_type:timer"}, 1.0)

	return ret
}

// Stop tells the worker to stop listening for work requests.
//
// Note that the worker will only stop *after* it has finished its work.
func (w *Worker) Stop() {
	close(w.QuitChan)
}
