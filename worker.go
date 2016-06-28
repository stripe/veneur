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
	id       int
	WorkChan chan Metric
	QuitChan chan struct{}
	metrics  int64
	mutex    *sync.Mutex
	stats    *statsd.Client
	logger   *logrus.Logger
	wm       WorkerMetrics
}

// just a plain struct bundling together the flushed contents of a worker
type WorkerMetrics struct {
	counters   map[uint32]*Counter
	gauges     map[uint32]*Gauge
	histograms map[uint32]*Histo
	sets       map[uint32]*Set
	timers     map[uint32]*Histo
}

func NewWorkerMetrics() WorkerMetrics {
	return WorkerMetrics{
		counters:   make(map[uint32]*Counter),
		gauges:     make(map[uint32]*Gauge),
		histograms: make(map[uint32]*Histo),
		sets:       make(map[uint32]*Set),
		timers:     make(map[uint32]*Histo),
	}
}

// NewWorker creates, and returns a new Worker object.
func NewWorker(id int, stats *statsd.Client, logger *logrus.Logger) *Worker {
	return &Worker{
		id:       id,
		WorkChan: make(chan Metric),
		QuitChan: make(chan struct{}),
		metrics:  0,
		mutex:    &sync.Mutex{},
		stats:    stats,
		logger:   logger,
		wm:       NewWorkerMetrics(),
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
func (w *Worker) ProcessMetric(m *Metric) {
	w.mutex.Lock()
	defer w.mutex.Unlock()
	w.metrics++
	switch m.Type {
	case "counter":
		_, present := w.wm.counters[m.Digest]
		if !present {
			w.logger.WithField("name", m.Name).Debug("New counter")
			w.wm.counters[m.Digest] = NewCounter(m.Name, m.Tags)
		}
		w.wm.counters[m.Digest].Sample(m.Value.(float64), m.SampleRate)
	case "gauge":
		_, present := w.wm.gauges[m.Digest]
		if !present {
			w.logger.WithField("name", m.Name).Debug("New gauge")
			w.wm.gauges[m.Digest] = NewGauge(m.Name, m.Tags)
		}
		w.wm.gauges[m.Digest].Sample(m.Value.(float64), m.SampleRate)
	case "histogram":
		_, present := w.wm.histograms[m.Digest]
		if !present {
			w.logger.WithField("name", m.Name).Debug("New histogram")
			w.wm.histograms[m.Digest] = NewHist(m.Name, m.Tags)
		}
		w.wm.histograms[m.Digest].Sample(m.Value.(float64), m.SampleRate)
	case "set":
		_, present := w.wm.sets[m.Digest]
		if !present {
			w.logger.WithField("name", m.Name).Debug("New set")
			w.wm.sets[m.Digest] = NewSet(m.Name, m.Tags)
		}
		w.wm.sets[m.Digest].Sample(m.Value.(string), m.SampleRate)
	case "timer":
		_, present := w.wm.timers[m.Digest]
		if !present {
			w.logger.WithField("name", m.Name).Debug("New timer")
			w.wm.timers[m.Digest] = NewHist(m.Name, m.Tags)
		}
		w.wm.timers[m.Digest].Sample(m.Value.(float64), m.SampleRate)
	default:
		w.logger.WithField("type", m.Type).Error("Unknown metric type")
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
	w.stats.Count("worker.metrics_processed_total", w.metrics, []string{fmt.Sprintf("worker:%d", w.id)}, 1.0)

	w.wm = NewWorkerMetrics()
	w.metrics = 0
	w.mutex.Unlock()

	// Track how much time each worker takes to flush.
	w.stats.TimeInMilliseconds(
		"flush.worker_duration_ns",
		float64(time.Now().Sub(start).Nanoseconds()),
		nil,
		1.0,
	)

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
