package importsrv_test

import (
	"github.com/axiomhq/hyperloglog"
	"github.com/stripe/veneur/samplers/metricpb"
	"github.com/stripe/veneur/tdigest"
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
	td := tdigest.NewMerging(1000, true)
	for _, s := range values {
		td.Add(s, 1)
	}
	m := &metricpb.Metric{
		Name:  name,
		Value: &metricpb.Metric_Histogram{Histogram: &metricpb.HistogramValue{TDigest: td.Data()}},
		Type:  metricpb.Type_Histogram,
	}

	for _, opt := range opts {
		opt(m)
	}
	return m
}

func pbset(name string, values []string, opts ...func(m *metricpb.Metric)) *metricpb.Metric {
	hll := hyperloglog.New()
	for _, s := range values {
		hll.Insert([]byte(s))
	}

	v, err := hll.MarshalBinary()
	if err != nil {
		panic(err)
	}
	m := &metricpb.Metric{
		Name:  name,
		Value: &metricpb.Metric_Set{Set: &metricpb.SetValue{HyperLogLog: v}},
		Type:  metricpb.Type_Set,
	}

	for _, opt := range opts {
		opt(m)
	}
	return m
}
