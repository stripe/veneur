package proxy

import (
	"context"
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stripe/veneur/v14/forwardrpc"
	"github.com/stripe/veneur/v14/samplers/metricpb"
	metrictest "github.com/stripe/veneur/v14/samplers/metricpb/testutils"
	"github.com/stripe/veneur/v14/trace"
)

type testMetricIngester struct {
	metrics []*metricpb.Metric
}

func (mi *testMetricIngester) IngestMetrics(ms []*metricpb.Metric) {
	mi.metrics = append(mi.metrics, ms...)
}

func (mi *testMetricIngester) clear() {
	mi.metrics = mi.metrics[:0]
}

// Test that sending the same metric to a Veneur results in it being hashed
// to the same worker every time
func TestSendMetrics_ConsistentHash(t *testing.T) {
	ingesters := []*testMetricIngester{{}, {}}

	casted := make([]MetricIngester, len(ingesters))
	for i, ingester := range ingesters {
		casted[i] = ingester
	}
	logger := logrus.NewEntry(logrus.New())
	s := New("localhost", casted, logger)

	inputs := []*metricpb.Metric{{
		Name: "test.counter",
		Type: metricpb.Type_Counter,
		Tags: []string{"tag:1"},
	}, {
		Name: "test.gauge",
		Type: metricpb.Type_Gauge,
	}, {
		Name: "test.histogram",
		Type: metricpb.Type_Histogram,
		Tags: []string{"type:histogram"},
	}, {
		Name: "test.set",
		Type: metricpb.Type_Set,
	}, {
		Name: "test.gauge3",
		Type: metricpb.Type_Gauge,
	}}

	// Send the same inputs many times
	for i := 0; i < 10; i++ {
		s.SendMetrics(context.Background(), &forwardrpc.MetricList{Metrics: inputs})

		assert.Equal(t, []*metricpb.Metric{inputs[0], inputs[4]},
			ingesters[0].metrics, "Ingester 0 has the wrong metrics")
		assert.Equal(t, []*metricpb.Metric{inputs[1], inputs[2], inputs[3]},
			ingesters[1].metrics, "Ingester 1 has the wrong metrics")

		for _, ingester := range ingesters {
			ingester.clear()
		}
	}
}

func TestSendMetrics_Empty(t *testing.T) {
	ingester := &testMetricIngester{}
	logger := logrus.NewEntry(logrus.New())
	s := New("localhost", []MetricIngester{ingester}, logger)
	s.SendMetrics(context.Background(), &forwardrpc.MetricList{})

	assert.Empty(t, ingester.metrics,
		"The server shouldn't have submitted any metrics")
}

func TestOptions_WithTraceClient(t *testing.T) {
	logger := logrus.NewEntry(logrus.New())
	c, err := trace.NewClient(trace.DefaultVeneurAddress)
	assert.NoError(t, err, "failed to initialize a trace client")

	s := New("localhost", []MetricIngester{}, logger, WithTraceClient(c))
	assert.Equal(t, c, s.opts.traceClient,
		"WithTraceClient didn't correctly set the trace client")
}

type noopChannelMetricIngester struct {
	in   chan []*metricpb.Metric
	quit chan struct{}
}

func newNoopChannelMetricIngester() *noopChannelMetricIngester {
	return &noopChannelMetricIngester{
		in:   make(chan []*metricpb.Metric),
		quit: make(chan struct{}),
	}
}

func (mi *noopChannelMetricIngester) start() {
	go func() {
		for {
			select {
			case <-mi.in:
			case <-mi.quit:
				return
			}
		}
	}()
}

func (mi *noopChannelMetricIngester) stop() {
	mi.quit <- struct{}{}
}

func (mi *noopChannelMetricIngester) IngestMetrics(ms []*metricpb.Metric) {
	mi.in <- ms
}

func BenchmarkImportServerSendMetrics(b *testing.B) {
	rand.Seed(time.Now().Unix())

	metrics := metrictest.RandomForwardMetrics(10000)
	for _, inputSize := range []int{10, 100, 1000, 10000} {
		ingesters := make([]MetricIngester, 100)
		for i := range ingesters {
			ingester := newNoopChannelMetricIngester()
			ingester.start()
			defer ingester.stop()
			ingesters[i] = ingester
		}
		logger := logrus.NewEntry(logrus.New())
		s := New("localhost", ingesters, logger)
		ctx := context.Background()
		input := &forwardrpc.MetricList{Metrics: metrics[:inputSize]}

		b.Run(fmt.Sprintf("InputSize=%d", inputSize), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				s.SendMetrics(ctx, input)
			}
		})
	}
}
