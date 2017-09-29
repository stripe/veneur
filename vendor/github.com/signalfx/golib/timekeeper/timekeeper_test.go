package timekeeper

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestTimer(t *testing.T) {
	r := RealTime{}
	timer := r.NewTimer(time.Millisecond)
	v := <-timer.Chan()
	assert.NotNil(t, v)
}

func TestStop(t *testing.T) {
	r := RealTime{}
	timer := r.NewTimer(time.Minute)
	assert.True(t, timer.Stop())
	assert.False(t, timer.Stop())
}

func TestAfterClose(t *testing.T) {
	x := time.NewTimer(time.Millisecond * 100)
	x.Stop()
	select {
	case <-x.C:
		panic("NOPE")
	case <-time.After(time.Millisecond * 200):
	}

}

func TestRealTime(t *testing.T) {
	r := RealTime{}
	now := r.Now()
	r.Sleep(time.Millisecond)
	<-r.After(time.Millisecond)
	<-r.NewTimer(time.Millisecond).Chan()
	assert.True(t, time.Now().After(now))

}
