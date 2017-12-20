package log

import (
	"bytes"
	"errors"
	"github.com/signalfx/golib/eventcounter"
	. "github.com/smartystreets/goconvey/convey"
	"io"
	"strconv"
	"sync"
	"testing"
	"time"
)

func TestWithConcurrentInternal(t *testing.T) {
	// Create some buckets to count how many events each goroutine logs.
	const goroutines = 10
	counts := [goroutines]int{}

	// This logger extracts a goroutine id from the last value field and
	// increments the referenced bucket.
	logger := LoggerFunc(func(kv ...interface{}) {
		goroutine := kv[len(kv)-1].(int)
		counts[goroutine]++
		if len(kv) != 10 {
			panic(kv)
		}
	})

	// With must be careful about handling slices that can grow without
	// copying the underlying array, so give it a challenge.
	l := NewContext(logger).With(make([]interface{}, 2, 40)...).With(make([]interface{}, 2, 40)...)

	// Start logging concurrently. Each goroutine logs its id so the logger
	// can bucket the event counts.
	var wg sync.WaitGroup
	wg.Add(goroutines)
	const n = 1000
	for i := 0; i < goroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			for j := 0; j < n; j++ {
				l.With("a", "b").WithPrefix("c", "d").Log("goroutineIdx", idx)
			}
		}(i)
	}
	wg.Wait()

	for bucket, have := range counts {
		if want := n; want != have {
			t.Errorf("bucket %d: want %d, have %d", bucket, want, have) // note Errorf
		}
	}
}

func TestErrorHandlerFunc(t *testing.T) {
	Convey("ErrorHandlerFunc should wrap", t, func() {
		c := &Counter{}
		f := ErrorHandlerFunc(func(error) Logger {
			return c
		})
		f.ErrorLogger(nil).Log()
		So(c.Count, ShouldEqual, 1)
	})
}

func TestContextOptimizations(t *testing.T) {
	Convey("A normal context", t, func() {
		count := Counter{}
		c := NewContext(&count)
		Convey("should wrap itself", func() {
			So(NewContext(c), ShouldEqual, c)
		})
		Convey("should early exit empty with", func() {
			So(c.With(), ShouldEqual, c)
			So(c.WithPrefix(), ShouldEqual, c)
		})
	})
}

func toStr(in []interface{}) []string {
	ret := make([]string, 0, len(in))
	for i := range in {
		ret = append(ret, in[i].(string))
	}
	return ret
}

func TestLoggingBasics(t *testing.T) {
	Convey("A nil logger should work as expected", t, func() {
		var l *Context
		var l2 Logger
		So(IsDisabled(l), ShouldBeTrue)
		So(l.With("a", "b"), ShouldBeNil)
		So(l.WithPrefix("a", "b"), ShouldBeNil)
		So(IsDisabled(NewContext(nil)), ShouldBeTrue)
		So(IsDisabled(NewContext(l2)), ShouldBeTrue)
	})
	Convey("A normal logger", t, func() {
		mem := NewChannelLogger(10, nil)
		c := NewContext(mem)
		Convey("Should not remember with statements", func() {
			c.With("name", "john")
			c.Log()
			So(len(<-mem.Out), ShouldEqual, 0)
		})
		Convey("panic with odd With params", func() {
			So(func() {
				c.With("name")
			}, ShouldPanic)
		})
		Convey("panic with odd WithPrefix params", func() {
			So(func() {
				c.WithPrefix("name")
			}, ShouldPanic)
		})
		Convey("should be disablable", func() {
			c.Logger = Discard
			So(IsDisabled(c), ShouldBeTrue)
		})
		Convey("should not even out context values on log", func() {
			c.Log("name")
			So(len(<-mem.Out), ShouldEqual, 1)
		})
		Convey("Should convey params using with", func() {
			c = c.With("name", "john")
			c.Log()
			msgs := <-mem.Out
			So(len(msgs), ShouldEqual, 2)
			So(msgs[0].(string), ShouldEqual, "name")
			So(msgs[1].(string), ShouldEqual, "john")
			Convey("and Log()", func() {
				c.Log("age", "10")
				So(toStr(<-mem.Out), ShouldResemble, []string{"name", "john", "age", "10"})
			})
			Convey("should put WithPrefix first", func() {
				c = c.WithPrefix("name", "jack")
				c.Log()
				So(toStr(<-mem.Out), ShouldResemble, []string{"name", "jack", "name", "john"})
			})
		})
	})
}

func BenchmarkEmptyLogDisabled(b *testing.B) {
	c := NewContext(Discard)
	for i := 0; i < b.N; i++ {
		c.Log()
	}
}

func BenchmarkEmptyLogNotDisabled(b *testing.B) {
	count := Counter{}
	c := NewContext(&count)
	for i := 0; i < b.N; i++ {
		c.Log()
	}
}

func BenchmarkContextWithLog(b *testing.B) {
	b.Logf("At %d", b.N)
	count := Counter{}
	c := NewContext(&count)
	c = c.With("hello", "world").With("hi", "bob")
	for i := 0; i < b.N; i++ {
		c.Log("hello", "world")
	}
}

func BenchmarkContextWithOnly(b *testing.B) {
	count := Counter{}
	c := NewContext(&count)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.With("", "")
	}
}

func BenchmarkContextWithWithLog(b *testing.B) {
	count := Counter{}
	c := NewContext(&count)
	c = c.With("hello", "world")
	for i := 0; i < b.N; i++ {
		c.With("type", "dog").Log("name", "bob")
	}
}
func BenchmarkDiscard(b *testing.B) {
	logger := Discard
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		logger.Log("k", "v")
	}
}

