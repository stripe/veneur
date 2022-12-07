package proxysrv

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stripe/veneur/v14/forwardrpc"
	"github.com/stripe/veneur/v14/internal/forwardtest"
	"github.com/stripe/veneur/v14/samplers/metricpb"
	metrictest "github.com/stripe/veneur/v14/samplers/metricpb/testutils"
	"stathat.com/c/consistent"
)

func createTestForwardServers(t *testing.T, n int, handler forwardtest.SendMetricHandler) []*forwardtest.Server {
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

		server := newServer(t, ring)
		err := server.sendMetrics(context.Background(), &forwardrpc.MetricList{expected})
		assert.NoError(t, err, "sendMetrics shouldn't have failed")

		assert.ElementsMatch(t, expected, actual)
	}
}

func TestNoDestinations(t *testing.T) {
	server := newServer(t, consistent.New())
	err := server.sendMetrics(context.Background(),
		&forwardrpc.MetricList{metrictest.RandomForwardMetrics(10)})
	assert.Error(t, err, "sendMetrics should have returned an error when there "+
		"are no valid destinations")
}

func TestUnreachableDestinations(t *testing.T) {
	ring := consistent.New()
	ring.Add("not-a-real-host:9001")
	ring.Add("another-bad-host:9001")

	server := newServer(t, ring, WithForwardTimeout(500*time.Millisecond))
	err := server.sendMetrics(context.Background(),
		&forwardrpc.MetricList{metrictest.RandomForwardMetrics(10)})
	assert.Error(t, err, "sendMetrics should have returned an error when all "+
		"of the destinations are unreachable")
}

func TestTimeout(t *testing.T) {
	dests := createTestForwardServers(t, 3, nil)
	defer stopTestForwardServers(dests)

	ring := consistent.New()
	ring.Set(addrsFromServers(dests))

	server := newServer(t, ring, WithForwardTimeout(1*time.Nanosecond))
	err := server.sendMetrics(context.Background(),
		&forwardrpc.MetricList{metrictest.RandomForwardMetrics(10)})
	assert.Error(t, err, "sendMetrics should have returned an error when the "+
		"timeout was set to effectively zero")
}

func TestSetDestinations(t *testing.T) {
	// create a set of servers that we will use first (the "original" servers)
	receivedByOriginal := false
	var originalMtx sync.Mutex
	original := createTestForwardServers(t, 3, func(_ []*metricpb.Metric) {
		originalMtx.Lock()
		defer originalMtx.Unlock()
		receivedByOriginal = true
	})
	defer stopTestForwardServers(original)

	// create a set of test servers
	receivedByNew := false
	var newMtx sync.Mutex
	new := createTestForwardServers(t, 3, func(_ []*metricpb.Metric) {
		newMtx.Lock()
		defer newMtx.Unlock()
		receivedByNew = true
	})
	defer stopTestForwardServers(new)

	// put all of the original servers into the ring
	ring := consistent.New()
	ring.Set(addrsFromServers(original))

	// Send some metrics.  This should go to the original set of servers
	server := newServer(t, ring)
	defer server.Stop()
	err := server.sendMetrics(context.Background(),
		&forwardrpc.MetricList{metrictest.RandomForwardMetrics(10)})
	assert.NoError(t, err, "sendMetrics should not have returned an error")
	assert.True(t, receivedByOriginal, "the original set of servers should have gotten some requests, but didn't")
	assert.False(t, receivedByNew, "the new servers shouldn't have gotten RPCs")

	// Now reset the check and change the servers, then run it again
	receivedByOriginal = false
	ring.Set(addrsFromServers(new))
	assert.NoError(t, server.SetDestinations(ring), "setting the destinations failed")
	err = server.sendMetrics(context.Background(),
		&forwardrpc.MetricList{metrictest.RandomForwardMetrics(10)})
	assert.NoError(t, err, "sendMetrics should not have returned an error")
	assert.True(t, receivedByNew, "the new servers should have had RPCs")
	assert.False(t, receivedByOriginal, "the old servers should not have gotten RPCs")

	// Make a group of both, and set that.
	receivedByOriginal = false
	receivedByNew = false
	both := []*forwardtest.Server{original[0], original[1], new[1], new[2]}
	ring.Set(addrsFromServers(both))
	assert.NoError(t, server.SetDestinations(ring), "setting the destinations failed")
	err = server.sendMetrics(context.Background(),
		&forwardrpc.MetricList{metrictest.RandomForwardMetrics(100)})
	assert.NoError(t, err, "sendMetrics should not have returned an error")
	assert.True(t, receivedByNew, "the new servers should have had RPCs")
	assert.True(t, receivedByOriginal, "the old servers should have gotten RPCs")
}

