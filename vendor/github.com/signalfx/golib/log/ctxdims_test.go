package log

import (
	"bytes"
	. "github.com/smartystreets/goconvey/convey"
	"golang.org/x/net/context"
	"testing"
)

func TestCtxDims(t *testing.T) {
	Convey("default ctx dims", t, func() {
		cd := &CtxDimensions{}
		ctx := context.Background()
		Convey("default get should be empty list", func() {
			So(len(cd.From(ctx)), ShouldEqual, 0)
		})
		Convey("Odd append should panic", func() {
			So(func() {
				cd.Append(ctx, "ood_num")
			}, ShouldPanic)
		})
		Convey("empty append should do nothing", func() {
			So(cd.Append(ctx), ShouldEqual, ctx)
		})
		Convey("And a default append", func() {
			ctx2 := cd.Append(ctx, "name", "john")
			So(len(cd.From(ctx2)), ShouldEqual, 2)
			So(len(cd.From(ctx)), ShouldEqual, 0)
			Convey("With should append dims", func() {
				buf := &bytes.Buffer{}
				l := NewLogfmtLogger(buf, Panic)
				cd.With(ctx2, l).Log()
				So(buf.String(), ShouldContainSubstring, "john")
			})
		})
	})
}
