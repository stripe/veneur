package kafka

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/Shopify/sarama"
	"github.com/Shopify/sarama/mocks"
	"github.com/gogo/protobuf/proto"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stripe/veneur/v14"
	"github.com/stripe/veneur/v14/samplers"
	"github.com/stripe/veneur/v14/ssf"
	"github.com/stripe/veneur/v14/trace"
)

func TestMetricFlush(t *testing.T) {
	config := sarama.NewConfig()
	config.Producer.Return.Successes = true
	producerMock := mocks.NewAsyncProducer(t, config)

	producerMock.ExpectInputAndSucceed()

	sink, err := CreateMetricSink(
		&veneur.Server{
			TraceClient: nil,
		},
		"kafka",
		logrus.NewEntry(logrus.StandardLogger()),
		veneur.Config{},
		KafkaMetricSinkConfig{
			Broker:               "testing",
			CheckTopic:           "testCheckTopic",
			EventTopic:           "testEventTopic",
			MetricTopic:          "testMetricTopic",
			MetricRequireAcks:    "all",
			Partitioner:          "hash",
			RetryMax:             0,
			MetricBufferBytes:    0,
			MetricBufferMessages: 0,
		},
	)
	assert.NoError(t, err)
	kafkaSink, ok := sink.(*KafkaMetricSink)
	assert.True(t, ok)

	kafkaSink.Start(trace.DefaultClient)

	kafkaSink.producer = producerMock
	metric := samplers.InterMetric{
		Name:      "a.b.c",
		Timestamp: 1476119058,
		Value:     float64(100),
		Tags: []string{
			"foo:bar",
			"baz:quz",
		},
		Type: samplers.GaugeMetric,
	}
	ferr := kafkaSink.Flush(context.Background(), []samplers.InterMetric{metric})
	assert.NoError(t, ferr)

	msg := <-producerMock.Successes()
	assert.Equal(t, "testMetricTopic", msg.Topic)
	contents, err := msg.Value.Encode()
	assert.NoError(t, err)
	assert.Contains(t, string(contents), metric.Name)
}

func TestMetricFlushRouting(t *testing.T) {
	tests := []struct {
		name   string
		metric samplers.InterMetric
		expect bool
	}{
		{
			"any sink",
			samplers.InterMetric{
				Name:      "to.any.sink",
				Timestamp: 1476119058,
				Value:     float64(100),
				Tags: []string{
					"foo:bar",
					"baz:quz",
				},
				Type: samplers.GaugeMetric,
			},
			true,
		},
		{
			"kafka directly",
			samplers.InterMetric{
				Name:      "exactly.kafka",
				Timestamp: 1476119058,
				Value:     float64(100),
				Tags: []string{
					"foo:bar",
					"baz:quz",
					"veneursinkonly:kafka",
				},
				Type:  samplers.GaugeMetric,
				Sinks: samplers.RouteInformation{"kafka": struct{}{}},
			},
			true,
		},
		{
			"not us",
			samplers.InterMetric{
				Name:      "not.us",
				Timestamp: 1476119058,
				Value:     float64(100),
				Tags: []string{
					"foo:bar",
					"baz:quz",
					"veneursinkonly:anyone_else",
				},
				Type:  samplers.GaugeMetric,
				Sinks: samplers.RouteInformation{"anyone_else": struct{}{}},
			},
			false,
		},
	}
	for _, elt := range tests {
		test := elt
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			config := sarama.NewConfig()
			config.Producer.Return.Successes = true
			producerMock := mocks.NewAsyncProducer(t, config)
			if test.expect {
				producerMock.ExpectInputAndSucceed()
			}

			sink, err := CreateMetricSink(
				&veneur.Server{
					TraceClient: nil,
				},
				"kafka",
				logrus.NewEntry(logrus.StandardLogger()),
				veneur.Config{},
				KafkaMetricSinkConfig{
					Broker:               "testing",
					CheckTopic:           "testCheckTopic",
					EventTopic:           "testEventTopic",
					MetricTopic:          "testMetricTopic",
					MetricRequireAcks:    "all",
					Partitioner:          "hash",
					RetryMax:             0,
					MetricBufferBytes:    0,
					MetricBufferMessages: 0,
				},
			)
			assert.NoError(t, err)
			sink.Start(trace.DefaultClient)
			kafkaSink, ok := sink.(*KafkaMetricSink)
			assert.True(t, ok)

			kafkaSink.producer = producerMock

			ferr := kafkaSink.Flush(
				context.Background(), []samplers.InterMetric{test.metric})
			producerMock.Close()
			assert.NoError(t, ferr)
			if test.expect {
				contents, err := (<-producerMock.Successes()).Value.Encode()
				assert.NoError(t, err)
				assert.Contains(t, string(contents), test.metric.Name)
			} else {
				_, ok := <-producerMock.Successes()
				if ok {
					t.Fatal("Expected no input for this case")
				}
			}
		})
	}
}

