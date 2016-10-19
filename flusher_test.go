package veneur

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stripe/veneur/samplers"
)

func TestServerTags(t *testing.T) {
	metrics := []samplers.DDMetric{{
		Name:       "foo.bar.baz",
		Value:      [1][2]float64{{float64(time.Now().Unix()), float64(1.0)}},
		Tags:       []string{"gorch:frobble", "x:e"},
		MetricType: "rate",
		Interval:   10,
	}}

	finalizeMetrics("somehostname", []string{"a:b", "c:d"}, metrics)
	assert.Equal(t, "somehostname", metrics[0].Hostname, "Metric hostname uses argument")
	assert.Contains(t, metrics[0].Tags, "a:b", "Tags should contain server tags")
}

func TestHostMagicTag(t *testing.T) {
	metrics := []samplers.DDMetric{{
		Name:       "foo.bar.baz",
		Value:      [1][2]float64{{float64(time.Now().Unix()), float64(1.0)}},
		Tags:       []string{"gorch:frobble", "host:abc123", "x:e"},
		MetricType: "rate",
		Interval:   10,
	}}

	finalizeMetrics("badhostname", []string{"a:b", "c:d"}, metrics)
	assert.Equal(t, "abc123", metrics[0].Hostname, "Metric hostname should be from tag")
	assert.NotContains(t, metrics[0].Tags, "host:abc123", "Host tag should be removed")
	assert.Contains(t, metrics[0].Tags, "x:e", "Last tag is still around")
}

func TestDeviceMagicTag(t *testing.T) {
	metrics := []samplers.DDMetric{{
		Name:       "foo.bar.baz",
		Value:      [1][2]float64{{float64(time.Now().Unix()), float64(1.0)}},
		Tags:       []string{"gorch:frobble", "device:abc123", "x:e"},
		MetricType: "rate",
		Interval:   10,
	}}

	finalizeMetrics("badhostname", []string{"a:b", "c:d"}, metrics)
	assert.Equal(t, "abc123", metrics[0].DeviceName, "Metric devicename should be from tag")
	assert.NotContains(t, metrics[0].Tags, "device:abc123", "Host tag should be removed")
	assert.Contains(t, metrics[0].Tags, "x:e", "Last tag is still around")
}
