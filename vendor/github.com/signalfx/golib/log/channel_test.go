package log

import (
	. "github.com/smartystreets/goconvey/convey"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestChannelLogger(t *testing.T) {
	Convey("A channel logger", t, func() {
		c := NewChannelLogger(0, nil)
		Convey("Should block if full", func() {
			shouldPanic := int64(0)
			wg := sync.WaitGroup{}
			wg.Add(1)
			go func() {
				c.Log()
				if atomic.LoadInt64(&shouldPanic) == 0 {
					panic("I didn't block!")
				}
				wg.Done()
			}()
			time.Sleep(time.Millisecond)
			atomic.StoreInt64(&shouldPanic, 1)
			<-c.Out
			wg.Wait()
		})
		Convey("with a full handler", func() {
			count := Counter{}
			c = NewChannelLogger(1, &count)
			Convey("should work if not full", func() {
				c.Log("hi")
				So(count.Count, ShouldEqual, 0)
			})
			Convey("should call handler if full", func() {
				c.Log("hi")
				c.Log("hi")
				So(count.Count, ShouldEqual, 1)
			})
		})
	})
}
