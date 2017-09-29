package web

import (
	"testing"
	"time"

	"golang.org/x/net/context"

	"github.com/signalfx/golib/timekeeper/timekeepertest"
	. "github.com/smartystreets/goconvey/convey"
)

type limitStubThing time.Duration

func (l *limitStubThing) Get() time.Duration {
	return time.Duration(*l)
}

func TestReqLatencyCounter(t *testing.T) {
	starttime := time.Date(1981, time.March, 19, 6, 0, 0, 0, time.UTC)
	fastRequestLimit := 100 * time.Millisecond
	limitStub := limitStubThing(fastRequestLimit)

	Convey("When setup,", t, func() {
		timeStub := timekeepertest.NewStubClock(starttime)
		counter := NewReqLatencyCounter(&limitStub)
		counter.timeKeeper = timeStub
		ctx := context.Background()
		Convey("having no requests results in 2 stat metrics.", func() {
			stats := counter.Stats(map[string]string{})
			So(stats, ShouldNotBeNil)
			So(len(stats), ShouldEqual, 2)
		})
		Convey("having extra dimensions adds dimensions to metrics", func() {
			stats := counter.Stats(map[string]string{"testDim": "testVal"})
			So(stats, ShouldNotBeNil)
			for _, stat := range stats {
				So(stat.Dimensions, ShouldContainKey, "testDim")
				So(stat.Dimensions["testDim"], ShouldEqual, "testVal")
			}
		})
		Convey("slow increment the slow request count.", func() {
			ctx = AddTime(ctx, starttime)
			timeStub.Incr(fastRequestLimit + 1)
			counter.ModStats(ctx)
			So(counter.slowRequests, ShouldEqual, 1)
			So(counter.fastRequests, ShouldEqual, 0)
		})
		Convey("fast increment the fast request count.", func() {
			ctx = AddTime(ctx, starttime)
			timeStub.Incr(fastRequestLimit - 1)
			counter.ModStats(ctx)
			So(counter.slowRequests, ShouldEqual, 0)
			So(counter.fastRequests, ShouldEqual, 1)
		})
		Convey("having request results in 2 stat metrics.", func() {
			counter.fastRequests++
			counter.slowRequests++
			stats := counter.Stats(map[string]string{})
			So(stats, ShouldNotBeNil)
			So(len(stats), ShouldEqual, 2)
		})
	})

}
