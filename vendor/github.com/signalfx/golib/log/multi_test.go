package log

import (
	. "github.com/smartystreets/goconvey/convey"
	"testing"
)

func TestMultiLogger(t *testing.T) {
	Convey("A mixed multi logger", t, func() {
		c1 := Counter{}
		c2 := Counter{}
		g1 := &Gate{
			Logger: &c1,
		}
		g2 := &Gate{
			Logger: &c2,
		}
		m := MultiLogger([]Logger{g1, g2, Discard})
		Convey("Should call both", func() {
			m.Log()
			So(c1.Count, ShouldEqual, 1)
			So(c2.Count, ShouldEqual, 1)
			So(m.Disabled(), ShouldBeFalse)
		})
		Convey("Should verify disabled", func() {
			g1.Disable()
			g2.Disable()
			So(m.Disabled(), ShouldBeTrue)
		})
	})
}