func TestCreateMetricSink(t *testing.T) {
	sink, err := CreateMetricSink(
		&veneur.Server{
			TraceClient: nil,
		}, "kafka",
		logrus.NewEntry(logrus.StandardLogger()),
		veneur.Config{},
		KafkaMetricSinkConfig{
			Broker:                "testing",
			CheckTopic:            "veneur_checks",
			EventTopic:            "veneur_events",
			MetricTopic:           "veneur_metrics",
			MetricRequireAcks:     "all",
			Partitioner:           "hash",
			RetryMax:              1,
			MetricBufferBytes:     2,
			MetricBufferMessages:  3,
			MetricBufferFrequency: time.Duration(10 * time.Second),
		})
	assert.NoError(t, err)
	kafkaSink, ok := sink.(*KafkaMetricSink)
	assert.True(t, ok)

	assert.Equal(t, "kafka", kafkaSink.Name())

	assert.Equal(t, "veneur_checks", kafkaSink.checkTopic,
		"check topic did not set correctly")
	assert.Equal(t, "veneur_events", kafkaSink.eventTopic,
		"event topic did not set correctly")
	assert.Equal(t, "veneur_metrics", kafkaSink.metricTopic,
		"metric topic did not set correctly")

	assert.Equal(t, sarama.WaitForAll, kafkaSink.config.Producer.RequiredAcks,
		"ack did not set correctly")
	assert.Equal(t, 1, kafkaSink.config.Producer.Retry.Max,
		"retries did not set correctly")
	assert.Equal(t, 2, kafkaSink.config.Producer.Flush.Bytes,
		"buffer bytes did not set correctly")
	assert.Equal(t, 3, kafkaSink.config.Producer.Flush.Messages,
		"buffer messages did not set correctly")
	assert.Equal(t, time.Second*10, kafkaSink.config.Producer.Flush.Frequency,
		"flush frequency did not set correctly")
}

func TestCreateMetricSinkWithoutTopics(t *testing.T) {
	_, err := CreateMetricSink(
		&veneur.Server{
			TraceClient: nil,
		},
		"kafka",
		logrus.NewEntry(logrus.StandardLogger()),
		veneur.Config{},
		KafkaMetricSinkConfig{
			Broker:                "testing",
			CheckTopic:            "",
			EventTopic:            "",
			MetricTopic:           "",
			MetricRequireAcks:     "all",
			Partitioner:           "hash",
			RetryMax:              1,
			MetricBufferBytes:     2,
			MetricBufferMessages:  3,
			MetricBufferFrequency: time.Duration(10 * time.Second),
		})
	assert.Error(t, err)
}

func TestCreateSpanSinkWithoutTopic(t *testing.T) {
	_, err := CreateSpanSink(
		&veneur.Server{
			TraceClient: nil,
		},
		"kafka",
		logrus.NewEntry(logrus.StandardLogger()),
		veneur.Config{},
		KafkaSpanSinkConfig{
			Broker:                  "testing",
			Partitioner:             "hash",
			RetryMax:                1,
			SpanBufferBytes:         2,
			SpanBufferFrequency:     0,
			SpanBufferMesages:       3,
			SpanRequireAcks:         "all",
			SpanSampleRatePercent:   100,
			SpanSampleTag:           "",
			SpanSerializationFormat: "",
			SpanTopic:               "",
		},
		context.Background(),
	)
	assert.Error(t, err)
}

