package dplocal

import (
	"github.com/signalfx/golib/datapoint"
	"os"
	"time"
)

var osXXXHostname = os.Hostname

// NewOnHostDatapoint is like NewSingleNameDataPointWithType but also a source
// of this host
func NewOnHostDatapoint(metric string, value datapoint.Value, metricType datapoint.MetricType) *datapoint.Datapoint {
	return NewOnHostDatapointDimensions(metric, value, metricType, map[string]string{})
}

// NewOnHostDatapointDimensions is like NewOnHostDatapoint but also with optional dimensions
func NewOnHostDatapointDimensions(metric string, value datapoint.Value, metricType datapoint.MetricType, dimensions map[string]string) *datapoint.Datapoint {
	return Wrap(datapoint.New(metric, dimensions, value, metricType, time.Now()))
}

// Wrap a normal datapoint to give it dimensions about the current hostname
func Wrap(dp *datapoint.Datapoint) *datapoint.Datapoint {
	hostname, err := osXXXHostname()
	if dp.Dimensions == nil {
		dp.Dimensions = make(map[string]string, 2)
	}
	if err != nil {
		hostname = "unknown"
	}
	dp.Dimensions["host"] = hostname
	dp.Dimensions["source"] = "proxy"
	return dp
}
