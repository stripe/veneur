package schedexec

import (
	"time"

	"sync"
	"sync/atomic"

	"github.com/signalfx/golib/errors"
	"github.com/signalfx/golib/timekeeper"
)

//ScheduledExecutor is an abstraction for running things on a schedule
type ScheduledExecutor struct {
	scheduleRate int64
	TimeKeeper   timekeeper.TimeKeeper
	closeChan    chan struct{}
	wg           sync.WaitGroup
}

//NewScheduledExecutor returns a new ScheduledExecutor that will file
func NewScheduledExecutor(scheduleRate time.Duration) *ScheduledExecutor {
	return &ScheduledExecutor{
		scheduleRate: int64(scheduleRate),
		TimeKeeper:   timekeeper.RealTime{},
		closeChan:    make(chan struct{}),
	}
}

//SetScheduleRate sets the new rate at which runs should run for this ScheduledExecutor
func (se *ScheduledExecutor) SetScheduleRate(scheduleRate time.Duration) {
	atomic.StoreInt64(&se.scheduleRate, int64(scheduleRate))
}

// Start starts the execution of a func on the schedule of the ScheduledExecutor. This method blocks until
// the ScheduledExecutor's Close method is called or iterationFunc returns an error
func (se *ScheduledExecutor) Start(iterationFunc func() error) error {
	return se.StartWithMsgChan(iterationFunc, nil)
}

// StartWithMsgChan starts the execution of a func on the schedule of the ScheduledExecutor. This method blocks until
// the ScheduledExecutor's Close method is called or iterationFunc returns an error. msgChan receives the duration of
// how long the ScheduledExecutor will sleep before the start of every iteration.
func (se *ScheduledExecutor) StartWithMsgChan(iterationFunc func() error, msgChan chan time.Duration) error {
	sleepDuration := time.Duration(atomic.LoadInt64(&se.scheduleRate))
	timer := se.TimeKeeper.NewTimer(sleepDuration)
	if msgChan != nil {
		msgChan <- sleepDuration
	}

	se.wg.Add(1)
	defer se.wg.Done()
	for {
		select {
		case <-timer.Chan():
			if err := iterationFunc(); err != nil {
				return errors.Annotate(err, "iteration function failure")
			}
			sleepDuration = time.Duration(atomic.LoadInt64(&se.scheduleRate))
			timer = se.TimeKeeper.NewTimer(sleepDuration)
			if msgChan != nil {
				msgChan <- sleepDuration
			}
		case <-se.closeChan:
			timer.Stop()
			return nil
		}
	}
}

//Close stops the ScheduledExecutor
func (se *ScheduledExecutor) Close() error {
	close(se.closeChan)
	se.wg.Wait()
	return nil
}
