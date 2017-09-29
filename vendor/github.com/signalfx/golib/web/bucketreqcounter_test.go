package web

import (
	"github.com/signalfx/golib/sfxclient"
	"github.com/signalfx/golib/timekeeper/timekeepertest"
	. "github.com/smartystreets/goconvey/convey"
	"net/http"
	"testing"
	"time"
)

func TestBucketRequestCounter(t *testing.T) {
	Convey("With an empty rolling bucket", t, func() {
		r := sfxclient.NewRollingBucket("mname", nil)
		tk := timekeepertest.NewStubClock(time.Now())
		r.Timer = tk
		rq := BucketRequestCounter{
			Bucket: r,
		}
		Convey("Wrap should work", func() {
			wrapped := rq.Wrap(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
			So(wrapped, ShouldNotBeNil)
			wrapped.ServeHTTP(nil, nil)
		})
		Convey("A single request should get the right points", func() {
			rq.ServeHTTP(nil, nil, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
				tk.Incr(time.Second * 2)
			}))
			rq.ServeHTTP(nil, nil, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
				tk.Incr(time.Second)
			}))
			tk.Incr(sfxclient.DefaultBucketWidth)
			dps := rq.Datapoints()
			So(len(dps), ShouldEqual, 1+5+len(r.Quantiles))
			for _, dp := range dps {
				t.Log(dp.String())
			}
		})
	})
}
