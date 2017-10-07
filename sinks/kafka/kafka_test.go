package kafka

import (
	"context"
	"testing"

	"github.com/DataDog/datadog-go/statsd"
	"github.com/Shopify/sarama/mocks"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stripe/veneur/samplers"
)

func TestFlushSuccess(t *testing.T) {
	producerMock := mocks.NewAsyncProducer(t, nil)
	producerMock.ExpectInputAndSucceed()

	// I would use the logrus test logger but the package needs to be
	// updated from Sirupsen/logrus to sirupsen/logrus
	// https://github.com/stripe/veneur/issues/277
	logger := logrus.StandardLogger()
	stats, _ := statsd.NewBuffered("localhost:1235", 1024)

	sink := NewKafkaMetricSink(logger, "testing", "testCheckTopic", "testEventTopic", "testMetricTopic", "all", "hash", 0, 0, 0, "", "json", stats)

	sink.producer = producerMock
	err := sink.Flush(context.Background(), []samplers.InterMetric{
		samplers.InterMetric{
			Name:      "a.b.c.max",
			Timestamp: 1476119058,
			Value:     float64(100),
			Tags: []string{
				"foo:bar",
				"baz:quz",
			},
			Type: samplers.GaugeMetric,
		},
	})

	assert.NoError(t, err)
}
