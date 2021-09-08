package veneur

import (
	"context"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stripe/veneur/v14/samplers"
)

type dummyPlugin struct {
	logger *logrus.Logger
	flush  func(context.Context, []samplers.InterMetric) error
}

func (dp *dummyPlugin) Flush(ctx context.Context, metrics []samplers.InterMetric) error {
	return dp.flush(ctx, metrics)
}

func (dp *dummyPlugin) Name() string {
	return "dummy_plugin"
}

// TestGlobalServerPluginFlush tests that we are able to
// register a dummy plugin on the server, and that when we do,
// flushing on the server causes the plugin to flush
func TestGlobalServerPluginFlush(t *testing.T) {

	RemoteResponseChan := make(chan struct{}, 1)
	defer func() {
		select {
		case <-RemoteResponseChan:
			// all is safe
			return
		case <-time.After(DefaultServerTimeout):
			assert.Fail(t, "Global server did not complete all responses before test terminated!")
		}
	}()

	metricValues, expectedMetrics := generateMetrics()
	config := globalConfig()
	f := newFixture(t, config, nil, nil)
	defer f.Close()

	dp := &dummyPlugin{logger: log}

	dp.flush = func(ctx context.Context, metrics []samplers.InterMetric) error {
		assert.Equal(t, len(expectedMetrics), len(metrics))

		firstName := metrics[0].Name
		assert.Equal(t, expectedMetrics[firstName], metrics[0].Value)

		RemoteResponseChan <- struct{}{}
		return nil
	}

	f.server.registerPlugin(dp)

	for _, value := range metricValues {
		f.server.Workers[0].ProcessMetric(&samplers.UDPMetric{
			MetricKey: samplers.MetricKey{
				Name: "a.b.c",
				Type: "histogram",
			},
			Value:      value,
			Digest:     12345,
			SampleRate: 1.0,
			Scope:      samplers.LocalOnly,
		})
	}

	f.server.Flush(context.Background())
}

// TestLocalFilePluginRegister tests that we are able to register
// a local file as a flush output for Veneur.
func TestLocalFilePluginRegister(t *testing.T) {
	config := globalConfig()
	config.FlushFile = "/dev/null"

	server, err := NewFromConfig(ServerConfig{
		Logger: logrus.New(),
		Config: config,
	})
	if err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, 1, len(server.getPlugins()))
}
