package newrelic

import (
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stripe/veneur/v14"
	"github.com/stripe/veneur/v14/ssf"
	"github.com/stripe/veneur/v14/trace"
	"github.com/stripe/veneur/v14/util"
)

func TestCreateSpanSink(t *testing.T) {
	t.Parallel()

	logger := logrus.NewEntry(logrus.New())
	sink, err := CreateSpanSink(nil, "newrelic", logger,
		veneur.Config{}, NewRelicSpanSinkConfig{
			CommonTags:       []string{},
			InsertKey:        util.StringSecret{Value: testNewRelicApiKey},
			TraceObserverURL: testNewRelicSpanURL,
		})

	require.NotNil(t, sink)
	assert.NoError(t, err)

	newRelicSink := sink.(*NewRelicSpanSink)

	assert.NotNil(t, newRelicSink.log)
	assert.Equal(t, logger, newRelicSink.log)
	assert.NotNil(t, newRelicSink.harvester)

	assert.Equal(t, "newrelic", newRelicSink.Name())
}

func TestNewRelicSpanSink_Start(t *testing.T) {
	t.Parallel()

	logger := logrus.NewEntry(logrus.New())
	sink, err := CreateSpanSink(nil, "newrelic", logger,
		veneur.Config{}, NewRelicSpanSinkConfig{
			CommonTags:       []string{},
			InsertKey:        util.StringSecret{Value: testNewRelicApiKey},
			TraceObserverURL: testNewRelicSpanURL,
		})

	require.NotNil(t, sink)
	require.NoError(t, err)

	tc := &trace.Client{}
	err = sink.Start(tc)
	assert.NoError(t, err)
}

// Integration test, to be implemented
func TestNewRelicSpanSink_Ingest(t *testing.T) {
	t.Parallel()

	logger := logrus.NewEntry(logrus.New())
	sink, err := CreateSpanSink(nil, "newrelic", logger,
		veneur.Config{}, NewRelicSpanSinkConfig{
			CommonTags:       []string{},
			InsertKey:        util.StringSecret{Value: testNewRelicApiKey},
			TraceObserverURL: testNewRelicSpanURL,
		})

	require.NotNil(t, sink)
	require.NoError(t, err)

	ssfSpan := &ssf.SSFSpan{}

	err = sink.Ingest(ssfSpan)
	assert.Error(t, err)
}

// Integration test, to be implemented
func TestNewRelicSpanSink_Flush(t *testing.T) {
	t.Parallel()

	logger := logrus.NewEntry(logrus.New())
	sink, err := CreateSpanSink(nil, "newrelic", logger,
		veneur.Config{}, NewRelicSpanSinkConfig{
			CommonTags:       []string{},
			InsertKey:        util.StringSecret{Value: testNewRelicApiKey},
			TraceObserverURL: testNewRelicSpanURL,
		})

	require.NotNil(t, sink)
	require.NoError(t, err)

	sink.Flush()
}
