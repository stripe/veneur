package veneur

import (
	"fmt"
	"sync"
	"time"

	log "github.com/Sirupsen/logrus"
)

// Worker is the doodad that does work.
type Worker struct {
	id         int
	WorkChan   chan Metric
	QuitChan   chan struct{}
	metrics    int64
	counters   map[uint32]*Counter
	gauges     map[uint32]*Gauge
	histograms map[uint32]*Histo
	sets       map[uint32]*Set
	timers     map[uint32]*Histo
	mutex      *sync.Mutex
}

// NewWorker creates, and returns a new Worker object.
func NewWorker(id int) *Worker {
	return &Worker{
		id:         id,
		WorkChan:   make(chan Metric), // TODO Configurable!
		QuitChan:   make(chan struct{}),
		metrics:    0,
		counters:   make(map[uint32]*Counter),
		gauges:     make(map[uint32]*Gauge),
		histograms: make(map[uint32]*Histo),
		sets:       make(map[uint32]*Set),
		timers:     make(map[uint32]*Histo),
		mutex:      &sync.Mutex{},
	}
}

// Start "starts" the worker by starting a goroutine, that is
// an infinite "for-select" loop.
func (w *Worker) Start() {

	go func() {
		for {
			select {
			case m := <-w.WorkChan:
				w.ProcessMetric(&m)
			case <-w.QuitChan:
				// We have been asked to stop.
				log.WithField("worker", w.id).Error("Stopping")
				return
			}
		}
	}()
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
		_, present := w.counters[m.Digest]
		if !present {
			log.WithField("name", m.Name).Debug("New counter")
			w.counters[m.Digest] = NewCounter(m.Name, m.Tags)
		}
		w.counters[m.Digest].Sample(m.Value, m.SampleRate)
	case "gauge":
		_, present := w.gauges[m.Digest]
		if !present {
			log.WithField("name", m.Name).Debug("New gauge")
			w.gauges[m.Digest] = NewGauge(m.Name, m.Tags)
		}
		w.gauges[m.Digest].Sample(m.Value, m.SampleRate)
	case "histogram":
		_, present := w.histograms[m.Digest]
		if !present {
			log.WithField("name", m.Name).Debug("New histogram")
			w.histograms[m.Digest] = NewHist(m.Name, m.Tags, Config.Percentiles)
		}
		w.histograms[m.Digest].Sample(m.Value, m.SampleRate)
	case "set":
		_, present := w.sets[m.Digest]
		if !present {
			log.WithField("name", m.Name).Debug("New set")
			w.sets[m.Digest] = NewSet(m.Name, m.Tags, Config.SetSize, Config.SetAccuracy)
		}
		w.sets[m.Digest].Sample(m.Value, m.SampleRate)
	case "timer":
		_, present := w.timers[m.Digest]
		if !present {
			log.WithField("name", m.Name).Debug("New timer")
			w.timers[m.Digest] = NewHist(m.Name, m.Tags, Config.Percentiles)
		}
		w.timers[m.Digest].Sample(m.Value, m.SampleRate)
	default:
		log.WithField("type", m.Type).Error("Unknown metric type")
	}
}

// Flush generates DDMetrics to emit.
func (w *Worker) Flush() []DDMetric {
	// We preallocate a reasonably sized slice such that hopefully we won't need to reallocate.
	postMetrics := make([]DDMetric, 0,
		// Number of each metric, with 3 + percentiles for histograms (count, max, min)
		len(w.counters)+len(w.gauges)+len(w.histograms)*(3+len(Config.Percentiles)),
	)
	start := time.Now()
	// This is a critical spot. The worker can't process metrics while this
	// mutex is held! So we try and minimize it by copying the maps of values
	// and assigning new ones.
	w.mutex.Lock()
	counters := w.counters
	gauges := w.gauges
	histograms := w.histograms
	sets := w.sets
	timers := w.timers
	Stats.Count("worker.metrics_processed_total", w.metrics, []string{fmt.Sprintf("worker:%d", w.id)}, 1.0)

	w.counters = make(map[uint32]*Counter)
	w.gauges = make(map[uint32]*Gauge)
	w.histograms = make(map[uint32]*Histo)
	w.sets = make(map[uint32]*Set)
	w.timers = make(map[uint32]*Histo)
	w.metrics = 0
	w.mutex.Unlock()

	// Track how much time each worker takes to flush.
	Stats.TimeInMilliseconds(
		"flush.worker_duration_ns",
		float64(time.Now().Sub(start).Nanoseconds()),
		nil,
		1.0,
	)

	Stats.Count("flush.metrics_total", int64(len(counters)), []string{"metric_type:counter"}, 1.0)
	for _, v := range counters {
		postMetrics = append(postMetrics, v.Flush()...)
	}
	Stats.Count("flush.metrics_total", int64(len(gauges)), []string{"metric_type:gauge"}, 1.0)
	for _, v := range gauges {
		postMetrics = append(postMetrics, v.Flush()...)
	}
	Stats.Count("flush.metrics_total", int64(len(histograms)), []string{"metric_type:histogram"}, 1.0)
	for _, v := range histograms {
		postMetrics = append(postMetrics, v.Flush()...)
	}
	Stats.Count("flush.metrics_total", int64(len(sets)), []string{"metric_type:set"}, 1.0)
	for _, v := range sets {
		postMetrics = append(postMetrics, v.Flush()...)
	}
	Stats.Count("flush.metrics_total", int64(len(timers)), []string{"metric_type:timer"}, 1.0)
	for _, v := range timers {
		postMetrics = append(postMetrics, v.Flush()...)
	}

	return postMetrics
}

// Stop tells the worker to stop listening for work requests.
//
// Note that the worker will only stop *after* it has finished its work.
func (w *Worker) Stop() {
	close(w.QuitChan)
}
