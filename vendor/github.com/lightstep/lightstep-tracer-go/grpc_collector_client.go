package lightstep

import (
	"fmt"
	"math/rand"
	"reflect"
	"time"

	"golang.org/x/net/context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	// N.B.(jmacd): Do not use google.golang.org/glog in this package.

	google_protobuf "github.com/golang/protobuf/ptypes/timestamp"
	cpb "github.com/lightstep/lightstep-tracer-go/collectorpb"
	ot "github.com/opentracing/opentracing-go"
)

const (
	spansDropped     = "spans.dropped"
	logEncoderErrors = "log_encoder.errors"
)

var (
	intType = reflect.TypeOf(int64(0))
)

// grpcCollectorClient specifies how to send reports back to a LightStep
// collector via grpc.
type grpcCollectorClient struct {
	// auth and runtime information
	attributes map[string]string
	reporterID uint64

	// accessToken is the access token used for explicit trace
	// collection requests.
	accessToken string

	verbose            bool          // whether to print verbose messages
	maxLogKeyLen       int           // see GrpcOptions.MaxLogKeyLen
	maxLogValueLen     int           // see GrpcOptions.MaxLogValueLen
	maxReportingPeriod time.Duration // set by GrpcOptions.MaxReportingPeriod
	reconnectPeriod    time.Duration // set by GrpcOptions.ReconnectPeriod
	reportingTimeout   time.Duration // set by GrpcOptions.ReportTimeout

	// Remote service that will receive reports.
	hostPort      string
	grpcClient    cpb.CollectorServiceClient
	connTimestamp time.Time
	dialOptions   []grpc.DialOption

	// For testing purposes only
	grpcConnectorFactory ConnectorFactory
}

func newGrpcCollectorClient(opts Options, reporterID uint64, attributes map[string]string) *grpcCollectorClient {
	rec := &grpcCollectorClient{
		accessToken:          opts.AccessToken,
		attributes:           attributes,
		maxReportingPeriod:   opts.ReportingPeriod,
		reportingTimeout:     opts.ReportTimeout,
		verbose:              opts.Verbose,
		maxLogKeyLen:         opts.MaxLogKeyLen,
		maxLogValueLen:       opts.MaxLogValueLen,
		reporterID:           reporterID,
		hostPort:             opts.Collector.HostPort(),
		reconnectPeriod:      time.Duration(float64(opts.ReconnectPeriod) * (1 + 0.2*rand.Float64())),
		grpcConnectorFactory: opts.ConnFactory,
	}

	rec.dialOptions = append(rec.dialOptions, grpc.WithDefaultCallOptions(grpc.MaxCallSendMsgSize(opts.GRPCMaxCallSendMsgSizeBytes)))
	if opts.Collector.Plaintext {
		rec.dialOptions = append(rec.dialOptions, grpc.WithInsecure())
	} else {
		rec.dialOptions = append(rec.dialOptions, grpc.WithTransportCredentials(credentials.NewClientTLSFromCert(nil, "")))
	}

	return rec
}

func (client *grpcCollectorClient) ConnectClient() (Connection, error) {
	now := time.Now()
	var conn Connection
	if client.grpcConnectorFactory != nil {
		uncheckedClient, transport, err := client.grpcConnectorFactory()
		if err != nil {
			return nil, err
		}

		grpcClient, ok := uncheckedClient.(cpb.CollectorServiceClient)
		if !ok {
			return nil, fmt.Errorf("Grpc connector factory did not provide valid client!")
		}

		conn = transport
		client.grpcClient = grpcClient
	} else {
		transport, err := grpc.Dial(client.hostPort, client.dialOptions...)
		if err != nil {
			return nil, err
		}

		conn = transport
		client.grpcClient = cpb.NewCollectorServiceClient(transport)
	}
	client.connTimestamp = now
	return conn, nil
}

func (client *grpcCollectorClient) ShouldReconnect() bool {
	return time.Now().Sub(client.connTimestamp) > client.reconnectPeriod
}

func (client *grpcCollectorClient) translateTags(tags ot.Tags) []*cpb.KeyValue {
	kvs := make([]*cpb.KeyValue, 0, len(tags))
	for key, tag := range tags {
		kv := client.convertToKeyValue(key, tag)
		kvs = append(kvs, kv)
	}
	return kvs
}

func (client *grpcCollectorClient) convertToKeyValue(key string, value interface{}) *cpb.KeyValue {
	kv := cpb.KeyValue{Key: key}
	v := reflect.ValueOf(value)
	k := v.Kind()
	switch k {
	case reflect.String:
		kv.Value = &cpb.KeyValue_StringValue{StringValue: v.String()}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64, reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		kv.Value = &cpb.KeyValue_IntValue{IntValue: v.Convert(intType).Int()}
	case reflect.Float32, reflect.Float64:
		kv.Value = &cpb.KeyValue_DoubleValue{DoubleValue: v.Float()}
	case reflect.Bool:
		kv.Value = &cpb.KeyValue_BoolValue{BoolValue: v.Bool()}
	default:
		kv.Value = &cpb.KeyValue_StringValue{StringValue: fmt.Sprint(v)}
		maybeLogInfof("value: %v, %T, is an unsupported type, and has been converted to string", client.verbose, v, v)
	}
	return &kv
}

