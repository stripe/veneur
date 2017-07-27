package lightstep_test

import (
	"strings"
	. "testing"
)

// Provided solely as a relative value to compare against
func BenchmarkStringsRepeatBaseline(b *B) {
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		strings.Repeat("LongMessage", 4096)
	}
	return
}

func BenchmarkShortMessage(b *B) {
	tracer := newTestTracer(nil)
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		span := tracer.StartSpan("operation")
		span.LogEvent("ShortMessage")
		span.Finish()
	}
	return
}

func BenchmarkLongMessage(b *B) {
	tracer := newTestTracer(nil)
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		span := tracer.StartSpan("operation")
		span.LogEvent(strings.Repeat("LongMessage", 4096))
		span.Finish()
	}
	return
}
