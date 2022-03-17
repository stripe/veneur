package kafka

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"hash/crc32"
	"math"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/Shopify/sarama"
	"github.com/gogo/protobuf/proto"
	gometrics "github.com/rcrowley/go-metrics"
	"github.com/sirupsen/logrus"
	veneur "github.com/stripe/veneur/v14"
	"github.com/stripe/veneur/v14/samplers"
	"github.com/stripe/veneur/v14/sinks"
	"github.com/stripe/veneur/v14/ssf"
	"github.com/stripe/veneur/v14/trace"
	"github.com/stripe/veneur/v14/trace/metrics"
	"github.com/stripe/veneur/v14/util"
)

func init() {
	gometrics.UseNilMetrics = true
}

const IngestTimeout = 5 * time.Second

var ErrIngestTimeout = errors.New("timed out writing to Kafka producer")

type KafkaMetricSinkConfig struct {
	Broker                string        `yaml:"broker"`
	CheckTopic            string        `yaml:"check_topic"`
	EventTopic            string        `yaml:"event_topic"`
	MetricBufferBytes     int           `yaml:"metric_buffer_bytes"`
	MetricBufferFrequency time.Duration `yaml:"metric_buffer_frequency"`
	MetricBufferMessages  int           `yaml:"metric_buffer_messages"`
	MetricRequireAcks     string        `yaml:"metric_require_acks"`
	MetricTopic           string        `yaml:"metric_topic"`
	Partitioner           string        `yaml:"partitioner"`
	RetryMax              int           `yaml:"retry_max"`
}

type KafkaMetricSink struct {
	brokers     string
	checkTopic  string
	config      *sarama.Config
	eventTopic  string
	logger      *logrus.Entry
	metricTopic string
	name        string
	producer    sarama.AsyncProducer
	traceClient *trace.Client
}

type KafkaSpanSinkConfig struct {
	Broker                  string        `yaml:"broker"`
	Partitioner             string        `yaml:"partitioner"`
	RetryMax                int           `yaml:"retry_max"`
	SpanBufferBytes         int           `yaml:"span_buffer_bytes"`
	SpanBufferFrequency     time.Duration `yaml:"span_buffer_frequency"`
	SpanBufferMesages       int           `yaml:"span_buffer_mesages"`
	SpanRequireAcks         string        `yaml:"span_require_acks"`
	SpanSampleRatePercent   float64       `yaml:"span_sample_rate_percent"`
	SpanSampleTag           string        `yaml:"span_sample_tag"`
	SpanSerializationFormat string        `yaml:"span_serialization_format"`
	SpanTopic               string        `yaml:"span_topic"`
}

type KafkaSpanSink struct {
	brokers         string
	config          *sarama.Config
	logger          *logrus.Entry
	name            string
	producer        sarama.AsyncProducer
	sampleTag       string
	sampleThreshold uint32
	serializer      string
	spansFlushed    int64
	topic           string
	traceClient     *trace.Client
}

func MigrateConfig(conf *veneur.Config) error {
	if conf.KafkaBroker == "" {
		return nil
	}
	if conf.KafkaMetricTopic != "" ||
		conf.KafkaCheckTopic != "" ||
		conf.KafkaEventTopic != "" {
		conf.MetricSinks = append(conf.MetricSinks, veneur.SinkConfig{
			Kind: "kafka",
			Name: "kafka",
			Config: KafkaMetricSinkConfig{
				Broker:                conf.KafkaBroker,
				CheckTopic:            conf.KafkaCheckTopic,
				EventTopic:            conf.KafkaEventTopic,
				MetricBufferBytes:     conf.KafkaMetricBufferBytes,
				MetricBufferFrequency: conf.KafkaMetricBufferFrequency,
				MetricBufferMessages:  conf.KafkaMetricBufferMessages,
				MetricRequireAcks:     conf.KafkaMetricRequireAcks,
				MetricTopic:           conf.KafkaMetricTopic,
				Partitioner:           conf.KafkaPartitioner,
				RetryMax:              conf.KafkaRetryMax,
			},
		})
	}

	if conf.KafkaSpanTopic != "" {
		conf.SpanSinks = append(conf.SpanSinks, veneur.SinkConfig{
			Kind: "kafka",
			Name: "kafka",
			Config: KafkaSpanSinkConfig{
				Broker:                  conf.KafkaBroker,
				Partitioner:             conf.KafkaPartitioner,
				RetryMax:                conf.KafkaRetryMax,
				SpanBufferBytes:         conf.KafkaSpanBufferBytes,
				SpanBufferFrequency:     conf.KafkaSpanBufferFrequency,
				SpanBufferMesages:       conf.KafkaSpanBufferMesages,
				SpanRequireAcks:         conf.KafkaSpanRequireAcks,
				SpanSampleRatePercent:   conf.KafkaSpanSampleRatePercent,
				SpanSampleTag:           conf.KafkaSpanSampleTag,
				SpanSerializationFormat: conf.KafkaSpanSerializationFormat,
				SpanTopic:               conf.KafkaSpanTopic,
			},
		})
	}
	return nil
}

