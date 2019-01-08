package ssf

import (
	"math/rand"
	"sync"
	"time"
)

var rngPool = sync.Pool{
	New: func() interface{} {
		return rand.New(rand.NewSource(rand.Int63()))
	},
}

// defaultRandFloat32 returns a random float32
// using the default math/rand source
func defaultRandFloat32() float32 {
	return rand.Float32()
}

// Samples is a batch of SSFSamples, not attached to an SSF span, that
// can be submitted with package metrics's Report function.
type Samples struct {
	Batch []*SSFSample
}

// Add appends a sample to the batch of samples.
func (s *Samples) Add(sample ...*SSFSample) {
	if s.Batch == nil {
		s.Batch = []*SSFSample{}
	}
	s.Batch = append(s.Batch, sample...)
}

// NamePrefix is a string prepended to every SSFSample name generated
// by the constructors in this package. As no separator is added
// between this prefix and the metric name, users must take care to
// attach any separators to the prefix themselves.
var NamePrefix string

// SampleOption is a functional option that can be used for less
// commonly needed fields in sample creation helper functions. The
// options are applied by order of arguments (left to right), so when
// setting multiple of the same option, the rightmost wins.
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
	base.Name = NamePrefix + base.Name
	for _, opt := range opts {
		opt(base)
	}
	return base
}

// RandomlySample takes a rate and a set of measurements, and returns
// a new set of measurements as if sampling had been performed: Each
// original measurement gets rejected/included in the result based on
// a random roll of the RNG according to the rate, and each included
// measurement has its SampleRate field adjusted to be its original
// SampleRate * rate.
func RandomlySample(rate float32, samples ...*SSFSample) []*SSFSample {
	res := make([]*SSFSample, 0, len(samples))

	randFloat := defaultRandFloat32

	r, ok := rngPool.Get().(*rand.Rand)
	if ok {
		randFloat = r.Float32
		defer rngPool.Put(r)
	}

	for _, s := range samples {
		if randFloat() <= rate {
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

// Set returns an SSFSample representing a value on a set, useful for
// counting the unique values that occur in a certain time bound.
func Set(name string, value string, tags map[string]string, opts ...SampleOption) *SSFSample {
	return create(&SSFSample{
		Metric:     SSFSample_SET,
		Name:       name,
		Message:    value,
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

// Status returns an SSFSample capturing the reported state
// of a service
func Status(name string, state SSFSample_Status, tags map[string]string, opts ...SampleOption) *SSFSample {
	return create(&SSFSample{
		Metric:     SSFSample_STATUS,
		Name:       name,
		Status:     state,
		Tags:       tags,
		SampleRate: 1.0,
	}, opts)
}
