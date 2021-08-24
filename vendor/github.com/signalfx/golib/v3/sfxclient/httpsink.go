package sfxclient

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode"
	"unsafe"

	"github.com/gogo/protobuf/proto"

	"github.com/mailru/easyjson"
	sfxmodel "github.com/signalfx/com_signalfx_metrics_protobuf/model"
	"github.com/signalfx/golib/v3/datapoint"
	"github.com/signalfx/golib/v3/errors"
	"github.com/signalfx/golib/v3/event"
	"github.com/signalfx/golib/v3/sfxclient/spanfilter"
	"github.com/signalfx/golib/v3/trace"
	traceformat "github.com/signalfx/golib/v3/trace/format"
	"github.com/signalfx/golib/v3/trace/translator"
)

const (
	// ClientVersion is the version of this library and is embedded into the user agent
	ClientVersion = "1.0"

	// IngestEndpointV2 is the v2 version of the signalfx ingest endpoint
	IngestEndpointV2 = "https://ingest.signalfx.com/v2/datapoint"

	// EventIngestEndpointV2 is the v2 version of the signalfx event endpoint
	EventIngestEndpointV2 = "https://ingest.signalfx.com/v2/event"

	// TraceIngestEndpointV1 is the v1 version of the signalfx trace endpoint
	TraceIngestEndpointV1 = "https://ingest.signalfx.com/v1/trace"

	// TraceIngestSAPMEndpointV2 is the of the sapm trace endpoint
	TraceIngestSAPMEndpointV2 = "https://ingest.signalfx.com/v2/trace"

	// DefaultTimeout is the default time to fail signalfx datapoint requests if they don't succeed
	DefaultTimeout = time.Second * 5

	contentTypeHeaderJSON = "application/json"
	contentTypeHeaderSAPM = "application/x-protobuf"
)

// DefaultUserAgent is the UserAgent string sent to signalfx
var DefaultUserAgent = fmt.Sprintf("golib-sfxclient/%s (gover %s)", ClientVersion, runtime.Version())

// HTTPSink -
type HTTPSink struct {
	AuthToken          string
	UserAgent          string
	EventEndpoint      string
	DatapointEndpoint  string
	TraceEndpoint      string
	AdditionalHeaders  map[string]string
	ResponseCallback   func(resp *http.Response, responseBody []byte)
	Client             *http.Client
	protoMarshaler     func(pb proto.Message) ([]byte, error)
	traceMarshal       func(v []*trace.Span) ([]byte, error)
	DisableCompression bool
	zippers            sync.Pool
	contentTypeHeader  string

	stats struct {
		readingBody int64
	}
}

// SFXAPIError is returned when the API returns a status code other than 200.
type SFXAPIError struct {
	StatusCode   int
	ResponseBody string
	Endpoint     string
}

func (se SFXAPIError) Error() string {
	return fmt.Sprintf("invalid status code %d: %s", se.StatusCode, se.ResponseBody)
}

// TooManyRequestError is returned when the API returns HTTP 429 error.
// see https://developer.mozilla.org/en-US/docs/Web/HTTP/Status/429 fot details.
type TooManyRequestError struct {
	// Throttle-Type header returned with the 429 which indicates what the reason.
	// This is a SignalFx-specific header, and the value may be empty.
	ThrottleType string

	// The client should retry after certain intervals.
	// See https://developer.mozilla.org/en-US/docs/Web/HTTP/Headers/Retry-After for details.
	RetryAfter time.Duration

	// wrapped error.
	Err error
}

func (e TooManyRequestError) Error() string {
	return fmt.Sprintf("[%s] too many requests, retry after %.3f seconds",
		e.ThrottleType, e.RetryAfter.Seconds())
}

type responseValidator func(respBody []byte) error

func (h *HTTPSink) handleResponse(resp *http.Response, respValidator responseValidator) (err error) {
	defer func() {
		closeErr := errors.Annotate(resp.Body.Close(), "failed to close response body")
		err = errors.NewMultiErr([]error{err, closeErr})
	}()
	atomic.AddInt64(&h.stats.readingBody, 1)
	respBody, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("cannot fully read response body: %w: %v", err, resp.Header)
	}

	// all 2XXs
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		baseErr := &SFXAPIError{
			StatusCode:   resp.StatusCode,
			ResponseBody: string(respBody),
			Endpoint:     resp.Request.URL.Path,
		}

		if resp.StatusCode == http.StatusTooManyRequests {
			retryAfter, err := parseRetryAfterHeader(resp.Header.Get("Retry-After"))
			if err == nil && retryAfter > 0 {
				return &TooManyRequestError{
					ThrottleType: resp.Header.Get("Throttle-Type"),
					RetryAfter:   retryAfter,
					Err:          baseErr,
				}
			}
		}

		return baseErr
	}
	if h.ResponseCallback != nil {
		h.ResponseCallback(resp, respBody)
	}
	return respValidator(respBody)
}

