package log

import (
	"github.com/signalfx/golib/timekeeper/timekeepertest"
	. "github.com/smartystreets/goconvey/convey"
	"testing"
	"time"
)

func BenchmarkValueBindingTimestamp(b *testing.B) {
	benchmarkValueBindingTimestamp(b, Discard)
}

func BenchmarkValueBindingTimestampCount(b *testing.B) {
	benchmarkValueBindingTimestamp(b, &Counter{})
}

func benchmarkValueBindingTimestamp(b *testing.B, logger Logger) {
	lc := NewContext(logger).With("ts", DefaultTimestamp)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		lc.Log("k", "v")
	}
}

func BenchmarkValueBindingCaller(b *testing.B) {
	benchmarkValueBindingCaller(b, Discard)
}

func BenchmarkValueBindingCallerCount(b *testing.B) {
	benchmarkValueBindingCaller(b, &Counter{})
}

func benchmarkValueBindingCaller(b *testing.B, logger Logger) {
	lc := NewContext(logger).With("caller", DefaultCaller)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		lc.Log("k", "v")
	}
}

func between(start, mid, end time.Time) bool {
	return !mid.Before(start) && !end.Before(mid)
}

func TestTimeSince(t *testing.T) {
	Convey("Normal time since", t, func() {
		td := &TimeSince{}
		c := NewChannelLogger(1, nil)
		l := NewContext(c)
		Convey("Empty should work", func() {
			l.Log("td", td)
			msg := <-c.Out
			_, ok := msg[1].(time.Duration)
			So(ok, ShouldBeTrue)
		})
		Convey("Should allow overrides", func() {
			start := time.Now()
			tk := timekeepertest.NewStubClock(start)
			td.TimeKeeper = tk
			td.Start = start
			tk.Incr(time.Second)
			l.Log("td", td)
			msg := <-c.Out
			So(msg[1].(time.Duration), ShouldEqual, time.Second)
		})
	})
}

func TestTimeDynamic(t *testing.T) {
	Convey("Normal time dynamic", t, func() {
		td := &TimeDynamic{}
		c := NewChannelLogger(1, nil)
		l := NewContext(c)
		Convey("Empty should work", func() {
			start := time.Now()
			l.Log("ts", td)
			later := time.Now()
			msg := <-c.Out
			So(between(start, msg[1].(time.Time), later), ShouldBeTrue)
		})
		Convey("UTC should convert", func() {
			td.UTC = true
			l.Log("ts", td)
			msg := <-c.Out
			So(msg[1].(time.Time).Location(), ShouldEqual, time.UTC)
		})
		Convey("Layout should change", func() {
			td.AsString = true
			l.Log("ts", td)
			msg1 := <-c.Out
			td.Layout = time.RFC3339Nano
			l.Log("ts", td)
			msg2 := <-c.Out
			So(msg1[1].(string), ShouldNotResemble, msg2[1].(string))
		})
	})
}
