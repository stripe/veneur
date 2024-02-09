package ssfmetrics_test

import (
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stripe/veneur/v14"
	"github.com/stripe/veneur/v14/samplers"
	"github.com/stripe/veneur/v14/sinks"
	"github.com/stripe/veneur/v14/sinks/ssfmetrics"
	"github.com/stripe/veneur/v14/ssf"
)

func TestMetricExtractor(t *testing.T) {
	logger := logrus.StandardLogger()
	worker := veneur.NewWorker(0, true, false, nil, logger, nil, samplers.MixedScope)
	workers := []ssfmetrics.Processor{worker}
	sink, err := ssfmetrics.NewMetricExtractionSink(workers, "foo", "", nil, logger, &samplers.Parser{})
	require.NoError(t, err)

	start := time.Now()
	end := start.Add(5 * time.Second)
	span := &ssf.SSFSpan{
		Id:             5,
		TraceId:        5,
		Name:           "foo",
		StartTimestamp: start.UnixNano(),
		EndTimestamp:   end.UnixNano(),
		Metrics: []*ssf.SSFSample{
			ssf.Count("some.counter", 1, map[string]string{"purpose": "testing"}),
			ssf.Gauge("some.gauge", 20, map[string]string{"purpose": "testing"}),
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
	worker := veneur.NewWorker(0, true, false, nil, logger, nil, samplers.MixedScope)
	workers := []ssfmetrics.Processor{worker}
	sink, err := ssfmetrics.NewMetricExtractionSink(workers, "foo", "", nil, logger, &samplers.Parser{})
	if err != nil {
		panic(err)
	}

	start := time.Now()
	end := start.Add(5 * time.Second)
	span := &ssf.SSFSpan{
		Id:             5,
		TraceId:        5,
		Name:           "foo",
		StartTimestamp: start.UnixNano(),
		EndTimestamp:   end.UnixNano(),
		Metrics: []*ssf.SSFSample{
			ssf.Count("some.counter", 1, map[string]string{"purpose": "testing"}),
			ssf.Gauge("some.gauge", 20, map[string]string{"purpose": "testing"}),
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
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			sink.Ingest(span)
		}
	})
}

func TestIndicatorMetricExtractor(t *testing.T) {
	logger := logrus.StandardLogger()
	worker := veneur.NewWorker(0, true, false, nil, logger, nil, samplers.MixedScope)
	workers := []ssfmetrics.Processor{worker}
	sink, err := ssfmetrics.NewMetricExtractionSink(workers, "foo", "bar", nil, logger, &samplers.Parser{})
	require.NoError(t, err)

	start := time.Now()
	end := start.Add(5 * time.Second)
	span := &ssf.SSFSpan{
		Id:             5,
		TraceId:        5,
		Service:        "indicator_testing",
		Name:           "spline.reticulate",
		StartTimestamp: start.UnixNano(),
		EndTimestamp:   end.UnixNano(),
		Indicator:      true,
	}
	done := make(chan int)
	go func() {
		n := 0
		for m := range worker.PacketChan {
			if m.Name == "foo" {
				assert.Contains(t, m.Tags, "service:indicator_testing")
			} else if m.Name == "bar" {
				assert.Contains(t, m.Tags, "service:indicator_testing")
				assert.Contains(t, m.Tags, "objective:spline.reticulate")
			} else {
				t.Logf("Received additional metric %#v", m)
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
