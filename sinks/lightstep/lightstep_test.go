package lightstep

import (
	"net/url"
	"sync"
	"testing"
	"time"

	opentracing "github.com/opentracing/opentracing-go"
	otlog "github.com/opentracing/opentracing-go/log"
	"github.com/sirupsen/logrus"

	"github.com/stretchr/testify/assert"
	"github.com/stripe/veneur/v14"
	"github.com/stripe/veneur/v14/ssf"
	"github.com/stripe/veneur/v14/util"
)

type testLSTracer struct {
	finishedSpans []*testLSSpan
}

func (ft *testLSTracer) StartSpan(operationName string, opts ...opentracing.StartSpanOption) opentracing.Span {
	return &testLSSpan{
		name:   operationName,
		opts:   opts,
		client: ft,
	}
}

func (ft *testLSTracer) Inject(sm opentracing.SpanContext, format interface{}, carrier interface{}) error {
	panic("not implemented")
}

func (ft *testLSTracer) Extract(format interface{}, carrier interface{}) (opentracing.SpanContext, error) {
	panic("not implemented")
}

var _ opentracing.Tracer = &testLSTracer{}

type testLSSpan struct {
	name   string
	tags   map[string]interface{}
	opts   []opentracing.StartSpanOption
	client *testLSTracer
}

func (tls *testLSSpan) Finish() {
	tls.FinishWithOptions(opentracing.FinishOptions{})
}

func (tls *testLSSpan) FinishWithOptions(opts opentracing.FinishOptions) {
	tls.client.finishedSpans = append(tls.client.finishedSpans, tls)
}

func (tls *testLSSpan) Context() opentracing.SpanContext {
	return nil
}

func (tls *testLSSpan) SetOperationName(operationName string) opentracing.Span {
	tls.name = operationName
	return tls
}

func (tls *testLSSpan) SetTag(key string, value interface{}) opentracing.Span {
	if tls.tags == nil {
		tls.tags = make(map[string]interface{})
	}
	tls.tags[key] = value
	return tls
}

func (tls *testLSSpan) LogFields(fields ...otlog.Field) {
	panic("not implemented")
}

func (tls *testLSSpan) LogKV(alternatingKeyValues ...interface{}) {
	panic("not implemented")
}

func (tls *testLSSpan) SetBaggageItem(restrictedKey string, value string) opentracing.Span {
	panic("not implemented")
}

func (tls *testLSSpan) BaggageItem(restrictedKey string) string {
	panic("not implemented")
}

func (tls *testLSSpan) Tracer() opentracing.Tracer {
	return tls.client
}

func (tls *testLSSpan) LogEvent(event string) {
	panic("not implemented")
}

func (tls *testLSSpan) LogEventWithPayload(event string, payload interface{}) {
	panic("not implemented")
}

func (tls *testLSSpan) Log(data opentracing.LogData) {
	panic("not implemented")
}

func TestLSSinkConstructor(t *testing.T) {
	_, err := CreateSpanSink(nil, "lightstep", logrus.NewEntry(logrus.New()),
		veneur.Config{}, LightStepSpanSinkConfig{
			AccessToken: util.StringSecret{Value: "secret"},
			CollectorHost: util.Url{
				Value: &url.URL{
					Scheme: "http",
					Host:   "example.com",
				},
			},
			ReconnectPeriod: 5 * time.Minute,
			MaximumSpans:    1000,
			NumClients:      1,
		})
	assert.NoError(t, err)
}

func TestLSSpanSinkIngest(t *testing.T) {
	tracer := &testLSTracer{}
	ls := &LightStepSpanSink{
		tracers:      []opentracing.Tracer{tracer},
		serviceCount: sync.Map{},
		mutex:        &sync.Mutex{},
	}
	start := time.Now()
	end := start.Add(2 * time.Second)

	testSpan := &ssf.SSFSpan{
		TraceId:        1,
		ParentId:       1,
		Id:             2,
		StartTimestamp: int64(start.UnixNano()),
		EndTimestamp:   int64(end.UnixNano()),
		Error:          false,
		Service:        "farts-srv",
		Tags: map[string]string{
			"baz": "qux",
		},
		Indicator: false,
		Name:      "farting farty farts",
	}
	err := ls.Ingest(testSpan)
	assert.NoError(t, err)

	if assert.Equal(t, 1, len(tracer.finishedSpans)) {
		count, ok := ls.serviceCount.Load("farts-srv")
		assert.True(t, ok, "should have counted")
		assert.EqualValues(t, 1, *count.(*int64))

		span := tracer.finishedSpans[0]
		assert.Equal(t, "farting farty farts", span.name)
		assert.Contains(t, span.tags, "baz")
	}
}
