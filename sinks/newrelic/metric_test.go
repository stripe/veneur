package newrelic

import (
	"context"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stripe/veneur/v14/samplers"
	"github.com/stripe/veneur/v14/ssf"
	"github.com/stripe/veneur/v14/trace"
)

func TestNewNewRelicMetricSink(t *testing.T) {
	t.Parallel()

	logger := logrus.New()
	s, err := NewNewRelicMetricSink(testNewRelicApiKey, testNewRelicAccount, testNewRelicRegion, testNewRelicEventType, []string{}, logger, testNewRelicServiceEventType)

	require.NotNil(t, s)
	assert.NoError(t, err)

	assert.NotNil(t, s.log)
	assert.Equal(t, logger, s.log)
	assert.NotNil(t, s.harvester)

	assert.Equal(t, "newrelic", s.Name())
}

func TestNewRelicMetricSink_Start(t *testing.T) {
	t.Parallel()

	logger := logrus.New()
	s, err := NewNewRelicMetricSink(testNewRelicApiKey, testNewRelicAccount, testNewRelicRegion, testNewRelicEventType, []string{}, logger, testNewRelicServiceEventType)

	require.NotNil(t, s)
	require.NoError(t, err)

	tc := &trace.Client{}
	err = s.Start(tc)
	assert.NoError(t, err)
}

// Integration test, to be implemented
func TestNewRelicMetricSink_Flush(t *testing.T) {
	t.Parallel()

	logger := logrus.New()
	s, err := NewNewRelicMetricSink(testNewRelicApiKey, testNewRelicAccount, testNewRelicRegion, testNewRelicEventType, []string{}, logger, testNewRelicServiceEventType)

	require.NotNil(t, s)
	require.NoError(t, err)

	samples := []samplers.InterMetric{}

	s.Flush(context.TODO(), samples)
}

// Integration test, to be implemented
func TestNewRelicMetricSink_FlushOtherSamples(t *testing.T) {
	t.Parallel()

	logger := logrus.New()
	s, err := NewNewRelicMetricSink(testNewRelicApiKey, testNewRelicAccount, testNewRelicRegion, testNewRelicEventType, []string{}, logger, testNewRelicServiceEventType)

	require.NotNil(t, s)
	require.NoError(t, err)

	samples := []ssf.SSFSample{}

	s.FlushOtherSamples(context.TODO(), samples)
}
