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
	WorkChan chan UDPMetric
	QuitChan chan struct{}
	metrics  int64
	mutex    *sync.Mutex
	stats    *statsd.Client
	logger   *logrus.Logger
	wm       WorkerMetrics
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

// NewWorker creates, and returns a new Worker object.
func NewWorker(id int, stats *statsd.Client, logger *logrus.Logger) *Worker {
	return &Worker{
		id:       id,
		WorkChan: make(chan UDPMetric),
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
func (w *Worker) ProcessMetric(m *UDPMetric) {
	w.mutex.Lock()
	defer w.mutex.Unlock()
	w.metrics++
	switch m.Type {
	case "counter":
		_, present := w.wm.counters[m.MetricKey]
		if !present {
			w.logger.WithField("name", m.Name).Debug("New counter")
			w.wm.counters[m.MetricKey] = NewCounter(m.Name, m.Tags)
		}
		w.wm.counters[m.MetricKey].Sample(m.Value.(float64), m.SampleRate)
	case "gauge":
		_, present := w.wm.gauges[m.MetricKey]
		if !present {
			w.logger.WithField("name", m.Name).Debug("New gauge")
			w.wm.gauges[m.MetricKey] = NewGauge(m.Name, m.Tags)
		}
		w.wm.gauges[m.MetricKey].Sample(m.Value.(float64), m.SampleRate)
	case "histogram":
		_, present := w.wm.histograms[m.MetricKey]
		if !present {
			w.logger.WithField("name", m.Name).Debug("New histogram")
			w.wm.histograms[m.MetricKey] = NewHist(m.Name, m.Tags)
		}
		w.wm.histograms[m.MetricKey].Sample(m.Value.(float64), m.SampleRate)
	case "set":
		_, present := w.wm.sets[m.MetricKey]
		if !present {
			w.logger.WithField("name", m.Name).Debug("New set")
			w.wm.sets[m.MetricKey] = NewSet(m.Name, m.Tags)
		}
		w.wm.sets[m.MetricKey].Sample(m.Value.(string), m.SampleRate)
	case "timer":
		_, present := w.wm.timers[m.MetricKey]
		if !present {
			w.logger.WithField("name", m.Name).Debug("New timer")
			w.wm.timers[m.MetricKey] = NewHist(m.Name, m.Tags)
		}
		w.wm.timers[m.MetricKey].Sample(m.Value.(float64), m.SampleRate)
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
