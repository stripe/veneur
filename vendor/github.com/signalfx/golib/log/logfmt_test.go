package log

import (
	"bytes"
	"errors"
	. "github.com/smartystreets/goconvey/convey"
	"strings"
	"testing"
)

type errMarshall struct{}

func (e *errMarshall) MarshalText() (text []byte, err error) {
	return nil, &errMarshall{}
}

func (e *errMarshall) Error() string {
	return "I am an error"
}

type errWriter struct{}

func (e *errWriter) Write([]byte) (int, error) {
	return 0, errors.New("nope")
}

func TestNewLogfmtLogger(t *testing.T) {
	Convey("A NewLogfmtLogger logger", t, func() {
		buf := &bytes.Buffer{}
		l := NewLogfmtLogger(buf, Panic)
		Convey("should write messages", func() {
			l.Log("name", "john")
			So(strings.TrimSpace(buf.String()), ShouldResemble, "name=john")
		})
		Convey("should write old len messages", func() {
			l.Log("name")
			So(strings.TrimSpace(buf.String()), ShouldResemble, "msg=name")
		})
		Convey("should write escaped messages", func() {
			l.Log("name", "john doe")
			So(strings.TrimSpace(buf.String()), ShouldResemble, `name="john doe"`)
		})
		Convey("should forward marshall errors", func() {
			So(func() {
				l.Log(&errMarshall{}, &errMarshall{})
			}, ShouldPanic)
		})
		Convey("should forward writer errors", func() {
			l := NewLogfmtLogger(&errWriter{}, Panic)
			So(func() {
				l.Log("hi", "hi")
			}, ShouldPanic)
		})
	})
}
