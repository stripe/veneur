package basictracer

import (
	"reflect"
	"testing"

	opentracing "github.com/opentracing/opentracing-go"
)

const (
	knownCarrier1 = "EigJOjioEaYHBgcRNmifUO7/xlgYASISCgdjaGVja2VkEgdiYWdnYWdl"
	knownCarrier2 = "EigJEX+FpwZ/EmYR2gfYQbxCMskYASISCgdjaGVja2VkEgdiYWdnYWdl"
	badCarrier1   = "Y3QbxCMskYASISCgdjaGVja2VkEgd"
)

var (
	knownContext1 = SpanContext{
		SpanID:  6397081719746291766,
		TraceID: 506100417967962170,
		Baggage: map[string]string{"checked": "baggage"},
	}
	knownContext2 = SpanContext{
		SpanID:  14497723526785009626,
		TraceID: 7355080808006516497,
		Baggage: map[string]string{"checked": "baggage"},
	}
	testContext1 = SpanContext{
		SpanID:  123,
		TraceID: 456,
		Baggage: nil,
	}
	testContext2 = SpanContext{
		SpanID:  123000000000,
		TraceID: 456000000000,
		Baggage: map[string]string{
			"a": "1",
			"b": "2",
			"c": "3",
		},
	}
)

func TestBinaryCarrier(t *testing.T) {
	recorder := NewInMemoryRecorder()
	tracer := NewWithOptions(Options{
		Recorder: recorder,
	})

	checkExtract := func(carrier interface{}, expect opentracing.SpanContext) {
		if ctx, err := tracer.Extract(BinaryCarrier, carrier); err != nil {
			if expect != nil {
				t.Error("extract failed", err)
			}
		} else if !reflect.DeepEqual(expect, ctx) {
			t.Error("extract wrong context", expect, "!=", ctx)
		}
	}

	checkExtract(knownCarrier1, knownContext1)
	checkExtract([]byte(knownCarrier1), knownContext1)
	checkExtract(knownCarrier2, knownContext2)
	checkExtract([]byte(knownCarrier2), knownContext2)
	checkExtract(badCarrier1, nil)
	checkExtract([]byte(badCarrier1), nil)
	checkExtract("", nil)
	checkExtract([]byte(nil), nil)

	checkInject1 := func(orig opentracing.SpanContext, carrier interface{}) {
		if err := tracer.Inject(orig, BinaryCarrier, carrier); err != nil {
			if orig != nil {
				t.Error("inject failed", err)
			}
		}
		if orig == nil {
			return
		}
		if ctx, err := tracer.Extract(BinaryCarrier, carrier); err != nil {
			t.Error("extract of inject failed", carrier, ctx, err)
		} else if !reflect.DeepEqual(ctx, orig) {
			t.Error("extract of inject !=", ctx, orig)
		}
	}

	checkInject := func(ctx opentracing.SpanContext) {
		var cs string
		var cb []byte
		checkInject1(ctx, &cs)
		checkInject1(ctx, &cb)
	}

	checkInject(knownContext1)
	checkInject(knownContext2)
	checkInject(testContext1)
	checkInject(testContext2)
	checkInject(nil)
}