func TestCreateSpanSinkWithoutBroker(t *testing.T) {
	_, err := CreateSpanSink(
		&veneur.Server{
			TraceClient: nil,
		},
		"kafka",
		logrus.NewEntry(logrus.StandardLogger()),
		veneur.Config{},
		KafkaSpanSinkConfig{
			Broker:                  "",
			Partitioner:             "hash",
			RetryMax:                1,
			SpanBufferBytes:         2,
			SpanBufferFrequency:     0,
			SpanBufferMesages:       3,
			SpanRequireAcks:         "all",
			SpanSampleRatePercent:   100,
			SpanSampleTag:           "",
			SpanSerializationFormat: "",
			SpanTopic:               "veneur_spans",
		},
		context.Background(),
	)
	assert.Error(t, err)
}

func TestCreateSpanSinkWithSampleRateTooLow(t *testing.T) {
	_, err := CreateSpanSink(
		&veneur.Server{
			TraceClient: nil,
		},
		"kafka",
		logrus.NewEntry(logrus.StandardLogger()),
		veneur.Config{},
		KafkaSpanSinkConfig{
			Broker:                  "testing",
			Partitioner:             "hash",
			RetryMax:                1,
			SpanBufferBytes:         2,
			SpanBufferFrequency:     time.Duration(10 * time.Second),
			SpanBufferMesages:       3,
			SpanRequireAcks:         "all",
			SpanSampleRatePercent:   -1,
			SpanSampleTag:           "",
			SpanSerializationFormat: "",
			SpanTopic:               "veneur_spans",
		},
		context.Background(),
	)
	assert.Error(t, err)
}

func TestCreateSpanSinkWithSampleRateZero(t *testing.T) {
	_, err := CreateSpanSink(
		&veneur.Server{
			TraceClient: nil,
		},
		"kafka",
		logrus.NewEntry(logrus.StandardLogger()),
		veneur.Config{},
		KafkaSpanSinkConfig{
			Broker:                  "testing",
			Partitioner:             "hash",
			RetryMax:                1,
			SpanBufferBytes:         2,
			SpanBufferFrequency:     time.Duration(10 * time.Second),
			SpanBufferMesages:       3,
			SpanRequireAcks:         "all",
			SpanSampleRatePercent:   0,
			SpanSampleTag:           "",
			SpanSerializationFormat: "",
			SpanTopic:               "veneur_spans",
		},
		context.Background(),
	)
	assert.NoError(t, err)
}

func TestCreateSpanSinkWithSampleRateTooHigh(t *testing.T) {
	_, err := CreateSpanSink(
		&veneur.Server{
			TraceClient: nil,
		},
		"kafka",
		logrus.NewEntry(logrus.StandardLogger()),
		veneur.Config{},
		KafkaSpanSinkConfig{
			Broker:                  "testing",
			Partitioner:             "hash",
			RetryMax:                1,
			SpanBufferBytes:         2,
			SpanBufferFrequency:     time.Duration(10 * time.Second),
			SpanBufferMesages:       3,
			SpanRequireAcks:         "all",
			SpanSampleRatePercent:   101,
			SpanSampleTag:           "",
			SpanSerializationFormat: "",
			SpanTopic:               "veneur_spans",
		},
		context.Background(),
	)
	assert.Error(t, err)
}

func TestCreateSpanSinkWithRequireAcksNone(t *testing.T) {
	sink, err := CreateSpanSink(
		&veneur.Server{
			TraceClient: nil,
		},
		"kafka",
		logrus.NewEntry(logrus.StandardLogger()),
		veneur.Config{},
		KafkaSpanSinkConfig{
			Broker:                  "testing",
			Partitioner:             "hash",
			RetryMax:                1,
			SpanBufferBytes:         2,
			SpanBufferFrequency:     time.Duration(10 * time.Second),
			SpanBufferMesages:       3,
			SpanRequireAcks:         "none",
			SpanSampleRatePercent:   100,
			SpanSampleTag:           "",
			SpanSerializationFormat: "",
			SpanTopic:               "veneur_spans",
		},
		context.Background(),
	)
	assert.NoError(t, err)
	kafkaSink, ok := sink.(*KafkaSpanSink)
	assert.True(t, ok)
	assert.Equal(t, sarama.NoResponse, kafkaSink.config.Producer.RequiredAcks)
}