var _ Sink = &HTTPSink{}

// TokenHeaderName is the header key for the auth token in the HTTP request
const TokenHeaderName = "X-Sf-Token"

func (h *HTTPSink) doBottom(ctx context.Context, f func() (io.Reader, bool, error), contentType, endpoint string,
	respValidator responseValidator) error {
	if ctx.Err() != nil {
		return errors.Annotate(ctx.Err(), "context already closed")
	}
	body, compressed, err := f()
	if err != nil {
		return errors.Annotate(err, "cannot encode datapoints into "+contentType)
	}
	req, err := http.NewRequest("POST", endpoint, body)
	if err != nil {
		return errors.Annotatef(err, "cannot parse new HTTP request to %s", endpoint)
	}
	req = req.WithContext(ctx)
	for k, v := range h.AdditionalHeaders {
		req.Header.Set(k, v)
	}
	h.setHeadersOnBottom(ctx, req, contentType, compressed)
	resp, err := h.Client.Do(req)
	if err != nil {
		// According to docs, resp can be ignored since err is non-nil, so we
		// don't have to close body.
		return fmt.Errorf("failed to send/receive http request: %w: %v", err, req.Header)
	}

	return h.handleResponse(resp, respValidator)
}

type xKeyContextValue string

var (
	// XDebugID Debugs the transaction via signalscope if value matches known secret
	XDebugID xKeyContextValue = "X-Debug-Id"
	// XTracingDebug Sets debug flag on trace if value matches known secret
	XTracingDebug xKeyContextValue = "X-SF-Trace-Token"
	// XTracingID if set accompanies the tracingDebug and gives a client the ability to put a value into a tag on the ingest span
	XTracingID xKeyContextValue = "X-SF-Tracing-ID"
)

func (h *HTTPSink) setTokenHeader(ctx context.Context, req *http.Request) {
	if tok := ctx.Value(TokenHeaderName); tok != nil {
		req.Header.Set(TokenHeaderName, tok.(string))
	} else {
		req.Header.Set(TokenHeaderName, h.AuthToken)
	}
}

func (h *HTTPSink) setHeadersOnBottom(ctx context.Context, req *http.Request, contentType string, compressed bool) {
	// set these below so if someone accidentally uses the same as below we wil override appropriately
	req.Header.Set("Content-Type", contentType)
	h.setTokenHeader(ctx, req)
	req.Header.Set("User-Agent", h.UserAgent)
	req.Header.Set("Connection", "keep-alive")
	if v := ctx.Value(XDebugID); v != nil {
		req.Header.Set(string(XDebugID), v.(string))
	}
	if v := ctx.Value(XTracingDebug); v != nil {
		req.Header.Set(string(XTracingDebug), v.(string))
		if v := ctx.Value(XTracingID); v != nil {
			req.Header.Set(string(XTracingID), v.(string))
		}
	}
	if compressed {
		req.Header.Set("Content-Encoding", "gzip")
	}
}

// AddDatapoints forwards the datapoints to SignalFx.
func (h *HTTPSink) AddDatapoints(ctx context.Context, points []*datapoint.Datapoint) (err error) {
	if len(points) == 0 || h.DatapointEndpoint == "" {
		return nil
	}
	return h.doBottom(ctx, func() (io.Reader, bool, error) {
		return h.encodePostBodyProtobufV2(points)
	}, "application/x-protobuf", h.DatapointEndpoint, datapointAndEventResponseValidator)
}

func datapointAndEventResponseValidator(respBody []byte) error {
	var bodyStr string
	err := json.Unmarshal(respBody, &bodyStr)
	if err != nil {
		return errors.Annotatef(err, "cannot unmarshal response body %s", respBody)
	}
	if bodyStr != "OK" {
		return errors.Errorf("invalid response body %s", bodyStr)
	}
	return nil
}

var toMTMap = map[datapoint.MetricType]sfxmodel.MetricType{
	datapoint.Counter:   sfxmodel.MetricType_CUMULATIVE_COUNTER,
	datapoint.Count:     sfxmodel.MetricType_COUNTER,
	datapoint.Enum:      sfxmodel.MetricType_GAUGE,
	datapoint.Gauge:     sfxmodel.MetricType_GAUGE,
	datapoint.Rate:      sfxmodel.MetricType_GAUGE,
	datapoint.Timestamp: sfxmodel.MetricType_GAUGE,
}

