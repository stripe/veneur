package dplocal

import (
	"errors"
	"os"
	"testing"

	"time"

	"github.com/signalfx/golib/datapoint"
	"github.com/stretchr/testify/assert"
)

func TestNewOnHostDatapoint(t *testing.T) {
	hostname, _ := os.Hostname()

	dp1 := NewOnHostDatapoint("metrica", nil, datapoint.Counter)
	assert.Equal(t, "metrica", dp1.Metric)
	dp := Wrap(datapoint.New("metric", nil, datapoint.NewFloatValue(3.0), datapoint.Counter, time.Now()))
	assert.Equal(t, hostname, dp.Dimensions["host"], "Should get source back")

	osXXXHostname = func() (string, error) { return "", errors.New("unable to get hostname") }

	dp = Wrap(datapoint.New("metric", map[string]string{}, datapoint.NewFloatValue(3.0), datapoint.Counter, time.Now()))
	assert.Equal(t, "unknown", dp.Dimensions["host"], "Should get source back")
	osXXXHostname = os.Hostname
}