func TestCreateSpanSinkWithRequireAcksLocal(t *testing.T) {
	sink, err := CreateSpanSink(
		&veneur.Server{
			TraceClient: nil,
		},
		"kafka",
		logrus.NewEntry(logrus.StandardLogger()),
		veneur.Config{},
		KafkaSpanSinkConfig{
			Broker:                  "testing",
			Partitioner:             "hash",
			RetryMax:                1,
			SpanBufferBytes:         2,
			SpanBufferFrequency:     time.Duration(10 * time.Second),
			SpanBufferMesages:       3,
			SpanRequireAcks:         "local",
			SpanSampleRatePercent:   100,
			SpanSampleTag:           "",
			SpanSerializationFormat: "",
			SpanTopic:               "veneur_spans",
		},
		context.Background(),
	)
	assert.NoError(t, err)
	kafkaSink, ok := sink.(*KafkaSpanSink)
	assert.True(t, ok)
	assert.Equal(t, sarama.WaitForLocal, kafkaSink.config.Producer.RequiredAcks)
}

func TestCreateSpanSinkWithRequireAcksInvalid(t *testing.T) {
	sink, err := CreateSpanSink(
		&veneur.Server{
			TraceClient: nil,
		},
		"kafka",
		logrus.NewEntry(logrus.StandardLogger()),
		veneur.Config{},
		KafkaSpanSinkConfig{
			Broker:                  "testing",
			Partitioner:             "hash",
			RetryMax:                1,
			SpanBufferBytes:         2,
			SpanBufferFrequency:     time.Duration(10 * time.Second),
			SpanBufferMesages:       3,
			SpanRequireAcks:         "invalid",
			SpanSampleRatePercent:   100,
			SpanSampleTag:           "",
			SpanSerializationFormat: "",
			SpanTopic:               "veneur_spans",
		},
		context.Background(),
	)
	assert.NoError(t, err)
	kafkaSink, ok := sink.(*KafkaSpanSink)
	assert.True(t, ok)
	assert.Equal(t, sarama.WaitForAll, kafkaSink.config.Producer.RequiredAcks)
}

func TestCreateSpanSink(t *testing.T) {
	sink, err := CreateSpanSink(
		&veneur.Server{
			TraceClient: nil,
		},
		"kafka",
		logrus.NewEntry(logrus.StandardLogger()),
		veneur.Config{},
		KafkaSpanSinkConfig{
			Broker:                  "testing",
			Partitioner:             "hash",
			RetryMax:                1,
			SpanBufferBytes:         2,
			SpanBufferFrequency:     time.Duration(10 * time.Second),
			SpanBufferMesages:       3,
			SpanRequireAcks:         "all",
			SpanSampleRatePercent:   100,
			SpanSampleTag:           "foo",
			SpanSerializationFormat: "",
			SpanTopic:               "veneur_spans",
		},
		context.Background(),
	)
	assert.NoError(t, err)
	kafkaSink, ok := sink.(*KafkaSpanSink)

	assert.True(t, ok)
	assert.NoError(t, err)
	assert.Equal(t, "kafka", sink.Name())

	assert.Equal(t, "protobuf", kafkaSink.serializer)
	assert.Equal(t, "veneur_spans", kafkaSink.topic)

	assert.Equal(t, uint32(math.MaxUint32), kafkaSink.sampleThreshold)
	assert.Equal(t, "foo", kafkaSink.sampleTag)

	assert.Equal(t, sarama.WaitForAll, kafkaSink.config.Producer.RequiredAcks)
	assert.Equal(t, 1, kafkaSink.config.Producer.Retry.Max)
	assert.Equal(t, 2, kafkaSink.config.Producer.Flush.Bytes)
	assert.Equal(t, 3, kafkaSink.config.Producer.Flush.Messages)
	assert.Equal(t, 10*time.Second, kafkaSink.config.Producer.Flush.Frequency)
}

