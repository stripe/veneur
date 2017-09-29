package sfxclient

import (
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"fmt"

	"github.com/signalfx/golib/datapoint"
	"github.com/signalfx/golib/errors"
	"github.com/signalfx/golib/timekeeper/timekeepertest"
	. "github.com/smartystreets/goconvey/convey"
	"golang.org/x/net/context"
)

type testSink struct {
	retErr         error
	lastDatapoints chan []*datapoint.Datapoint
}

func (t *testSink) AddDatapoints(ctx context.Context, points []*datapoint.Datapoint) (err error) {
	t.lastDatapoints <- points
	return t.retErr
}

func TestNewScheduler(t *testing.T) {
	Convey("Default error handler should not panic", t, func() {
		So(func() { errors.PanicIfErr(DefaultErrorHandler(errors.New("test")), "unexpected") }, ShouldNotPanic)
	})

	Convey("with a testing scheduler", t, func() {
		s := NewScheduler()

		tk := timekeepertest.NewStubClock(time.Now())
		s.Timer = tk

		sink := &testSink{
			lastDatapoints: make(chan []*datapoint.Datapoint, 1),
		}
		s.Sink = sink
		s.ReportingDelay(time.Second)

		var handledErrors []error
		var handleErrRet error
		s.ErrorHandler = func(e error) error {
			handledErrors = append(handledErrors, e)
			return errors.Wrap(handleErrRet, e)
		}

		ctx := context.Background()

		Convey("removing a callback that doesn't exist should work", func() {
			s.RemoveCallback(nil)
			s.RemoveGroupedCallback("_", nil)
		})
		Convey("CollectorFunc should work", func() {
			c := 0
			cf := CollectorFunc(func() []*datapoint.Datapoint {
				c++
				return []*datapoint.Datapoint{}
			})
			So(len(cf.Datapoints()), ShouldEqual, 0)
			So(c, ShouldEqual, 1)
			s.AddCallback(cf)
		})

		Convey("callbacks should be removable", func() {
			s.AddCallback(GoMetricsSource)
			So(s.ReportOnce(ctx), ShouldBeNil)
			firstPoints := <-sink.lastDatapoints
			So(len(firstPoints), ShouldEqual, 30)
			s.RemoveCallback(GoMetricsSource)
			So(s.ReportOnce(ctx), ShouldBeNil)
			firstPoints = <-sink.lastDatapoints
			So(len(firstPoints), ShouldEqual, 0)
		})

		Convey("a single report should work", func() {
			So(s.ReportOnce(ctx), ShouldBeNil)
			firstPoints := <-sink.lastDatapoints
			So(len(firstPoints), ShouldEqual, 0)
			So(len(sink.lastDatapoints), ShouldEqual, 0)
		})
		Convey("with a single callback", func() {
			dimsToSend := map[string]string{"type": "test"}
			collector := CollectorFunc(func() []*datapoint.Datapoint {
				return []*datapoint.Datapoint{
					Gauge("mname", dimsToSend, 1),
				}
			})
			s.AddCallback(collector)
			Convey("empty dims should be sendable", func() {
				dimsToSend = nil
				So(s.ReportOnce(ctx), ShouldBeNil)
				firstPoints := <-sink.lastDatapoints
				So(len(firstPoints), ShouldEqual, 1)
				So(firstPoints[0].Dimensions, ShouldResemble, map[string]string{})
				Convey("so should only defaults", func() {
					s.DefaultDimensions(map[string]string{"host": "bob"})
					So(s.ReportOnce(ctx), ShouldBeNil)
					firstPoints := <-sink.lastDatapoints
					So(len(firstPoints), ShouldEqual, 1)
					So(firstPoints[0].Dimensions, ShouldResemble, map[string]string{"host": "bob"})
				})
			})
			Convey("default dimensions should set", func() {
				s.DefaultDimensions(map[string]string{"host": "bob"})
				s.GroupedDefaultDimensions("_", map[string]string{"host": "bob2"})
				So(s.ReportOnce(ctx), ShouldBeNil)
				firstPoints := <-sink.lastDatapoints
				So(len(firstPoints), ShouldEqual, 1)
				So(firstPoints[0].Dimensions, ShouldResemble, map[string]string{"host": "bob", "type": "test"})
				So(firstPoints[0].Metric, ShouldEqual, "mname")
				So(firstPoints[0].MetricType, ShouldEqual, datapoint.Gauge)
				So(firstPoints[0].Timestamp.UnixNano(), ShouldEqual, tk.Now().UnixNano())
				Convey("and var should return previous points", func() {
					So(s.Var().String(), ShouldContainSubstring, "bob")
				})
			})
			Convey("zero time should be sendable", func() {
				s.SendZeroTime = true
				So(s.ReportOnce(ctx), ShouldBeNil)
				firstPoints := <-sink.lastDatapoints
				So(len(firstPoints), ShouldEqual, 1)
				So(firstPoints[0].Timestamp.IsZero(), ShouldBeTrue)
			})
			Convey("and made to error out", func() {
				sink.retErr = errors.New("nope bad done")
				handleErrRet = errors.New("handle error is bad")
				Convey("scheduled should end", func() {
					scheduleOver := int64(0)
					go func() {
						for atomic.LoadInt64(&scheduleOver) == 0 {
							tk.Incr(time.Duration(s.ReportingDelayNs))
							runtime.Gosched()
						}
					}()
					err := s.Schedule(ctx)
					atomic.StoreInt64(&scheduleOver, 1)
					So(err.Error(), ShouldEqual, "nope bad done")
					So(errors.Details(err), ShouldContainSubstring, "handle error is bad")
				})
			})
			Convey("and scheduled", func() {
				scheduledContext, cancelFunc := context.WithCancel(ctx)
				scheduleRetErr := make(chan error)
				go func() {
					scheduleRetErr <- s.Schedule(scheduledContext)
				}()
				Convey("should collect when time advances", func() {
					for len(sink.lastDatapoints) == 0 {
						tk.Incr(time.Duration(s.ReportingDelayNs))
						runtime.Gosched()
					}
					firstPoints := <-sink.lastDatapoints
					So(len(firstPoints), ShouldEqual, 1)
					Convey("and should skip an interval if we sleep too long", func() {
						// Should eventually end
						for atomic.LoadInt64(&s.stats.resetIntervalCounts) == 0 {
							// Keep skipping time and draining outstanding points until we skip an interval
							tk.Incr(time.Duration(s.ReportingDelayNs) * 3)
							for len(sink.lastDatapoints) > 0 {
								<-sink.lastDatapoints
							}
							runtime.Gosched()
						}
					})
				})
				Reset(func() {
					cancelFunc()
					<-scheduleRetErr
				})
			})
			Reset(func() {
				s.RemoveCallback(collector)
			})
		})

		Reset(func() {
			close(sink.lastDatapoints)
		})
	})
}

func ExampleScheduler() {
	s := NewScheduler()
	s.Sink.(*HTTPSink).AuthToken = "ABCD-XYZ"
	s.AddCallback(GoMetricsSource)
	bucket := NewRollingBucket("req.time", map[string]string{"env": "test"})
	s.AddCallback(bucket)
	bucket.Add(1.2)
	bucket.Add(3)
	ctx := context.Background()
	err := s.Schedule(ctx)
	fmt.Println("Schedule result: ", err)
}
