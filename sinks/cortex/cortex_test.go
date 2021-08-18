package cortex

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stripe/veneur/v14/sinks"
)

func TestName(t *testing.T) {
	// Implicitly test that CortexMetricsSink implements MetricSink
	var sink sinks.MetricSink
	sink, err := NewCortexMetricSink()

	assert.NoError(t, err)
	assert.Equal(t, "cortex", sink.Name())
}
