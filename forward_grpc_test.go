package veneur

import (
	"github.com/stripe/veneur/samplers"
)

const (
	grpcTestMetricPrefix = "test.grpc."
)

// TODO This file will contain additional end-to-end tests for gRPC forwarding.
// For now it just contains some helper functions that are utilized elsewhere.

func testGRPCMetric(name string) string {
	return grpcTestMetricPrefix + name
}

func forwardGRPCTestMetrics() []*samplers.UDPMetric {
	return []*samplers.UDPMetric{
		&samplers.UDPMetric{
			MetricKey: samplers.MetricKey{
				Name: testGRPCMetric("histogram"),
				Type: histogramTypeName,
			},
			Value:      20.0,
			Digest:     12345,
			SampleRate: 1.0,
			Scope:      samplers.MixedScope,
		},
		&samplers.UDPMetric{
			MetricKey: samplers.MetricKey{
				Name: testGRPCMetric("gauge"),
				Type: gaugeTypeName,
			},
			Value:      1.0,
			SampleRate: 1.0,
			Scope:      samplers.GlobalOnly,
		},
		&samplers.UDPMetric{
			MetricKey: samplers.MetricKey{
				Name: testGRPCMetric("counter"),
				Type: counterTypeName,
			},
			Value:      2.0,
			SampleRate: 1.0,
			Scope:      samplers.GlobalOnly,
		},
		&samplers.UDPMetric{
			MetricKey: samplers.MetricKey{
				Name: testGRPCMetric("timer"),
				Type: timerTypeName,
			},
			Value:      100.0,
			Digest:     12345,
			SampleRate: 1.0,
			Scope:      samplers.GlobalOnly,
		},
		&samplers.UDPMetric{
			MetricKey: samplers.MetricKey{
				Name: testGRPCMetric("set"),
				Type: setTypeName,
			},
			Value:      "test",
			SampleRate: 1.0,
			Scope:      samplers.GlobalOnly,
		},
		// Only global metrics should be forwarded
		&samplers.UDPMetric{
			MetricKey: samplers.MetricKey{
				Name: testGRPCMetric("counter.local"),
				Type: counterTypeName,
			},
			Value:      100.0,
			Digest:     12345,
			SampleRate: 1.0,
			Scope:      samplers.MixedScope,
		},
	}
}
