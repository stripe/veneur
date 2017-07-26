package lightstep

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	google_protobuf "github.com/golang/protobuf/ptypes/timestamp"
	"github.com/lightstep/lightstep-tracer-go/basictracer"
	cpb "github.com/lightstep/lightstep-tracer-go/collectorpb"
	"github.com/lightstep/lightstep-tracer-go/thrift_rpc"
	ot "github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/log"
)

const (
	arbitraryTimestampSecs = 1473442150
)

func makeSpanSlice(length int) []basictracer.RawSpan {
	return make([]basictracer.RawSpan, length)
}

func makeExpectedLogs() []*cpb.Log {
	eRes := make([]*cpb.Log, 8)
	for i := 0; i < 8; i++ {
		pl, _ := json.Marshal([]interface{}{i, i, true, "suhhh"})
		eRes[i] = &cpb.Log{
			Timestamp: &google_protobuf.Timestamp{arbitraryTimestampSecs, 0},
			Keyvalues: []*cpb.KeyValue{
				&cpb.KeyValue{Key: "string", Value: &cpb.KeyValue_StringValue{fmt.Sprintf("foo%d", i)}},
				&cpb.KeyValue{Key: "object", Value: &cpb.KeyValue_JsonValue{string(pl)}},
				&cpb.KeyValue{
					Key:   "too_long-----------…",
					Value: &cpb.KeyValue_StringValue{"---------------------------------------…"},
				},
			},
		}
	}
	return eRes
}

func TestTranslateLogs(t *testing.T) {
	fakeRecorder := Recorder{
		maxLogKeyLen:   20,
		maxLogValueLen: 40,
	}
	ts := time.Unix(arbitraryTimestampSecs, 0)
	otLogs := make([]ot.LogRecord, 8)
	for i := 0; i < 8; i++ {
		otLogs[i] = ot.LogRecord{
			Timestamp: ts,
			Fields: []log.Field{
				log.String("string", fmt.Sprintf("foo%d", i)),
				log.Object("object", []interface{}{i, i, true, "suhhh"}),
				log.String("too_long"+strings.Repeat("-", 50), strings.Repeat("-", 110)),
			},
		}
	}
	res := fakeRecorder.translateLogs(otLogs, nil)
	eRes := makeExpectedLogs()
	if !reflect.DeepEqual(res, eRes) {
		t.Errorf("%v does not equal %v", res, eRes)
	}
}

func TestConvertToKeyValue(t *testing.T) {
	r := Recorder{}
	k := "testing"
	type fakeString string
	type fakeBool bool
	type fakeInt64 int64
	type fakeFloat64 float64
	var a fakeString = "testing"
	kv := r.convertToKeyValue(k, a)
	if kv.GetStringValue() != "testing" {
		t.Errorf("the fakeString value failed to be set")
	}
	var b fakeBool = true
	kv = r.convertToKeyValue(k, b)
	if kv.GetBoolValue() != true {
		t.Errorf("the fakeBool value failed to be set")
	}
	var c fakeInt64 = 3
	kv = r.convertToKeyValue(k, c)
	if kv.GetIntValue() != int64(3) {
		t.Errorf("the fakeInt64 value failed to be set")
	}
	var d fakeFloat64 = 3
	kv = r.convertToKeyValue(k, d)
	if kv.GetDoubleValue() != float64(3) {
		t.Errorf("the fakeFloat64 value failed to be set")
	}
	// make sure these don't panic
	r.convertToKeyValue(k, nil)
	var p *int
	r.convertToKeyValue(k, p)
}

func TestMaxBufferSize(t *testing.T) {
	recorder := NewTracer(Options{
		AccessToken: "0987654321",
		UseGRPC:     true,
	}).(basictracer.Tracer).Options().Recorder.(*Recorder)

	checkCapSize := func(spanLen, spanCap int) {
		recorder.lock.Lock()
		defer recorder.lock.Unlock()

		if cap(recorder.buffer.rawSpans) != spanCap {
			t.Errorf("Unexpected buffer cap: %v != %v", cap(recorder.buffer.rawSpans), spanCap)
		}
		if len(recorder.buffer.rawSpans) != spanLen {
			t.Errorf("Unexpected buffer size: %v != %v", len(recorder.buffer.rawSpans), spanLen)
		}
	}

	checkCapSize(0, defaultMaxSpans)

	spans := makeSpanSlice(defaultMaxSpans)
	for _, span := range spans {
		recorder.RecordSpan(span)
	}

	checkCapSize(defaultMaxSpans, defaultMaxSpans)

	spans = append(spans, makeSpanSlice(defaultMaxSpans)...)
	for _, span := range spans {
		recorder.RecordSpan(span)
	}

	checkCapSize(defaultMaxSpans, defaultMaxSpans)

	maxBuffer := 10
	recorder = NewTracer(Options{
		AccessToken:      "0987654321",
		MaxBufferedSpans: maxBuffer,
		UseGRPC:          true,
	}).(basictracer.Tracer).Options().Recorder.(*Recorder)

	checkCapSize(0, maxBuffer)

	spans = append(spans, makeSpanSlice(100*defaultMaxSpans)...)
	for _, span := range spans {
		recorder.RecordSpan(span)
	}

	checkCapSize(maxBuffer, maxBuffer)

	_ = NewTracer(Options{
		AccessToken: "0987654321",
		UseGRPC:     false,
	}).(basictracer.Tracer).Options().Recorder.(*thrift_rpc.Recorder)
}

func TestDoubleClose(t *testing.T) {
	rec := NewTracer(Options{
		AccessToken: "0987654321",
		UseGRPC:     true,
	}).(basictracer.Tracer).Options().Recorder.(*Recorder)
	rec.Close()
	rec.Close()
}
