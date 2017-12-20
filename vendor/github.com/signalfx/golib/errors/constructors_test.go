package errors

import (
	"errors"
	"os"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestBaseError(t *testing.T) {
	Convey("When there is a base error", t, func() {
		baseErr := errors.New("base error")
		Convey("It should Annotate and show itself", func() {
			e := Annotate(baseErr, "hello world")
			So(e.Error(), ShouldEqual, "base error")
			So(Tail(e).Error(), ShouldEqual, "base error")
			So(Next(e), ShouldEqual, baseErr)
			So(Details(e), ShouldContainSubstring, "hello world")
		})
		Convey("It should Annotatef and show itself", func() {
			e := Annotatef(baseErr, "hello %s", "world")
			So(e.Error(), ShouldEqual, "base error")
			So(Details(e), ShouldContainSubstring, "hello world")
			So(Next(e), ShouldEqual, baseErr)
			So(Tail(e).Error(), ShouldEqual, "base error")
		})
		Convey("It should wrap OK", func() {
			pathErr := &os.PathError{
				Err: os.ErrNotExist,
			}
			So(Wrap(baseErr, nil), ShouldEqual, baseErr)
			wrappedErr := Wrap(baseErr, pathErr).(*ErrorChain)
			So(Tail(wrappedErr), ShouldEqual, pathErr)
			So(Next(wrappedErr), ShouldEqual, pathErr)
			So(wrappedErr.Error(), ShouldContainSubstring, "file does not exist")
			So(Details(wrappedErr), ShouldContainSubstring, "base error")
			So(Details(wrappedErr), ShouldContainSubstring, "file does not exist")

			So(wrappedErr.Head(), ShouldEqual, baseErr)
			So(wrappedErr.Next(), ShouldEqual, pathErr)
			So(wrappedErr.Tail(), ShouldEqual, pathErr)

			So(Matches(wrappedErr, os.IsNotExist), ShouldBeTrue)
		})
	})
}

func TestNoBaseError(t *testing.T) {
	Convey("When there is no base error", t, func() {
		So(Wrap(nil, nil), ShouldBeNil)
		Convey("New should work", func() {
			e := New("hello world")
			So(e.Error(), ShouldEqual, "hello world")
			So(Details(e), ShouldContainSubstring, "hello world")
			So(Tail(e).Error(), ShouldEqual, "hello world")
			So(Next(e), ShouldEqual, nil)
		})
		Convey("Errorf should work", func() {
			e := Errorf("hello %s", "world")
			So(e.Error(), ShouldEqual, "hello world")
			So(Details(e), ShouldContainSubstring, "hello world")
			So(Tail(e).Error(), ShouldEqual, "hello world")
			So(Next(e), ShouldEqual, nil)
		})
		Convey("Annotates should fail", func() {
			So(Annotate(nil, "hello world"), ShouldBeNil)
			So(Annotatef(nil, "hello world"), ShouldBeNil)
		})
	})
}
