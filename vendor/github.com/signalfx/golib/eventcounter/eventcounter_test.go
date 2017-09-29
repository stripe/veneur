package eventcounter

import (
	"testing"
	"time"

	"runtime"
	"sync"

	"github.com/stretchr/testify/assert"
)

func TestEventCounter(t *testing.T) {
	now := time.Now()
	a := New(now, time.Minute)
	assert.Equal(t, int64(1), a.Event(now))
	assert.Equal(t, int64(2), a.Event(now.Add(time.Second)))
	assert.Equal(t, int64(1), a.Event(now.Add(time.Second*60)))
	assert.Equal(t, int64(2), a.Event(now.Add(time.Second*61)))
	assert.Equal(t, int64(3), a.Event(now))
}

func TestEventsCounter(t *testing.T) {
	now := time.Now()
	a := New(now, time.Minute)
	assert.Equal(t, int64(2), a.Events(now, 2))
	assert.Equal(t, int64(4), a.Events(now.Add(time.Second), 2))
	assert.Equal(t, int64(6), a.Events(now.Add(time.Second*59), 2))
	assert.Equal(t, int64(2), a.Events(now.Add(time.Second*60), 2))
	assert.Equal(t, int64(4), a.Events(now, 2))
}

func TestLots(t *testing.T) {
	now := time.Now()
	a := New(now, time.Minute)
	wg := sync.WaitGroup{}
	loopCount := 1000
	for i := 0; i < loopCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			runtime.Gosched()
			for i := 0; i < 59; i++ {
				curtime := now.Add(time.Duration(time.Second.Nanoseconds() * int64(i)))
				a.Event(curtime)
			}
		}()
	}
	wg.Wait()
	assert.Equal(t, int64(loopCount*59), a.Events(now.Add(time.Second*59), 0))
	assert.Equal(t, int64(1), a.Events(now.Add(time.Second*61), 1))
}

func BenchmarkEventCounterSameBucket(b *testing.B) {
	now := time.Now()
	ec := New(now, time.Second)
	for i := 0; i < b.N; i++ {
		ec.Event(now)
	}
}

func BenchmarkEventCounterDifferentBuckets(b *testing.B) {
	now := time.Now()
	ec := New(now, time.Second*2)
	for i := 0; i < b.N; i++ {
		now = now.Add(time.Second)
		ec.Event(now)
	}
}

func BenchmarkEventCounterDifferentBuckets20(b *testing.B) {
	wg := sync.WaitGroup{}
	wg.Add(20)
	for t := 0; t < 20; t++ {
		go func() {
			now := time.Now()
			ec := New(now, time.Second*2)
			for i := 0; i < b.N; i++ {
				now = now.Add(time.Millisecond)
				ec.Event(now)
			}
			wg.Done()
		}()
	}
	wg.Wait()
}
