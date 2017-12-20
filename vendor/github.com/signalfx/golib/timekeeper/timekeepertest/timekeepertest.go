package timekeepertest

import (
	"container/heap"
	"runtime"
	"sync"
	"time"

	"github.com/signalfx/golib/timekeeper"
)

type chanTime struct {
	ch          chan time.Time
	triggerTime time.Time
}

type triggerHeap []*chanTime

func (t triggerHeap) Len() int {
	return len(t)
}

func (t triggerHeap) Less(i, j int) bool {
	return t[i].triggerTime.Before(t[j].triggerTime)
}

func (t triggerHeap) Swap(i, j int) {
	t[i], t[j] = t[j], t[i]
}

func (t *triggerHeap) Push(x interface{}) {
	*t = append(*t, x.(*chanTime))
}

func (t *triggerHeap) Pop() interface{} {
	old := *t
	n := len(old)
	x := old[n-1]
	*t = old[0 : n-1]
	return x
}

// StubClock is a way to stub the progress of time
type StubClock struct {
	currentTime  time.Time
	waitingChans triggerHeap

	mu sync.Mutex
}

// NewStubClock creates a new stubbable clock
func NewStubClock(now time.Time) *StubClock {
	return &StubClock{
		currentTime:  now,
		waitingChans: triggerHeap([]*chanTime{}),
	}
}

var _ timekeeper.TimeKeeper = &StubClock{}

// Now simulates time.Now and returns the current time of the stubbed clock
func (s *StubClock) Now() time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.currentTime
}

// Sleep pauses and returns when the clock has advanced dur duration
func (s *StubClock) Sleep(dur time.Duration) {
	<-s.After(dur)
}

// Incr the stored time by dur, causing waiting channels to send
func (s *StubClock) Incr(dur time.Duration) {
	s.mu.Lock()
	s.currentTime = s.currentTime.Add(dur)
	s.mu.Unlock()
	s.triggerChans()
	runtime.Gosched()
}

// Set the time to t
func (s *StubClock) Set(t time.Time) {
	s.mu.Lock()
	s.currentTime = t
	s.mu.Unlock()
	s.triggerChans()
	runtime.Gosched()
}

func (s *StubClock) triggerChans() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for len(s.waitingChans) > 0 {
		firstItem := s.waitingChans[0]
		if !firstItem.triggerTime.After(s.currentTime) {
			i := heap.Pop(&s.waitingChans).(*chanTime)
			i.ch <- i.triggerTime
			runtime.Gosched()
		} else {
			break
		}
	}
}

// After stubs time.After
func (s *StubClock) After(d time.Duration) <-chan time.Time {
	ch := make(chan time.Time, 1)
	s.mu.Lock()
	defer s.mu.Unlock()
	heap.Push(&s.waitingChans, &chanTime{
		ch:          ch,
		triggerTime: s.currentTime.Add(d),
	})
	return ch
}

func (s *StubClock) removeChan(ch <-chan time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := 0; i < len(s.waitingChans); i++ {
		if s.waitingChans[i].ch == ch {
			heap.Remove(&s.waitingChans, i)
			return
		}
	}
}

// NewTimer stubs time.NewTimer
func (s *StubClock) NewTimer(d time.Duration) timekeeper.Timer {
	ch := s.After(d)
	return &simpleTimer{
		ch:     ch,
		parent: s,
	}
}

type simpleTimer struct {
	ch      <-chan time.Time
	stopped bool
	parent  *StubClock
}

func (s *simpleTimer) Chan() <-chan time.Time {
	return s.ch
}

func (s *simpleTimer) Stop() bool {
	if s.stopped {
		return false
	}
	s.stopped = true
	s.parent.removeChan(s.ch)
	return true
}
