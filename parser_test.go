package veneur

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParser(t *testing.T) {
	m, _ := ParseMetric([]byte("a.b.c:1|c"))
	assert.NotNil(t, m, "Got nil metric!")
	assert.Equal(t, "a.b.c", m.Name, "Name")
	assert.Equal(t, float64(1), m.Value, "Value")
	assert.Equal(t, "counter", m.Type, "Type")
}

func TestParserGauge(t *testing.T) {
	m, _ := ParseMetric([]byte("a.b.c:1|g"))
	assert.NotNil(t, m, "Got nil metric!")
	assert.Equal(t, "a.b.c", m.Name, "Name")
	assert.Equal(t, float64(1), m.Value, "Value")
	assert.Equal(t, "gauge", m.Type, "Type")
}

func TestParserHistogram(t *testing.T) {
	m, _ := ParseMetric([]byte("a.b.c:1|h"))
	assert.NotNil(t, m, "Got nil metric!")
	assert.Equal(t, "a.b.c", m.Name, "Name")
	assert.Equal(t, float64(1), m.Value, "Value")
	assert.Equal(t, "histogram", m.Type, "Type")
}

func TestParserTimer(t *testing.T) {
	m, _ := ParseMetric([]byte("a.b.c:1|ms"))
	assert.NotNil(t, m, "Got nil metric!")
	assert.Equal(t, "a.b.c", m.Name, "Name")
	assert.Equal(t, float64(1), m.Value, "Value")
	assert.Equal(t, "timer", m.Type, "Type")
}

func TestParserSet(t *testing.T) {
	m, _ := ParseMetric([]byte("a.b.c:foo|s"))
	assert.NotNil(t, m, "Got nil metric!")
	assert.Equal(t, "a.b.c", m.Name, "Name")
	assert.Equal(t, "foo", m.Value, "Value")
	assert.Equal(t, "set", m.Type, "Type")
}

func TestParserWithTags(t *testing.T) {
	m, _ := ParseMetric([]byte("a.b.c:1|c|#foo:bar,baz:gorch"))
	assert.NotNil(t, m, "Got nil metric!")
	assert.Equal(t, "a.b.c", m.Name, "Name")
	assert.Equal(t, float64(1), m.Value, "Value")
	assert.Equal(t, "counter", m.Type, "Type")
	assert.Equal(t, 2, len(m.Tags), "# of Tags")

	_, valueError := ParseMetric([]byte("a.b.c:fart|c"))
	assert.NotNil(t, valueError, "No errors when parsing")
	assert.Contains(t, valueError.Error(), "Invalid number", "Invalid number error missing")
}

func TestParserWithSampleRate(t *testing.T) {
	m, _ := ParseMetric([]byte("a.b.c:1|c|@0.1"))
	assert.NotNil(t, m, "Got nil metric!")
	assert.Equal(t, "a.b.c", m.Name, "Name")
	assert.Equal(t, float64(1), m.Value, "Value")
	assert.Equal(t, "counter", m.Type, "Type")
	assert.Equal(t, float32(0.1), m.SampleRate, "Sample Rate")

	_, valueError := ParseMetric([]byte("a.b.c:fart|c"))
	assert.NotNil(t, valueError, "No errors when parsing")
	assert.Contains(t, valueError.Error(), "Invalid number", "Invalid number error missing")

	_, valueError = ParseMetric([]byte("a.b.c:1|g|@0.1"))
	assert.NotNil(t, valueError, "No errors when parsing")
	assert.Contains(t, valueError.Error(), "sample rate", "Sample rate error missing")
}

func TestParserWithSampleRateAndTags(t *testing.T) {
	m, _ := ParseMetric([]byte("a.b.c:1|c|@0.1|#foo:bar,baz:gorch"))
	assert.NotNil(t, m, "Got nil metric!")
	assert.Equal(t, "a.b.c", m.Name, "Name")
	assert.Equal(t, float64(1), m.Value, "Value")
	assert.Equal(t, "counter", m.Type, "Type")
	assert.Equal(t, float32(0.1), m.SampleRate, "Sample Rate")
	assert.Len(t, m.Tags, 2, "Tags")

	_, valueError := ParseMetric([]byte("a.b.c:fart|c"))
	assert.NotNil(t, valueError, "No errors when parsing")
	assert.Contains(t, valueError.Error(), "Invalid number", "Invalid number error missing")
}

func TestInvalidPackets(t *testing.T) {
	table := map[string]string{
		"foo":               "1 colon",
		"foo:1":             "1 pipe",
		"foo:1||":           "metric type",
		"foo:|c|":           "metric value",
		"foo:1|foo|":        "Invalid type",
		"foo:1|c||":         "pipes",
		"foo:1|c|foo":       "unknown section",
		"foo:1|c|@-0.1":     ">0",
		"foo:1|c|@1.1":      "<=1",
		"foo:1|c|@0.5|@0.2": "multiple sample rates",
		"foo:1|c|#foo|#bar": "multiple tag sections",
	}

	for packet, errContent := range table {
		_, err := ParseMetric([]byte(packet))
		assert.NotNil(t, err, "Should have gotten error parsing: ", packet)
		assert.Contains(t, err.Error(), errContent, "Error should have contained text")
	}
}

func TestLocalOnlyEscape(t *testing.T) {
	m, err := ParseMetric([]byte("a.b.c:1|h|#veneurlocalonly"))
	assert.NoError(t, err, "should have no error parsing")
	assert.True(t, m.LocalOnly, "should have gotten local only metric")
	for _, thisTag := range m.Tags {
		assert.NotEqual(t, "veneurlocalonly", thisTag, "veneurlocalonly should not actually be a tag")
	}
}