func toMT(mt datapoint.MetricType) sfxmodel.MetricType {
	ret, exists := toMTMap[mt]
	if exists {
		return ret
	}
	panic(fmt.Sprintf("Unknown metric type: %d\n", mt))
}

func toEC(ec event.Category) sfxmodel.EventCategory {
	// Check if the event.Category does not have a corresponding sfxmodel.EventCategory
	if _, ok := sfxmodel.EventCategory_name[int32(ec)]; !ok {
		panic(fmt.Sprintf("Unknown event category: %v\n", ec))
	}
	// Return the sfxmodel.EventCategory
	return sfxmodel.EventCategory(ec)
}

func datumForPoint(pv datapoint.Value) sfxmodel.Datum {
	switch t := pv.(type) {
	case datapoint.IntValue:
		x := t.Int()
		return sfxmodel.Datum{IntValue: &x}
	case datapoint.FloatValue:
		x := t.Float()
		return sfxmodel.Datum{DoubleValue: &x}
	default:
		x := t.String()
		return sfxmodel.Datum{StrValue: &x}
	}
}

func mapToDimensions(dimensions map[string]string) []*sfxmodel.Dimension {
	ret := make([]*sfxmodel.Dimension, 0, len(dimensions))
	for k, v := range dimensions {
		if k == "" || v == "" {
			continue
		}
		ret = append(ret, &sfxmodel.Dimension{
			Key:   filterSignalfxKey(k),
			Value: v,
		})
	}
	return ret
}

func filterSignalfxKey(str string) string {
	return strings.Map(runeFilterMap, str)
}

func runeFilterMap(r rune) rune {
	if unicode.IsDigit(r) || unicode.IsLetter(r) || r == '_' || r == '-' {
		return r
	}
	return '_'
}

func rawToProtobuf(raw interface{}) *sfxmodel.PropertyValue {
	switch t := raw.(type) {
	case int64:
		return &sfxmodel.PropertyValue{
			IntValue: &t,
		}
	case int:
		return &sfxmodel.PropertyValue{
			IntValue: proto.Int64(int64(t)),
		}
	case float64:
		return &sfxmodel.PropertyValue{
			DoubleValue: &t,
		}
	case bool:
		return &sfxmodel.PropertyValue{
			BoolValue: &t,
		}
	case string:
		return &sfxmodel.PropertyValue{
			StrValue: &t,
		}
	}
	return nil
}

func (h *HTTPSink) coreDatapointToProtobuf(point *datapoint.Datapoint) *sfxmodel.DataPoint {
	m := point.Metric
	var ts int64
	if point.Timestamp.IsZero() {
		ts = 0
	} else {
		ts = point.Timestamp.UnixNano() / time.Millisecond.Nanoseconds()
	}
	mt := toMT(point.MetricType)
	dp := &sfxmodel.DataPoint{
		Metric:     m,
		Timestamp:  ts,
		Value:      datumForPoint(point.Value),
		MetricType: &mt,
		Dimensions: mapToDimensions(point.Dimensions),
	}
	return dp
}

// avoid attempting to compress things that fit into a single ethernet frame
func (h *HTTPSink) getReader(b []byte) (io.Reader, bool, error) {
	var err error
	if !h.DisableCompression && len(b) > 1500 {
		buf := new(bytes.Buffer) // TODO use a pool for this too?
		w := h.zippers.Get().(*gzip.Writer)
		defer h.zippers.Put(w)
		w.Reset(buf)
		_, err = w.Write(b)
		if err == nil {
			err = w.Close()
			if err == nil {
				return buf, true, nil
			}
		}
	}
	return bytes.NewReader(b), false, err
}

func (h *HTTPSink) encodePostBodyProtobufV2(datapoints []*datapoint.Datapoint) (io.Reader, bool, error) {
	dps := make([]*sfxmodel.DataPoint, 0, len(datapoints))
	for _, dp := range datapoints {
		dps = append(dps, h.coreDatapointToProtobuf(dp))
	}
	msg := &sfxmodel.DataPointUploadMessage{
		Datapoints: dps,
	}
	body, err := h.protoMarshaler(msg)
	if err != nil {
		return nil, false, errors.Annotate(err, "protobuf marshal failed")
	}
	return h.getReader(body)
}

