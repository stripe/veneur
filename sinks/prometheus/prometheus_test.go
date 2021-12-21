package prometheus

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"testing"

	"github.com/stripe/veneur/v14"
	"github.com/stripe/veneur/v14/samplers"
	"github.com/stripe/veneur/v14/trace"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

func testLogger() *logrus.Entry {
	return logrus.NewEntry(logrus.New())
}

func TestCreateMetricSink(t *testing.T) {
	logger := testLogger()
	for name, tc := range map[string]struct {
		addr    string
		network string
		wantErr bool
	}{
		"valid udp addr and network": {
			addr:    "http://127.0.0.1:5000",
			network: "udp",
			wantErr: false,
		},
		"valid tcp addr and network": {
			addr:    "localhost:5000",
			network: "tcp",
			wantErr: false,
		},
		"invalid network type": {
			addr:    "http://127.0.0.1:5000",
			network: "ip4",
			wantErr: true,
		},
		"invalid address": {
			addr:    "hi",
			network: "tcp",
			wantErr: true,
		},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := CreateMetricSink(
				&veneur.Server{
					TraceClient: nil,
				},
				"prometheus",
				logger,
				veneur.Config{},
				PrometheusMetricSinkConfig{
					RepeaterAddress: tc.addr,
					NetworkType:     tc.network,
				})
			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestMetricSinkName(t *testing.T) {
	sink, err := CreateMetricSink(
		&veneur.Server{
			TraceClient: nil,
		},
		"prometheus-test",
		testLogger(),
		veneur.Config{},
		PrometheusMetricSinkConfig{
			RepeaterAddress: "http://127.0.0.1:5000",
			NetworkType:     "tcp",
		})
	assert.NotNil(t, sink)
	assert.NoError(t, err)
	assert.Equal(t, "prometheus-test", sink.Name())
}

func TestMetricFlush(t *testing.T) {
	// Create a TCP server emulating the statsd exporter, replaying the
	// requests back for testing.
	ln, err := net.Listen("tcp", ":0")
	assert.NoError(t, err)
	defer ln.Close()

	// Limit batchSize for testing.
	batchSize = 2
	expectedMessages := []string{
		"a.b.gauge:100|g|#foo:bar,baz:quz\na.b.counter:2|c|#foo:bar\n",
		"a.b.status:5|g|#\n",
	}

	errChan := make(chan error)
	resChan := make(chan string)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			errChan <- err
			return
		}
		defer conn.Close()

		for i := 0; i < len(expectedMessages); i++ {
			// By forcing the receive buffer to be the size of the message,
			// TCP would block and ensure that another message doesn't come
			// in.
			buf := make([]byte, len(expectedMessages[i]))
			_, err = conn.Read(buf)
			if err != nil {
				errChan <- err
				return
			}
			resChan <- string(bytes.Trim(buf, "\x00"))
		}
	}()

	logger := testLogger()
	port := ln.Addr().(*net.TCPAddr).Port
	sink, err := CreateMetricSink(
		&veneur.Server{
			TraceClient: nil,
		},
		"prometheus",
		logger,
		veneur.Config{},
		PrometheusMetricSinkConfig{
			RepeaterAddress: fmt.Sprintf("localhost:%d", port),
			NetworkType:     "tcp",
		})
	assert.NoError(t, err)

	assert.NoError(t, sink.Start(trace.DefaultClient))
	assert.NoError(t, sink.Flush(context.Background(), []samplers.InterMetric{
		samplers.InterMetric{
			Name:      "a.b.gauge",
			Timestamp: 1,
			Value:     float64(100),
			Tags: []string{
				"foo:bar",
				"baz:quz",
			},
			Type: samplers.GaugeMetric,
		},
		samplers.InterMetric{
			Name:      "a.b.counter",
			Timestamp: 1,
			Value:     float64(2),
			Tags: []string{
				"foo:bar",
			},
			Type: samplers.CounterMetric,
		},
		samplers.InterMetric{
			Name:      "a.b.status",
			Timestamp: 1,
			Value:     float64(5),
			Type:      samplers.StatusMetric,
		},
	}))

	for _, want := range expectedMessages {
		select {
		case res := <-resChan:
			assert.Equal(t, want, res)
		case err := <-errChan:
			// Give up here since it's likely if it failed, it may hang.
			assert.NoError(t, err)
			break
		}
	}
}

func TestParseConfig(t *testing.T) {
	parsedConfig, err := ParseMetricConfig("prometheus", map[string]interface{}{
		"repeater_address": "127.0.0.1:5000",
		"network_type":     "udp",
	})
	prometheusConfig := parsedConfig.(PrometheusMetricSinkConfig)
	assert.NoError(t, err)
	assert.Equal(t, prometheusConfig.RepeaterAddress, "127.0.0.1:5000")
	assert.Equal(t, prometheusConfig.NetworkType, "udp")
}
