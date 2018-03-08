package importsrv

import (
	"context"
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stripe/veneur/forwardrpc"
	"github.com/stripe/veneur/samplers/metricpb"
	metrictest "github.com/stripe/veneur/samplers/metricpb/testutils"
	"github.com/stripe/veneur/trace"
)

type testMetricIngester struct {
	metrics []*metricpb.Metric
}

func (mi *testMetricIngester) IngestMetric(m *metricpb.Metric) {
	mi.metrics = append(mi.metrics, m)
}

func (mi *testMetricIngester) clear() {
	mi.metrics = mi.metrics[:0]
}

// Test that sending the same metric to a Veneur results in it being hashed
// to the same worker every time
func TestSendMetrics_ConsistentHash(t *testing.T) {
	ingesters := []*testMetricIngester{&testMetricIngester{}, &testMetricIngester{}}

	casted := make([]MetricIngester, len(ingesters))
	for i, ingester := range ingesters {
		casted[i] = ingester
	}
	s := New(casted)

	inputs := []*metricpb.Metric{
		&metricpb.Metric{Name: "test.counter", Type: metricpb.Type_Counter, Tags: []string{"tag:1"}},
		&metricpb.Metric{Name: "test.gauge", Type: metricpb.Type_Gauge},
		&metricpb.Metric{Name: "test.histogram", Type: metricpb.Type_Histogram, Tags: []string{"type:histogram"}},
		&metricpb.Metric{Name: "test.set", Type: metricpb.Type_Set},
		&metricpb.Metric{Name: "test.gauge3", Type: metricpb.Type_Gauge},
	}

	// Send the same inputs many times
	for i := 0; i < 10; i++ {
		s.SendMetrics(context.TODO(), &forwardrpc.MetricList{inputs})

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
	s := New([]MetricIngester{ingester})
	s.SendMetrics(context.Background(), &forwardrpc.MetricList{})

	assert.Empty(t, ingester.metrics, "The server shouldn't have submitted "+
		"any metrics")
}

func TestOptions_WithTraceClient(t *testing.T) {
	c, err := trace.NewClient(trace.DefaultVeneurAddress)
	if err != nil {
		t.Fatalf("failed to initialize a trace client: %v", err)
	}

	s := New([]MetricIngester{}, WithTraceClient(c))
	assert.Equal(t, c, s.opts.traceClient, "WithTraceClient didn't correctly "+
		"set the trace client")
}

type noopChannelMetricIngester struct {
	in   chan *metricpb.Metric
	quit chan struct{}
}

func newNoopChannelMetricIngester() *noopChannelMetricIngester {
	return &noopChannelMetricIngester{
		in:   make(chan *metricpb.Metric),
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

func (mi *noopChannelMetricIngester) IngestMetric(m *metricpb.Metric) {
	mi.in <- m
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
		s := New(ingesters)
		ctx := context.Background()
		input := &forwardrpc.MetricList{Metrics: metrics[:inputSize]}

		b.Run(fmt.Sprintf("InputSize=%d", inputSize), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				s.SendMetrics(ctx, input)
			}
		})
	}
}
