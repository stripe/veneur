package proxysrv

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stripe/veneur/forwardrpc"
	"github.com/stripe/veneur/forwardrpc/forwardtest"
	"github.com/stripe/veneur/samplers/metricpb"
	metrictest "github.com/stripe/veneur/samplers/metricpb/testutils"
	"stathat.com/c/consistent"
)

func createTestForwardServers(t *testing.T, n int, handler forwardtest.SendMetricHandler) []*forwardtest.Server {
	t.Helper()

	res := make([]*forwardtest.Server, n)
	for i := range res {
		res[i] = forwardtest.NewServer(handler)
		res[i].Start(t)
	}

	return res
}

func stopTestForwardServers(ss []*forwardtest.Server) {
	for _, s := range ss {
		s.Stop()
	}
}

// Test that it forwards a decent number of input metrics to many different
// destinations
func TestManyDestinations(t *testing.T) {
	// Test with many different numbers of forwarding destinations
	for numDests := 1; numDests < 10; numDests++ {
		var actual []*metricpb.Metric
		var mtx sync.Mutex
		dests := createTestForwardServers(t, numDests, func(ms []*metricpb.Metric) {
			mtx.Lock()
			defer mtx.Unlock()
			actual = append(actual, ms...)
		})
		defer stopTestForwardServers(dests)

		ring := consistent.New()
		for _, dest := range dests {
			ring.Add(dest.Addr().String())
		}

		expected := metrictest.RandomForwardMetrics(100)

		server := New(ring)
		err := server.sendMetrics(context.Background(), &forwardrpc.MetricList{expected})
		assert.NoError(t, err, "sendMetrics shouldn't have failed")

		assert.ElementsMatch(t, expected, actual)
	}
}

func TestNoDestinations(t *testing.T) {
	server := New(consistent.New())
	err := server.sendMetrics(context.Background(),
		&forwardrpc.MetricList{metrictest.RandomForwardMetrics(10)})
	assert.Error(t, err, "sendMetrics should have returned an error when there "+
		"are no valid destinations")
}

func TestUnreachableDestinations(t *testing.T) {
	ring := consistent.New()
	ring.Add("not-a-real-host:9001")
	ring.Add("another-bad-host:9001")

	server := New(ring)
	err := server.sendMetrics(context.Background(),
		&forwardrpc.MetricList{metrictest.RandomForwardMetrics(10)})
	assert.Error(t, err, "sendMetrics should have returned an error when all "+
		"of the destinations are unreachable")
}

func TestTimeout(t *testing.T) {
	dests := createTestForwardServers(t, 3, nil)
	defer stopTestForwardServers(dests)

	ring := consistent.New()
	for _, dest := range dests {
		ring.Add(dest.Addr().String())
	}

	server := New(ring, WithForwardTimeout(1*time.Nanosecond))
	err := server.sendMetrics(context.Background(),
		&forwardrpc.MetricList{metrictest.RandomForwardMetrics(10)})
	assert.Error(t, err, "sendMetrics should have returned an error when the "+
		"timeout was set to effectively zero")
}
