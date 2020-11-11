package logzio

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/logzio/logzio-go"
	"github.com/sirupsen/logrus"
	"github.com/stripe/veneur/samplers"
	"github.com/stripe/veneur/sinks"
	"github.com/stripe/veneur/ssf"
	"github.com/stripe/veneur/trace"
)

type LogzioMetricsSink struct {
	logzioMetricsToken string
	logzioLogsToken    string
	logzioListener     string
	dimensions         map[string]string
	traceClient        *trace.Client
	log                *logrus.Logger
	metricSender       *logzio.LogzioSender
	eventSender        *logzio.LogzioSender
}

type LogzioMetric struct {
	Metric     map[string]float64 `json:"metrics"`
	Dimensions map[string]string  `json:"dimensions,omitempty"`
	MetricType string             `json:"type"`
	Timestamp  string             `json:"@timestamp"`
}

const (
	defaultFlushInterval = 10 * time.Second
	logzioType           = "veneur"
	logzioListenerPort   = "8070"
)

var logzioRegionCodes = []string{"us", "au", "ca", "eu", "nl", "uk", "wa"}

var _ sinks.MetricSink = &LogzioMetricsSink{}

// NewLogzioMetricSink creates a new LogzioMetricsSink that sends data to Logzio
func NewLogzioMetricSink(logzioMetricsToken string, logzioLogsToken string, logzioRegion string, logzioCustomListener string, log *logrus.Logger, dimensions map[string]string) (*LogzioMetricsSink, error) {

	tokensErr := ValidTokens(logzioMetricsToken, logzioLogsToken)

	if tokensErr != nil {
		return nil, tokensErr
	}

	listener, listenerErr := GetLogzioListenerUrl(logzioRegion, logzioCustomListener)
	if listenerErr != nil {
		return nil, listenerErr
	}

	metricsSender, errMetrics := CreateLogzioSender(logzioMetricsToken, listener)

	if errMetrics != nil {
		log.WithError(errMetrics).Error("couldn't create logzio sender for metrics")
		return nil, errMetrics
	}

	eventSender, errEvents := CreateLogzioSender(logzioLogsToken, listener)

	if errEvents != nil {
		log.WithError(errEvents).Error("couldn't create logzio sender for events")
		return nil, errEvents
	}

	return &LogzioMetricsSink{
		logzioMetricsToken: logzioMetricsToken,
		logzioLogsToken:    logzioLogsToken,
		logzioListener:     listener,
		log:                log,
		dimensions:         dimensions,
		metricSender:       metricsSender,
		eventSender:        eventSender,
	}, nil
}

// Name returns the name of the sink
func (logz *LogzioMetricsSink) Name() string {
	return "logzio"
}

// Start sets the sink up.
func (logz *LogzioMetricsSink) Start(cl *trace.Client) error {
	logz.traceClient = cl
	return nil
}

// Flush sends metrics to logzio
func (logz *LogzioMetricsSink) Flush(ctx context.Context, metrics []samplers.InterMetric) error {
	span, _ := trace.StartSpanFromContext(ctx, "")
	defer span.ClientFinish(logz.traceClient)

	if len(metrics) <= 0 {
		return nil
	}

	for _, m := range metrics {
		if !sinks.IsAcceptableMetric(m, logz) {
			continue
		}

		metric := CreateLogzioMetric(m, logz)

		metricByte, err := json.Marshal(metric)

		if err != nil {
			logz.log.WithError(err).Error("failed to marshal metric")
		}

		logz.metricSender.Send(metricByte)
	}

	logz.metricSender.Drain()

	return nil
}

func CreateLogzioMetric(interMetric samplers.InterMetric, logz *LogzioMetricsSink) LogzioMetric {
	dims := CreateDimensions(interMetric.Tags, logz)
	AddDimensionsFromInterMetric(interMetric, dims)

	metricData := map[string]float64{interMetric.Name: interMetric.Value}
	timestamp := time.Unix(interMetric.Timestamp, 0).UTC().Format(time.RFC3339)
	metric := LogzioMetric{
		Metric:     metricData,
		MetricType: logzioType,
		Dimensions: dims,
		Timestamp:  timestamp,
	}

	return metric
}

// FlushOtherSamples handles non-metric, non-span samples.
func (logz *LogzioMetricsSink) FlushOtherSamples(ctx context.Context, samples []ssf.SSFSample) {
	if len(samples) == 0 {
		return
	}

	span, _ := trace.StartSpanFromContext(ctx, "")
	defer span.ClientFinish(logz.traceClient)

	for _, sample := range samples {
		event := CreateLogzioEvent(sample)

		eventByte, err := json.Marshal(event)
		if err != nil {
			logz.log.WithError(err).Error("failed to marshal event")
		}
		logz.eventSender.Send(eventByte)
	}

	logz.eventSender.Drain()
}

