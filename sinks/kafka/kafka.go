package kafka

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/DataDog/datadog-go/statsd"
	"github.com/Shopify/sarama"
	"github.com/sirupsen/logrus"
	"github.com/stripe/veneur/samplers"
	"github.com/stripe/veneur/sinks"
	"github.com/stripe/veneur/ssf"
	"github.com/stripe/veneur/trace"
)

var _ sinks.MetricSink = &KafkaMetricSink{}
var _ sinks.SpanSink = &KafkaSpanSink{}

type KafkaMetricSink struct {
	logger      *logrus.Entry
	producer    sarama.AsyncProducer
	statsd      *statsd.Client
	checkTopic  string
	eventTopic  string
	metricTopic string
	brokers     string
	serializer  string
	config      *sarama.Config
}

type KafkaSpanSink struct {
	logger     *logrus.Entry
	producer   sarama.AsyncProducer
	statsd     *statsd.Client
	topic      string
	brokers    string
	serializer string
	config     *sarama.Config
}

// NewKafkaMetricSink creates a new Kafka Plugin.
func NewKafkaMetricSink(logger *logrus.Logger, brokers string, checkTopic string, eventTopic string, metricTopic string, ackRequirement string, partitioner string, retries int, bufferBytes int, bufferMessages int, bufferDuration string, serializationFormat string, stats *statsd.Client) *KafkaMetricSink {

	finalCheckTopic := metricTopic
	if finalCheckTopic == "" {
		finalCheckTopic = "veneur_checks"
	}

	finalEventTopic := eventTopic
	if finalEventTopic == "" {
		finalEventTopic = "veneur_events"
	}

	finalMetricTopic := metricTopic
	if finalMetricTopic == "" {
		finalMetricTopic = "veneur_metrics"
	}

	ll := logger.WithField("metric_sink", "kafka")

	var finalBufferDuration time.Duration
	if bufferDuration != "" {
		finalBufferDuration, _ = time.ParseDuration(bufferDuration)
		// TODO Return an error!
	}

	serializer := serializationFormat
	if serializer != "json" && serializer != "protobuf" {
		ll.WithField("serializer", serializer).Warn("Unknown serializer, defaulting to protobuf")
		// TODO Test this
		serializer = "protobuf"
	}

	config, _ := newProducerConfig(ll, ackRequirement, partitioner, retries, bufferBytes, bufferMessages, finalBufferDuration)

	ll.WithFields(logrus.Fields{
		"brokers":         brokers,
		"check topic":     finalCheckTopic,
		"event topic":     finalEventTopic,
		"metric topic":    finalMetricTopic,
		"partitioner":     partitioner,
		"ack requirement": ackRequirement,
		"max retries":     retries,
	})

	return &KafkaMetricSink{
		logger:      ll,
		statsd:      stats,
		serializer:  serializer,
		checkTopic:  finalCheckTopic,
		eventTopic:  finalEventTopic,
		metricTopic: finalMetricTopic,
		brokers:     brokers,
		config:      config,
	}
}

func newProducerConfig(logger *logrus.Entry, ackRequirement string, partitioner string, retries int, bufferBytes int, bufferMessages int, bufferFrequency time.Duration) (*sarama.Config, error) {

	config := sarama.NewConfig()
	// TODO Stringer?
	switch ackRequirement {
	case "all":
		config.Producer.RequiredAcks = sarama.WaitForAll
	case "none":
		config.Producer.RequiredAcks = sarama.NoResponse
	case "local":
		config.Producer.RequiredAcks = sarama.WaitForLocal
	default:
		logrus.WithField("ack_requirement", ackRequirement).Warn("Unknown ack requirement, defaulting to all")
		config.Producer.RequiredAcks = sarama.WaitForAll
	}

	switch partitioner {
	case "random":
		config.Producer.Partitioner = sarama.NewRandomPartitioner
	default:
		config.Producer.Partitioner = sarama.NewHashPartitioner
	}

	if bufferBytes != 0 {
		config.Producer.Flush.Bytes = bufferBytes
	}
	if bufferMessages != 0 {
		config.Producer.Flush.Messages = bufferMessages
	}
	if bufferFrequency != 0 {
		config.Producer.Flush.Frequency = bufferFrequency

	}

	config.Producer.Retry.Max = retries

	return config, nil
}

// newConfiguredProducer returns a configured Sarama SyncProducer
func newConfiguredProducer(logger *logrus.Entry, brokerString string, config *sarama.Config) (sarama.AsyncProducer, error) {
	brokerList := strings.Split(brokerString, ",")

	if len(brokerList) < 1 {
		logger.WithField("addrs", brokerString).Error("No brokers?")
		return nil, errors.New("No brokers in broker list")
	}

	logger.WithField("addrs", brokerList).Info("Connecting to Kafka")
	producer, err := sarama.NewAsyncProducer(brokerList, config)

	if err != nil {
		logger.Error("Error Connecting to Kafka. client error: ", err)
	}

	return producer, nil
}

