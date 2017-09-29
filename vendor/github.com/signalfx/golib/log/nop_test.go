package log

import "testing"
import . "github.com/smartystreets/goconvey/convey"

func TestNop(t *testing.T) {
	Convey("A nop logger", t, func() {
		c := Discard
		Convey("Should always be disabled", func() {
			So(c.Disabled(), ShouldBeTrue)
		})
		Convey("log should know nothing", func() {
			c.Log("john_snow")
		})
		Convey("Should error handle itself", func() {
			c.ErrorLogger(nil).Log("john_snow")
		})
	})
}
