package ssf

import (
	"math/rand"
	"time"
)

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

// SampleRate sets the rate at which a measurement is sampled. The
// rate is a number on the interval (0..1] (1 means that the value is
// not sampled). Any numbers outside this interval result in no change
// to the sample rate (by default, all SSFSamples created with the
// helpers in this package have a SampleRate=1).
func SampleRate(rate float32) SampleOption {
	return func(s *SSFSample) {
		if rate > 0 && rate <= 1 {
			s.SampleRate = rate
		}
	}
}

var resolutions = map[time.Duration]string{
	time.Nanosecond:  "ns",
	time.Microsecond: "µs",
	time.Millisecond: "ms",
	time.Second:      "s",
	time.Minute:      "min",
	time.Hour:        "h",
}

// TimeUnit sets the unit on a sample to the given resolution's SI
// unit symbol. Valid resolutions are the time duration constants from
// Nanosecond through Hour. The non-SI units "minute" and "hour" are
// represented by "min" and "h" respectively.
//
// If a resolution is passed that does not correspond exactly to the
// duration constants in package time, this option does not affect the
// sample at all.
func TimeUnit(resolution time.Duration) SampleOption {
	return func(s *SSFSample) {
		if unit, ok := resolutions[resolution]; ok {
			s.Unit = unit
		}
	}
}

func create(base *SSFSample, opts []SampleOption) *SSFSample {
	for _, opt := range opts {
		opt(base)
	}
	return base
}

// Sampled takes a rate and a set of measurements, and returns them as
// if sampling had been performed: Each measurement gets
// rejected/included in the result based on a random roll of the RNG
// according to the rate, and each measurement has its SampleRate
// field adjusted to match the existing SampleRate * rate.
func Sampled(rate float32, samples ...*SSFSample) []*SSFSample {
	res := make([]*SSFSample, 0, len(samples))

	for _, s := range samples {
		if rand.Float32() <= rate {
			if rate > 0 && rate <= 1 {
				s.SampleRate = s.SampleRate * rate
			}
			res = append(res, s)
		}
	}
	return res
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

// Timing returns an SSFSample (really a histogram) representing the
// timing in the given resolution.
func Timing(name string, value time.Duration, resolution time.Duration, tags map[string]string, opts ...SampleOption) *SSFSample {
	time := float32(value / resolution)
	return Histogram(name, time, tags, append(opts, TimeUnit(resolution))...)
}
