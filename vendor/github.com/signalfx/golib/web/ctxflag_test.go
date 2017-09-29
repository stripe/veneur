package web

import (
	"github.com/signalfx/golib/log"
	. "github.com/smartystreets/goconvey/convey"
	"golang.org/x/net/context"
	"net/http"
	"net/http/httptest"
	"testing"
)

type handleCall struct {
	ctx context.Context
	rw  http.ResponseWriter
	r   *http.Request
}

type handleStack []handleCall

func (h *handleStack) ServeHTTPC(ctx context.Context, rw http.ResponseWriter, r *http.Request) {
	*h = append(*h, handleCall{ctx, rw, r})
}

func TestHeadersInReq(t *testing.T) {
	Convey("When setup,", t, func() {
		h := HeadersInRequest{
			Headers: map[string]string{
				"X-Hello": "world",
			},
		}
		ctx := context.Background()
		r := &Recorder{
			Queue: make(chan Request, 10),
		}
		chain := NewHandler(ctx, r.AsHandler()).Add(&h)
		rw := httptest.NewRecorder()
		chain.ServeHTTPC(ctx, rw, nil)
		So(rw.Header().Get("X-Hello"), ShouldEqual, "world")
	})
}

func TestCtxWithFlag(t *testing.T) {
	Convey("When setup,", t, func() {
		d := log.CtxDimensions{}
		c := CtxWithFlag{
			CtxFlagger: &d,
			HeaderName: "X-Test",
		}
		ctx := context.Background()
		r := &Recorder{
			Queue: make(chan Request, 10),
		}
		chain := NewHandler(ctx, r.AsHandler()).Add(&c)
		rw := httptest.NewRecorder()
		req, err := http.NewRequest("POST", "", nil)
		So(err, ShouldBeNil)
		chain.ServeHTTPC(ctx, rw, req)
		So(rw.Header().Get("X-Test"), ShouldNotEqual, "")
	})
}

func TestRecorder(t *testing.T) {
	Convey("When setup,", t, func() {
		r := &Recorder{
			Queue: make(chan Request, 10),
		}
		Convey("Should add with ServeHTTP,", func() {
			r.ServeHTTP(nil, nil)
			So(len(r.Queue), ShouldEqual, 1)
		})
		Convey("Should call next,", func() {
			i := 0
			c := HandlerFunc(func(ctx context.Context, rw http.ResponseWriter, r *http.Request) {
				i++
			})
			r.ServeHTTPC(context.Background(), nil, nil, c)
			So(len(r.Queue), ShouldEqual, 1)
			So(i, ShouldEqual, 1)
		})
	})
}

func TestHeaderCtxFlag(t *testing.T) {
	Convey("When setup,", t, func() {
		h := HeaderCtxFlag{}
		hs := handleStack([]handleCall{})
		ctx := context.Background()
		Convey("should not setup by default,", func() {
			req, err := http.NewRequest("", "", nil)
			So(err, ShouldBeNil)
			rw := httptest.NewRecorder()
			h.CreateMiddleware(&hs).ServeHTTPC(ctx, rw, req)
			So(len(hs), ShouldEqual, 1)
			So(h.HasFlag(hs[0].ctx), ShouldBeFalse)
		})
		Convey("With headername set,", func() {
			h.HeaderName = "X-Debug"
			Convey("And flag set,", func() {
				h.SetFlagStr("enabled")
				Convey("should fail when not set correctly,", func() {
					req, err := http.NewRequest("", "", nil)
					So(err, ShouldBeNil)
					req.Header.Add(h.HeaderName, "not-enabled")
					rw := httptest.NewRecorder()
					h.ServeHTTPC(ctx, rw, req, &hs)
					So(len(hs), ShouldEqual, 1)
					So(h.HasFlag(hs[0].ctx), ShouldBeFalse)
				})
				Convey("should check headers,", func() {
					req, err := http.NewRequest("", "", nil)
					So(err, ShouldBeNil)
					req.Header.Add(h.HeaderName, "enabled")
					rw := httptest.NewRecorder()
					h.ServeHTTPC(ctx, rw, req, &hs)
					So(len(hs), ShouldEqual, 1)
					So(h.HasFlag(hs[0].ctx), ShouldBeTrue)
				})
				Convey("should check query params,", func() {
					req, err := http.NewRequest("GET", "http://localhost?X-Debug=enabled", nil)
					So(err, ShouldBeNil)
					rw := httptest.NewRecorder()
					h.ServeHTTPC(ctx, rw, req, &hs)
					So(len(hs), ShouldEqual, 1)
					So(h.HasFlag(hs[0].ctx), ShouldBeTrue)
				})
			})

		})
	})

}
