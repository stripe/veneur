package main

import (
	"log"
	"sync"
	"time"

	"github.com/rcrowley/go-metrics"
)

// NewWorker creates, and returns a new Worker object. Its only argument
// is a channel that the worker will receive work from.
func NewWorker(id int, flushInterval int) Worker {
	// Create, and return the worker.
	worker := Worker{
		id:            id,
		flushInterval: flushInterval,
		WorkChan:      make(chan Metric, 100),
		QuitChan:      make(chan bool)}

	return worker
}

type Worker struct {
	id            int
	flushInterval int
	WorkChan      chan Metric
	QuitChan      chan bool
}

// This function "starts" the worker by starting a goroutine, that is
// an infinite "for-select" loop.
func (w Worker) Start() {
	counters := make(map[string]int64)
	gauges := make(map[string]int64)
	histograms := make(map[string]metrics.Histogram)

	mu := &sync.Mutex{}

	ticker := time.NewTicker(time.Duration(w.flushInterval) * time.Second)
	go func() {
		for t := range ticker.C {
			mu.Lock()
			t.Day()
			for k, v := range counters {
				log.Printf("W %d, Counter %q at %d, %f/second\n", w.id, k, v, float64(v)/float64(w.flushInterval))
				delete(counters, k)
			}
			for k, v := range gauges {
				log.Printf("W %d, Gauge %q at %d\n", w.id, k, v)
				delete(gauges, k)
			}
			for k, v := range histograms {
				j := v.Percentiles([]float64{0.5, 0.75, 0.99})
				log.Printf("W %d, Histogram %s [ count: %d, min: %d, p50: %f, p75: %f, p99: %f, max: %d ]", w.id, k, v.Count(), v.Min(), j[0], j[1], j[2], v.Max())
				delete(histograms, k)
			}
			log.Printf("W %d, Flush complete", w.id)
			mu.Unlock()
		}
	}()

	go func() {
		for {
			select {
			case m := <-w.WorkChan:
				start := time.Now()
				mu.Lock()
				switch m.Type {
				case "c":
					// log.Printf("Got counter %q", m.Name)
					counters[m.Name] += m.Value
				case "g":
					// log.Printf("Got gauge %q", m.Name)
					gauges[m.Name] = m.Value
				case "h":
					// log.Printf("Got histogram %q (%d)", m.Name, m.Value)
					hist := histograms[m.Name]
					if hist == nil {
						hist = metrics.NewHistogram(metrics.NewExpDecaySample(1028, 0.015))
						histograms[m.Name] = hist
					}
					hist.Update(m.Value)
				}
				counters["veneur.stats.packets"]++

				hist := histograms["veneur.stats.process_duration_ns"]
				if hist == nil {
					hist = metrics.NewHistogram(metrics.NewExpDecaySample(1028, 0.015))
					histograms["veneur.stats.process_duration_ns"] = hist
				}
				hist.Update(time.Now().Sub(start).Nanoseconds())

				mu.Unlock()

			case <-w.QuitChan:
				// We have been asked to stop.
				log.Printf("worker%d stopping\n", w.id)
				return
			}
		}
	}()
}

// Stop tells the worker to stop listening for work requests.
//
// Note that the worker will only stop *after* it has finished its work.
func (w Worker) Stop() {
	go func() {
		w.QuitChan <- true
	}()
}
