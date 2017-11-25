package lightstep_test

import (
	"context"

	lightstep "github.com/lightstep/lightstep-tracer-go"
	cpb "github.com/lightstep/lightstep-tracer-go/collectorpb"
	cpbfakes "github.com/lightstep/lightstep-tracer-go/collectorpb/collectorpbfakes"
	"github.com/lightstep/lightstep-tracer-go/lightstepfakes"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	opentracing "github.com/opentracing/opentracing-go"
)

var _ = Describe("Tracerv0_14", func() {
	var tracer lightstep.Tracerv0_14
	var opts lightstep.Options

	const accessToken = "ACCESS_TOKEN"

	var fakeClient *cpbfakes.FakeCollectorServiceClient
	var fakeConn lightstep.ConnectorFactory

	var fakeRecorder *lightstepfakes.FakeSpanRecorder

	var eventHandler lightstep.EventHandler
	var eventChan <-chan lightstep.Event
	const eventBufferSize = 10

	BeforeEach(func() {
		fakeClient = new(cpbfakes.FakeCollectorServiceClient)
		fakeClient.ReportReturns(&cpb.ReportResponse{}, nil)
		fakeConn = fakeGrpcConnection(fakeClient)
		fakeRecorder = new(lightstepfakes.FakeSpanRecorder)

		eventHandler, eventChan = lightstep.NewEventChannel(eventBufferSize)
		lightstep.SetGlobalEventHandler(eventHandler)
	})

	JustBeforeEach(func() {
		opts = lightstep.Options{
			AccessToken: accessToken,
			ConnFactory: fakeConn,
		}
		tracer = lightstep.NewTracerv0_14(opts)
	})

	AfterEach(func() {
		closeTestTracer(tracer)
	})

	Describe("Helper functions", func() {
		It("GetLightStepAccessToken returns the access token", func() {
			Expect(lightstep.GetLightStepAccessToken(tracer)).To(Equal(accessToken))
		})

		It("Close closes the tracer", func() {
			lightstep.Close(context.Background(), tracer)
		})

		It("CloseTracer closes the tracer", func() {
			err := lightstep.CloseTracer(tracer)
			Expect(err).ToNot(HaveOccurred())
		})

		It("Flush flushes the tracer", func() {
			lightstep.Flush(context.Background(), tracer)
		})

		It("FlushLightStepTracer flushes the tracer", func() {
			err := lightstep.FlushLightStepTracer(tracer)
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Describe("provides its ReporterID", func() {
		It("is non-zero", func() {
			rid, err := lightstep.GetLightStepReporterID(opentracing.Tracer(tracer))
			Expect(err).To(BeNil())
			Expect(rid).To(Not(BeZero()))
		})
	})
})
