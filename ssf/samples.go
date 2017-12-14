package ssf

// Count returns an SSFSample representing an increment / decrement of
// a counter. It's a convenience wrapper around constructing SSFSample
// objects.
func Count(name string, value float32, tags map[string]string) *SSFSample {
	return &SSFSample{
		Metric:     SSFSample_COUNTER,
		Name:       name,
		Value:      value,
		Tags:       tags,
		SampleRate: 1.0,
	}
}

// Gauge returns an SSFSample representing a gauge at a certain
// value. It's a convenience wrapper around constructing SSFSample
// objects.
func Gauge(name string, value float32, tags map[string]string) *SSFSample {
	return &SSFSample{
		Metric:     SSFSample_GAUGE,
		Name:       name,
		Value:      value,
		Tags:       tags,
		SampleRate: 1.0,
	}
}

// Histogram returns an SSFSample representing a value on a histogram,
// like a timer or other range. It's a convenience wrapper around
// constructing SSFSample objects.
func Histogram(name string, value float32, tags map[string]string) *SSFSample {
	return &SSFSample{
		Metric:     SSFSample_HISTOGRAM,
		Name:       name,
		Value:      value,
		Tags:       tags,
		SampleRate: 1.0,
	}
}
