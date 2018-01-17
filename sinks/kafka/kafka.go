package kafka

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"hash/crc32"
	"io/ioutil"
	"math"
	"strings"
	"sync/atomic"
	"time"

	"github.com/DataDog/datadog-go/statsd"
	"github.com/Shopify/sarama"
	"github.com/gogo/protobuf/proto"
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
	config      *sarama.Config
}

type KafkaSpanSink struct {
	logger          *logrus.Entry
	producer        sarama.AsyncProducer
	statsd          *statsd.Client
	topic           string
	brokers         string
	serializer      string
	sampleTag       string
	sampleThreshold uint32
	config          *sarama.Config
	spansFlushed    int64
}

// NewKafkaMetricSink creates a new Kafka Plugin.
func NewKafkaMetricSink(logger *logrus.Logger, brokers string, checkTopic string, eventTopic string, metricTopic string, ackRequirement string, partitioner string, retries int, bufferBytes int, bufferMessages int, bufferDuration string, stats *statsd.Client) (*KafkaMetricSink, error) {
	if logger == nil {
		logger = &logrus.Logger{Out: ioutil.Discard}
	}

	if checkTopic == "" && eventTopic == "" && metricTopic == "" {
		return nil, errors.New("Unable to start Kafka sink with no valid topic names")
	}

	ll := logger.WithField("metric_sink", "kafka")

	var finalBufferDuration time.Duration
	if bufferDuration != "" {
		var err error
		finalBufferDuration, err = time.ParseDuration(bufferDuration)
		if err != nil {
			return nil, err
		}
	}

	config, _ := newProducerConfig(ll, ackRequirement, partitioner, retries, bufferBytes, bufferMessages, finalBufferDuration)

	ll.WithFields(logrus.Fields{
		"brokers":         brokers,
		"check_topic":     checkTopic,
		"event_topic":     eventTopic,
		"metric_topic":    metricTopic,
		"partitioner":     partitioner,
		"ack_requirement": ackRequirement,
		"max_retries":     retries,
		"buffer_bytes":    bufferBytes,
		"buffer_messages": bufferMessages,
		"buffer_duration": bufferDuration,
	}).Info("Created Kafka metric sink")

	return &KafkaMetricSink{
		logger:      ll,
		statsd:      stats,
		checkTopic:  checkTopic,
		eventTopic:  eventTopic,
		metricTopic: metricTopic,
		brokers:     brokers,
		config:      config,
	}, nil
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

	if len(interMetrics) == 0 {
		k.logger.Info("Nothing to flush, skipping.")
		return nil
	}

	sendMetricStart := time.Now()

	successes := int64(0)
	for _, metric := range interMetrics {
		if !sinks.IsAcceptableMetric(metric, k) {
			continue
		}

		k.logger.Debug("Emitting Metric: ", metric.Name)
		j, err := json.Marshal(metric)
		if err != nil {
			k.logger.Error("Error marshalling metric: ", metric.Name)
			k.statsd.Count("kafka.marshal.error_total", 1, nil, 1.0)
			return err
		}

		k.producer.Input() <- &sarama.ProducerMessage{
			Topic: k.metricTopic,
			Value: sarama.StringEncoder(j),
		}
		successes++
	}
	k.statsd.Count("kafka.metrics_success_total", successes, nil, 1.0)
	k.statsd.TimeInMilliseconds("kafka.metrics_flush_duration_ns", float64(time.Now().Sub(sendMetricStart).Nanoseconds()), nil, 1.0)

	return nil
}

// FlushEventsChecks flushes Events and Checks
func (k *KafkaMetricSink) FlushEventsChecks(ctx context.Context, events []samplers.UDPEvent, checks []samplers.UDPServiceCheck) {
	// TODO
}

