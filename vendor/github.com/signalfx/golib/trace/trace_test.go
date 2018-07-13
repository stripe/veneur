package trace

import (
	"context"
	"encoding/json"
	. "github.com/smartystreets/goconvey/convey"
	"sync/atomic"
	"testing"
)

type end struct {
	count int64
}

func (t *end) AddSpans(ctx context.Context, traces []*Span) error {
	atomic.AddInt64(&t.count, 1)
	return nil
}

type middle struct{}

func (t *middle) AddSpans(ctx context.Context, traces []*Span, sink Sink) error {
	return sink.AddSpans(ctx, traces)
}

func Test(t *testing.T) {
	Convey("test middle trace", t, func() {
		nextSink := &middle{}
		next := NextWrap(nextSink)
		So(next, ShouldNotBeNil)
		bottom := &end{}
		top := FromChain(bottom, next)
		So(top, ShouldNotBeNil)
		err := top.AddSpans(context.Background(), []*Span{})
		So(err, ShouldBeNil)
		So(atomic.LoadInt64(&bottom.count), ShouldEqual, int64(1))
	})
}

func TestData(t *testing.T) {
	Convey("test some data", t, func() {
		tests := []struct {
			desc string
			json string
		}{
			{"valid", ValidJSON},
		}
		for _, test := range tests {
			Convey(test.desc, func() {
				var traces Trace
				err := json.Unmarshal([]byte(test.json), &traces)
				if err != nil {
					Println(err)
				}
				So(err, ShouldBeNil)
				_, err = json.Marshal(traces)
				if err != nil {
					Println(err)
				}
				So(err, ShouldBeNil)
			})
		}
	})
}