// AddEvents forwards the events to SignalFx.
func (h *HTTPSink) AddEvents(ctx context.Context, events []*event.Event) (err error) {
	if len(events) == 0 || h.EventEndpoint == "" {
		return nil
	}
	return h.doBottom(ctx, func() (io.Reader, bool, error) {
		return h.encodePostBodyProtobufV2Events(events)
	}, "application/x-protobuf", h.EventEndpoint, datapointAndEventResponseValidator)
}

func (h *HTTPSink) encodePostBodyProtobufV2Events(events []*event.Event) (io.Reader, bool, error) {
	evs := make([]*sfxmodel.Event, 0, len(events))
	for _, ev := range events {
		evs = append(evs, h.coreEventToProtobuf(ev))
	}
	msg := &sfxmodel.EventUploadMessage{
		Events: evs,
	}
	body, err := h.protoMarshaler(msg)
	if err != nil {
		return nil, false, errors.Annotate(err, "protobuf marshal failed")
	}
	return h.getReader(body)
}

func (h *HTTPSink) coreEventToProtobuf(event *event.Event) *sfxmodel.Event {
	var ts int64
	if event.Timestamp.IsZero() {
		ts = 0
	} else {
		ts = event.Timestamp.UnixNano() / time.Millisecond.Nanoseconds()
	}
	etype := event.EventType
	ecat := toEC(event.Category)
	ev := &sfxmodel.Event{
		EventType:  etype,
		Category:   &ecat,
		Dimensions: mapToDimensions(event.Dimensions),
		Properties: mapToProperties(event.Properties),
		Timestamp:  ts,
	}
	return ev
}

func mapToProperties(properties map[string]interface{}) []*sfxmodel.Property {
	response := make([]*sfxmodel.Property, 0, len(properties))
	for k, v := range properties {
		kv := k
		pv := rawToProtobuf(v)
		if pv != nil && k != "" {
			response = append(response, &sfxmodel.Property{
				Key:   kv,
				Value: pv,
			})
		}
	}
	return response
}

const (
	respBodyStrOk = `"OK"`
)

func spanResponseValidator(respBody []byte) error {
	body := string(respBody)
	if body != respBodyStrOk && body != "" {
		return spanfilter.ReturnInvalidOrError(respBody)
	}

	return nil
}

// AddSpans forwards the traces to SignalFx.
func (h *HTTPSink) AddSpans(ctx context.Context, traces []*trace.Span) (err error) {
	if len(traces) == 0 || h.TraceEndpoint == "" {
		return nil
	}

	return h.doBottom(ctx, func() (io.Reader, bool, error) {
		b, err := h.traceMarshal(traces)
		if spanfilter.IsInvalid(err) {
			return nil, false, errors.Annotate(err, "cannot encode traces")
		}
		return h.getReader(b)
	}, h.contentTypeHeader, h.TraceEndpoint, spanResponseValidator)
}

func jsonMarshal(v []*trace.Span) ([]byte, error) {
	// Yeah, i did that.
	y := (*traceformat.Trace)(unsafe.Pointer(&v))
	return easyjson.Marshal(y)
}

func sapmMarshal(v []*trace.Span) ([]byte, error) {
	msg, sm := translator.SFXToSAPMPostRequest(v)
	bb, err := proto.Marshal(msg)
	if err == nil {
		err = sm
	}
	return bb, err
}

func parseRetryAfterHeader(v string) (time.Duration, error) {
	// Retry-After: <http-date>
	// https://developer.mozilla.org/en-US/docs/Web/HTTP/Headers/Date
	if t, err := http.ParseTime(v); err == nil {
		return time.Until(t), nil
	}

	// Retry-After: <delay-seconds>
	i, err := strconv.Atoi(v)
	if err != nil {
		return 0, err
	}
	return time.Duration(i) * time.Second, nil
}

// NewHTTPSink creates a default NewHTTPSink using package level constants as
// defaults, including an empty auth token.  If sending directly to SignalFx, you will be required
// to explicitly set the AuthToken
func NewHTTPSink(opts ...HTTPSinkOption) *HTTPSink {
	s := &HTTPSink{
		EventEndpoint:     EventIngestEndpointV2,
		DatapointEndpoint: IngestEndpointV2,
		TraceEndpoint:     TraceIngestEndpointV1,
		UserAgent:         DefaultUserAgent,
		Client: &http.Client{
			Timeout: DefaultTimeout,
		},
		protoMarshaler: proto.Marshal,
		zippers: sync.Pool{New: func() interface{} {
			return gzip.NewWriter(nil)
		}},
		traceMarshal:      jsonMarshal,
		contentTypeHeader: contentTypeHeaderJSON,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}
