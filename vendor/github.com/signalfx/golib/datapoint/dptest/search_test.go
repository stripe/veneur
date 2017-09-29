package dptest

import (
	"github.com/signalfx/golib/datapoint"
	. "github.com/smartystreets/goconvey/convey"
	"testing"
)

func TestExactlyOne(t *testing.T) {
	Convey("with datapoints", t, func() {
		dp1 := DP()
		dp1.Metric = "a"
		dp2 := DP()
		dp2.Metric = "b"
		dps := []*datapoint.Datapoint{dp1, dp2}
		Convey("should find one", func() {
			So(ExactlyOne(dps, "a"), ShouldNotBeNil)
		})
		Convey("should panic when not found", func() {
			So(func() {
				ExactlyOne(dps, "c")
			}, ShouldPanic)
		})
		Convey("should match dims", func() {
			dp3 := DP()
			dp3.Metric = "a"
			dp3.Dimensions = map[string]string{"name": "bob"}
			dps = append(dps, dp3)
			So(ExactlyOneDims(dps, "a", map[string]string{"name": "bob"}), ShouldNotBeNil)
			So(func() {
				ExactlyOneDims(dps, "a", map[string]string{"name": "bob2"})
			}, ShouldPanic)
			So(func() {
				dp1.Metric = "a"
				dp1.Dimensions = map[string]string{"name": "bob"}
				ExactlyOneDims(dps, "a", map[string]string{"name": "bob"})
			}, ShouldPanic)
		})
	})
}
