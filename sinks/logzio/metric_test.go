package logzio

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stripe/veneur/samplers"
	"github.com/stripe/veneur/ssf"
	"github.com/stripe/veneur/trace"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const (
	testLogzioToken          = "testTokentestTestTestTestTokenTe"
	testLogzioRegion         = "us"
	testLogzioCustomListener = "http://example.com:9200"
	testSplitTagKey          = 0
	testSplitTagValue        = 1
)

var dimensions = map[string]string{
	"dim1": "value1",
	"dim2": "value2",
}

func TestNewLogzioMetricSinkWithLogzioRegion(t *testing.T) {
	t.Parallel()

	logger := logrus.New()
	logzSink, err := NewLogzioMetricSink(testLogzioToken, testLogzioToken, testLogzioRegion, "", logger, dimensions)

	require.NotNil(t, logzSink)
	assert.NoError(t, err)

	assert.NotNil(t, logzSink.log)
	assert.Equal(t, logger, logzSink.log)
	assert.NotNil(t, logzSink.metricSender)
	assert.NotNil(t, logzSink.eventSender)

	assert.Equal(t, "logzio", logzSink.Name())
}

func TestNewLogzioMetricSinkWithLogzioCustomListener(t *testing.T) {
	t.Parallel()

	logger := logrus.New()
	logzSink, err := NewLogzioMetricSink(testLogzioToken, testLogzioToken, "", testLogzioCustomListener, logger, dimensions)

	require.NotNil(t, logzSink)
	assert.NoError(t, err)

	assert.NotNil(t, logzSink.log)
	assert.Equal(t, logger, logzSink.log)
	assert.NotNil(t, logzSink.metricSender)
	assert.NotNil(t, logzSink.eventSender)

	assert.Equal(t, "logzio", logzSink.Name())
}

func TestLogzioMetricsSink_Start(t *testing.T) {
	t.Parallel()

	logger := logrus.New()
	logzSink, err := NewLogzioMetricSink(testLogzioToken, testLogzioToken, testLogzioRegion, "", logger, dimensions)

	require.NotNil(t, logzSink)
	require.NoError(t, err)

	traceClient := &trace.Client{}
	err = logzSink.Start(traceClient)
	assert.NoError(t, err)
}

func TestLogzioMetricsSink_Flush(t *testing.T) {
	t.Parallel()

	logger := logrus.New()
	logzSink, err := NewLogzioMetricSink(testLogzioToken, testLogzioToken, testLogzioRegion, "", logger, dimensions)

	require.NotNil(t, logzSink)
	require.NoError(t, err)

	testMetric := samplers.InterMetric{
		Name:      "test.metric",
		Type:      samplers.StatusMetric,
		Message:   "test",
		Timestamp: 1604487565,
		Value:     123,
		Tags: []string{
			"tag:val",
			"foo:bar",
		},
	}

	err = logzSink.Flush(context.TODO(), []samplers.InterMetric{testMetric})
	assert.NoError(t, err)
}

func TestLogzioMetricsSink_FlushOtherSamples(t *testing.T) {
	t.Parallel()

	logger := logrus.New()
	logzSink, err := NewLogzioMetricSink(testLogzioToken, testLogzioToken, testLogzioRegion, "", logger, dimensions)

	require.NotNil(t, logzSink)
	require.NoError(t, err)

	testSample := ssf.SSFSample{
		Name:      "test.sample",
		Message:   "test message",
		Status:    ssf.SSFSample_WARNING,
		Timestamp: 1604487565,
		Tags: map[string]string{
			"tag": "val",
			"foo": "bar",
		},
	}

	logzSink.FlushOtherSamples(context.TODO(), []ssf.SSFSample{testSample})
	assert.NoError(t, err)
}

func TestNewLogzioMetricSinkWithInvalidShippingToken(t *testing.T) {
	t.Parallel()

	invalidTokens := []string{
		"34234786471271498130478163476257",
		"acnDFejkjdfmksd3fdkDSslfjdEdjkdj",
		"ahjksfhgishfgnsfjnvjgksfhgjhsfsw",
		"DMKDNFKSJDHFKNSDMNVKSDNFLJKSKHGA",
		"dkmsdjWEGRSFGskldsdDmfSDFksfkDs"}

	for _, token := range invalidTokens {
		t.Run(token, func(tt *testing.T) {
			logger := logrus.New()
			logzSink, err := NewLogzioMetricSink(token, token, testLogzioRegion, "", logger, dimensions)

			require.Nil(tt, logzSink)
			assert.Error(tt, err)
		})
	}
}

