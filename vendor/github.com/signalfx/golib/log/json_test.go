package log

import (
	"bytes"
	"github.com/signalfx/golib/errors"
	. "github.com/smartystreets/goconvey/convey"
	"testing"
)

type panicError struct{}

func (p *panicError) Error() string {
	panic("failure?")
}

type unjsonable struct{}

var errNope = errors.New("nope")

func (u unjsonable) MarshalJSON() ([]byte, error) {
	return nil, errNope
}

func TestJSONLoggerInternal(t *testing.T) {
	Convey("JSON logger to a buffer", t, func() {
		b := &bytes.Buffer{}
		l := NewJSONLogger(b, Panic)
		Convey("should convert non string keys", func() {
			l.Log(5678, 1234)
			So(b.String(), ShouldContainSubstring, "1234")
			So(b.String(), ShouldContainSubstring, "5678")
		})
		Convey("should throw normal panics", func() {
			So(func() {
				l.Log("err", &panicError{})
			}, ShouldPanicWith, "failure?")

		})
		Convey("should forward errors", func() {
			c := NewChannelLogger(1, Panic)
			l := NewJSONLogger(b, c)
			l.Log("type", unjsonable{})
			err := errors.Tail(<-c.Err)
			So(err, ShouldNotBeNil)
		})
	})
}
