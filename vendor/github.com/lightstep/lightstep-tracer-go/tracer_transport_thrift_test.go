package lightstep_test

import (
	. "github.com/lightstep/lightstep-tracer-go"
	"github.com/lightstep/lightstep-tracer-go/lightstep_thrift"
	thriftfakes "github.com/lightstep/lightstep-tracer-go/lightstep_thrift/lightstep_thriftfakes"
	"strconv"
)

type thriftFakeClient struct {
	thriftfakes.FakeReportingService
}

type thriftSpan struct {
	lightstep_thrift.SpanRecord
}

type thriftReference struct {
	lightstep_thrift.TraceJoinId
}

func (fakeClient *thriftFakeClient) ConnectorFactory() ConnectorFactory {
	return fakeThriftConnectionFactory(&fakeClient.FakeReportingService)
}

func (fakeClient *thriftFakeClient) getSpans() []*lightstep_thrift.SpanRecord {
	return getReportedThriftSpans(&fakeClient.FakeReportingService)
}

func (fakeClient *thriftFakeClient) GetSpansLen() int {
	return len(fakeClient.getSpans())
}

func (fakeClient *thriftFakeClient) GetSpan(i int) Span {
	return &thriftSpan{
		SpanRecord: *fakeClient.getSpans()[i],
	}
}

func (span *thriftSpan) GetOperationName() string {
	return *span.SpanRecord.SpanName
}

func (span *thriftSpan) GetSpanContext() SpanContext {
	return toThriftSpanContext(&span.SpanRecord)
}

func (span *thriftSpan) GetTags() interface{} {
	return span.SpanRecord.GetAttributes()
}

func (span *thriftSpan) GetReferences() interface{} {
	return span.SpanRecord.GetJoinIds()
}

func (span *thriftSpan) GetReference(i int) Reference {
	return &thriftReference{
		TraceJoinId: *span.SpanRecord.GetJoinIds()[i],
	}
}

func (reference *thriftReference) GetSpanContext() SpanContext {
	return SpanContext{}
	// return toThriftSpanContext(reference.TraceJoinId.TraceKey)
}

func (span *thriftSpan) GetLogs() []interface{} {
	logs := make([]interface{}, 0, len(span.SpanRecord.GetLogRecords()))
	for _, log := range span.SpanRecord.GetLogRecords() {
		logs = append(logs, log)
	}
	return logs
}

func toThriftSpanContext(record *lightstep_thrift.SpanRecord) SpanContext {
	traceId, _ := strconv.ParseUint(*record.TraceGuid, 16, 64)
	spanId, _ := strconv.ParseUint(*record.SpanGuid, 16, 64)

	return SpanContext{
		TraceID: traceId,
		SpanID:  spanId,
	}
}

func newThriftFakeClient() fakeCollectorClient {
	fakeClient := new(thriftfakes.FakeReportingService)
	fakeClient.ReportReturns(&lightstep_thrift.ReportResponse{}, nil)
	return &thriftFakeClient{FakeReportingService: *fakeClient}
}
