package dptest

import (
	"fmt"
	"github.com/signalfx/golib/datapoint"
)

// ExactlyOne returns the datapoint with metricName and panics if there is not exactly one in dps
func ExactlyOne(dps []*datapoint.Datapoint, metricName string) *datapoint.Datapoint {
	return ExactlyOneDims(dps, metricName, nil)
}

// ExactlyOneDims returns the datapoint with metricName and dims, and panics if there is not exactly one
func ExactlyOneDims(dps []*datapoint.Datapoint, metricName string, dims map[string]string) *datapoint.Datapoint {
	var found *datapoint.Datapoint
	for _, dp := range dps {
		if dp.Metric == metricName {
			dimsMatch := true
			for k, v := range dims {
				v2, exists := dp.Dimensions[k]
				if !exists || v2 != v {
					dimsMatch = false
					break
				}
			}
			if !dimsMatch {
				continue
			}
			if found != nil {
				panic(fmt.Sprintf("Found metric name twice %s vs %s", dp, found))
			}
			found = dp
		}
	}
	if found == nil {
		panic("could not find metric name")
	}
	return found
}
