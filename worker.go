package veneur

import (
	"sync"
	"time"

	log "github.com/Sirupsen/logrus"
)

// Worker is the doodad that does work.
type Worker struct {
	id         int
	WorkChan   chan Metric
	QuitChan   chan bool
	counters   map[uint32]*Counter
	gauges     map[uint32]*Gauge
	histograms map[uint32]*Histo
	sets       map[uint32]*Set
	mutex      *sync.Mutex
}

// NewWorker creates, and returns a new Worker object.
func NewWorker(id int) *Worker {
	return &Worker{
		id:         id,
		WorkChan:   make(chan Metric), // TODO Configurable!
		QuitChan:   make(chan bool),
		counters:   make(map[uint32]*Counter),
		gauges:     make(map[uint32]*Gauge),
		histograms: make(map[uint32]*Histo),
		sets:       make(map[uint32]*Set),
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
				log.WithFields(log.Fields{
					"worker": w.id,
				}).Error("Stopping")
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
	switch m.Type {
	case "counter":
		_, present := w.counters[m.Digest]
		if !present {
			log.WithFields(log.Fields{
				"name": m.Name,
			}).Debug("New counter")
			w.counters[m.Digest] = NewCounter(m.Name, m.Tags)
		}
		w.counters[m.Digest].Sample(m.Value)
	case "gauge":
		_, present := w.gauges[m.Digest]
		if !present {
			log.WithFields(log.Fields{
				"name": m.Name,
			}).Debug("New gauge")
			w.gauges[m.Digest] = NewGauge(m.Name, m.Tags)
		}
		w.gauges[m.Digest].Sample(m.Value)
	case "histogram", "timer":
		_, present := w.histograms[m.Digest]
		if !present {
			log.WithFields(log.Fields{
				"name": m.Name,
			}).Debug("New histogram")
			w.histograms[m.Digest] = NewHist(m.Name, m.Tags, Config.Percentiles)
		}
		w.histograms[m.Digest].Sample(m.Value)
	case "set":
		_, present := w.sets[m.Digest]
		if !present {
			log.WithFields(log.Fields{
				"name": m.Name,
			}).Debug("New set")
			w.sets[m.Digest] = NewSet(m.Name, m.Tags, Config.SetSize, Config.SetAccuracy)
		}
	default:
		log.WithFields(log.Fields{
			"type": m.Type,
		}).Error("Unknown metric type")
	}
}

// Flush generates DDMetrics to emit. Uses the supplied time
// to judge expiry of metrics for removal.
func (w *Worker) Flush(currTime time.Time) []DDMetric {
	// We preallocate a reasonably sized slice such that hopefully we won't need to reallocate.
	postMetrics := make([]DDMetric, 0,
		// Number of each metric, with 3 + percentiles for histograms (count, max, min)
		len(w.counters)+len(w.gauges)+len(w.histograms)*(3+len(Config.Percentiles)),
	)
	start := time.Now()
	w.mutex.Lock()
	counters := w.counters
	gauges := w.gauges
	histograms := w.histograms

	w.counters = make(map[uint32]*Counter)
	w.gauges = make(map[uint32]*Gauge)
	w.histograms = make(map[uint32]*Histo)
	w.mutex.Unlock()

	// Track how much time each worker takes to flush.
	Stats.TimeInMilliseconds(
		"flush.worker_duration_ns",
		float64(time.Now().Sub(start).Nanoseconds()),
		nil,
		1.0,
	)

	// TODO This should probably be a single function that passes in the metrics and the
	// map
	for k, v := range counters {
		if currTime.Sub(v.lastSampleTime) > Config.Expiry {
			log.WithFields(log.Fields{
				"name": v.name,
			}).Debug("Expiring counter")
			delete(w.counters, k)
		} else {
			postMetrics = append(postMetrics, v.Flush()...)
		}
	}
	for k, v := range gauges {
		if currTime.Sub(v.lastSampleTime) > Config.Expiry {
			log.WithFields(log.Fields{
				"name": v.name,
			}).Debug("Expiring gauge")
			delete(w.gauges, k)
		} else {
			postMetrics = append(postMetrics, v.Flush()...)
		}
	}
	for k, v := range histograms {
		if currTime.Sub(v.lastSampleTime) > Config.Expiry {
			log.WithFields(log.Fields{
				"name": v.name,
			}).Debug("Expiring histogram")
			delete(w.histograms, k)
		} else {
			postMetrics = append(postMetrics, v.Flush()...)
		}
	}
	return postMetrics
}

// Stop tells the worker to stop listening for work requests.
//
// Note that the worker will only stop *after* it has finished its work.
func (w *Worker) Stop() {
	go func() {
		w.QuitChan <- true
	}()
}