//CreateLogzioEvent converts an SSFSample to a logzio event format and removes empty fields
func CreateLogzioEvent(sample ssf.SSFSample) map[string]interface{} {
	var timestamp string

	if sample.Timestamp == 0 {
		timestamp = time.Now().UTC().Format(time.RFC3339)
	} else {
		timestamp = time.Unix(sample.Timestamp, 0).UTC().Format(time.RFC3339)
	}

	event := map[string]interface{}{
		"type":       logzioType,
		"value":      sample.Value,
		"@timestamp": timestamp,
		"status":     sample.Status,
		"rate":       sample.SampleRate,
		"scope":      sample.Scope,
	}

	// don't add empty fields
	if sample.Name != "" {
		event["name"] = sample.Name
	}

	if sample.Message != "" {
		event["message"] = sample.Message
	}

	if sample.Unit != "" {
		event["unit"] = sample.Unit
	}

	// each tag will be in its own field.
	// for example, the tag `foo: bar` will appear in logzio as the field: `veneur.tags.foo: bar`
	for tagKey, tagVal := range sample.Tags {
		eventKey := "veneur.tags." + tagKey
		event[eventKey] = tagVal
	}

	return event
}

//CreateDimensions converts tags to dimensions
func CreateDimensions(tags []string, logz *LogzioMetricsSink) map[string]string {
	dims := map[string]string{}

	for _, tag := range tags {
		t := strings.Split(tag, ":")
		if len(t) < 2 {
			logz.log.Warn(fmt.Sprintf("tag is not in a key:value form. dropping tag: %s", tag))
			continue
		}

		dims[t[0]] = t[1]
	}

	if len(logz.dimensions) > 0 {
		AddDimensionsFromClient(logz.dimensions, dims)
	}

	return dims
}

//AddDimensionsFromClient adds dimensions from config
func AddDimensionsFromClient(dimsFromSink map[string]string, dimsMap map[string]string) {
	for key, value := range dimsFromSink {
		dimsMap[key] = value
	}
}

func CreateLogzioSender(token string, listener string) (*logzio.LogzioSender, error) {
	sender, err := logzio.New(
		token,
		logzio.SetUrl(listener),
		logzio.SetDrainDuration(defaultFlushInterval),
	)

	if err != nil {
		return nil, err
	}

	return sender, nil
}

//AddDimensionsFromInterMetric adds dimensions to logzio metric from fields in the InterMetric
func AddDimensionsFromInterMetric(im samplers.InterMetric, dims map[string]string) {
	if im.Message != "" {
		dims["message"] = im.Message
	}

	if im.HostName != "" {
		dims["hostname"] = im.HostName
	}

	if im.Type.String() != "" {
		dims["metric_type"] = im.Type.String()
	}
}

func IsShippingTokenValid(token string) bool {
	hasUpper := false
	hasLower := false

	if len(token) != 32 {
		return false
	}

	for _, r := range token {
		if unicode.IsUpper(r) {
			hasUpper = true
		} else if unicode.IsLower(r) {
			hasLower = true
		} else {
			return false
		}
	}

	if hasLower && hasUpper {
		return true
	}

	return false
}

func ValidTokens(metricsToken string, logsToken string) error {
	isMetricsTokenValid := IsShippingTokenValid(metricsToken)

	if !isMetricsTokenValid {
		return errors.New("invalid logzio metrics token")
	}

	isLogsTokenValid := IsShippingTokenValid(logsToken)

	if !isLogsTokenValid {
		return errors.New("invalid logzio logs token")
	}

	return nil
}

func GetLogzioListenerUrl(regionCode string, customListener string) (string, error) {
	if customListener != "" {
		return customListener, nil
	}

	isInRegionList := false
	for _, region := range logzioRegionCodes {
		if regionCode == region {
			isInRegionList = true
			break
		}
	}

	if !isInRegionList {
		return "", errors.New("logzio region code is invalid")
	}

	regionSuffix := ""
	if regionCode != "us" {
		regionSuffix = fmt.Sprintf("-%s", regionCode)
	}

	listenerUrl := fmt.Sprintf("http://listener%s.logz.io:%s", regionSuffix, logzioListenerPort)

	return listenerUrl, nil
}