func TestNewLogzioMetricSinkWithInvalidRegionCode(t *testing.T) {
	t.Parallel()

	invalidListeners := []string{
		"aer",
		"fo",
	}

	for _, listener := range invalidListeners {
		t.Run(listener, func(tt *testing.T) {
			logger := logrus.New()
			logzSink, err := NewLogzioMetricSink(testLogzioToken, testLogzioToken, listener, "", logger, dimensions)

			require.Nil(tt, logzSink)
			assert.Error(tt, err)
		})
	}
}

func TestCreateLogzioMetric(t *testing.T) {
	t.Parallel()

	testMetric := samplers.InterMetric{
		Name:      "test.metric",
		Type:      samplers.StatusMetric,
		Message:   "test",
		Timestamp: 1604487565,
		Value:     123,
		Tags: []string{
			"tag:val",
			"foo:bar",
		},
	}

	logger := logrus.New()
	logzSink, err := NewLogzioMetricSink(testLogzioToken, testLogzioToken, testLogzioRegion, "", logger, dimensions)

	require.NotNil(t, logzSink)
	require.NoError(t, err)

	logzMetric := CreateLogzioMetric(testMetric, logzSink)

	require.NotNil(t, logzMetric)
	require.Equal(t, map[string]float64{testMetric.Name: testMetric.Value}, logzMetric.Metric)
	require.Equal(t, time.Unix(testMetric.Timestamp, 0).UTC().Format(time.RFC3339), logzMetric.Timestamp)
	require.Equal(t, logzioType, logzMetric.MetricType)

	for _, tag := range testMetric.Tags {
		splitTag := strings.Split(tag, ":")
		require.Equal(t, splitTag[testSplitTagValue], logzMetric.Dimensions[splitTag[testSplitTagKey]])
	}
}

func TestCreateLogzioEvent(t *testing.T) {
	t.Parallel()

	testSample := ssf.SSFSample{
		Name:      "test.sample",
		Message:   "test message",
		Status:    ssf.SSFSample_WARNING,
		Timestamp: 1604487565,
		Tags: map[string]string{
			"tag": "val",
			"foo": "bar",
		},
	}

	logger := logrus.New()
	logzSink, err := NewLogzioMetricSink(testLogzioToken, testLogzioToken, testLogzioRegion, "", logger, dimensions)

	require.NotNil(t, logzSink)
	require.NoError(t, err)

	logzEvent := CreateLogzioEvent(testSample)

	require.NotNil(t, logzEvent)
	require.Equal(t, testSample.Name, logzEvent["name"])
	require.Equal(t, testSample.Message, logzEvent["message"])
	require.Equal(t, testSample.Status, logzEvent["status"])
	require.Equal(t, time.Unix(testSample.Timestamp, 0).UTC().Format(time.RFC3339), logzEvent["@timestamp"])

	for key := range testSample.Tags {
		eventKey := fmt.Sprintf("veneur.tags.%s", key)
		require.Equal(t, testSample.Tags[key], logzEvent[eventKey])
	}
}

func TestLogzioSender(t *testing.T) {
	t.Parallel()
	var recordedRequests []byte
	logger := logrus.New()

	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		recordedRequests, _ = ioutil.ReadAll(req.Body)
		rw.WriteHeader(http.StatusOK)
	}))

	require.NotNil(t, server)
	defer server.Close()

	logzSink, err := NewLogzioMetricSink(testLogzioToken, testLogzioToken, "", server.URL, logger, dimensions)

	require.NotNil(t, logzSink)
	require.NoError(t, err)

	testMetric := samplers.InterMetric{
		Name:      "test.metric",
		Type:      samplers.StatusMetric,
		Message:   "test",
		Timestamp: 1604487565,
		Value:     123,
		Tags: []string{
			"tag:val",
			"foo:bar",
		},
	}

	err = logzSink.Flush(context.TODO(), []samplers.InterMetric{testMetric})
	assert.NoError(t, err)

	requests := strings.Split(string(recordedRequests), "\n")
	var logzMetric LogzioMetric
	assert.NoError(t, json.Unmarshal([]byte(requests[0]), &logzMetric))
	require.Equal(t, map[string]float64{testMetric.Name: testMetric.Value}, logzMetric.Metric)
	require.Equal(t, time.Unix(testMetric.Timestamp, 0).UTC().Format(time.RFC3339), logzMetric.Timestamp)
	require.Equal(t, logzioType, logzMetric.MetricType)

	for _, tag := range testMetric.Tags {
		splitTag := strings.Split(tag, ":")
		require.Equal(t, splitTag[testSplitTagValue], logzMetric.Dimensions[splitTag[testSplitTagKey]])
	}
}
