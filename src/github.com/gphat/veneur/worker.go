package main

import (
	"encoding/json"
	"fmt"
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
		percentiles:   []float64{0.5, 0.75, 0.99},
		WorkChan:      make(chan Metric, 100),
		QuitChan:      make(chan bool)}

	return worker
}

type Worker struct {
	id            int
	flushInterval int
	percentiles   []float64
	WorkChan      chan Metric
	QuitChan      chan bool
}

type PostMetric struct {
	Name       string  `json:"name"`
	Timestamp  int64   `json:"timestamp"`
	Value      float32 `json:"value"`
	Tags       string  `json:"tags"`
	MetricType string  `json:"metric_type"`
	// hostname
	// devicename
	Interval int32 `json:"interval"`
}

func NewPostMetric(name string, v float32, tags string, metricType string, interval int) PostMetric {
	return PostMetric{
		Name:      name,
		Timestamp: time.Now().Unix(),
		Value:     float32(v), // TODO FIX ME!
		Tags:      tags,
		// hostname
		// deviceName
		MetricType: metricType,
		Interval:   int32(interval),
	}
}

// This function "starts" the worker by starting a goroutine, that is
// an infinite "for-select" loop.
func (w Worker) Start() {
	// TODO These simple maps won't work when tags are included, as we'll need to key by metric?
	counters := make(map[Metric]int32)
	gauges := make(map[Metric]int32)
	histograms := make(map[Metric]metrics.Histogram)

	mu := &sync.Mutex{}

	ticker := time.NewTicker(time.Duration(w.flushInterval) * time.Second)
	go func() {
		for t := range ticker.C {
			var postMetrics []PostMetric
			mu.Lock()
			t.Day()
			for k, v := range counters {
				p := NewPostMetric(k.Name, float32(v)/float32(w.flushInterval), k.Tags, "rate", w.flushInterval)
				postMetrics = append(postMetrics, p)
				delete(counters, k)
			}
			for k, v := range gauges {
				p := NewPostMetric(k.Name, float32(v), k.Tags, "gauge", w.flushInterval)
				postMetrics = append(postMetrics, p)
				delete(gauges, k)
			}
			for k, v := range histograms {
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
			mu.Unlock()
			metricCount := len(postMetrics)
			if metricCount > 0 {
				// Make a metric for how many metrics we metriced (be sure to add one to the total for it!)
				postMetrics = append(
					postMetrics,
					// TODO Is this the right type?
					NewPostMetric("veneur.stats.metrics_posted", float32(metricCount+1), "", "counter", w.flushInterval),
				)
				postJSON, _ := json.MarshalIndent(postMetrics, "", "  ")
				log.Println(string(postJSON))
			}
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
					counters[m] += m.Value
				case "g":
					gauges[m] = m.Value
				case "h", "ms":
					hist := histograms[m]
					if hist == nil {
						hist = metrics.NewHistogram(metrics.NewExpDecaySample(1028, 0.015))
						histograms[m] = hist
					}
					hist.Update(int64(m.Value))
				}
				counters[Metric{Name: "veneur.stats.packets"}]++

				hist := histograms[Metric{Name: "veneur.stats.process_duration_ns"}]
				if hist == nil {
					hist = metrics.NewHistogram(metrics.NewExpDecaySample(1028, 0.015))
					ph := Metric{Name: "veneur.stats.process_duration_ns"}
					histograms[ph] = hist
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