// ParseMetricConfig decodes the map config for a Kafka metric sink into a
// KafkaMetricSinkConfig struct.
func ParseMetricConfig(
	name string, config interface{},
) (veneur.MetricSinkConfig, error) {
	kafkaConfig := KafkaMetricSinkConfig{}
	err := util.DecodeConfig(name, config, &kafkaConfig)
	if err != nil {
		return nil, err
	}
	return kafkaConfig, nil
}

// CreateMetricSink creates a new Kafka sink for metrics. This function
// should match the signature of a value in veneur.MetricSinkTypes, and is
// intended to be passed into veneur.NewFromConfig to be called based on the
// provided configuration.
func CreateMetricSink(
	server *veneur.Server, name string, logger *logrus.Entry,
	config veneur.Config, sinkConfig veneur.MetricSinkConfig,
) (sinks.MetricSink, error) {
	kafkaConfig := sinkConfig.(KafkaMetricSinkConfig)

	if kafkaConfig.Broker == "" {
		return nil, errors.New("cannot start Kafka metric sink with no broker")
	}
	if kafkaConfig.CheckTopic == "" &&
		kafkaConfig.EventTopic == "" &&
		kafkaConfig.MetricTopic == "" {
		return nil, errors.New(
			"cannot start Kafka metic sink with no valid topic names")
	}

	logger.WithField("config", fmt.Sprintf("%+v", kafkaConfig)).
		Info("Created Kafka metric sink")

	return &KafkaMetricSink{
		brokers:    kafkaConfig.Broker,
		checkTopic: kafkaConfig.CheckTopic,
		config: newProducerConfig(
			kafkaConfig.MetricRequireAcks,
			kafkaConfig.Partitioner,
			kafkaConfig.RetryMax,
			kafkaConfig.MetricBufferBytes,
			kafkaConfig.MetricBufferMessages,
			kafkaConfig.MetricBufferFrequency,
		),
		eventTopic:  kafkaConfig.EventTopic,
		logger:      logger,
		metricTopic: kafkaConfig.MetricTopic,
		name:        name,
		traceClient: server.TraceClient,
	}, nil
}

