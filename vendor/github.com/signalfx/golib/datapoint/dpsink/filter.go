package dpsink

import (
	"sync/atomic"

	"github.com/signalfx/golib/datapoint"
	"golang.org/x/net/context"
)

// EmptyMetricFilter filters empty metric name datapoints
type EmptyMetricFilter struct {
	EmptyMetricFiltered int64
}

// FilterDatapoints returns points that have a non empty metric name
func (e *EmptyMetricFilter) FilterDatapoints(points []*datapoint.Datapoint) []*datapoint.Datapoint {
	validDatapoints := make([]*datapoint.Datapoint, 0, len(points))
	for _, dat := range points {
		if dat.Metric != "" {
			validDatapoints = append(validDatapoints, dat)
		}
	}
	invalidDatumCount := int64(len(points) - len(validDatapoints))
	if invalidDatumCount != 0 {
		atomic.AddInt64(&e.EmptyMetricFiltered, invalidDatumCount)
	}
	return validDatapoints
}

// AddDatapoints will send points to the next sink that have a non empty metric name
func (e *EmptyMetricFilter) AddDatapoints(ctx context.Context, points []*datapoint.Datapoint, next Sink) error {
	validDatapoints := e.FilterDatapoints(points)
	if len(validDatapoints) == 0 {
		return nil
	}
	return next.AddDatapoints(ctx, validDatapoints)
}
