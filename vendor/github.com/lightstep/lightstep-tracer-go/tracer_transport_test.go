package lightstep_test

import (
	"encoding/json"
	"strings"
	"time"

	. "github.com/lightstep/lightstep-tracer-go"
	"github.com/lightstep/lightstep-tracer-go/collectorpb"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	ot "github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/log"
	"strconv"
)

// Interfaces

type Reference interface {
	GetSpanContext() SpanContext
}

type Span interface {
	GetOperationName() string

	GetSpanContext() SpanContext

	GetTags() interface{}

	GetReferences() interface{}

	GetReference(i int) Reference

	GetLogs() []interface{}
}

type fakeCollectorClient interface {
	ConnectorFactory() ConnectorFactory

	ReportCallCount() int

	GetSpansLen() int

	GetSpan(i int) Span
}

// Describe Test

var _ = Describe("Tracer Transports", func() {
	var options *Options
	var tracer ot.Tracer
	var fakeClient fakeCollectorClient
	const port = 9090

	BeforeEach(func() {
		options = &Options{}
	})

	JustBeforeEach(func() {
		options.ConnFactory = fakeClient.ConnectorFactory()
		tracer = NewTracer(*options)
		// make sure the fake client is working
		Eventually(fakeClient.ReportCallCount).ShouldNot(BeZero())
	})

	AfterEach(func() {
		closeTestTracer(tracer)
	})

	ItShouldBehaveLikeATracer := func(tracerTestOptions ...testOption) {
		testOptions := toTestOptions(tracerTestOptions)
		Context("with default options", func() {
			BeforeEach(func() {
				options.AccessToken = "0987654321"
				options.Collector = Endpoint{"localhost", port, true}
				options.ReportingPeriod = 1 * time.Millisecond
				options.MinReportingPeriod = 1 * time.Millisecond
				options.ReportTimeout = 10 * time.Millisecond
			})

			It("Should record baggage info internally", func() {
				span := tracer.StartSpan("x")
				span.SetBaggageItem("x", "y")
				Expect(span.BaggageItem("x")).To(Equal("y"))
			})

			It("Should send span operation names to the collector", func() {
				tracer.StartSpan("smooth").Finish()

				Eventually(fakeClient.GetSpansLen).Should(Equal(1))
				Expect(fakeClient.GetSpan(0).GetOperationName()).To(Equal("smooth"))
			})

			It("Should send tags back to the collector", func() {
				span := tracer.StartSpan("brokay doogle")
				span.SetTag("tag", "you're it!")
				span.Finish()

				Eventually(fakeClient.GetSpansLen).Should(Equal(1))
				Expect(fakeClient.GetSpan(0).GetTags()).To(HaveKeyValues(KeyValue("tag", "you're it!")))
			})

			if testOptions.supportsBaggage {
				It("Should send baggage info to the collector", func() {
					span := tracer.StartSpan("x")
					span.SetBaggageItem("x", "y")
					span.Finish()

					Eventually(fakeClient.GetSpansLen).Should(Equal(1))
					Expect(fakeClient.GetSpan(0).GetSpanContext().Baggage).To(BeEquivalentTo(map[string]string{"x": "y"}))
				})

				It("ForeachBaggageItem", func() {
					span := tracer.StartSpan("x")
					span.SetBaggageItem("x", "y")
					baggage := map[string]string{}
					span.Context().ForeachBaggageItem(func(k, v string) bool {
						baggage[k] = v
						return true
					})
					Expect(baggage).To(BeEquivalentTo(map[string]string{"x": "y"}))

					span.SetBaggageItem("a", "b")
					baggage = map[string]string{}
					span.Context().ForeachBaggageItem(func(k, v string) bool {
						baggage[k] = v
						return false // exit early
					})
					Expect(baggage).To(HaveLen(1))
					span.Finish()

					Eventually(fakeClient.GetSpansLen).Should(Equal(1))
					Expect(fakeClient.GetSpan(0).GetSpanContext().Baggage).To(HaveLen(2))
				})
			}

			Describe("CloseTracer", func() {
				It("Should not explode when called twice", func() {
					closeTestTracer(tracer)
					closeTestTracer(tracer)
				})

				It("Should behave nicely", func() {
					By("Not hanging")
					closeTestTracer(tracer)

					By("Stop communication with server")
					lastCallCount := fakeClient.ReportCallCount()
					Consistently(fakeClient.ReportCallCount, 0.5, 0.05).Should(Equal(lastCallCount))

					By("Allowing other tracers to reconnect to the server")
					tracer = NewTracer(*options)
					Eventually(fakeClient.ReportCallCount).ShouldNot(Equal(lastCallCount))
				})
			})

			Describe("Options", func() {
				const expectedTraceID uint64 = 1
				const expectedSpanID uint64 = 2
				const expectedParentSpanID uint64 = 3

				Context("when the TraceID is set", func() {
					JustBeforeEach(func() {
						tracer.StartSpan("x", SetTraceID(expectedTraceID)).Finish()
					})

					It("should set the specified options", func() {
						Eventually(fakeClient.GetSpansLen).Should(Equal(1))
						Expect(fakeClient.GetSpan(0).GetSpanContext().TraceID).To(Equal(expectedTraceID))
						Expect(fakeClient.GetSpan(0).GetSpanContext().SpanID).ToNot(Equal(uint64(0)))
						if testOptions.supportsReference {
							Expect(fakeClient.GetSpan(0).GetReferences()).To(BeEmpty())
						}
					})
				})

				Context("when both the TraceID and SpanID are set", func() {
					JustBeforeEach(func() {
						tracer.StartSpan("x", SetTraceID(expectedTraceID), SetSpanID(expectedSpanID)).Finish()
					})

					It("Should set the specified options", func() {
						Eventually(fakeClient.GetSpansLen).Should(Equal(1))
						Expect(fakeClient.GetSpan(0).GetSpanContext().TraceID).To(Equal(expectedTraceID))
						Expect(fakeClient.GetSpan(0).GetSpanContext().SpanID).To(Equal(expectedSpanID))
						if testOptions.supportsReference {
							Expect(fakeClient.GetSpan(0).GetReferences()).To(BeEmpty())
						}
					})
				})

				Context("when TraceID, SpanID, and ParentSpanID are set", func() {
					JustBeforeEach(func() {
						tracer.StartSpan("x", SetTraceID(expectedTraceID), SetSpanID(expectedSpanID), SetParentSpanID(expectedParentSpanID)).Finish()
					})

					It("Should set the specified options", func() {
						Eventually(fakeClient.GetSpansLen).Should(Equal(1))
						Expect(fakeClient.GetSpan(0).GetSpanContext().TraceID).To(Equal(expectedTraceID))
						Expect(fakeClient.GetSpan(0).GetSpanContext().SpanID).To(Equal(expectedSpanID))
						if testOptions.supportsReference {
							Expect(fakeClient.GetSpan(0).GetReferences()).ToNot(BeEmpty())
							Expect(fakeClient.GetSpan(0).GetReference(0).GetSpanContext().SpanID).To(Equal(expectedParentSpanID))
						}
					})
				})
			})

			Describe("Binary Carriers", func() {
				const knownCarrier1 = "EigJOjioEaYHBgcRNmifUO7/xlgYASISCgdjaGVja2VkEgdiYWdnYWdl"
				const knownCarrier2 = "EigJEX+FpwZ/EmYR2gfYQbxCMskYASISCgdjaGVja2VkEgdiYWdnYWdl"
				const badCarrier1 = "Y3QbxCMskYASISCgdjaGVja2VkEgd"

				var knownContext1 = SpanContext{
					SpanID:  6397081719746291766,
					TraceID: 506100417967962170,
					Baggage: map[string]string{"checked": "baggage"},
				}
				var knownContext2 = SpanContext{
					SpanID:  14497723526785009626,
					TraceID: 7355080808006516497,
					Baggage: map[string]string{"checked": "baggage"},
				}
				var testContext1 = SpanContext{
					SpanID:  123,
					TraceID: 456,
					Baggage: nil,
				}
				var testContext2 = SpanContext{
					SpanID:  123000000000,
					TraceID: 456000000000,
					Baggage: map[string]string{"a": "1", "b": "2", "c": "3"},
				}

				Context("tracer inject", func() {
					var carrierString string
					var carrierBytes []byte

					BeforeEach(func() {
						carrierString = ""
						carrierBytes = []byte{}
					})

					It("Should support injecting into strings ", func() {
						for _, origContext := range []SpanContext{knownContext1, knownContext2, testContext1, testContext2} {
							err := tracer.Inject(origContext, BinaryCarrier, &carrierString)
							Expect(err).ToNot(HaveOccurred())

							context, err := tracer.Extract(BinaryCarrier, carrierString)
							Expect(err).ToNot(HaveOccurred())
							Expect(context).To(BeEquivalentTo(origContext))
						}
					})

					It("Should support injecting into byte arrays", func() {
						for _, origContext := range []SpanContext{knownContext1, knownContext2, testContext1, testContext2} {
							err := tracer.Inject(origContext, BinaryCarrier, &carrierBytes)
							Expect(err).ToNot(HaveOccurred())

							context, err := tracer.Extract(BinaryCarrier, carrierBytes)
							Expect(err).ToNot(HaveOccurred())
							Expect(context).To(BeEquivalentTo(origContext))
						}
					})

					It("Should return nil for nil contexts", func() {
						err := tracer.Inject(nil, BinaryCarrier, carrierString)
						Expect(err).To(HaveOccurred())

						err = tracer.Inject(nil, BinaryCarrier, carrierBytes)
						Expect(err).To(HaveOccurred())
					})
				})

				Context("tracer extract", func() {
					It("Should extract SpanContext from carrier as string", func() {
						context, err := tracer.Extract(BinaryCarrier, knownCarrier1)
						Expect(context).To(BeEquivalentTo(knownContext1))
						Expect(err).To(BeNil())

						context, err = tracer.Extract(BinaryCarrier, knownCarrier2)
						Expect(context).To(BeEquivalentTo(knownContext2))
						Expect(err).To(BeNil())
					})

					It("Should extract SpanContext from carrier as []byte", func() {
						context, err := tracer.Extract(BinaryCarrier, []byte(knownCarrier1))
						Expect(context).To(BeEquivalentTo(knownContext1))
						Expect(err).To(BeNil())

						context, err = tracer.Extract(BinaryCarrier, []byte(knownCarrier2))
						Expect(context).To(BeEquivalentTo(knownContext2))
						Expect(err).To(BeNil())
					})

					It("Should return nil for bad carriers", func() {
						for _, carrier := range []interface{}{badCarrier1, []byte(badCarrier1), "", []byte(nil)} {
							context, err := tracer.Extract(BinaryCarrier, carrier)
							Expect(context).To(BeNil())
							Expect(err).To(HaveOccurred())
						}
					})
				})
			})
		})

		Context("With custom log length", func() {
			BeforeEach(func() {
				options.AccessToken = "0987654321"
				options.Collector = Endpoint{"localhost", port, true}
				options.ReportingPeriod = 1 * time.Millisecond
				options.MinReportingPeriod = 1 * time.Millisecond
				options.ReportTimeout = 10 * time.Millisecond
				options.MaxLogKeyLen = 10
				options.MaxLogValueLen = 11
			})

			Describe("Logging", func() {
				JustBeforeEach(func() {
					span := tracer.StartSpan("spantastic")
					span.LogFields(
						log.String("donut", "bacon"),
						log.Object("key", []interface{}{"gr", 8}),
						log.String("donut army"+strings.Repeat("O", 50), strings.Repeat("O", 110)),
						log.Int("life", 42),
					)
					span.Finish()
				})

				It("Should send logs back to the collector", func() {
					Eventually(fakeClient.GetSpansLen).Should(Equal(1))

					obj, _ := json.Marshal([]interface{}{"gr", 8})

					expectedKeyValues := []*collectorpb.KeyValue{KeyValue("donut", "bacon")}

					if testOptions.supportsTypedValues {
						expectedKeyValues = append(expectedKeyValues,
							KeyValue("key", string(obj), true),
							KeyValue("donut arm…", "OOOOOOOOOO…"),
							KeyValue("life", 42),
						)
					} else {
						expectedKeyValues = append(expectedKeyValues,
							KeyValue("key", string(obj)),
							KeyValue("donut arm…", "OOOOOOOOOO…"),
							KeyValue("life", "42"),
						)
					}

					Expect(fakeClient.GetSpan(0).GetLogs()).To(HaveLen(1))
					Expect(fakeClient.GetSpan(0).GetLogs()[0]).To(HaveKeyValues(expectedKeyValues...))
				})
			})
		})

		Context("With custom MaxBufferedSpans", func() {
			BeforeEach(func() {
				options.AccessToken = "0987654321"
				options.Collector = Endpoint{"localhost", port, true}
				options.ReportingPeriod = 1 * time.Millisecond
				options.MinReportingPeriod = 1 * time.Millisecond
				options.ReportTimeout = 10 * time.Millisecond
				options.MaxLogKeyLen = 10
				options.MaxLogValueLen = 11
				options.MaxBufferedSpans = 10
			})

			Describe("SpanBuffer", func() {
				It("should respect MaxBufferedSpans", func() {
					startNSpans(10, tracer)
					Eventually(fakeClient.GetSpansLen).Should(Equal(10))

					startNSpans(10, tracer)
					Eventually(fakeClient.GetSpansLen).Should(Equal(10))
				})
			})
		})

		Context("With DropSpanLogs set", func() {
			BeforeEach(func() {
				options.AccessToken = "0987654321"
				options.Collector = Endpoint{"localhost", port, true}
				options.ReportingPeriod = 1 * time.Millisecond
				options.MinReportingPeriod = 1 * time.Millisecond
				options.ReportTimeout = 10 * time.Millisecond
				options.DropSpanLogs = true
			})

			It("Should not record logs", func() {
				span := tracer.StartSpan("x")
				span.LogFields(log.String("Led", "Zeppelin"), log.Uint32("32bit", 4294967295))
				span.SetTag("tag", "value")
				span.Finish()

				Eventually(fakeClient.GetSpansLen).Should(Equal(1))
				Expect(fakeClient.GetSpan(0).GetOperationName()).To(Equal("x"))
				Expect(fakeClient.GetSpan(0).GetTags()).To(HaveKeyValues(KeyValue("tag", "value")))
				Expect(fakeClient.GetSpan(0).GetLogs()).To(BeEmpty())
			})
		})

		Context("With MaxLogsPerSpan set", func() {
			BeforeEach(func() {
				options.AccessToken = "0987654321"
				options.Collector = Endpoint{"localhost", port, true}
				options.ReportingPeriod = 1 * time.Millisecond
				options.MinReportingPeriod = 1 * time.Millisecond
				options.ReportTimeout = 10 * time.Millisecond
				options.MaxLogsPerSpan = 10
			})

			It("keeps all logs if they don't exceed MaxLogsPerSpan", func() {
				const logCount = 10
				span := tracer.StartSpan("span")
				for i := 0; i < logCount; i++ {
					span.LogKV("id", i)
				}
				span.Finish()

				Eventually(fakeClient.GetSpansLen).Should(Equal(1))
				Expect(fakeClient.GetSpan(0).GetOperationName()).To(Equal("span"))
				Expect(fakeClient.GetSpan(0).GetLogs()).To(HaveLen(10))

				for i, log := range fakeClient.GetSpan(0).GetLogs() {
					if testOptions.supportsTypedValues {
						Expect(log).To(HaveKeyValues(KeyValue("id", i)))
					} else {
						Expect(log).To(HaveKeyValues(KeyValue("id", strconv.FormatInt(int64(i), 10))))
					}
				}
			})

			It("throws away the middle logs when they exceed MaxLogsPerSpan", func() {
				const logCount = 50
				span := tracer.StartSpan("span")
				for i := 0; i < logCount; i++ {
					span.LogKV("id", i)
				}
				span.Finish()

				Eventually(fakeClient.GetSpansLen).Should(Equal(1))
				Expect(fakeClient.GetSpan(0).GetOperationName()).To(Equal("span"))
				Expect(fakeClient.GetSpan(0).GetLogs()).To(HaveLen(10))

				split := (len(fakeClient.GetSpan(0).GetLogs()) - 1) / 2
				firstLogs := fakeClient.GetSpan(0).GetLogs()[:split]
				for i, log := range firstLogs {
					if testOptions.supportsTypedValues {
						Expect(log).To(HaveKeyValues(KeyValue("id", i)))
					} else {
						Expect(log).To(HaveKeyValues(KeyValue("id", strconv.FormatInt(int64(i), 10))))
					}
				}

				warnLog := fakeClient.GetSpan(0).GetLogs()[split]

				expectedKeyValues := []*collectorpb.KeyValue{
					KeyValue("event", "dropped Span logs"),
				}

				if testOptions.supportsTypedValues {
					expectedKeyValues = append(expectedKeyValues,
						KeyValue("dropped_log_count", logCount-len(fakeClient.GetSpan(0).GetLogs())+1),
					)
				} else {
					expectedKeyValues = append(expectedKeyValues,
						KeyValue("dropped_log_count", strconv.FormatInt(int64(logCount-len(fakeClient.GetSpan(0).GetLogs())+1), 10)),
					)
				}

				expectedKeyValues = append(expectedKeyValues, KeyValue("component", "basictracer"))

				Expect(warnLog).To(HaveKeyValues(expectedKeyValues...))

				lastLogs := fakeClient.GetSpan(0).GetLogs()[split+1:]
				for i, log := range lastLogs {
					if testOptions.supportsTypedValues {
						Expect(log).To(HaveKeyValues(KeyValue("id", logCount-len(lastLogs)+i)))
					} else {
						Expect(log).To(HaveKeyValues(KeyValue("id", strconv.FormatInt(int64(logCount-len(lastLogs)+i), 10))))
					}
				}
			})
		})
	}

	Context("with http enabled", func() {
		BeforeEach(func() {
			options.UseHttp = true
		})

		// TODO(dolan) - Add http tests.
	})

	Context("with grpc enabled", func() {
		BeforeEach(func() {
			options.UseGRPC = true
			fakeClient = newGrpcFakeClient()
		})

		ItShouldBehaveLikeATracer(
			thatSupportsBaggage(),
			thatSupportsReference(),
			thatSupportsTypedValues(),
		)
	})

	Context("with thrift enabled", func() {
		BeforeEach(func() {
			options.UseThrift = true
			fakeClient = newThriftFakeClient()
		})

		ItShouldBehaveLikeATracer()
	})
})