// Name returns the name of this sink.
func (k *KafkaMetricSink) Name() string {
	return "kafka"
}

// Start performs final adjustments on the sink.
func (k *KafkaMetricSink) Start(cl *trace.Client) error {
	producer, err := newConfiguredProducer(k.logger, k.brokers, k.config)
	if err != nil {
		return err
	}
	k.producer = producer
	return nil
}

// Flush sends a slice of metrics to Kafka
func (k *KafkaMetricSink) Flush(ctx context.Context, interMetrics []samplers.InterMetric) error {
	k.statsd.Gauge("flush.kafka.metrics_total", float64(len(interMetrics)), nil, 1.0)

	if len(interMetrics) == 0 {
		k.logger.Info("Nothing to flush, skipping.")
		return nil
	}

	sendMetricStart := time.Now()

	for _, metric := range interMetrics {

		k.logger.Debug("Emitting Metric: ", metric.Name)
		var enc sarama.Encoder
		switch k.serializer {
		case "json":
			j, err := json.Marshal(metric)
			if err != nil {
				k.logger.Error("Error marshalling metric: ", metric.Name)
				k.statsd.Count("kafka.marshal.error_total", 1, nil, 1.0)
				return err
			}
			enc = sarama.StringEncoder(j)
		case "protobuf":
			// TODO
		default:
			return errors.New(fmt.Sprintf("Unknown serialization format for encoding Kafka message: %s", k.serializer))
		}

		k.producer.Input() <- &sarama.ProducerMessage{
			Topic: k.metricTopic,
			Value: enc,
		}
		k.statsd.Count("kafka.producer.success_total", 1, nil, 1.0)
	}

	k.statsd.TimeInMilliseconds("kafka.flush_batch.duration_ns", float64(time.Now().Sub(sendMetricStart).Nanoseconds()), nil, 1.0)

	return nil
}

// FlushEventsChecks flushes Events and Checks
func (k *KafkaMetricSink) FlushEventsChecks(ctx context.Context, events []samplers.UDPEvent, checks []samplers.UDPServiceCheck) {
	// TODO
}

// NewKafkaSpanSink creates a new Kafka Plugin.
func NewKafkaSpanSink(logger *logrus.Logger, brokers string, topic string, partitioner string, ackRequirement string, retries int, bufferBytes int, bufferMessages int, bufferFrequency time.Duration, serializationFormat string, stats *statsd.Client) *KafkaSpanSink {

	finalTopic := topic
	if finalTopic == "" {
		finalTopic = "veneur_spans"
	}

	ll := logger.WithField("span_sink", "kafka")

	serializer := serializationFormat
	if serializer != "json" && serializer != "protobuf" {
		ll.WithField("serializer", serializer).Warn("Unknown serializer, defaulting to protobuf")
	}

	config, _ := newProducerConfig(ll, ackRequirement, partitioner, retries, bufferBytes, bufferMessages, bufferFrequency)

	ll.WithFields(logrus.Fields{
		"brokers":         brokers,
		"topic":           finalTopic,
		"partitioner":     partitioner,
		"ack requirement": ackRequirement,
		"max retries":     retries,
	})

	return &KafkaSpanSink{
		logger:  ll,
		statsd:  stats,
		topic:   finalTopic,
		brokers: brokers,
		config:  config,
	}
}

// Name returns the name of this sink.
func (k *KafkaSpanSink) Name() string {
	return "kafka"
}

// Start performs final adjustments on the sink.
func (k *KafkaSpanSink) Start(cl *trace.Client) error {
	producer, err := newConfiguredProducer(k.logger, k.brokers, k.config)
	if err != nil {
		return err
	}
	k.producer = producer
	return nil
}

// Ingest takes the span and adds it to the ringbuffer.
func (k *KafkaSpanSink) Ingest(span ssf.SSFSpan) error {

	j, err := json.Marshal(span)

	if err != nil {
		k.logger.WithError(err).Error("Error marshaling span")
		k.statsd.Count("kafka.marshal.error_total", 1, nil, 1.0)
	} else {
		k.producer.Input() <- &sarama.ProducerMessage{
			Topic: k.topic,
			Value: sarama.StringEncoder(j),
		}
		k.statsd.Count("kafka.producer.success_total", 1, nil, 1.0)
	}

	return nil
}

// Flush TODO TODO
func (k *KafkaSpanSink) Flush() {
	// TODO
}
