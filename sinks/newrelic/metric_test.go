package newrelic

import (
	"context"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stripe/veneur/v14"
	"github.com/stripe/veneur/v14/samplers"
	"github.com/stripe/veneur/v14/ssf"
	"github.com/stripe/veneur/v14/trace"
	"github.com/stripe/veneur/v14/util"
)

func TestCreateMetricSink(t *testing.T) {
	t.Parallel()

	logger := logrus.NewEntry(logrus.New())
	sink, err := CreateMetricSink(
		nil, "newrelic", logger, veneur.Config{}, NewRelicMetricSinkConfig{
			AccountID:             testNewRelicAccount,
			CommonTags:            []string{},
			EventType:             testNewRelicEventType,
			InsertKey:             util.StringSecret{Value: testNewRelicApiKey},
			Region:                testNewRelicRegion,
			ServiceCheckEventType: testNewRelicServiceEventType,
		})

	require.NotNil(t, sink)
	assert.NoError(t, err)
	assert.Equal(t, "newrelic", sink.Name())

	newRelicSink := sink.(*NewRelicMetricSink)

	assert.NotNil(t, newRelicSink.log)
	assert.Equal(t, logger, newRelicSink.log)
	assert.NotNil(t, newRelicSink.harvester)
}

func TestNewRelicMetricSink_Start(t *testing.T) {
	t.Parallel()

	logger := logrus.NewEntry(logrus.New())
	sink, err := CreateMetricSink(
		nil, "newrelic", logger, veneur.Config{}, NewRelicMetricSinkConfig{
			AccountID:             testNewRelicAccount,
			CommonTags:            []string{},
			EventType:             testNewRelicEventType,
			InsertKey:             util.StringSecret{Value: testNewRelicApiKey},
			Region:                testNewRelicRegion,
			ServiceCheckEventType: testNewRelicServiceEventType,
		})

	require.NotNil(t, sink)
	require.NoError(t, err)

	tc := &trace.Client{}
	err = sink.Start(tc)
	assert.NoError(t, err)
}

// Integration test, to be implemented
func TestNewRelicMetricSink_Flush(t *testing.T) {
	t.Parallel()

	logger := logrus.NewEntry(logrus.New())
	sink, err := CreateMetricSink(
		nil, "newrelic", logger, veneur.Config{}, NewRelicMetricSinkConfig{
			AccountID:             testNewRelicAccount,
			CommonTags:            []string{},
			EventType:             testNewRelicEventType,
			InsertKey:             util.StringSecret{Value: testNewRelicApiKey},
			Region:                testNewRelicRegion,
			ServiceCheckEventType: testNewRelicServiceEventType,
		})

	require.NotNil(t, sink)
	require.NoError(t, err)

	samples := []samplers.InterMetric{}

	sink.Flush(context.TODO(), samples)
}

// Integration test, to be implemented
func TestNewRelicMetricSink_FlushOtherSamples(t *testing.T) {
	t.Parallel()

	logger := logrus.NewEntry(logrus.New())
	sink, err := CreateMetricSink(
		nil, "newrelic", logger, veneur.Config{}, NewRelicMetricSinkConfig{
			AccountID:             testNewRelicAccount,
			CommonTags:            []string{},
			EventType:             testNewRelicEventType,
			InsertKey:             util.StringSecret{Value: testNewRelicApiKey},
			Region:                testNewRelicRegion,
			ServiceCheckEventType: testNewRelicServiceEventType,
		})

	require.NotNil(t, sink)
	require.NoError(t, err)

	samples := []ssf.SSFSample{}

	sink.FlushOtherSamples(context.TODO(), samples)
}
