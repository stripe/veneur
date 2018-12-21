package samplers

import (
	"encoding/json"
	"strings"
)

type TestMetric struct {
	Name     string
	Tags     string
	Value    float64
	Hostname string
	Type     MetricType
	Message  string
	Sinks    RouteInformation
}

func (t TestMetric) String() string {
	m, err := json.MarshalIndent(t, "", "    ")
	if err != nil {
		return ""
	}
	return string(m)
}

func TMetrics(rs ...TestMetric) []TestMetric {
	return rs
}

func TCounter(name string, value float64, opts ...func(r TestMetric) TestMetric) TestMetric {
	r := TestMetric{
		Name:  name,
		Value: value,
		Type:  CounterMetric,
	}
	for _, opt := range opts {
		r = opt(r)
	}
	return r
}

func TGauge(name string, value float64, opts ...func(r TestMetric) TestMetric) TestMetric {
	r := TestMetric{
		Name:  name,
		Value: value,
		Type:  GaugeMetric,
	}
	for _, opt := range opts {
		r = opt(r)
	}
	return r
}

func TStatus(name string, v float64, message string, opts ...func(r TestMetric) TestMetric) TestMetric {
	r := TestMetric{
		Name:    name,
		Value:   v,
		Message: message,
		Type:    StatusMetric,
	}
	for _, opt := range opts {
		r = opt(r)
	}
	return r
}

func OptTags(ts ...string) func(r TestMetric) TestMetric {
	return func(r TestMetric) TestMetric {
		r.Tags = strings.Join(ts, ",")
		return r
	}
}

func OptHostname(hn string) func(r TestMetric) TestMetric {
	return func(r TestMetric) TestMetric {
		r.Hostname = hn
		return r
	}
}

func OptMessage(msg string) func(r TestMetric) TestMetric {
	return func(r TestMetric) TestMetric {
		r.Message = msg
		return r
	}
}

func OptSinks(ri RouteInformation) func(r TestMetric) TestMetric {
	return func(r TestMetric) TestMetric {
		r.Sinks = ri
		return r
	}
}

func ToTestMetrics(ms []InterMetric) (outs []TestMetric) {
	for _, inm := range ms {
		outs = append(outs, TestMetric{
			Name:     inm.Name,
			Tags:     strings.Join(inm.Tags, ","),
			Value:    inm.Value,
			Type:     inm.Type,
			Message:  inm.Message,
			Hostname: inm.HostName,
			Sinks:    inm.Sinks,
		})
	}
	return outs
}