func TestSpanTraceIdSampling(t *testing.T) {
	config := sarama.NewConfig()
	config.Producer.Return.Successes = true
	producerMock := mocks.NewAsyncProducer(t, config)

	producerMock.ExpectInputAndSucceed()

	sink, err := CreateSpanSink(
		&veneur.Server{
			TraceClient: nil,
		},
		"kafka",
		logrus.NewEntry(logrus.StandardLogger()),
		veneur.Config{},
		KafkaSpanSinkConfig{
			Broker:                  "testing",
			SpanTopic:               "testSpanTopic",
			Partitioner:             "hash",
			RetryMax:                0,
			SpanBufferBytes:         0,
			SpanBufferMesages:       0,
			SpanRequireAcks:         "all",
			SpanSerializationFormat: "json",
			SpanSampleTag:           "",
			SpanSampleRatePercent:   50,
		},
		context.Background(),
	)
	assert.NoError(t, err)
	kafkaSink, ok := sink.(*KafkaSpanSink)
	assert.True(t, ok)

	kafkaSink.producer = producerMock

	start := time.Now()
	end := start.Add(2 * time.Second)
	// This span's traceID is set so that it will hash in a way that will not allow it to pass
	testSpanBad := ssf.SSFSpan{
		TraceId:        1,
		ParentId:       1,
		Id:             2,
		StartTimestamp: int64(start.UnixNano()),
		EndTimestamp:   int64(end.UnixNano()),
		Error:          false,
		Service:        "farts-srv",
		Tags:           map[string]string{},
		Indicator:      false,
		Name:           "farting farty farts",
	}
	// This span's traceID is set so that it will hash in a way that will allow it to pass
	testSpanGood := ssf.SSFSpan{
		TraceId:        3,
		ParentId:       3,
		Id:             4,
		StartTimestamp: int64(start.UnixNano()),
		EndTimestamp:   int64(end.UnixNano()),
		Error:          false,
		Service:        "farts2-srv",
		Tags:           map[string]string{},
		Indicator:      false,
		Name:           "farting farty farts",
	}

	sink.Ingest(&testSpanGood)
	sink.Ingest(&testSpanBad)

	err = producerMock.Close()
	assert.NoError(t, err)

	for msg := range producerMock.Successes() {
		contents, err := msg.Value.Encode()
		assert.NoError(t, err)
		assert.Contains(t, string(contents), testSpanGood.Tags["baz"])
	}
}

func TestSpanTagSampling(t *testing.T) {
	config := sarama.NewConfig()
	config.Producer.Return.Successes = true
	producerMock := mocks.NewAsyncProducer(t, config)

	producerMock.ExpectInputAndSucceed()

	sink, err := CreateSpanSink(
		&veneur.Server{
			TraceClient: nil,
		},
		"kafka",
		logrus.NewEntry(logrus.StandardLogger()),
		veneur.Config{},
		KafkaSpanSinkConfig{
			Broker:                  "testing",
			SpanTopic:               "testSpanTopic",
			Partitioner:             "hash",
			RetryMax:                0,
			SpanBufferBytes:         0,
			SpanBufferMesages:       0,
			SpanRequireAcks:         "all",
			SpanSerializationFormat: "json",
			SpanSampleTag:           "baz",
			SpanSampleRatePercent:   50,
		},
		context.Background(),
	)
	assert.NoError(t, err)
	kafkaSink, ok := sink.(*KafkaSpanSink)
	assert.True(t, ok)
	kafkaSink.producer = producerMock

	start := time.Now()
	end := start.Add(2 * time.Second)
	testSpanBad := ssf.SSFSpan{
		TraceId:        1,
		ParentId:       1,
		Id:             2,
		StartTimestamp: int64(start.UnixNano()),
		EndTimestamp:   int64(end.UnixNano()),
		Error:          false,
		Service:        "farts-srv",
		Tags: map[string]string{
			"bar": "qux",
		},
		Indicator: false,
		Name:      "farting farty farts",
	}
	// This span's value for the "baz" tag is determined to hash correctly so that it'll pass at 50%
	testSpanGood := ssf.SSFSpan{
		TraceId:        1,
		ParentId:       1,
		Id:             2,
		StartTimestamp: int64(start.UnixNano()),
		EndTimestamp:   int64(end.UnixNano()),
		Error:          false,
		Service:        "farts2-srv",
		Tags: map[string]string{
			"baz": "farts0",
		},
		Indicator: false,
		Name:      "farting farty farts",
	}
	// This span's value for the "baz" tag is determined to hash correctly so that it'll be dropped at 50%
	testSpanLongRejected := ssf.SSFSpan{
		TraceId:        1,
		ParentId:       1,
		Id:             2,
		StartTimestamp: int64(start.UnixNano()),
		EndTimestamp:   int64(end.UnixNano()),
		Error:          false,
		Service:        "farts2-srv",
		Tags: map[string]string{
			"baz": "pthbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbt290",
		},
		Indicator: false,
		Name:      "farting farty farts",
	}
	// This span's value for the "baz" tag is determined to hash correctly so that it'll be dropped at 50%
	testSpanRejected := ssf.SSFSpan{
		TraceId:        1,
		ParentId:       1,
		Id:             2,
		StartTimestamp: int64(start.UnixNano()),
		EndTimestamp:   int64(end.UnixNano()),
		Error:          false,
		Service:        "farts2-srv",
		Tags: map[string]string{
			"baz": "farts3",
		},
		Indicator: false,
		Name:      "farting farty farts",
	}

	sink.Ingest(&testSpanGood)
	sink.Ingest(&testSpanBad)
	sink.Ingest(&testSpanRejected)
	sink.Ingest(&testSpanLongRejected)

	err = producerMock.Close()
	assert.NoError(t, err)

	for msg := range producerMock.Successes() {
		contents, err := msg.Value.Encode()
		assert.NoError(t, err)
		assert.Contains(t, string(contents), testSpanGood.Tags["baz"])
	}
}

