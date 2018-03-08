package veneur

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stripe/veneur/forwardrpc/forwardtest"
	"github.com/stripe/veneur/samplers/metricpb"
)

func TestServerFlushGRPC(t *testing.T) {
	done := make(chan []string)
	testServer := forwardtest.NewServer(func(ms []*metricpb.Metric) {
		var names []string
		for _, m := range ms {
			names = append(names, m.Name)
		}
		done <- names
	})
	testServer.Start(t)
	defer testServer.Stop()

	localCfg := localConfig()
	localCfg.ForwardAddress = testServer.Addr().String()
	localCfg.ForwardUseGrpc = true
	local := setupVeneurServer(t, localCfg, nil, nil, nil)
	defer local.Shutdown()

	inputs := forwardGRPCTestMetrics()
	for _, input := range inputs {
		local.Workers[0].ProcessMetric(input)
	}

	local.Flush(context.TODO())

	expected := []string{
		testGRPCMetric("histogram"),
		testGRPCMetric("timer"),
		testGRPCMetric("counter"),
		testGRPCMetric("gauge"),
		testGRPCMetric("set"),
	}

	select {
	case v := <-done:
		assert.ElementsMatch(t, expected, v,
			"Flush didn't output the right metrics")
	case <-time.After(time.Second):
		t.Fatal("Timed out waiting for the gRPC server to receive the flush")
	}
}

// Just test that a flushing to a bad address is handled
func TestServerFlushGRPCBadAddress(t *testing.T) {
	localCfg := localConfig()
	localCfg.ForwardAddress = "bad-address:123"
	localCfg.ForwardUseGrpc = true
	local := setupVeneurServer(t, localCfg, nil, nil, nil)
	defer local.Shutdown()

	local.Workers[0].ProcessMetric(forwardGRPCTestMetrics()[0])
	local.Flush(context.TODO())
}
