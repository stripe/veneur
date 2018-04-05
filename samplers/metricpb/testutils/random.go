package testutils

import (
	"math/rand"
	"strconv"

	"github.com/stripe/veneur/samplers/metricpb"
)

// RandomForwardMetric generates a metricpb.Metric with most fields
// initialized to random values.
func RandomForwardMetric() *metricpb.Metric {
	return &metricpb.Metric{
		Name:  strconv.Itoa(rand.Int()),
		Type:  metricpb.Type_Counter,
		Tags:  []string{strconv.Itoa(rand.Int())},
		Value: &metricpb.Metric_Counter{&metricpb.CounterValue{rand.Int63()}},
	}
}

// RandomForwardMetrics returns a slice of n random metricpb.Metric's
func RandomForwardMetrics(n int) []*metricpb.Metric {
	res := make([]*metricpb.Metric, n)
	for i := range res {
		res[i] = RandomForwardMetric()
	}
	return res
}