func TestSpanFlushJson(t *testing.T) {
	config := sarama.NewConfig()
	config.Producer.Return.Successes = true
	producerMock := mocks.NewAsyncProducer(t, config)

	producerMock.ExpectInputAndSucceed()

	sink, err := CreateSpanSink(
		&veneur.Server{
			TraceClient: nil,
		},
		"kafka",
		logrus.NewEntry(logrus.StandardLogger()),
		veneur.Config{},
		KafkaSpanSinkConfig{
			Broker:                  "testing",
			SpanTopic:               "testSpanTopic",
			Partitioner:             "hash",
			RetryMax:                0,
			SpanBufferBytes:         0,
			SpanBufferMesages:       0,
			SpanRequireAcks:         "all",
			SpanSerializationFormat: "json",
			SpanSampleTag:           "",
			SpanSampleRatePercent:   100,
		},
		context.Background(),
	)
	assert.NoError(t, err)
	kafkaSink, ok := sink.(*KafkaSpanSink)
	assert.True(t, ok)
	kafkaSink.producer = producerMock

	start := time.Now()
	end := start.Add(2 * time.Second)
	testSpan := ssf.SSFSpan{
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
	sink.Ingest(&testSpan)
	assert.NoError(t, err)

	msg := <-producerMock.Successes()
	assert.Equal(t, "testSpanTopic", msg.Topic)
	contents, err := msg.Value.Encode()
	assert.NoError(t, err)
	assert.Contains(t, string(contents), testSpan.Service)
}

func TestSpanFlushProtobuf(t *testing.T) {
	config := sarama.NewConfig()
	config.Producer.Return.Successes = true
	producerMock := mocks.NewAsyncProducer(t, config)

	producerMock.ExpectInputAndSucceed()

	sink, err := CreateSpanSink(
		&veneur.Server{
			TraceClient: nil,
		},
		"kafka",
		logrus.NewEntry(logrus.StandardLogger()),
		veneur.Config{},
		KafkaSpanSinkConfig{
			Broker:                  "testing",
			SpanTopic:               "testSpanTopic",
			Partitioner:             "hash",
			RetryMax:                0,
			SpanBufferBytes:         0,
			SpanBufferMesages:       0,
			SpanRequireAcks:         "all",
			SpanSerializationFormat: "protobuf",
			SpanSampleTag:           "",
			SpanSampleRatePercent:   100,
		},
		context.Background(),
	)
	assert.NoError(t, err)
	kafkaSink, ok := sink.(*KafkaSpanSink)
	assert.True(t, ok)

	kafkaSink.producer = producerMock

	start := time.Now()
	end := start.Add(2 * time.Second)
	testSpan := ssf.SSFSpan{
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
	sink.Ingest(&testSpan)
	assert.NoError(t, err)

	msg := <-producerMock.Successes()
	assert.Equal(t, "testSpanTopic", msg.Topic)
	contents, err := msg.Value.Encode()
	assert.NoError(t, err)

	span := ssf.SSFSpan{}
	marshalErr := proto.Unmarshal(contents, &span)
	assert.NoError(t, marshalErr)

	assert.Equal(t, testSpan.Service, span.Service)
}