func BenchmarkOneWith(b *testing.B) {
	logger := Discard
	lc := NewContext(logger).With("k", "v")
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		lc.Log("k", "v")
	}
}

func BenchmarkTwoWith(b *testing.B) {
	logger := Discard
	lc := NewContext(logger).With("k", "v")
	for i := 1; i < 2; i++ {
		lc = lc.With("k", "v")
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		lc.Log("k", "v")
	}
}

func BenchmarkTenWith(b *testing.B) {
	logger := Discard
	lc := NewContext(logger).With("k", "v")
	for i := 1; i < 10; i++ {
		lc = lc.With("k", "v")
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		lc.Log("k", "v")
	}
}

func TestDisabledLog(t *testing.T) {
	Convey("a nil log", t, func() {
		var logger Logger
		Convey("should be disabled", func() {
			So(IsDisabled(logger), ShouldBeTrue)
		})
	})
	Convey("A log that is disabled", t, func() {
		count := Counter{}
		gate := Gate{
			Logger: &count,
		}
		gate.Disable()
		c := NewContext(&gate)
		Convey("should not log", func() {
			wrapped := c.With("hello", "world")
			c.Log("hello", "world")
			wrapped.Log("do not", "do it")
			c.WithPrefix("hello", "world").Log("do not", "do it")

			So(count.Count, ShouldEqual, 0)
			Convey("until disabled is off", func() {
				gate.Enable()
				c.Log("hi")
				wrapped.Log("hi")
				So(count.Count, ShouldEqual, 2)

				gate.Disable()
				c.Log("hi")
				So(count.Count, ShouldEqual, 2)
			})
		})
	})
}

type lockWriter struct {
	out io.Writer
	mu  sync.Mutex
}

func (l *lockWriter) Write(b []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.out.Write(b)
}

func TestNewChannelLoggerRace(t *testing.T) {
	fullyVerifyLogger(t, func() (Logger, *bytes.Buffer) {
		return NewChannelLogger(0, Discard), nil
	})
}

func TestNewJSONLoggerRace(t *testing.T) {
	fullyVerifyLogger(t, func() (Logger, *bytes.Buffer) {
		b := &bytes.Buffer{}
		l := NewJSONLogger(&lockWriter{out: b}, Discard)
		return l, b
	})
}

func TestNewLogfmtLoggerRace(t *testing.T) {
	fullyVerifyLogger(t, func() (Logger, *bytes.Buffer) {
		b := &bytes.Buffer{}
		l := NewLogfmtLogger(&lockWriter{out: b}, Discard)
		return l, b
	})
}

func TestHierarchyRace(t *testing.T) {
	b := Discard
	fullyVerifyLogger(t, func() (Logger, *bytes.Buffer) {
		return NewHierarchy(b), nil
	})
}

func TestCounterRace(t *testing.T) {
	fullyVerifyLogger(t, func() (Logger, *bytes.Buffer) {
		return &Counter{}, nil
	})
}

func TestMultiRace(t *testing.T) {
	fullyVerifyLogger(t, func() (Logger, *bytes.Buffer) {
		b := &bytes.Buffer{}
		l := NewJSONLogger(&lockWriter{out: b}, Discard)
		return MultiLogger([]Logger{Discard, l}), b
	})
}

func TestRateLimitedLoggerRace(t *testing.T) {
	fullyVerifyLogger(t, func() (Logger, *bytes.Buffer) {
		l := &RateLimitedLogger{
			EventCounter: eventcounter.New(time.Now(), time.Millisecond),
			Limit:        10,
			Logger:       &Counter{},
			Now:          time.Now,
		}
		return l, nil
	})
}

func TestIfErr(t *testing.T) {
	b := &bytes.Buffer{}
	l := NewLogfmtLogger(b, Panic)
	IfErr(l, nil)
	if b.String() != "" {
		t.Error("Expected empty string")
	}
	IfErr(l, errors.New("nope"))
	if b.String() == "" {
		t.Error("Expected error result")
	}
}

func TestChannelLoggerRace(t *testing.T) {
	fullyVerifyLogger(t, func() (Logger, *bytes.Buffer) {
		return NewChannelLogger(10, Discard), nil
	})
}

func fullyVerifyLogger(t *testing.T, factory func() (Logger, *bytes.Buffer)) {
	Convey("When verifying a logger", t, func() {
		Convey("Racing should be ok", func() {
			l, _ := factory()
			raceCheckerIter(l, 3, 10)
		})
		Convey("Odd writes should not panic", func() {
			l, b := factory()
			l.Log("hello")
			if b != nil {
				So(b.String(), ShouldContainSubstring, "hello")
			}
		})
		Convey("Even writes should not panic", func() {
			l, b := factory()
			l.Log("hello", "world")
			if b != nil {
				So(b.String(), ShouldContainSubstring, "hello")
			}
		})
	})

}

func raceCheckerIter(l Logger, deep int, iter int) {
	if deep == 0 {
		return
	}
	l.Log("deep", deep)
	ctx := NewContext(l)
	wg := sync.WaitGroup{}
	wg.Add(iter)
	for i := 0; i < iter; i++ {
		go func(i int) {
			raceCheckerIter(ctx.With(strconv.FormatInt(int64(deep), 10), i), deep-1, iter)
			wg.Done()
		}(i)
	}
	wg.Wait()
}
