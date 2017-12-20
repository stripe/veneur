package kafka

import (
	"context"
	"testing"
	"time"

	"github.com/DataDog/datadog-go/statsd"
	"github.com/Shopify/sarama"
	"github.com/Shopify/sarama/mocks"
	"github.com/gogo/protobuf/proto"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stripe/veneur/samplers"
	"github.com/stripe/veneur/ssf"
	"github.com/stripe/veneur/trace"
)

func TestMetricFlush(t *testing.T) {
	config := sarama.NewConfig()
	config.Producer.Return.Successes = true
	producerMock := mocks.NewAsyncProducer(t, config)

	producerMock.ExpectInputAndSucceed()

	// I would use the logrus test logger but the package needs to be
	// updated from Sirupsen/logrus to sirupsen/logrus
	// https://github.com/stripe/veneur/issues/277
	logger := logrus.StandardLogger()
	stats, _ := statsd.NewBuffered("localhost:1235", 1024)

	sink, err := NewKafkaMetricSink(logger, "testing", "testCheckTopic", "testEventTopic", "testMetricTopic", "all", "hash", 0, 0, 0, "", stats)
	assert.NoError(t, err)
	sink.Start(trace.DefaultClient)

	sink.producer = producerMock
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
	ferr := sink.Flush(context.Background(), []samplers.InterMetric{metric})
	assert.NoError(t, ferr)

	msg := <-producerMock.Successes()
	assert.Equal(t, "testMetricTopic", msg.Topic)
	contents, err := msg.Value.Encode()
	assert.NoError(t, err)
	assert.Contains(t, string(contents), metric.Name)
}

func TestMetricConstructor(t *testing.T) {
	logger := logrus.StandardLogger()
	stats, _ := statsd.NewBuffered("localhost:1235", 1024)

	sink, err := NewKafkaMetricSink(logger, "testing", "veneur_checks", "veneur_events", "veneur_metrics", "all", "hash", 1, 2, 3, "10s", stats)
	assert.NoError(t, err)

	assert.Equal(t, "kafka", sink.Name())

	assert.Equal(t, "veneur_checks", sink.checkTopic, "check topic did not set correctly")
	assert.Equal(t, "veneur_events", sink.eventTopic, "event topic did not set correctly")
	assert.Equal(t, "veneur_metrics", sink.metricTopic, "metric topic did not set correctly")

	assert.Equal(t, sarama.WaitForAll, sink.config.Producer.RequiredAcks, "ack did not set correctly")
	assert.Equal(t, 1, sink.config.Producer.Retry.Max, "retries did not set correctly")
	assert.Equal(t, 2, sink.config.Producer.Flush.Bytes, "buffer bytes did not set correctly")
	assert.Equal(t, 3, sink.config.Producer.Flush.Messages, "buffer messages did not set correctly")
	assert.Equal(t, time.Second*10, sink.config.Producer.Flush.Frequency, "flush frequency did not set correctly")
}

func TestMetricInstantiateFailure(t *testing.T) {
	logger := logrus.StandardLogger()
	stats, _ := statsd.NewBuffered("localhost:1235", 1024)

	// Busted duration
	_, err1 := NewKafkaMetricSink(logger, "testing", "veneur_checks", "veneur_events", "veneur_metrics", "all", "hash", 1, 2, 3, "farts", stats)
	assert.Error(t, err1)

	// No topics
	_, err := NewKafkaMetricSink(logger, "testing", "", "", "", "all", "hash", 1, 2, 3, "10s", stats)
	assert.Error(t, err)
}

func TestSpanInstantiateFailure(t *testing.T) {
	logger := logrus.StandardLogger()
	stats, _ := statsd.NewBuffered("localhost:1235", 1024)

	// Busted duration
	_, err := NewKafkaSpanSink(logger, "testing", "veneur_spans", "hash", "all", 1, 2, 3, "farts", "", stats)
	assert.Error(t, err)

	// Missing topic
	_, err2 := NewKafkaSpanSink(logger, "testing", "", "hash", "all", 1, 2, 3, "farts", "", stats)
	assert.Error(t, err2)

	// Missing brokers
	_, err3 := NewKafkaSpanSink(logger, "", "farts", "hash", "all", 1, 2, 3, "farts", "", stats)
	assert.Error(t, err3)
}

