package veneur

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stripe/veneur/samplers"
	"github.com/stripe/veneur/sinks"
	"github.com/stripe/veneur/ssf"
)

type forwardFixture struct {
	t      testing.TB
	proxy  *Proxy
	global *Server

	globalTS *httptest.Server
	proxyTS  *httptest.Server
	server   *Server
}

// newForwardingFixture constructs and returns a chain of veneur
// servers that represent a fairly typical metrics pipeline:
//     [local veneur] -> [veneur proxy] -> [global veneur]
//
// The globalSink argument is the global veneur's metric sink, and the
// localConfig argument is the local veneur agent's config, which will
// be amended to include the forwarder and proxy addresses.
func newForwardingFixture(t testing.TB, localConfig Config, transport http.RoundTripper, globalSink sinks.MetricSink) *forwardFixture {
	ff := &forwardFixture{t: t}

	// Make the global veneur:
	ff.global = setupVeneurServer(t, globalConfig(), transport, globalSink, nil)
	ff.globalTS = httptest.NewServer(handleImport(ff.global))

	// Make the proxy that sends to the global veneur:
	proxyCfg := generateProxyConfig()
	proxyCfg.ForwardAddress = ff.globalTS.URL
	proxyCfg.ConsulTraceServiceName = ""
	proxyCfg.ConsulForwardServiceName = ""
	proxy, err := NewProxyFromConfig(logrus.New(), proxyCfg)
	require.NoError(t, err)
	ff.proxy = &proxy

	ff.proxy.Start()
	ff.proxyTS = httptest.NewServer(ff.proxy.Handler())

	// Now make the local server, have it forward to the proxy:
	localConfig.ForwardAddress = ff.proxyTS.URL
	ff.server = setupVeneurServer(t, localConfig, transport, nil, nil)

	return ff
}

// Close shuts down the chain of veneur servers.
func (ff *forwardFixture) Close() {
	ff.proxy.Shutdown()
	ff.proxyTS.Close()
	ff.global.Shutdown()
	ff.globalTS.Close()
	ff.server.Shutdown()
}

// Flush synchronously waits until all metrics have been flushed along
// the chain of veneur servers.
func (ff *forwardFixture) Flush(ctx context.Context) {
	ff.server.Flush(ctx)
	// The proxy proxies synchronously, so when the local server
	// returns, we assume it has proxied everything to the global
	// veneur.
	ff.global.Flush(ctx)
}

// IngestSpan synchronously writes a span to the forwarding fixture's
// local veneur's span worker. The fixture must be flushed via the
// (*forwardFixture).Flush method so the ingestion effects can be
// observed.
func (ff *forwardFixture) IngestSpan(span *ssf.SSFSpan) {
	ff.server.SpanWorker.SpanChan <- span
}

// IngestMetric synchronously writes a metric to the forwarding
// fixture's local veneur span worker. The fixture must be flushed via
// the (*forwardFixture).Flush method so the ingestion effects can be
// observed.
func (ff *forwardFixture) IngestMetric(m *samplers.UDPMetric) {
	ff.server.Workers[0].ProcessMetric(m)
}

// TestForwardingIndicatorMetrics ensures that the metrics extracted
// from indicator spans make it across the entire chain, from a local
// veneur, through a proxy, to a global veneur & get reported
// on the global veneur.
func TestE2EForwardingIndicatorMetrics(t *testing.T) {
	t.Parallel()
	ch := make(chan []samplers.InterMetric)
	sink, _ := NewChannelMetricSink(ch)
	cfg := localConfig()
	cfg.IndicatorSpanTimerName = "indicator.span.timer"
	ffx := newForwardingFixture(t, cfg, nil, sink)
	defer ffx.Close()

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
	ffx.IngestSpan(span)
	done := make(chan struct{})
	go func() {
		metrics := <-ch
		require.Equal(t, 3, len(metrics), "metrics:\n%#v", metrics)
		for _, suffix := range []string{".50percentile", ".75percentile", ".99percentile"} {
			mName := "indicator.span.timer" + suffix
			found := false
			for _, m := range metrics {
				if m.Name == mName {
					found = true
				}
			}
			assert.True(t, found, "Metric named %s missing", mName)
		}
		close(done)
	}()
	ffx.Flush(context.TODO())
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Timed out waiting for a metric after 5 seconds")
	}
}

func TestE2EForwardMetric(t *testing.T) {
	t.Parallel()
	ch := make(chan []samplers.InterMetric)
	sink, _ := NewChannelMetricSink(ch)
	cfg := localConfig()
	cfg.IndicatorSpanTimerName = "indicator.span.timer"
	ffx := newForwardingFixture(t, cfg, nil, sink)
	defer ffx.Close()

	ffx.IngestMetric(&samplers.UDPMetric{
		MetricKey: samplers.MetricKey{
			Name: "a.b.c",
			Type: "histogram",
		},
		Value:      20.0,
		Digest:     12345,
		SampleRate: 1.0,
		Scope:      samplers.MixedScope,
	})
	done := make(chan struct{})
	go func() {
		metrics := <-ch
		require.Equal(t, 3, len(metrics), "metrics:\n%#v", metrics)
		for _, suffix := range []string{".50percentile", ".75percentile", ".99percentile"} {
			mName := "a.b.c" + suffix
			found := false
			for _, m := range metrics {
				if m.Name == mName {
					found = true
				}
			}
			assert.True(t, found, "Metric named %s missing", mName)
		}
		close(done)
	}()
	ffx.Flush(context.TODO())
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Timed out waiting for a metric after 5 seconds")
	}
}
