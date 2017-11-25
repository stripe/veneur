package lightstep_test

import (
	"context"
	"fmt"
	"reflect"

	. "github.com/lightstep/lightstep-tracer-go"
	ot "github.com/opentracing/opentracing-go"

	cpb "github.com/lightstep/lightstep-tracer-go/collectorpb"
	cpbfakes "github.com/lightstep/lightstep-tracer-go/collectorpb/collectorpbfakes"

	"github.com/lightstep/lightstep-tracer-go/lightstep_thrift"
	thriftfakes "github.com/lightstep/lightstep-tracer-go/lightstep_thrift/lightstep_thriftfakes"

	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
)

func closeTestTracer(tracer ot.Tracer) {
	complete := make(chan struct{})
	go func() {
		Close(context.Background(), tracer)
		close(complete)
	}()
	Eventually(complete).Should(BeClosed())
}

func startNSpans(n int, tracer ot.Tracer) {
	for i := 0; i < n; i++ {
		tracer.StartSpan(string(i)).Finish()
	}
}

type haveKeyValuesMatcher []*cpb.KeyValue

func HaveKeyValues(keyValues ...*cpb.KeyValue) types.GomegaMatcher {
	return haveKeyValuesMatcher(keyValues)
}

func (matcher haveKeyValuesMatcher) Match(actual interface{}) (bool, error) {
	switch v := actual.(type) {
	case []*cpb.KeyValue:
		return matcher.MatchProtos(v)
	case *cpb.Log:
		return matcher.MatchProtos(v.GetFields())
	case []*lightstep_thrift.KeyValue:
		return matcher.MatchThrift(v)
	case *lightstep_thrift.LogRecord:
		return matcher.MatchThrift(v.GetFields())
	default:
		return false, fmt.Errorf("HaveKeyValues matcher expects either a []*KeyValue or a *Log/*LogRecord")
	}
}

func (matcher haveKeyValuesMatcher) MatchProtos(actualKeyValues []*cpb.KeyValue) (bool, error) {
	expectedKeyValues := []*cpb.KeyValue(matcher)
	if len(expectedKeyValues) != len(actualKeyValues) {
		return false, nil
	}

	for i := range actualKeyValues {
		if !reflect.DeepEqual(actualKeyValues[i], expectedKeyValues[i]) {
			return false, nil
		}
	}

	return true, nil
}

func (matcher haveKeyValuesMatcher) MatchThrift(actualKeyValues []*lightstep_thrift.KeyValue) (bool, error) {
	expectedKeyValues := []*cpb.KeyValue(matcher)
	if len(expectedKeyValues) != len(actualKeyValues) {
		return false, nil
	}

	for i := range actualKeyValues {
		if !reflect.DeepEqual(actualKeyValues[i].Key, expectedKeyValues[i].Key) {
			fmt.Println("KEY NOT EQUAL")
			return false, nil
		}

		if len(expectedKeyValues[i].GetStringValue()) == 0 {
			fmt.Println("NO STRING VALUE")
			return false, nil
		}

		if !reflect.DeepEqual(actualKeyValues[i].Value, expectedKeyValues[i].GetStringValue()) {
			fmt.Println("VALUE NOT EQUAL")
			return false, nil
		}
	}

	return true, nil
}

func (matcher haveKeyValuesMatcher) FailureMessage(actual interface{}) string {
	return fmt.Sprintf("Expected '%v' to have key values '%v'", actual, matcher)
}

func (matcher haveKeyValuesMatcher) NegatedFailureMessage(actual interface{}) string {
	return fmt.Sprintf("Expected '%v' to not have key values '%v'", actual, matcher)
}

func KeyValue(key string, value interface{}, storeAsJson ...bool) *cpb.KeyValue {
	tag := &cpb.KeyValue{Key: key}
	switch typedValue := value.(type) {
	case int:
		tag.Value = &cpb.KeyValue_IntValue{int64(typedValue)}
	case string:
		if len(storeAsJson) > 0 && storeAsJson[0] {
			tag.Value = &cpb.KeyValue_JsonValue{typedValue}
		} else {
			tag.Value = &cpb.KeyValue_StringValue{typedValue}
		}
	case bool:
		tag.Value = &cpb.KeyValue_BoolValue{typedValue}
	case float32:
		tag.Value = &cpb.KeyValue_DoubleValue{float64(typedValue)}
	case float64:
		tag.Value = &cpb.KeyValue_DoubleValue{typedValue}
	}
	return tag
}

//////////////////
// GRPC HELPERS //
//////////////////

func getReportedGRPCSpans(fakeClient *cpbfakes.FakeCollectorServiceClient) []*cpb.Span {
	callCount := fakeClient.ReportCallCount()
	spans := make([]*cpb.Span, 0)
	for i := 0; i < callCount; i++ {
		_, report, _ := fakeClient.ReportArgsForCall(i)
		spans = append(spans, report.GetSpans()...)
	}
	return spans
}

type dummyConnection struct{}

func (*dummyConnection) Close() error { return nil }

func fakeGrpcConnection(fakeClient *cpbfakes.FakeCollectorServiceClient) ConnectorFactory {
	return func() (interface{}, Connection, error) {
		return fakeClient, new(dummyConnection), nil
	}
}

////////////////////
// THRIFT HELPERS //
////////////////////

func getReportedThriftSpans(fakeClient *thriftfakes.FakeReportingService) []*lightstep_thrift.SpanRecord {
	callCount := fakeClient.ReportCallCount()
	spans := make([]*lightstep_thrift.SpanRecord, 0)
	for i := 0; i < callCount; i++ {
		_, report := fakeClient.ReportArgsForCall(i)
		spans = append(spans, report.GetSpanRecords()...)
	}
	return spans
}

func fakeThriftConnectionFactory(fakeClient lightstep_thrift.ReportingService) ConnectorFactory {
	return func() (interface{}, Connection, error) {
		return fakeClient, new(dummyConnection), nil
	}
}