func TestSpanConstructorAck(t *testing.T) {
	logger := logrus.StandardLogger()
	stats, _ := statsd.NewBuffered("localhost:1235", 1024)

	sink1, _ := NewKafkaSpanSink(logger, "testing", "veneur_spans", "hash", "none", 1, 2, 3, "10s", "", stats)
	assert.Equal(t, sarama.NoResponse, sink1.config.Producer.RequiredAcks, "ack did not set correctly")

	sink2, _ := NewKafkaSpanSink(logger, "testing", "veneur_spans", "hash", "local", 1, 2, 3, "10s", "", stats)
	assert.Equal(t, sarama.WaitForLocal, sink2.config.Producer.RequiredAcks, "ack did not set correctly")

	sink3, _ := NewKafkaSpanSink(logger, "testing", "veneur_spans", "random", "farts", 1, 2, 3, "10s", "", stats)
	assert.Equal(t, sarama.WaitForAll, sink3.config.Producer.RequiredAcks, "ack did not default correctly")
}

func TestSpanConstructor(t *testing.T) {
	logger := logrus.StandardLogger()
	stats, _ := statsd.NewBuffered("localhost:1235", 1024)

	sink, err := NewKafkaSpanSink(logger, "testing", "veneur_spans", "hash", "all", 1, 2, 3, "10s", "", stats)
	assert.NoError(t, err)
	assert.Equal(t, "kafka", sink.Name())

	assert.Equal(t, "protobuf", sink.serializer, "Serializer did not default correctly")
	assert.Equal(t, "veneur_spans", sink.topic, "Topic did not set correctly")

	assert.Equal(t, sarama.WaitForAll, sink.config.Producer.RequiredAcks, "ack did not set correctly")
	assert.Equal(t, 1, sink.config.Producer.Retry.Max, "retries did not set correctly")
	assert.Equal(t, 2, sink.config.Producer.Flush.Bytes, "buffer bytes did not set correctly")
	assert.Equal(t, 3, sink.config.Producer.Flush.Messages, "buffer messages did not set correctly")
	assert.Equal(t, time.Second*10, sink.config.Producer.Flush.Frequency, "flush frequency did not set correctly")
}

func TestBadDuration(t *testing.T) {
	logger := logrus.StandardLogger()
	stats, _ := statsd.NewBuffered("localhost:1235", 1024)

	_, err := NewKafkaSpanSink(logger, "testing", "", "hash", "all", 0, 0, 0, "pthbbbbbt", "", stats)
	assert.Error(t, err)
}

func TestSpanFlushJson(t *testing.T) {
	config := sarama.NewConfig()
	config.Producer.Return.Successes = true
	producerMock := mocks.NewAsyncProducer(t, config)

	producerMock.ExpectInputAndSucceed()

	// I would use the logrus test logger but the package needs to be
	// updated from Sirupsen/logrus to sirupsen/logrus
	// https://github.com/stripe/veneur/issues/277
	logger := logrus.StandardLogger()
	stats, _ := statsd.NewBuffered("localhost:1235", 1024)

	sink, err := NewKafkaSpanSink(logger, "testing", "testSpanTopic", "hash", "all", 0, 0, 0, "", "json", stats)
	assert.NoError(t, err)

	sink.producer = producerMock

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

	// I would use the logrus test logger but the package needs to be
	// updated from Sirupsen/logrus to sirupsen/logrus
	// https://github.com/stripe/veneur/issues/277
	logger := logrus.StandardLogger()
	stats, _ := statsd.NewBuffered("localhost:1235", 1024)

	sink, err := NewKafkaSpanSink(logger, "testing", "testSpanTopic", "hash", "all", 0, 0, 0, "", "protobuf", stats)
	assert.NoError(t, err)

	sink.producer = producerMock

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
