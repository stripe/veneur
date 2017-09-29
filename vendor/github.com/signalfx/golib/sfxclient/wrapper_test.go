package sfxclient

import (
	"sync/atomic"
	"testing"

	"github.com/signalfx/golib/datapoint"
	. "github.com/smartystreets/goconvey/convey"
	"golang.org/x/net/context"
)

func TestNewMultiCollector(t *testing.T) {
	Convey("a NewMultiCollector", t, func() {
		c1 := GoMetricsSource
		c2 := GoMetricsSource
		Convey("should return itself for one item", func() {
			So(NewMultiCollector(c1), ShouldEqual, c1)
		})
		Convey("should wrap multiple items", func() {
			c3 := NewMultiCollector(c1, c2)
			So(len(c3.Datapoints()), ShouldEqual, 2*len(c1.Datapoints()))
		})
	})
}

func TestWithDimensions(t *testing.T) {
	Convey("a WithDimensions should work", t, func() {
		c1 := GoMetricsSource
		c2 := WithDimensions{
			Collector:  c1,
			Dimensions: map[string]string{"name": "jack"},
		}
		dp0 := c2.Datapoints()[0]
		So(dp0.Dimensions["name"], ShouldEqual, "jack")

		c2.Dimensions = nil
		dp0 = c2.Datapoints()[0]
		So(dp0.Dimensions["name"], ShouldNotEqual, "jack")
	})
}

func ExampleNewMultiCollector() {
	var a Collector
	var b Collector
	c := NewMultiCollector(a, b)
	c.Datapoints()
}

func ExampleCumulativeP() {
	client := NewHTTPSink()
	ctx := context.Background()
	var countThing int64
	go func() {
		atomic.AddInt64(&countThing, 1)
	}()
	if err := client.AddDatapoints(ctx, []*datapoint.Datapoint{
		CumulativeP("server.request_count", nil, &countThing),
	}); err != nil {
		panic("Could not send datapoints")
	}
}

func ExampleWithDimensions() {
	sched := NewScheduler()
	sched.AddCallback(&WithDimensions{
		Collector: GoMetricsSource,
		Dimensions: map[string]string{
			"extra": "dimension",
		},
	})
}