func newProducerConfig(
	ackRequirement string, partitioner string, retries int, bufferBytes int,
	bufferMessages int, bufferFrequency time.Duration,
) *sarama.Config {
	config := sarama.NewConfig()
	switch ackRequirement {
	case "all":
		config.Producer.RequiredAcks = sarama.WaitForAll
	case "none":
		config.Producer.RequiredAcks = sarama.NoResponse
	case "local":
		config.Producer.RequiredAcks = sarama.WaitForLocal
	default:
		logrus.WithField("ack_requirement", ackRequirement).
			Warn("Unknown ack requirement, defaulting to all")
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

	// If either of these is set to true, you must
	// read from the corresponding channels in a separate
	// goroutine. Otherwise, the entire sink will back up.
	config.Producer.Return.Successes = false
	config.Producer.Return.Errors = false

	return config
}

// newConfiguredProducer returns a configured Sarama SyncProducer
func newConfiguredProducer(
	logger *logrus.Entry, brokerString string, config *sarama.Config,
) (sarama.AsyncProducer, error) {
	brokerList := strings.Split(brokerString, ",")

	if len(brokerList) < 1 {
		logger.WithField("addrs", brokerString).Error("No brokers?")
		return nil, errors.New("no brokers in broker list")
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
	return k.name
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
	samples := &ssf.Samples{}
	defer metrics.Report(k.traceClient, samples)

	if len(interMetrics) == 0 {
		k.logger.Info("Nothing to flush, skipping.")
		return nil
	}

	successes := int64(0)
	for _, metric := range interMetrics {
		if !sinks.IsAcceptableMetric(metric, k) {
			continue
		}

		k.logger.Debug("Emitting Metric: ", metric.Name)
		j, err := json.Marshal(metric)
		if err != nil {
			k.logger.Error("Error marshalling metric: ", metric.Name)
			samples.Add(ssf.Count("kafka.marshal.error_total", 1, nil))
			return err
		}

		k.producer.Input() <- &sarama.ProducerMessage{
			Topic: k.metricTopic,
			Value: sarama.StringEncoder(j),
		}
		successes++
	}
	samples.Add(ssf.Count(sinks.MetricKeyTotalMetricsFlushed, float32(successes), map[string]string{"sink": k.Name()}))
	return nil
}

// FlushOtherSamples flushes non-metric, non-span samples
func (k *KafkaMetricSink) FlushOtherSamples(ctx context.Context, samples []ssf.SSFSample) {
	// TODO
}

// ParseSpanConfig decodes the map config for a Kafka span sink into a
// KafkaSpanSinkConfig struct.
func ParseSpanConfig(
	name string, config interface{},
) (veneur.SpanSinkConfig, error) {
	kafkaConfig := KafkaSpanSinkConfig{}
	err := util.DecodeConfig(name, config, &kafkaConfig)
	if err != nil {
		return nil, err
	}
	return kafkaConfig, nil
}

// CreateSpanSink creates a new Kafka sink for spans. This function
// should match the signature of a value in veneur.SpanSinkTypes, and is
// intended to be passed into veneur.NewFromConfig to be called based on the
// provided configuration.
func CreateSpanSink(
	server *veneur.Server, name string, logger *logrus.Entry,
	config veneur.Config, sinkConfig veneur.SpanSinkConfig,
) (sinks.SpanSink, error) {
	kafkaConfig := sinkConfig.(KafkaSpanSinkConfig)

	if kafkaConfig.Broker == "" {
		return nil, errors.New("cannot start Kafka span sink with no broker")
	}
	if kafkaConfig.SpanTopic == "" {
		return nil, errors.New("cannot start Kafka span sink with no span topic")
	}

	serializer := kafkaConfig.SpanSerializationFormat
	if serializer != "json" && serializer != "protobuf" {
		logger.WithField("serializer", serializer).
			Warn("Unknown serializer, defaulting to protobuf")
		serializer = "protobuf"
	}

	var sampleThreshold uint32
	if kafkaConfig.SpanSampleRatePercent < 0 ||
		kafkaConfig.SpanSampleRatePercent > 100 {
		return nil, errors.New(
			"span sample rate percentage must be between 0.0 and 100.0")
	}

	// Set the sample threshold to (sample rate) * (maximum value of uint32), so that
	// we can store it as a uint32 instead of a float64 and compare apples-to-apples
	// with the output of our hashing algorithm.
	sampleThreshold = uint32(
		kafkaConfig.SpanSampleRatePercent * math.MaxUint32 / 100)

	logger.WithField("config", fmt.Sprintf("%+v", kafkaConfig)).
		Info("Started Kafka span sink")

	return &KafkaSpanSink{
		brokers: kafkaConfig.Broker,
		config: newProducerConfig(
			kafkaConfig.SpanRequireAcks,
			kafkaConfig.Partitioner,
			kafkaConfig.RetryMax,
			kafkaConfig.SpanBufferBytes,
			kafkaConfig.SpanBufferMesages,
			kafkaConfig.SpanBufferFrequency,
		),
		logger:          logger,
		name:            name,
		sampleTag:       kafkaConfig.SpanSampleTag,
		sampleThreshold: sampleThreshold,
		serializer:      serializer,
		topic:           kafkaConfig.SpanTopic,
	}, nil
}

// Name returns the name of this sink.
func (k *KafkaSpanSink) Name() string {
	return k.name
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
	samples := &ssf.Samples{}
	defer metrics.Report(k.traceClient, samples)
	// If we're sampling less than 100%, we should check whether a span should
	// be sampled:
	if k.sampleTag != "" || k.sampleThreshold < uint32(math.MaxUint32) {
		var hashKey uint32
		var sampleTagValue string

		if k.sampleTag == "" {
			// If we haven't set a sampleTag, we'll be hashing based on the traceID

			sampleTagValue = strconv.FormatInt(span.TraceId, 10)
		} else {
			// If we've set a sampleTag, we'll be hashing based off of that tag's value.

			var exists bool
			sampleTagValue, exists = span.Tags[k.sampleTag]
			if !exists {
				// If the span isn't tagged appropriately, we should drop it, regardless
				// of our sample rate.
				k.logger.Debug("Rejected span without appropriate tag")
				samples.Add(ssf.Count(sinks.MetricKeyTotalSpansDropped, 1, map[string]string{"sink": k.Name()}))
				return nil
			}
		}

		// Lifted from https://github.com/stathat/consistent/blob/75142be0209ec69bb014c7a1ac7d1a3c892c6424/consistent.go#L238-L245:
		// if the sample tag value that we're hashing is shorter than 64 bytes, we
		// need to pad it with zeroes for the crc32.ChecksumIEEE function.
		if len(sampleTagValue) < 64 {
			var scratch [64]byte
			copy(scratch[:], sampleTagValue)
			hashKey = crc32.ChecksumIEEE(scratch[:len(sampleTagValue)])
		} else {
			hashKey = crc32.ChecksumIEEE([]byte(sampleTagValue))
		}

		// Reject any spans whose hash keys end up greater than the threshold that
		// we previously computed.
		if hashKey > k.sampleThreshold {
			k.logger.WithField("traceId", span.TraceId).WithField("sampleTag", k.sampleTag).WithField("sampleTagValue", sampleTagValue).WithField("hashKey", hashKey).WithField("sampleThreshold", k.sampleThreshold).Debug("Rejected span based off of sampling rules")
			samples.Add(ssf.Count(sinks.MetricKeyTotalSpansSkipped, 1, map[string]string{"sink": k.Name()}))
			return nil
		}
	}
	var enc sarama.Encoder
	switch k.serializer {
	case "json":
		j, err := json.Marshal(span)
		if err != nil {
			k.logger.Error("Error marshalling span")
			samples.Add(ssf.Count("kafka.span_marshal_error_total", 1, nil))
			return err
		}
		enc = sarama.StringEncoder(j)
	case "protobuf":
		p, err := proto.Marshal(span)
		if err != nil {
			k.logger.Error("Error marshalling span")
			samples.Add(ssf.Count("kafka.span_marshal_error_total", 1, nil))
			return err
		}
		enc = sarama.ByteEncoder(p)
	default:
		return fmt.Errorf(
			"unknown serialization format for encoding Kafka message: %s",
			k.serializer)
	}

	message := &sarama.ProducerMessage{
		Topic: k.topic,
		Value: enc,
	}

	select {
	case k.producer.Input() <- message:
		atomic.AddInt64(&k.spansFlushed, 1)
		return nil
	case <-time.After(IngestTimeout):
		return ErrIngestTimeout
	}
}

// Flush emits metrics, since the spans have already been ingested and are
// sending async.
func (k *KafkaSpanSink) Flush() {
	// TODO We have no stuff in here for detecting failed writes from the async
	// producer. We should add that.
	k.logger.WithFields(logrus.Fields{
		"flushed_spans": atomic.LoadInt64(&k.spansFlushed),
	}).Debug("Checkpointing flushed spans for Kafka")
	metrics.ReportOne(k.traceClient, ssf.Count(sinks.MetricKeyTotalSpansFlushed, float32(atomic.LoadInt64(&k.spansFlushed)), map[string]string{"sink": k.Name()}))
	atomic.SwapInt64(&k.spansFlushed, 0)
}
