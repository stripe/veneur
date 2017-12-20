package timekeepertest

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestSleep(t *testing.T) {
	startTime := time.Now()
	x := NewStubClock(startTime)
	x.Set(startTime)
	done := make(chan struct{})
	go func() {
		timer := x.NewTimer(time.Millisecond * 3)
		timer2 := x.NewTimer(time.Millisecond * 1000)
		go func() {
			<-timer2.Chan()
			panic("Should never happen")
		}()
		<-timer.Chan()
		assert.Equal(t, startTime.Add(time.Millisecond*999), x.Now())
		assert.Equal(t, true, timer2.Stop())
		assert.Equal(t, false, timer2.Stop())
		x.Incr(time.Millisecond * 2)
	}()
	go func() {
		x.Sleep(time.Second)
		assert.Equal(t, startTime.Add(time.Millisecond*1001), x.Now())
		x.Incr(time.Millisecond * 2)
	}()
	go func() {
		x.Sleep(time.Millisecond * 1002)
		assert.Equal(t, startTime.Add(time.Millisecond*1003), x.Now())
		done <- struct{}{}
	}()
	time.Sleep(time.Millisecond * 2)
	x.Incr(time.Millisecond * 999)
	<-done
}