func TestCountActiveHandlers(t *testing.T) {
	t.Parallel()

	// Repeat this test for a couple different numbers of calls
	for _, n := range []int{0, 1, 5, 10} {
		n := n
		for _, protocol := range []string{"grpc", "grpc-stream"} {
			t.Run(fmt.Sprintf("handlers=%d protocol=%s", n, protocol), func(t *testing.T) {
				t.Parallel()

				// Create some test servers that will block until explicitly stopped
				done := make(chan struct{})
				blocking := createTestForwardServers(t, 3, func(_ []*metricpb.Metric) {
					<-done
				})
				defer stopTestForwardServers(blocking)
				// put all of the servers into a ring
				ring := consistent.New()
				ring.Set(addrsFromServers(blocking))

				metrics := &forwardrpc.MetricList{metrictest.RandomForwardMetrics(100)}

				opts := []Option{WithStatsInterval(10 * time.Nanosecond)}
				if protocol == "grpc-stream" {
					opts = append(opts, WithEnableStreaming(true))
				}
				s := newServer(t, ring, opts...)

				// Make the specified number of calls, all of these should spawn
				// goroutines that will block
				for i := 0; i < n; i++ {
					s.SendMetrics(context.Background(), metrics)
				}

				// Since the goroutines are forked immediately after the function
				// call, it might take a bit of time for all of them to start.
				// We should wait for a little bit
				tick := time.NewTicker(10 * time.Nanosecond)
				defer tick.Stop()

				timeout := time.NewTicker(10 * time.Second)
				defer timeout.Stop()
				for int64(n) != atomic.LoadInt64(s.activeProxyHandlers) {
					select {
					case <-tick.C:
						// Report statistics, just to exercise the funtion.  This
						// would normally be called by the server periodically
						s.reportStats()
					case <-timeout.C:
						assert.Failf(t, "The count of active proxy handlers didn't increase enough before the timeout",
							"Expected: %d\tCurrent: %d", n, atomic.LoadInt64(s.activeProxyHandlers))
						return
					}
				}

				// Stop all of the servers and check that the counter goes to zero
				close(done)
				timeout = time.NewTicker(10 * time.Second)
				defer timeout.Stop()
				for atomic.LoadInt64(s.activeProxyHandlers) != 0 {
					select {
					case <-tick.C:
						// Report statistics, just to exercise the funtion.  This
						// would normally be called by the server periodically
						s.reportStats()
					case <-timeout.C:
						assert.Failf(t, "The count of active proxy handlers didn't drop to zero",
							"Current: %d", atomic.LoadInt64(s.activeProxyHandlers))
						return
					}
				}
			})
		}
	}
}

func BenchmarkProxyServerSendMetrics(b *testing.B) {
	// Use a consistent seed for predictably comparable results
	rand.Seed(1522191080)

	ring := consistent.New()
	servers := make([]*forwardtest.Server, 5)
	for i := range servers {
		servers[i] = forwardtest.NewServer(func(_ []*metricpb.Metric) {})
		servers[i].Start(b)
		ring.Add(servers[i].Addr().String())
	}

	metrics := metrictest.RandomForwardMetrics(10000)
	for _, inputSize := range []int{10, 100, 1000, 10000} {
		s := newServer(b, ring)
		ctx := context.Background()
		input := &forwardrpc.MetricList{Metrics: metrics[:inputSize]}

		b.Run(fmt.Sprintf("InputSize=%d", inputSize), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				s.sendMetrics(ctx, input)
			}
		})
	}
}

func addrsFromServers(a []*forwardtest.Server) []string {
	res := make([]string, len(a))
	for i, s := range a {
		res[i] = s.Addr().String()
	}
	return res
}

func newServer(t testing.TB, ring *consistent.Consistent, opts ...Option) *Server {
	s, err := New(ring, opts...)
	assert.NoError(t, err, "creating a server shouldn't have returned an error")
	return s
}
