package importsrv_test

import (
	"github.com/stripe/veneur/samplers/metricpb"
)

//
// PROTOBUF
//

func pbmetrics(ms ...*metricpb.Metric) []*metricpb.Metric {
	return ms
}

// options

func pbtags(tags ...string) func(m *metricpb.Metric) {
	return func(m *metricpb.Metric) {
		m.Tags = tags
	}
}

func pbhostname(hostname string) func(m *metricpb.Metric) {
	return func(m *metricpb.Metric) {
		m.Hostname = hostname
	}
}

func pbscope(scope metricpb.Scope) func(m *metricpb.Metric) {
	return func(m *metricpb.Metric) {
		m.Scope = scope
	}
}

// metric types

func pbcounter(name string, value int64, opts ...func(m *metricpb.Metric)) *metricpb.Metric {
	c := &metricpb.Metric{
		Name:  name,
		Value: &metricpb.Metric_Counter{Counter: &metricpb.CounterValue{Value: value}},
		Type:  metricpb.Type_Counter,
	}

	for _, opt := range opts {
		opt(c)
	}
	return c
}

func pbgauge(name string, value float64, opts ...func(m *metricpb.Metric)) *metricpb.Metric {
	c := &metricpb.Metric{
		Name:  name,
		Value: &metricpb.Metric_Gauge{Gauge: &metricpb.GaugeValue{Value: value}},
		Type:  metricpb.Type_Gauge,
	}

	for _, opt := range opts {
		opt(c)
	}
	return c
}

func pbhisto(name string, values []float64, opts ...func(m *metricpb.Metric)) *metricpb.Metric {
	return nil
}

func pbset(name string, values []string, opts ...func(m *metricpb.Metric)) *metricpb.Metric {
	return nil
}
