package veneur

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewListener(t *testing.T) {

	config := localConfig()

	// circonus listener
	listenerType := "circonus"
	_, err := NewListener(listenerType, config)
	assert.NoError(t, err, "circonus listener should have created successfully")

	// datadog listener
	listenerType = "datadog"
	_, err = NewListener(listenerType, config)
	assert.NoError(t, err, "datadog listener should have created successfully")

	// prometheus listener
	listenerType = "prometheus"
	_, err = NewListener(listenerType, config)
	assert.NoError(t, err, "prometheus listener should have created successfully")

	// statsd listener
	listenerType = "statsd"
	_, err = NewListener(listenerType, config)
	assert.NoError(t, err, "statsd listener should have created successfully")
}
