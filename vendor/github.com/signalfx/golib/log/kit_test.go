package log

import (
	"github.com/go-kit/kit/log"
	. "github.com/smartystreets/goconvey/convey"
	"io/ioutil"
	"testing"
)

func TestFromGokit(t *testing.T) {
	Convey("Gokit logger", t, func() {
		lkit := log.NewJSONLogger(ioutil.Discard)
		Convey("should be able to convert", func() {
			lgolib := FromGokit(lkit)
			lgolib.Log("a", "b")
			So(lkit.Log("a", "b"), ShouldBeNil)
			Convey("and back", func() {
				backlog := ToGokit(lgolib)
				So(backlog, ShouldEqual, lkit)
				So(backlog.Log("a", "b"), ShouldBeNil)
			})
		})
	})
}

func TestToGokit(t *testing.T) {
	Convey("golib logger", t, func() {
		gkit := Discard
		Convey("should be able to convert", func() {
			gk := ToGokit(gkit)
			So(gk.Log(), ShouldBeNil)
		})
	})
}
