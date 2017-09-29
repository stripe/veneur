package errors

import (
	"bytes"
	"log"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestLogging(t *testing.T) {
	Convey("If logging", t, func() {
		buf := bytes.Buffer{}
		l := log.New(&buf, "", 0)
		Convey("nil should not log", func() {
			var e error
			LogIfErr(e, l, "hello world")
			So(buf.String(), ShouldEqual, "")
		})
		Convey("non nil should log", func() {
			e := New("err string")
			LogIfErr(e, l, "hello world")
			So(buf.String(), ShouldEqual, "err string: hello world\n")
		})
		Convey("non nil should log in a defer", func() {
			f := func() error {
				return New("defer log here!")
			}
			func() {
				defer DeferLogIfErr(f, l, "called")
			}()
			So(buf.String(), ShouldEqual, "defer log here!: called\n")
		})
	})
}

func TestPanics(t *testing.T) {
	Convey("If panic set", t, func() {
		Convey("nil should not panic", func() {
			So(func() {
				PanicIfErr(nil, "hello world")
			}, ShouldNotPanic)
		})
		Convey("non nil should panic", func() {
			So(func() {
				PanicIfErr(New("Err string"), "hello %s", "world")
			}, ShouldPanicWith, "Err string: hello world")
			So(func() {
				PanicIfErrWrite(10, New("Err string"))
			}, ShouldPanicWith, "Write err: Err string")
		})
	})
}
