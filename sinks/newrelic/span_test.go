package newrelic

import (
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stripe/veneur/v14/ssf"
	"github.com/stripe/veneur/v14/trace"
)

func TestNewNewRelicSpanSink(t *testing.T) {
	t.Parallel()

	logger := logrus.New()
	s, err := NewNewRelicSpanSink(testNewRelicApiKey, testNewRelicAccount, testNewRelicRegion, []string{}, testNewRelicSpanURL, logger)

	require.NotNil(t, s)
	assert.NoError(t, err)

	assert.NotNil(t, s.log)
	assert.Equal(t, logger, s.log)
	assert.NotNil(t, s.harvester)

	assert.Equal(t, "newrelic", s.Name())
}

func TestNewRelicSpanSink_Start(t *testing.T) {
	t.Parallel()

	logger := logrus.New()
	s, err := NewNewRelicSpanSink(testNewRelicApiKey, testNewRelicAccount, testNewRelicRegion, []string{}, testNewRelicSpanURL, logger)

	require.NotNil(t, s)
	require.NoError(t, err)

	tc := &trace.Client{}
	err = s.Start(tc)
	assert.NoError(t, err)
}

// Integration test, to be implemented
func TestNewRelicSpanSink_Ingest(t *testing.T) {
	t.Parallel()

	logger := logrus.New()
	s, err := NewNewRelicSpanSink(testNewRelicApiKey, testNewRelicAccount, testNewRelicRegion, []string{}, testNewRelicSpanURL, logger)

	require.NotNil(t, s)
	require.NoError(t, err)

	ssfSpan := &ssf.SSFSpan{}

	err = s.Ingest(ssfSpan)
	assert.Error(t, err)
}

// Integration test, to be implemented
func TestNewRelicSpanSink_Flush(t *testing.T) {
	t.Parallel()

	logger := logrus.New()
	s, err := NewNewRelicSpanSink(testNewRelicApiKey, testNewRelicAccount, testNewRelicRegion, []string{}, testNewRelicSpanURL, logger)

	require.NotNil(t, s)
	require.NoError(t, err)

	s.Flush()
}
