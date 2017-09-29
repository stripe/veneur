package dpsink

import (
	"errors"
	"testing"
	"time"

	"github.com/signalfx/golib/datapoint"
	"github.com/signalfx/golib/datapoint/dptest"
	"github.com/signalfx/golib/event"
	"github.com/signalfx/golib/log"
	. "github.com/smartystreets/goconvey/convey"
	"golang.org/x/net/context"
)

func TestRateLimitErrorLogging(t *testing.T) {
	Convey("Rate limited logger", t, func() {
		expectedErr := errors.New("nope")
		end := dptest.NewBasicSink()
		end.RetError(expectedErr)
		ctx := context.Background()
		// This logger will panic if it gets more than one item
		logger := log.NewChannelLogger(1, log.Panic)
		l := RateLimitErrorLogging{
			Logger:      logger,
			LogThrottle: time.Second,
		}
		Convey("Should limit datapoints", func() {
			dp := dptest.DP()
			for i := 0; i < 1000; i++ {
				So(l.AddDatapoints(ctx, []*datapoint.Datapoint{dp}, end), ShouldEqual, expectedErr)
			}
			So(len(logger.Out), ShouldEqual, 1)
			logOut := <-logger.Out
			So(logOut[1].(error), ShouldEqual, expectedErr)
		})
		Convey("Should limit events", func() {
			ev := dptest.E()
			for i := 0; i < 1000; i++ {
				So(l.AddEvents(ctx, []*event.Event{ev}, end), ShouldEqual, expectedErr)
			}
			So(len(logger.Out), ShouldEqual, 1)
			logOut := <-logger.Out
			So(logOut[1].(error), ShouldEqual, expectedErr)
		})
	})
}
