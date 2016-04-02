package main

import (
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/rcrowley/go-metrics"
)

type Worker struct {
	id            int
	flushInterval int
	percentiles   []float64
	WorkChan      chan Metric
	FlushChan     chan bool
	QuitChan      chan bool
	counters      map[Metric]int32
	gauges        map[Metric]int32
	histograms    map[Metric]metrics.Histogram
	mutex         *sync.Mutex
}

// NewWorker creates, and returns a new Worker object. Its only argument
// is a channel that the worker will receive work from.
func NewWorker(id int, flushInterval int) Worker {
	// Create, and return the worker.
	worker := Worker{
		id:            id,
		flushInterval: flushInterval,
		percentiles:   []float64{0.5, 0.75, 0.99},
		WorkChan:      make(chan Metric, 100),
		FlushChan:     make(chan bool),
		QuitChan:      make(chan bool),
		mutex:         &sync.Mutex{},
		counters:      make(map[Metric]int32),
		gauges:        make(map[Metric]int32),
		histograms:    make(map[Metric]metrics.Histogram),
	}

	return worker
}

type PostMetric struct {
	Name       string        `json:"metric"`
	Value      [1][2]float32 `json:"points"`
	Tags       string        `json:"tags"`
	MetricType string        `json:"type"`
	Hostname   string        `json:"host"`
	// devicename
	Interval int32 `json:"interval"`
}

func NewPostMetric(name string, v float32, tags string, metricType string, interval int) PostMetric {
	return PostMetric{
		Name:     name,
		Value:    [1][2]float32{{float32(time.Now().Unix()), float32(v)}}, // TODO FIX ME!
		Tags:     tags,
		Hostname: "testhost",
		// deviceName
		MetricType: metricType,
		Interval:   int32(interval),
	}
}

// This function "starts" the worker by starting a goroutine, that is
// an infinite "for-select" loop.
func (w Worker) Start() {

	go func() {
		for {
			select {
			case m := <-w.WorkChan:
				start := time.Now()
				w.mutex.Lock()
				switch m.Type {
				case "c":
					// log.Printf("Got counter %q", m.Name)
					w.counters[m] += m.Value
				case "g":
					w.gauges[m] = m.Value
				case "h", "ms":
					hist := w.histograms[m]
					if hist == nil {
						hist = metrics.NewHistogram(metrics.NewExpDecaySample(1028, 0.015))
						w.histograms[m] = hist
					}
					hist.Update(int64(m.Value))
				}
				// Keep track of how many packets we've processed
				w.counters[Metric{Name: "veneur.stats.packets", Tags: fmt.Sprintf("worker_id:%d", w.id)}]++
				// Keep track of how long it took us to process a packet
				hist := w.histograms[Metric{Name: "veneur.stats.process_duration_ns"}]
				if hist == nil {
					hist = metrics.NewHistogram(metrics.NewExpDecaySample(1028, 0.015))
					ph := Metric{Name: "veneur.stats.process_duration_ns", Tags: fmt.Sprintf("worker_id:%d", w.id)}
					w.histograms[ph] = hist
				}
				hist.Update(time.Now().Sub(start).Nanoseconds())

				w.mutex.Unlock()

			case <-w.FlushChan:
				log.Println("Received flush signal")
				start := time.Now()
				w.Flush()
				// TODO Dupe name, add method to make this less verbose
				hist := w.histograms[Metric{Name: "veneur.stats.flush_duration_ns", Tags: fmt.Sprintf("worker_id:%d", w.id)}]
				if hist == nil {
					hist = metrics.NewHistogram(metrics.NewExpDecaySample(1028, 0.015))
					ph := Metric{Name: "veneur.stats.flush_duration_ns"}
					w.histograms[ph] = hist
				}
				hist.Update(time.Now().Sub(start).Nanoseconds())

			case <-w.QuitChan:
				// We have been asked to stop.
				log.Printf("worker%d stopping\n", w.id)
				return
			}
		}
	}()
}

func (w Worker) Flush() []PostMetric {
	var postMetrics []PostMetric
	w.mutex.Lock()
	for k, v := range w.counters {
		p := NewPostMetric(k.Name, float32(v)/float32(w.flushInterval), k.Tags, "rate", w.flushInterval)
		postMetrics = append(postMetrics, p)
		delete(w.counters, k)
	}
	for k, v := range w.gauges {
		p := NewPostMetric(k.Name, float32(v), k.Tags, "gauge", w.flushInterval)
		postMetrics = append(postMetrics, p)
		delete(w.gauges, k)
	}
	for k, v := range w.histograms {
		postMetrics = append(
			postMetrics,
			// TODO Is this the right type?
			NewPostMetric(fmt.Sprintf("%s.count", k.Name), float32(v.Count()), k.Tags, "counter", w.flushInterval),
		)
		postMetrics = append(
			postMetrics,
			NewPostMetric(fmt.Sprintf("%s.max", k.Name), float32(v.Max()), k.Tags, "gauge", w.flushInterval),
		)
		postMetrics = append(
			postMetrics,
			NewPostMetric(fmt.Sprintf("%s.min", k.Name), float32(v.Min()), k.Tags, "gauge", w.flushInterval),
		)

		percentiles := v.Percentiles(w.percentiles)
		for i, p := range percentiles {
			postMetrics = append(
				postMetrics,
				// TODO Fix to allow for p999, etc
				NewPostMetric(fmt.Sprintf("%s.%dpercentile", k.Name, int(w.percentiles[i]*100)), float32(p), k.Tags, "gauge", w.flushInterval),
			)
		}
	}
	w.mutex.Unlock()
	return postMetrics
}

// Stop tells the worker to stop listening for work requests.
//
// Note that the worker will only stop *after* it has finished its work.
func (w Worker) Stop() {
	go func() {
		w.QuitChan <- true
	}()
}