func (client *grpcCollectorClient) translateLogs(lrs []ot.LogRecord, buffer *reportBuffer) []*cpb.Log {
	logs := make([]*cpb.Log, len(lrs))
	for i, lr := range lrs {
		logs[i] = &cpb.Log{
			Timestamp: translateTime(lr.Timestamp),
		}
		marshalFields(client, logs[i], lr.Fields, buffer)
	}
	return logs
}

func (client *grpcCollectorClient) translateRawSpan(rs RawSpan, buffer *reportBuffer) *cpb.Span {
	s := &cpb.Span{
		SpanContext:    translateSpanContext(rs.Context),
		OperationName:  rs.Operation,
		References:     translateParentSpanID(rs.ParentSpanID),
		StartTimestamp: translateTime(rs.Start),
		DurationMicros: translateDuration(rs.Duration),
		Tags:           client.translateTags(rs.Tags),
		Logs:           client.translateLogs(rs.Logs, buffer),
	}
	return s
}

func (client *grpcCollectorClient) convertRawSpans(buffer *reportBuffer) []*cpb.Span {
	spans := make([]*cpb.Span, len(buffer.rawSpans))
	for i, rs := range buffer.rawSpans {
		s := client.translateRawSpan(rs, buffer)
		spans[i] = s
	}
	return spans
}

func (client *grpcCollectorClient) makeReportRequest(buffer *reportBuffer) *cpb.ReportRequest {
	spans := client.convertRawSpans(buffer)
	reporter := convertToReporter(client.attributes, client.reporterID)

	req := cpb.ReportRequest{
		Reporter:        reporter,
		Auth:            &cpb.Auth{AccessToken: client.accessToken},
		Spans:           spans,
		InternalMetrics: convertToInternalMetrics(buffer),
	}
	return &req

}

func (client *grpcCollectorClient) Report(ctx context.Context, buffer *reportBuffer) (collectorResponse, error) {
	resp, err := client.grpcClient.Report(ctx, client.makeReportRequest(buffer))
	if err != nil {
		return nil, err
	}

	return resp, nil
}

func translateAttributes(atts map[string]string) []*cpb.KeyValue {
	tags := make([]*cpb.KeyValue, 0, len(atts))
	for k, v := range atts {
		tags = append(tags, &cpb.KeyValue{Key: k, Value: &cpb.KeyValue_StringValue{StringValue: v}})
	}
	return tags
}

func convertToReporter(atts map[string]string, id uint64) *cpb.Reporter {
	return &cpb.Reporter{
		ReporterId: id,
		Tags:       translateAttributes(atts),
	}
}

func generateMetricsSample(b *reportBuffer) []*cpb.MetricsSample {
	return []*cpb.MetricsSample{
		&cpb.MetricsSample{
			Name:  spansDropped,
			Value: &cpb.MetricsSample_IntValue{IntValue: b.droppedSpanCount},
		},
		&cpb.MetricsSample{
			Name:  logEncoderErrors,
			Value: &cpb.MetricsSample_IntValue{IntValue: b.logEncoderErrorCount},
		},
	}
}

func convertToInternalMetrics(b *reportBuffer) *cpb.InternalMetrics {
	return &cpb.InternalMetrics{
		StartTimestamp: translateTime(b.reportStart),
		DurationMicros: translateDurationFromOldestYoungest(b.reportStart, b.reportEnd),
		Counts:         generateMetricsSample(b),
	}
}

func translateSpanContext(sc SpanContext) *cpb.SpanContext {
	return &cpb.SpanContext{
		TraceId: sc.TraceID,
		SpanId:  sc.SpanID,
		Baggage: sc.Baggage,
	}
}

func translateParentSpanID(pid uint64) []*cpb.Reference {
	if pid == 0 {
		return nil
	}
	return []*cpb.Reference{
		&cpb.Reference{
			Relationship: cpb.Reference_CHILD_OF,
			SpanContext:  &cpb.SpanContext{SpanId: pid},
		},
	}
}

func translateTime(t time.Time) *google_protobuf.Timestamp {
	return &google_protobuf.Timestamp{
		Seconds: t.Unix(),
		Nanos:   int32(t.Nanosecond()),
	}
}

func translateDuration(d time.Duration) uint64 {
	return uint64(d) / 1000
}

func translateDurationFromOldestYoungest(ot time.Time, yt time.Time) uint64 {
	return translateDuration(yt.Sub(ot))
}
