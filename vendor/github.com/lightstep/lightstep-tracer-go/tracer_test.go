package lightstep_test

import (
	"time"

	. "github.com/lightstep/lightstep-tracer-go"

	cpb "github.com/lightstep/lightstep-tracer-go/collectorpb"
	cpbfakes "github.com/lightstep/lightstep-tracer-go/collectorpb/collectorpbfakes"
	"github.com/lightstep/lightstep-tracer-go/lightstepfakes"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("SpanRecorder", func() {
	var tracer Tracer

	Context("CloseLightstepTracer", func() {
		var fakeClient *cpbfakes.FakeCollectorServiceClient

		BeforeEach(func() {
			fakeClient = new(cpbfakes.FakeCollectorServiceClient)
			fakeClient.ReportReturns(&cpb.ReportResponse{}, nil)

			tracer = NewTracer(Options{
				AccessToken:        "token",
				ConnFactory:        fakeGrpcConnection(fakeClient),
				MinReportingPeriod: 100 * time.Second,
			})
		})

		It("flushes the buffer before closing", func() {
			tracer.StartSpan("span").Finish()
			err := CloseTracer(tracer)
			Expect(err).NotTo(HaveOccurred())
			Expect(fakeClient.ReportCallCount()).To(Equal(1))
			tracer.StartSpan("span2").Finish()
			Consistently(fakeClient.ReportCallCount).Should(Equal(1))
		})
	})

	Context("When tracer has a SpanRecorder", func() {
		var fakeRecorder *lightstepfakes.FakeSpanRecorder

		BeforeEach(func() {
			fakeRecorder = new(lightstepfakes.FakeSpanRecorder)
			tracer = NewTracer(Options{
				AccessToken: "value",
				ConnFactory: fakeGrpcConnection(new(cpbfakes.FakeCollectorServiceClient)),
				Recorder:    fakeRecorder,
			})
		})

		AfterEach(func() {
			closeTestTracer(tracer)
		})

		It("calls RecordSpan after finishing a span", func() {
			tracer.StartSpan("span").Finish()
			Expect(fakeRecorder.RecordSpanCallCount()).ToNot(BeZero())
		})
	})

	Context("When tracer does not have a SpanRecorder", func() {
		BeforeEach(func() {
			tracer = NewTracer(Options{
				AccessToken: "value",
				ConnFactory: fakeGrpcConnection(new(cpbfakes.FakeCollectorServiceClient)),
				Recorder:    nil,
			})
		})

		AfterEach(func() {
			closeTestTracer(tracer)
		})

		It("doesn't call RecordSpan after finishing a span", func() {
			span := tracer.StartSpan("span")
			Expect(span.Finish).ToNot(Panic())
		})
	})

})
