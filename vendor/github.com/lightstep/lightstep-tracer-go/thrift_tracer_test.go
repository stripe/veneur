package lightstep_test

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"

	. "github.com/lightstep/lightstep-tracer-go"
	"github.com/lightstep/lightstep-tracer-go/lightstep_thrift"
	thriftfakes "github.com/lightstep/lightstep-tracer-go/lightstep_thrift/lightstep_thriftfakes"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	ot "github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/log"
)

var _ = Describe("Thrift Tracer", func() {
	var tracer ot.Tracer
	const port = 9090

	Context("With thrift enabled", func() {
		var latestSpans func() []*lightstep_thrift.SpanRecord
		var fakeClient *thriftfakes.FakeReportingService

		BeforeEach(func() {
			fakeClient = new(thriftfakes.FakeReportingService)
			fakeClient.ReportReturns(&lightstep_thrift.ReportResponse{}, nil)
			latestSpans = func() []*lightstep_thrift.SpanRecord {
				return getReportedThriftSpans(fakeClient)
			}
		})

		AfterEach(func() {
			closeTestTracer(tracer)
		})

		Context("With default options", func() {
			BeforeEach(func() {
				tracer = NewTracer(Options{
					AccessToken:        "0987654321",
					Collector:          Endpoint{"localhost", port, true},
					ReportingPeriod:    1 * time.Millisecond,
					MinReportingPeriod: 1 * time.Millisecond,
					ReportTimeout:      10 * time.Millisecond,
					UseThrift:          true,
					ConnFactory:        fakeThriftConnectionFactory(fakeClient),
				})

				// make sure the fake client is working
				Eventually(fakeClient.ReportCallCount).ShouldNot(BeZero())
			})

			It("Should record baggage info internally", func() {
				span := tracer.StartSpan("x")
				span.SetBaggageItem("x", "y")
				Expect(span.BaggageItem("x")).To(Equal("y"))
			})

			It("Should send span operation names to the collector", func() {
				tracer.StartSpan("smooth").Finish()

				Eventually(latestSpans).Should(HaveLen(1))
				spans := latestSpans()
				Expect(spans[0].GetSpanName()).To(Equal("smooth"))
			})

			It("Should send tags back to the collector", func() {
				span := tracer.StartSpan("brokay doogle")
				span.SetTag("tag", "you're it!")
				span.Finish()

				Eventually(latestSpans).Should(HaveLen(1))
				spans := latestSpans()
				Expect(spans[0].GetAttributes()).To(HaveThriftKeyValues(ThriftKeyValue("tag", "you're it!")))
			})

			// NOTE: Baggage not yet implemented for Thrift
			// It("Should send baggage info to the collector", func() {
			// 	span := tracer.StartSpan("x")
			// 	span.SetBaggageItem("x", "y")
			// 	span.Finish()
			//
			// 	spans := latestSpans()
			// 	Expect(spans).To(HaveLen(1))
			// 	Expect(spans[0].GetSpanContext().GetBaggage()).To(BeEquivalentTo(map[string]string{"x": "y"}))
			// })

			// It("ForeachBaggageItem", func() {
			// 	span := tracer.StartSpan("x")
			// 	span.SetBaggageItem("x", "y")
			// 	baggage := make(map[string]string)
			// 	span.Context().ForeachBaggageItem(func(k, v string) bool {
			// 		baggage[k] = v
			// 		return true
			// 	})
			// 	Expect(baggage).To(BeEquivalentTo(map[string]string{"x": "y"}))
			//
			// 	span.SetBaggageItem("a", "b")
			// 	baggage = make(map[string]string)
			// 	span.Context().ForeachBaggageItem(func(k, v string) bool {
			// 		baggage[k] = v
			// 		return false // exit early
			// 	})
			// 	Expect(baggage).To(HaveLen(1))
			// 	span.Finish()
			//
			// 	spans := latestSpans()
			// 	Expect(spans).To(HaveLen(1))
			//
			// 	// Expect(spans[0].GetSpanContext().GetBaggage()).To(HaveLen(2))
			// })

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
					tracer = NewTracer(Options{
						AccessToken:        "0987654321",
						Collector:          Endpoint{"localhost", port, true},
						ReportingPeriod:    1 * time.Millisecond,
						MinReportingPeriod: 1 * time.Millisecond,
						ReportTimeout:      10 * time.Millisecond,
						UseThrift:          true,
						ConnFactory:        fakeThriftConnectionFactory(fakeClient),
					})
					Eventually(fakeClient.ReportCallCount).ShouldNot(Equal(lastCallCount))
				})
			})

			Describe("Options", func() {
				const expectedTraceID uint64 = 1
				const expectedSpanID uint64 = 2
				const expectedParentSpanID uint64 = 3

				Context("When the TraceID is set", func() {
					BeforeEach(func() {
						tracer.StartSpan("x", SetTraceID(expectedTraceID)).Finish()
					})

					It("Should set the specified options", func() {
						Eventually(latestSpans).Should(HaveLen(1))

						spans := latestSpans()
						Expect(spans[0].GetTraceGuid()).To(Equal(strconv.FormatUint(expectedTraceID, 16)))
						Expect(spans[0].GetSpanGuid()).ToNot(Equal("0"))
						Expect(spans[0].GetAttributes()).To(HaveLen(0))
					})
				})

				Context("When both the TraceID and SpanID are set", func() {
					BeforeEach(func() {
						tracer.StartSpan("x", SetTraceID(expectedTraceID), SetSpanID(expectedSpanID)).Finish()
					})

					It("Should set the specified options", func() {
						Eventually(latestSpans).Should(HaveLen(1))

						spans := latestSpans()
						Expect(spans[0].GetTraceGuid()).To(Equal(strconv.FormatUint(expectedTraceID, 16)))
						Expect(spans[0].GetSpanGuid()).To(Equal(strconv.FormatUint(expectedSpanID, 16)))
						Expect(spans[0].GetAttributes()).To(HaveLen(0))
					})
				})

				Context("When TraceID, SpanID, and ParentSpanID are set", func() {
					BeforeEach(func() {
						tracer.StartSpan("x", SetTraceID(expectedTraceID), SetSpanID(expectedSpanID), SetParentSpanID(expectedParentSpanID)).Finish()
					})

					It("Should set the specified options", func() {
						Eventually(latestSpans).Should(HaveLen(1))

						spans := latestSpans()
						Expect(spans[0].GetTraceGuid()).To(Equal(strconv.FormatUint(expectedTraceID, 16)))
						Expect(spans[0].GetSpanGuid()).To(Equal(strconv.FormatUint(expectedSpanID, 16)))
						Expect(spans[0].GetAttributes()).ToNot(BeEmpty())
						Expect(spans[0].GetAttributes()[0].GetKey()).To(Equal(ParentSpanGUIDKey))
						Expect(spans[0].GetAttributes()[0].GetValue()).To(Equal(strconv.FormatUint(expectedParentSpanID, 16)))
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

					It("Should support infjecting into byte arrays", func() {
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
				tracer = NewTracer(Options{
					AccessToken:        "0987654321",
					Collector:          Endpoint{"localhost", port, true},
					ReportingPeriod:    1 * time.Millisecond,
					MinReportingPeriod: 1 * time.Millisecond,
					ReportTimeout:      10 * time.Millisecond,
					MaxLogKeyLen:       10,
					MaxLogValueLen:     11,
					UseThrift:          true,
					ConnFactory:        fakeThriftConnectionFactory(fakeClient),
				})
				// make sure the fake client is working
				Eventually(fakeClient.ReportCallCount).ShouldNot(BeZero())
			})

			Describe("Logging", func() {
				var span ot.Span
				BeforeEach(func() {
					span = tracer.StartSpan("spantastic")
				})

				It("Should send logs back to the collector", func() {
					span.LogFields(
						log.String("donut", "bacon"),
						log.Object("key", []interface{}{"gr", 8}),
						log.String("donut army"+strings.Repeat("O", 50), strings.Repeat("O", 110)),
						log.Int("life", 42),
					)
					span.Finish()

					obj, _ := json.Marshal([]interface{}{"gr", 8})

					Eventually(latestSpans).Should(HaveLen(1))
					spans := latestSpans()
					Expect(spans[0].GetLogRecords()).To(HaveLen(1))
					Expect(spans[0].GetLogRecords()[0]).To(HaveThriftKeyValues(
						ThriftKeyValue("donut", "bacon"),
						ThriftKeyValue("key", string(obj)),
						ThriftKeyValue("donut arm…", "OOOOOOOOOO…"),
						ThriftKeyValue("life", "42"),
					))
				})
			})
		})

		Context("With custom MaxBufferedSpans", func() {
			BeforeEach(func() {
				tracer = NewTracer(Options{
					AccessToken:        "0987654321",
					Collector:          Endpoint{"localhost", port, true},
					ReportingPeriod:    1 * time.Millisecond,
					MinReportingPeriod: 1 * time.Millisecond,
					ReportTimeout:      10 * time.Millisecond,
					MaxLogKeyLen:       10,
					MaxLogValueLen:     11,
					MaxBufferedSpans:   10,
					UseThrift:          true,
					ConnFactory:        fakeThriftConnectionFactory(fakeClient),
				})

				// make sure the fake client is working
				Eventually(fakeClient.ReportCallCount).ShouldNot(BeZero())
			})

			Describe("SpanBuffer", func() {
				It("should respect MaxBufferedSpans", func() {
					startNSpans(10, tracer)
					Eventually(latestSpans).Should(HaveLen(10))

					startNSpans(10, tracer)
					Eventually(latestSpans).Should(HaveLen(10))
				})
			})
		})

		Context("With DropSpanLogs set", func() {
			BeforeEach(func() {
				tracer = NewTracer(Options{
					AccessToken:        "0987654321",
					Collector:          Endpoint{"localhost", port, true},
					ReportingPeriod:    1 * time.Millisecond,
					MinReportingPeriod: 1 * time.Millisecond,
					ReportTimeout:      10 * time.Millisecond,
					DropSpanLogs:       true,
					UseThrift:          true,
					ConnFactory:        fakeThriftConnectionFactory(fakeClient),
				})

				// make sure the fake client is working
				Eventually(fakeClient.ReportCallCount).ShouldNot(BeZero())
			})

			It("Should not record logs", func() {
				span := tracer.StartSpan("x")
				span.LogFields(log.String("Led", "Zeppelin"), log.Uint32("32bit", 4294967295))
				span.SetTag("tag", "value")
				span.Finish()

				Eventually(latestSpans).Should(HaveLen(1))

				spans := latestSpans()
				Expect(spans[0].GetSpanName()).To(Equal("x"))
				Expect(spans[0].GetAttributes()).To(HaveThriftKeyValues(ThriftKeyValue("tag", "value")))
				Expect(spans[0].GetLogRecords()).To(BeEmpty())
			})
		})

		Context("With MaxLogsPerSpan set", func() {
			BeforeEach(func() {
				tracer = NewTracer(Options{
					AccessToken:        "0987654321",
					Collector:          Endpoint{"localhost", port, true},
					ReportingPeriod:    1 * time.Millisecond,
					MinReportingPeriod: 1 * time.Millisecond,
					ReportTimeout:      10 * time.Millisecond,
					MaxLogsPerSpan:     10,
					UseThrift:          true,
					ConnFactory:        fakeThriftConnectionFactory(fakeClient),
				})

				// make sure the fake client is working
				Eventually(fakeClient.ReportCallCount).ShouldNot(BeZero())
			})

			It("keeps all logs if they don't exceed MaxLogsPerSpan", func() {
				const logCount = 10
				span := tracer.StartSpan("span")
				for i := 0; i < logCount; i++ {
					span.LogKV("id", i)
				}
				span.Finish()

				Eventually(latestSpans).Should(HaveLen(1))

				spans := latestSpans()
				Expect(spans[0].GetSpanName()).To(Equal("span"))
				Expect(spans[0].GetLogRecords()).To(HaveLen(10))
				for i, log := range spans[0].GetLogRecords() {
					Expect(log).To(HaveThriftKeyValues(ThriftKeyValue("id", strconv.FormatInt(int64(i), 10))))
				}
			})

			It("throws away the middle logs when they exceed MaxLogsPerSpan", func() {
				const logCount = 50
				span := tracer.StartSpan("span")
				for i := 0; i < logCount; i++ {
					span.LogKV("id", i)
				}
				span.Finish()

				Eventually(latestSpans).Should(HaveLen(1))

				spans := latestSpans()
				Expect(spans[0].GetSpanName()).To(Equal("span"))
				Expect(spans[0].GetLogRecords()).To(HaveLen(10))

				split := (len(spans[0].GetLogRecords()) - 1) / 2
				firstLogs := spans[0].GetLogRecords()[:split]
				for i, log := range firstLogs {
					Expect(log).To(HaveThriftKeyValues(ThriftKeyValue("id", strconv.FormatInt(int64(i), 10))))
				}

				warnLog := spans[0].GetLogRecords()[split]
				dropped_log_count := int64(logCount - len(spans[0].GetLogRecords()) + 1)
				Expect(warnLog).To(HaveThriftKeyValues(
					ThriftKeyValue("event", "dropped Span logs"),
					ThriftKeyValue("dropped_log_count", strconv.FormatInt(dropped_log_count, 10)),
					ThriftKeyValue("component", "basictracer"),
				))

				lastLogs := spans[0].GetLogRecords()[split+1:]
				for i, log := range lastLogs {
					id := int64(logCount - len(lastLogs) + i)
					Expect(log).To(HaveThriftKeyValues(ThriftKeyValue("id", strconv.FormatInt(id, 10))))
				}
			})
		})
	})
})
