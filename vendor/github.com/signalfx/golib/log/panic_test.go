package log

import (
	"errors"
	. "github.com/smartystreets/goconvey/convey"
	"testing"
)

func TestPanicLogger(t *testing.T) {
	Convey("panic logger", t, func() {
		l := Panic
		Convey("should error to itself", func() {
			So(l.ErrorLogger(nil), ShouldHaveSameTypeAs, l)
			e1 := errors.New("nope")
			So(func() {
				l.ErrorLogger(e1).Log("hello")
			}, ShouldPanic)
		})
		Convey("should panic", func() {
			So(func() {
				l.Log()
			}, ShouldPanic)
		})
	})
}
