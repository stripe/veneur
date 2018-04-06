package ssfmetrics_test

import (
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stripe/veneur"
	"github.com/stripe/veneur/sinks"
	"github.com/stripe/veneur/sinks/ssfmetrics"
	"github.com/stripe/veneur/ssf"
)

func TestMetricExtractor(t *testing.T) {
	logger := logrus.StandardLogger()
	worker := veneur.NewWorker(0, nil, logger)
	workers := []ssfmetrics.Processor{worker}
	sink, err := ssfmetrics.NewMetricExtractionSink(workers, "foo", nil, logger)
	require.NoError(t, err)

	cnt := ssf.Count("some.counter", 1, map[string]string{"purpose": "testing"})
	gauge := ssf.Gauge("some.gauge", 20, map[string]string{"purpose": "testing"})

	start := time.Now()
	end := start.Add(5 * time.Second)
	span := &ssf.SSFSpan{
		Id:             5,
		TraceId:        5,
		StartTimestamp: start.UnixNano(),
		EndTimestamp:   end.UnixNano(),
		Metrics: []*ssf.SSFSample{
			&cnt,
			&gauge,
		},
	}
	done := make(chan int)
	go func() {
		n := 0
		for m := range worker.PacketChan {
			hasP := false
			for _, tag := range m.Tags {
				hasP = (tag == "purpose:testing")
			}
			if !hasP {
				t.Logf("Received unexpected additional metric %#v", m)
				continue
			}
			n++
		}
		done <- n
	}()
	assert.NoError(t, sink.Ingest(span))
	close(worker.PacketChan)
	assert.Equal(t, 2, <-done, "Should have sent the right number of metrics")
}

func setupBench() (*ssf.SSFSpan, sinks.SpanSink) {
	logger := logrus.StandardLogger()
	worker := veneur.NewWorker(0, nil, logger)
	workers := []ssfmetrics.Processor{worker}
	sink, err := ssfmetrics.NewMetricExtractionSink(workers, "foo", nil, logger)
	if err != nil {
		panic(err)
	}

	cnt := ssf.Count("some.counter", 1, map[string]string{"purpose": "testing"})
	gauge := ssf.Gauge("some.gauge", 20, map[string]string{"purpose": "testing"})

	start := time.Now()
	end := start.Add(5 * time.Second)
	span := &ssf.SSFSpan{
		Id:             5,
		TraceId:        5,
		StartTimestamp: start.UnixNano(),
		EndTimestamp:   end.UnixNano(),
		Metrics: []*ssf.SSFSample{
			&cnt,
			&gauge,
		},
	}

	running := make(chan struct{})
	go func() {
		close(running)
		for {
			<-worker.PacketChan
		}
	}()
	<-running

	return span, sink
}

func BenchmarkMetricExtractor(b *testing.B) {
	span, sink := setupBench()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sink.Ingest(span)
	}
}

func BenchmarkParallelMetricExtractor(b *testing.B) {
	span, sink := setupBench()
	wg, wg2 := sync.WaitGroup{}, sync.WaitGroup{}

	var max int
	maxProcs, numCPU := runtime.GOMAXPROCS(0), runtime.NumCPU()
	if maxProcs < numCPU {
		max = maxProcs
	} else {
		max = numCPU
	}

	max = 16
	b.Logf("Running with parallelism %v", max)
	wg.Add(max)
	wg2.Add(max)

	f := func() {
		wg.Add(-1)
		wg.Wait()
		for i := 0; i < b.N; i++ {
			sink.Ingest(span)
		}
		wg2.Done()
	}
	b.ResetTimer()

	for i := 0; i < max; i++ {
		go f()
	}

	wg2.Wait()
}

func TestIndicatorMetricExtractor(t *testing.T) {
	logger := logrus.StandardLogger()
	worker := veneur.NewWorker(0, nil, logger)
	workers := []ssfmetrics.Processor{worker}
	sink, err := ssfmetrics.NewMetricExtractionSink(workers, "foo", nil, logger)
	require.NoError(t, err)

	start := time.Now()
	end := start.Add(5 * time.Second)
	span := &ssf.SSFSpan{
		Id:             5,
		TraceId:        5,
		Service:        "indicator_testing",
		StartTimestamp: start.UnixNano(),
		EndTimestamp:   end.UnixNano(),
		Indicator:      true,
	}
	done := make(chan int)
	go func() {
		n := 0
		for m := range worker.PacketChan {
			if m.Name != "foo" {
				t.Logf("Received additional metric %#v", m)
				continue
			}
			n++
		}
		done <- n
	}()
	assert.NoError(t, sink.Ingest(span))
	close(worker.PacketChan)
	assert.Equal(t, 1, <-done, "Should have sent the right number of metrics")
}
