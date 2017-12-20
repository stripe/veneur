package errors

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestNilMulti(t *testing.T) {
	Convey("For multierr", t, func() {
		Convey("nil should be nil", func() {
			So(NewMultiErr(nil), ShouldBeNil)
			So(NewMultiErr([]error{}), ShouldBeNil)
			So(NewMultiErr([]error{nil, nil}), ShouldBeNil)
		})
		Convey("nil should be filtered", func() {
			e1 := New("e1")
			So(NewMultiErr([]error{e1}), ShouldEqual, e1)
			So(NewMultiErr([]error{e1, nil}), ShouldEqual, e1)
		})
		Convey("multi should work", func() {
			e1 := New("e1")
			e2 := New("e2")
			So(NewMultiErr([]error{e1, e2}).Error(), ShouldContainSubstring, "e1")
			So(NewMultiErr([]error{e1, e2}).Error(), ShouldContainSubstring, "e2")
			So(NewMultiErr([]error{e1, e2, nil}).Error(), ShouldContainSubstring, "e2")
		})
	})
}
