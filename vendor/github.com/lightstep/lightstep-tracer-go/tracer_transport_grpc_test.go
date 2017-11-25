package lightstep_test

import (
	. "github.com/lightstep/lightstep-tracer-go"
	cpb "github.com/lightstep/lightstep-tracer-go/collectorpb"
	cpbfakes "github.com/lightstep/lightstep-tracer-go/collectorpb/collectorpbfakes"
)

type cpbfakesFakeClient struct {
	cpbfakes.FakeCollectorServiceClient
}

func newGrpcFakeClient() fakeCollectorClient {
	fakeClient := new(cpbfakes.FakeCollectorServiceClient)
	fakeClient.ReportReturns(&cpb.ReportResponse{}, nil)
	return &cpbfakesFakeClient{FakeCollectorServiceClient: *fakeClient}
}

func (fakeClient *cpbfakesFakeClient) ConnectorFactory() ConnectorFactory {
	return fakeGrpcConnection(&fakeClient.FakeCollectorServiceClient)
}

func (fakeClient *cpbfakesFakeClient) getSpans() []*cpb.Span {
	return getReportedGRPCSpans(&fakeClient.FakeCollectorServiceClient)
}

func (fakeClient *cpbfakesFakeClient) GetSpansLen() int {
	return len(fakeClient.getSpans())
}
