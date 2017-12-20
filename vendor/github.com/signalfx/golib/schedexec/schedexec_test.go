package schedexec

import (
	"testing"
	"time"

	"sync/atomic"

	"github.com/signalfx/golib/errors"
	"github.com/signalfx/golib/log"
	"github.com/signalfx/golib/timekeeper/timekeepertest"
	"github.com/stretchr/testify/assert"
)

type testScheduled struct {
	calledIteratorCount  int32
	runOneIterationError error
}

func (s *testScheduled) runOneIteration() error {
	atomic.AddInt32(&s.calledIteratorCount, 1)
	return s.runOneIterationError
}

func TestNewScheduleExecutor(t *testing.T) {
	scheduled := &testScheduled{}
	se := NewScheduledExecutor(time.Second)
	assert.NotNil(t, se)
	doneChan := make(chan struct{})
	var err error
	go func() {
		err = se.Start(scheduled.runOneIteration)
		close(doneChan)
	}()
	log.IfErr(log.Panic, se.Close())
	<-doneChan
	assert.Nil(t, err)
}

func TestScheduleExecutorTick(t *testing.T) {
	scheduled := &testScheduled{}
	se := NewScheduledExecutor(time.Second)
	stubTime := timekeepertest.NewStubClock(time.Now())
	se.TimeKeeper = stubTime

	doneChan := make(chan struct{})
	msgChan := make(chan time.Duration)
	var err error
	go func() {
		err = se.StartWithMsgChan(scheduled.runOneIteration, msgChan)
		close(doneChan)
	}()

	duration := <-msgChan
	assert.Equal(t, duration, time.Second)
	stubTime.Incr(duration)
	duration = <-msgChan

	assert.Equal(t, int32(1), atomic.LoadInt32(&scheduled.calledIteratorCount))

	scheduled.runOneIterationError = errors.New("MOO")
	assert.Equal(t, duration, time.Second)
	stubTime.Incr(duration)

	<-doneChan
	assert.Equal(t, scheduled.runOneIterationError, errors.Tail(err))

}

func TestScheduleExecutorUpdateScheduleRate(t *testing.T) {
	scheduled := &testScheduled{}
	se := NewScheduledExecutor(time.Second)
	stubTime := timekeepertest.NewStubClock(time.Now())
	se.TimeKeeper = stubTime

	doneChan := make(chan struct{})
	msgChan := make(chan time.Duration)
	var err error
	go func() {
		err = se.StartWithMsgChan(scheduled.runOneIteration, msgChan)
		close(doneChan)
	}()

	duration := <-msgChan
	assert.Equal(t, duration, time.Second)
	// make sure we only update the scheduled rate after the initial timer was created
	se.SetScheduleRate(2 * time.Second)

	stubTime.Incr(duration)
	duration = <-msgChan
	assert.Equal(t, int32(1), atomic.LoadInt32(&scheduled.calledIteratorCount))

	assert.Equal(t, duration, 2*time.Second)

	stubTime.Incr(duration)

	<-msgChan
	assert.Equal(t, int32(2), atomic.LoadInt32(&scheduled.calledIteratorCount))

	log.IfErr(log.Panic, se.Close())
	<-doneChan
	assert.Equal(t, scheduled.runOneIterationError, err)

}
