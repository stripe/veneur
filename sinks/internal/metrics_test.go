package internal

import (
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stripe/veneur"
	"github.com/stripe/veneur/ssf"
)

func TestMetricExtractor(t *testing.T) {
	logger := logrus.StandardLogger()
	sink := metricExtractionSink{
		workers:                []*veneur.Worker{veneur.NewWorker(0, nil, logger)},
		indicatorSpanTimerName: "foo",
	}

	start := time.Now()
	end := start.Add(5 * time.Second)
	span := &ssf.SSFSpan{
		Id:             5,
		TraceId:        5,
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
		for m := range sink.workers[0].PacketChan {
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
	close(sink.workers[0].PacketChan)
	assert.Equal(t, 2, <-done, "Should have sent the right number of metrics")
}

func TestIndicatorMetricExtractor(t *testing.T) {
	logger := logrus.StandardLogger()
	sink := metricExtractionSink{
		workers:                []*veneur.Worker{veneur.NewWorker(0, nil, logger)},
		indicatorSpanTimerName: "foo",
	}

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
		for m := range sink.workers[0].PacketChan {
			hasP := false
			for _, tag := range m.Tags {
				hasP = (tag == "service:indicator_testing")
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
	close(sink.workers[0].PacketChan)
	assert.Equal(t, 1, <-done, "Should have sent the right number of metrics")
}