// NewKafkaSpanSink creates a new Kafka Plugin.
func NewKafkaSpanSink(logger *logrus.Logger, brokers string, topic string, partitioner string, ackRequirement string, retries int, bufferBytes int, bufferMessages int, bufferDuration string, serializationFormat string, sampleTag string, sampleRatePercentage int, stats *statsd.Client) (*KafkaSpanSink, error) {

	if logger == nil {
		logger = &logrus.Logger{Out: ioutil.Discard}
	}

	if topic == "" {
		return nil, errors.New("Cannot start Kafka span sink with no span topic")
	}

	ll := logger.WithField("span_sink", "kafka")

	serializer := serializationFormat
	if serializer != "json" && serializer != "protobuf" {
		ll.WithField("serializer", serializer).Warn("Unknown serializer, defaulting to protobuf")
		serializer = "protobuf"
	}

	var sampleThreshold uint32
	if sampleRatePercentage <= 0 || sampleRatePercentage > 100 {
		return nil, errors.New("Span sample rate percentage must be greater than 0%% and less than or equal to 100%%")
	}

	// Set the sample threshold to (sample rate) * (maximum value of uint32), so that
	// we can store it as a uint32 instead of a float64 and compare apples-to-apples
	// with the output of our hashing algorithm.
	sampleThreshold = uint32(sampleRatePercentage * math.MaxUint32 / 100)

	var finalBufferDuration time.Duration
	if bufferDuration != "" {
		var err error
		finalBufferDuration, err = time.ParseDuration(bufferDuration)
		if err != nil {
			return nil, err
		}
	}

	config, _ := newProducerConfig(ll, ackRequirement, partitioner, retries, bufferBytes, bufferMessages, finalBufferDuration)

	ll.WithFields(logrus.Fields{
		"brokers":         brokers,
		"topic":           topic,
		"partitioner":     partitioner,
		"ack_requirement": ackRequirement,
		"max_retries":     retries,
		"buffer_bytes":    bufferBytes,
		"buffer_messages": bufferMessages,
		"buffer_duration": bufferDuration,
	}).Info("Started Kafka span sink")

	return &KafkaSpanSink{
		logger:          ll,
		statsd:          stats,
		topic:           topic,
		brokers:         brokers,
		config:          config,
		serializer:      serializer,
		sampleTag:       sampleTag,
		sampleThreshold: sampleThreshold,
	}, nil
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

// Ingest takes the span and adds it to Kafka producer for async flushing. The
// flushing is driven by the settings from KafkaSpanSink's constructor. Tune
// the bytes, messages and interval settings to your tastes!
func (k *KafkaSpanSink) Ingest(span *ssf.SSFSpan) error {

	// if we've set a sampleTag, we need to determine whether a span should be
	// sampled.
	if k.sampleTag != "" {
		var hashKey uint32
		sampleTag, exists := span.Tags[k.sampleTag]
		if !exists {
			// If the span isn't tagged appropriately, we should drop it, regardless
			// of our sample rate.
			k.logger.Debug("Rejected span without appropriate tag")
			return nil
		} else {
			// Lifted from https://github.com/stathat/consistent/blob/75142be0209ec69bb014c7a1ac7d1a3c892c6424/consistent.go#L238-L245:
			// if the sample tag value that we're hashing is shorter than 64 bytes, we
			// need to pad it with zeroes for the crc32.ChecksumIEEE function.
			if len(sampleTag) < 64 {
				var scratch [64]byte
				copy(scratch[:], sampleTag)
				hashKey = crc32.ChecksumIEEE(scratch[:len(sampleTag)])
			} else {
				hashKey = crc32.ChecksumIEEE([]byte(sampleTag))
			}
			// Reject any spans whose hash keys end up greater than the threshold that
			// we previously computed.
			if hashKey > k.sampleThreshold {
				k.logger.WithField("hashKey", hashKey).WithField("sampleThreshold", k.sampleThreshold).Debug("Rejected span with appropriate tag based off of sampling rules")
				return nil
			}
		}
	}

	var enc sarama.Encoder
	switch k.serializer {
	case "json":
		j, err := json.Marshal(span)
		if err != nil {
			k.logger.Error("Error marshalling span")
			k.statsd.Count("kafka.span_marshal_error_total", 1, nil, 1.0)
			return err
		}
		enc = sarama.StringEncoder(j)
	case "protobuf":
		p, err := proto.Marshal(span)
		if err != nil {
			k.logger.Error("Error marshalling span")
			k.statsd.Count("kafka.span_marshal_error_total", 1, nil, 1.0)
			return err
		}
		enc = sarama.ByteEncoder(p)
	default:
		return fmt.Errorf("Unknown serialization format for encoding Kafka message: %s", k.serializer)
	}

	k.producer.Input() <- &sarama.ProducerMessage{
		Topic: k.topic,
		Value: enc,
	}
	atomic.AddInt64(&k.spansFlushed, 1)

	return nil
}

// Flush emits metrics, since the spans have already been ingested and are
// sending async.
func (k *KafkaSpanSink) Flush() {
	// TODO We have no stuff in here for detecting failed writes from the async
	// producer. We should add that.
	k.statsd.Count("kafka.spans_flushed_total", atomic.LoadInt64(&k.spansFlushed), nil, 1.0)
	atomic.SwapInt64(&k.spansFlushed, 0)
}
