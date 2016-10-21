package veneur

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stripe/veneur/samplers"
)

func TestParser(t *testing.T) {
	m, _ := samplers.ParseMetric([]byte("a.b.c:1|c"))
	assert.NotNil(t, m, "Got nil metric!")
	assert.Equal(t, "a.b.c", m.Name, "Name")
	assert.Equal(t, float64(1), m.Value, "Value")
	assert.Equal(t, "counter", m.Type, "Type")
}

func TestParserGauge(t *testing.T) {
	m, _ := samplers.ParseMetric([]byte("a.b.c:1|g"))
	assert.NotNil(t, m, "Got nil metric!")
	assert.Equal(t, "a.b.c", m.Name, "Name")
	assert.Equal(t, float64(1), m.Value, "Value")
	assert.Equal(t, "gauge", m.Type, "Type")
}

func TestParserHistogram(t *testing.T) {
	m, _ := samplers.ParseMetric([]byte("a.b.c:1|h"))
	assert.NotNil(t, m, "Got nil metric!")
	assert.Equal(t, "a.b.c", m.Name, "Name")
	assert.Equal(t, float64(1), m.Value, "Value")
	assert.Equal(t, "histogram", m.Type, "Type")
}

func TestParserTimer(t *testing.T) {
	m, _ := samplers.ParseMetric([]byte("a.b.c:1|ms"))
	assert.NotNil(t, m, "Got nil metric!")
	assert.Equal(t, "a.b.c", m.Name, "Name")
	assert.Equal(t, float64(1), m.Value, "Value")
	assert.Equal(t, "timer", m.Type, "Type")
}

func TestParserSet(t *testing.T) {
	m, _ := samplers.ParseMetric([]byte("a.b.c:foo|s"))
	assert.NotNil(t, m, "Got nil metric!")
	assert.Equal(t, "a.b.c", m.Name, "Name")
	assert.Equal(t, "foo", m.Value, "Value")
	assert.Equal(t, "set", m.Type, "Type")
}

func TestParserWithTags(t *testing.T) {
	m, _ := samplers.ParseMetric([]byte("a.b.c:1|c|#foo:bar,baz:gorch"))
	assert.NotNil(t, m, "Got nil metric!")
	assert.Equal(t, "a.b.c", m.Name, "Name")
	assert.Equal(t, float64(1), m.Value, "Value")
	assert.Equal(t, "counter", m.Type, "Type")
	assert.Equal(t, 2, len(m.Tags), "# of Tags")

	_, valueError := samplers.ParseMetric([]byte("a.b.c:fart|c"))
	assert.NotNil(t, valueError, "No errors when parsing")
	assert.Contains(t, valueError.Error(), "Invalid number", "Invalid number error missing")
}

func TestParserWithSampleRate(t *testing.T) {
	m, _ := samplers.ParseMetric([]byte("a.b.c:1|c|@0.1"))
	assert.NotNil(t, m, "Got nil metric!")
	assert.Equal(t, "a.b.c", m.Name, "Name")
	assert.Equal(t, float64(1), m.Value, "Value")
	assert.Equal(t, "counter", m.Type, "Type")
	assert.Equal(t, float32(0.1), m.SampleRate, "Sample Rate")

	_, valueError := samplers.ParseMetric([]byte("a.b.c:fart|c"))
	assert.NotNil(t, valueError, "No errors when parsing")
	assert.Contains(t, valueError.Error(), "Invalid number", "Invalid number error missing")

	_, valueError = samplers.ParseMetric([]byte("a.b.c:1|g|@0.1"))
	assert.NoError(t, valueError, "Got errors when parsing")
}

func TestParserWithSampleRateAndTags(t *testing.T) {
	m, _ := samplers.ParseMetric([]byte("a.b.c:1|c|@0.1|#foo:bar,baz:gorch"))
	assert.NotNil(t, m, "Got nil metric!")
	assert.Equal(t, "a.b.c", m.Name, "Name")
	assert.Equal(t, float64(1), m.Value, "Value")
	assert.Equal(t, "counter", m.Type, "Type")
	assert.Equal(t, float32(0.1), m.SampleRate, "Sample Rate")
	assert.Len(t, m.Tags, 2, "Tags")

	_, valueError := samplers.ParseMetric([]byte("a.b.c:fart|c"))
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
		_, err := samplers.ParseMetric([]byte(packet))
		assert.NotNil(t, err, "Should have gotten error parsing %q", packet)
		assert.Contains(t, err.Error(), errContent, "Error should have contained text")
	}
}

