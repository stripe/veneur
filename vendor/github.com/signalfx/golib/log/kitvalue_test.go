package log_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/signalfx/golib/log"
	"github.com/signalfx/golib/timekeeper/timekeepertest"
)

func TestValueBinding(t *testing.T) {
	var output []interface{}

	logger := log.LoggerFunc(func(keyvals ...interface{}) {
		output = keyvals
	})

	start := time.Date(2015, time.April, 25, 0, 0, 0, 0, time.UTC)
	now := start
	mocktime := timekeepertest.NewStubClock(now)
	mocktime.Incr(time.Second)

	lc := log.NewContext(logger).With("ts", &log.TimeDynamic{TimeKeeper: mocktime}, "caller", log.DefaultCaller)

	lc.Log("foo", "bar")
	timestamp, ok := output[1].(time.Time)
	if !ok {
		t.Fatalf("want time.Time, have %T", output[1])
	}
	if want, have := start.Add(time.Second), timestamp; want != have {
		t.Errorf("output[1]: want %v, have %v", want, have)
	}
	if want, have := "kitvalue_test.go:26", fmt.Sprint(output[3]); want != have {
		t.Errorf("output[3]: want %s, have %s", want, have)
	}

	// A second attempt to confirm the bindings are truly dynamic.
	mocktime.Incr(time.Second)
	lc.Log("foo", "bar")
	timestamp, ok = output[1].(time.Time)
	if !ok {
		t.Fatalf("want time.Time, have %T", output[1])
	}
	if want, have := start.Add(2*time.Second), timestamp; want != have {
		t.Errorf("output[1]: want %v, have %v", want, have)
	}
	if want, have := "kitvalue_test.go:40", fmt.Sprint(output[3]); want != have {
		t.Errorf("output[3]: want %s, have %s", want, have)
	}
}

func TestValueBinding_loggingZeroKeyvals(t *testing.T) {
	var output []interface{}

	logger := log.Logger(log.LoggerFunc(func(keyvals ...interface{}) {
		output = keyvals
	}))

	start := time.Date(2015, time.April, 25, 0, 0, 0, 0, time.UTC)
	now := start
	mocktime := timekeepertest.NewStubClock(now)
	mocktime.Incr(time.Second)

	logger = log.NewContext(logger).With("ts", &log.TimeDynamic{TimeKeeper: mocktime})

	logger.Log()
	timestamp, ok := output[1].(time.Time)
	if !ok {
		t.Fatalf("want time.Time, have %T", output[1])
	}
	if want, have := start.Add(time.Second), timestamp; want != have {
		t.Errorf("output[1]: want %v, have %v", want, have)
	}

	mocktime.Incr(time.Second)
	// A second attempt to confirm the bindings are truly dynamic.
	logger.Log()
	timestamp, ok = output[1].(time.Time)
	if !ok {
		t.Fatalf("want time.Time, have %T", output[1])
	}
	if want, have := start.Add(2*time.Second), timestamp; want != have {
		t.Errorf("output[1]: want %v, have %v", want, have)
	}
}

func BenchmarkValueBindingTimestamp(b *testing.B) {
	logger := log.Discard
	lc := log.NewContext(logger).With("ts", log.DefaultTimestamp)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		lc.Log("k", "v")
	}
}

func BenchmarkValueBindingCaller(b *testing.B) {
	logger := log.Discard
	lc := log.NewContext(logger).With("caller", log.DefaultCaller)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		lc.Log("k", "v")
	}
}
