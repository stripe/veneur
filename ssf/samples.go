package ssf

import "time"

// SampleOption is a functional option that can be used for less
// commonly needed fields in sample creation helper functions.
type SampleOption func(*SSFSample)

// Unit is a functional option for creating an SSFSample. It sets the
// sample's unit name to the name passed.
func Unit(name string) SampleOption {
	return func(s *SSFSample) {
		s.Unit = name
	}
}

// Timestamp is a functional option for creating an SSFSample. It sets
// the timestamp field on the sample to the timestamp passed.
func Timestamp(ts time.Time) SampleOption {
	return func(s *SSFSample) {
		s.Timestamp = ts.UnixNano()
	}
}

func create(base *SSFSample, opts []SampleOption) *SSFSample {
	for _, opt := range opts {
		opt(base)
	}
	return base
}

// Count returns an SSFSample representing an increment / decrement of
// a counter. It's a convenience wrapper around constructing SSFSample
// objects.
func Count(name string, value float32, tags map[string]string, opts ...SampleOption) *SSFSample {
	return create(&SSFSample{
		Metric:     SSFSample_COUNTER,
		Name:       name,
		Value:      value,
		Tags:       tags,
		SampleRate: 1.0,
	}, opts)
}

// Gauge returns an SSFSample representing a gauge at a certain
// value. It's a convenience wrapper around constructing SSFSample
// objects.
func Gauge(name string, value float32, tags map[string]string, opts ...SampleOption) *SSFSample {
	return create(&SSFSample{
		Metric:     SSFSample_GAUGE,
		Name:       name,
		Value:      value,
		Tags:       tags,
		SampleRate: 1.0,
	}, opts)
}

// Histogram returns an SSFSample representing a value on a histogram,
// like a timer or other range. It's a convenience wrapper around
// constructing SSFSample objects.
func Histogram(name string, value float32, tags map[string]string, opts ...SampleOption) *SSFSample {
	return create(&SSFSample{
		Metric:     SSFSample_HISTOGRAM,
		Name:       name,
		Value:      value,
		Tags:       tags,
		SampleRate: 1.0,
	}, opts)
}