func TestLocalOnlyEscape(t *testing.T) {
	m, err := samplers.ParseMetric([]byte("a.b.c:1|h|#veneurlocalonly,tag2:quacks"))
	assert.NoError(t, err, "should have no error parsing")
	assert.Equal(t, samplers.LocalOnly, m.Scope, "should have gotten local only metric")
	assert.NotContains(t, m.Tags, "veneurlocalonly", "veneurlocalonly should not actually be a tag")
	assert.Contains(t, m.Tags, "tag2:quacks", "tag2 should be preserved in the list of tags after removing magic tags")
}

func TestGlobalOnlyEscape(t *testing.T) {
	m, err := samplers.ParseMetric([]byte("a.b.c:1|h|#veneurglobalonly,tag2:quacks"))
	assert.NoError(t, err, "should have no error parsing")
	assert.Equal(t, samplers.GlobalOnly, m.Scope, "should have gotten local only metric")
	assert.NotContains(t, m.Tags, "veneurglobalonly", "veneurlocalonly should not actually be a tag")
	assert.Contains(t, m.Tags, "tag2:quacks", "tag2 should be preserved in the list of tags after removing magic tags")
}

func TestEvents(t *testing.T) {
	evt, err := samplers.ParseEvent([]byte("_e{3,3}:foo|bar|k:foos|s:test|t:success|p:low|#foo:bar,baz:qux|d:1136239445|h:example.com"))
	assert.NoError(t, err, "should have parsed correctly")
	assert.EqualValues(t, &samplers.UDPEvent{
		Title:       "foo",
		Text:        "bar",
		Timestamp:   1136239445,
		Aggregation: "foos",
		Source:      "test",
		AlertLevel:  "success",
		Priority:    "low",
		Tags:        []string{"foo:bar", "baz:qux"},
		Hostname:    "example.com",
	}, evt, "should have parsed event")

	table := map[string]string{
		"_e{4,3}:foo|bar":               "title length",
		"_e{3,4}:foo|bar":               "text length",
		"_e{3,3}:foo|bar|d:abc":         "date",
		"_e{3,3}:foo|bar|p:baz":         "priority",
		"_e{3,3}:foo|bar|t:baz":         "alert",
		"_e{3,3}:foo|bar|t:info|t:info": "multiple alert",
		"_e{3,3}:foo|bar||":             "pipe",
		"_e{3,0}:foo||":                 "text length",
		"_e{3,3}:foo":                   "text",
		"_e{3,3}":                       "colon",
	}
	for packet, errContent := range table {
		_, err := samplers.ParseEvent([]byte(packet))
		assert.NotNil(t, err, "Should have gotten error parsing %q", packet)
		assert.Contains(t, err.Error(), errContent, "Error should have contained text")
	}
}

func TestServiceChecks(t *testing.T) {
	evt, err := samplers.ParseServiceCheck([]byte("_sc|foo.bar|0|d:1136239445|h:example.com"))
	assert.NoError(t, err, "should have parsed correctly")
	assert.EqualValues(t, &samplers.UDPServiceCheck{
		Name:      "foo.bar",
		Status:    0,
		Timestamp: 1136239445,
		Hostname:  "example.com",
	}, evt, "should have parsed event")

	table := map[string]string{
		"foo.bar|0":           "_sc",
		"_sc|foo.bar":         "status",
		"_sc|foo.bar|5":       "status",
		"_sc|foo.bar|0||":     "pipe",
		"_sc|foo.bar|0|d:abc": "date",
	}
	for packet, errContent := range table {
		_, err := samplers.ParseServiceCheck([]byte(packet))
		assert.NotNil(t, err, "Should have gotten error parsing %q", packet)
		assert.Contains(t, err.Error(), errContent, "Error should have contained text")
	}
}

func TestEventMessageUnescape(t *testing.T) {
	packet := []byte("_e{3,15}:foo|foo\\nbar\\nbaz\\n")
	evt, err := samplers.ParseEvent(packet)
	assert.NoError(t, err, "Should have parsed correctly")
	assert.Equal(t, "foo\nbar\nbaz\n", evt.Text, "Should contain newline")
}

func TestServiceCheckMessageUnescape(t *testing.T) {
	packet := []byte("_sc|foo|0|m:foo\\nbar\\nbaz\\n")
	svcheck, err := samplers.ParseServiceCheck(packet)
	assert.NoError(t, err, "Should have parsed correctly")
	assert.Equal(t, "foo\nbar\nbaz\n", svcheck.Message, "Should contain newline")
}
