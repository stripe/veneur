package veneur

import (
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/gogo/protobuf/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stripe/veneur/protocol"
	"github.com/stripe/veneur/samplers"
	"github.com/stripe/veneur/ssf"
)

func freshSSFMetric() *ssf.SSFSample {
	return &ssf.SSFSample{
		Metric:     0,
		Name:       "test.ssf_metric",
		Value:      5,
		Message:    "test_msg",
		Status:     0,
		SampleRate: 1,
		Tags: map[string]string{
			"tag1": "value1",
			"tag2": "value2",
		},
	}
}

func TestValidMetric(t *testing.T) {
	metric := freshSSFMetric()

	m, _ := samplers.ParseMetricSSF(metric)
	assert.True(t, samplers.ValidMetric(m))

	metric.Name = ""
	m, _ = samplers.ParseMetricSSF(metric)
	assert.False(t, samplers.ValidMetric(m))

	metric.SampleRate = 0
	m, _ = samplers.ParseMetricSSF(metric)
	assert.False(t, samplers.ValidMetric(m))
}

func TestValidTrace(t *testing.T) {
	trace := &ssf.SSFSpan{}
	assert.False(t, protocol.ValidTrace(trace))

	trace.Id = 1
	trace.TraceId = 1
	trace.StartTimestamp = 1
	trace.EndTimestamp = 5
	assert.True(t, protocol.ValidTrace(trace))
}

func TestParseSSFUnmarshal(t *testing.T) {
	test := []byte{'0'}
	sample, err := protocol.ParseSSF(test)

	assert.Nil(t, sample)
	assert.NotNil(t, err)
	assert.Error(t, err)
}

func TestParseSSFEmpty(t *testing.T) {
	trace := &ssf.SSFSpan{}

	buff, err := proto.Marshal(trace)
	assert.Nil(t, err)

	_, err = protocol.ParseSSF(buff)
	assert.NoError(t, err)
}

func TestParseSSFValidTraceInvalidMetric(t *testing.T) {
	trace := &ssf.SSFSpan{}
	trace.Id = 1
	trace.TraceId = 1
	trace.StartTimestamp = 1
	trace.EndTimestamp = 5

	buff, err := proto.Marshal(trace)
	assert.Nil(t, err)

	span, err := protocol.ParseSSF(buff)
	assert.Nil(t, err)
	if assert.NotNil(t, span) {
		assert.NoError(t, protocol.ValidateTrace(span))
		assert.NotNil(t, span)
		assert.NotNil(t, span.Tags)

		metrics, err := samplers.ConvertMetrics(span)
		assert.NoError(t, err)
		assert.Empty(t, metrics)
	}
}

func TestParseSSFInvalidTraceValidMetric(t *testing.T) {
	metric := freshSSFMetric()

	trace := &ssf.SSFSpan{}
	trace.Metrics = make([]*ssf.SSFSample, 0)
	trace.Metrics = append(trace.Metrics, metric)

	buff, err := proto.Marshal(trace)
	assert.Nil(t, err)

	span, err := protocol.ParseSSF(buff)
	assert.NoError(t, err)
	if assert.NotNil(t, span) {
		metrics, err := samplers.ConvertMetrics(span)
		assert.NoError(t, err)
		if assert.Equal(t, 1, len(metrics)) {
			m := metrics[0]
			assert.Equal(t, metric.Name, m.Name, "Name")
			assert.Equal(t, float64(metric.Value), m.Value, "Value")
			assert.Equal(t, "counter", m.Type, "Type")
		}

		err = protocol.ValidateTrace(span)
		_, isInvalid := err.(*protocol.InvalidTrace)
		assert.True(t, isInvalid)
		assert.NotNil(t, span)
	}
}

func TestParseSSFValid(t *testing.T) {
	metric := freshSSFMetric()

	trace := &ssf.SSFSpan{}
	trace.Id = 1
	trace.TraceId = 1
	trace.StartTimestamp = 1
	trace.EndTimestamp = 5

	trace.Metrics = make([]*ssf.SSFSample, 0)
	trace.Metrics = append(trace.Metrics, metric)

	buff, err := proto.Marshal(trace)
	assert.Nil(t, err)

	msg, err := protocol.ParseSSF(buff)
	assert.NoError(t, err)
	if assert.NotNil(t, msg) {
		metrics, err := samplers.ConvertMetrics(msg)
		assert.NoError(t, err)
		if assert.Equal(t, 1, len(metrics)) {
			m := metrics[0]
			assert.Equal(t, metric.Name, m.Name, "Name")
			assert.Equal(t, float64(metric.Value), m.Value, "Value")
			assert.Equal(t, "counter", m.Type, "Type")
		}
	}
}

func TestParseSSFIndicatorSpan(t *testing.T) {
	duration := 5 * time.Second
	start := time.Now()
	end := start.Add(duration)

	span := &ssf.SSFSpan{}
	span.Id = 1
	span.TraceId = 5
	span.Name = "foo"
	span.StartTimestamp = start.UnixNano()
	span.EndTimestamp = end.UnixNano()
	span.Indicator = true
	span.Service = "bar-srv"
	span.Tags = map[string]string{
		"this-tag":       "definitely gets ignored",
		"this-other-tag": "also gets dropped",
	}
	span.Metrics = make([]*ssf.SSFSample, 0)
	buff, err := proto.Marshal(span)
	assert.Nil(t, err)
	inSpan, err := protocol.ParseSSF(buff)
	assert.NoError(t, err)
	require.NotNil(t, inSpan)
	require.NoError(t, protocol.ValidateTrace(span))

	metrics, err := samplers.ConvertIndicatorMetrics(inSpan, "timer_name")
	assert.NoError(t, err)
	if assert.Equal(t, 1, len(metrics)) {
		m := metrics[0]
		assert.Equal(t, "timer_name", m.Name)
		assert.Equal(t, "histogram", m.Type)
		assert.InEpsilon(t, float32(duration/time.Nanosecond), m.Value, 0.001)
		if assert.Equal(t, 3, len(m.Tags)) {
			var tags sort.StringSlice = m.Tags
			sort.Sort(tags)
			assert.Equal(t, "error:false", tags[0])
			assert.Equal(t, fmt.Sprintf("name:%s", span.Name), tags[1])
			assert.Equal(t, fmt.Sprintf("service:%s", span.Service), tags[2])
		}
	}
}

func TestParseSSFIndicatorSpanWithError(t *testing.T) {
	duration := 5 * time.Second
	start := time.Now()
	end := start.Add(duration)

	span := &ssf.SSFSpan{}
	span.Id = 1
	span.TraceId = 5
	span.Name = "foo"
	span.StartTimestamp = start.UnixNano()
	span.EndTimestamp = end.UnixNano()
	span.Indicator = true
	span.Service = "bar-srv"
	span.Tags = map[string]string{
		"this-tag":       "definitely gets ignored",
		"this-other-tag": "also gets dropped",
	}
	span.Metrics = make([]*ssf.SSFSample, 0)
	span.Error = true
	buff, err := proto.Marshal(span)
	assert.Nil(t, err)
	inSpan, err := protocol.ParseSSF(buff)
	assert.NoError(t, err)
	require.NotNil(t, span)
	require.NoError(t, protocol.ValidateTrace(span))

	metrics, err := samplers.ConvertIndicatorMetrics(inSpan, "timer_name")
	assert.NoError(t, err)
	if assert.Equal(t, 1, len(metrics)) {
		m := metrics[0]
		assert.Equal(t, "timer_name", m.Name)
		assert.Equal(t, "histogram", m.Type)
		assert.InEpsilon(t, float32(duration/time.Nanosecond), m.Value, 0.001,
			"Duration seems incorrect: %f vs. %d", m.Value, duration/time.Nanosecond)
		if assert.Equal(t, 3, len(m.Tags)) {
			var tags sort.StringSlice = m.Tags
			sort.Sort(tags)
			assert.Equal(t, "error:true", tags[0])
			assert.Equal(t, fmt.Sprintf("name:%s", span.Name), tags[1])
			assert.Equal(t, fmt.Sprintf("service:%s", span.Service), tags[2])
		}
	}
}

func TestParseSSFIndicatorSpanNotNamed(t *testing.T) {
	duration := 5 * time.Second
	start := time.Now()
	end := start.Add(duration)

	span := &ssf.SSFSpan{}
	span.Id = 1
	span.TraceId = 5
	span.Name = "foo"
	span.StartTimestamp = start.UnixNano()
	span.EndTimestamp = end.UnixNano()
	span.Indicator = true
	span.Service = "bar-srv"
	span.Tags = map[string]string{
		"this-tag":       "definitely gets ignored",
		"this-other-tag": "also gets dropped",
	}
	span.Metrics = make([]*ssf.SSFSample, 0)
	buff, err := proto.Marshal(span)
	assert.Nil(t, err)
	inSpan, err := protocol.ParseSSF(buff)
	assert.NoError(t, err)
	require.NotNil(t, inSpan)
	require.NoError(t, protocol.ValidateTrace(inSpan))

	metrics, err := samplers.ConvertIndicatorMetrics(inSpan, "")
	assert.NoError(t, err)
	assert.Equal(t, 0, len(metrics))
}

func TestParseSSFNonIndicatorSpan(t *testing.T) {
	duration := 5 * time.Second
	start := time.Now()
	end := start.Add(duration)

	span := &ssf.SSFSpan{}
	span.Id = 1
	span.TraceId = 5
	span.Name = "no-timer"
	span.StartTimestamp = start.UnixNano()
	span.EndTimestamp = end.UnixNano()
	span.Indicator = false
	span.Service = "bar-srv"
	span.Tags = map[string]string{
		"this-tag":       "definitely gets ignored",
		"this-other-tag": "also gets dropped",
	}
	span.Metrics = make([]*ssf.SSFSample, 0)

	buff, err := proto.Marshal(span)
	assert.Nil(t, err)
	inSpan, err := protocol.ParseSSF(buff)
	require.NotNil(t, inSpan)
	require.NoError(t, err)
	require.NoError(t, protocol.ValidateTrace(inSpan))

	metrics, err := samplers.ConvertIndicatorMetrics(inSpan, "timer_name")
	assert.NoError(t, err)
	assert.Equal(t, 0, len(metrics))
}

func TestParseSSFBadMetric(t *testing.T) {
	metric := freshSSFMetric()
	metric.Metric = 5

	trace := &ssf.SSFSpan{}

	trace.Metrics = make([]*ssf.SSFSample, 0)
	trace.Metrics = append(trace.Metrics, metric)

	buff, err := proto.Marshal(trace)
	assert.Nil(t, err)

	span, err := protocol.ParseSSF(buff)
	assert.NoError(t, err)
	if assert.NotNil(t, span) {
		metrics, err := samplers.ConvertMetrics(span)
		assert.Error(t, err)
		assert.Equal(t, 0, len(metrics))
		invalid, ok := err.(samplers.InvalidMetrics)
		if assert.True(t, ok, "Error type incorrect: %v", err) {
			assert.Equal(t, 1, len(invalid.Samples()))
		}
	}
}

func TestParserSSF(t *testing.T) {
	standardMetric := freshSSFMetric()
	m, _ := samplers.ParseMetricSSF(standardMetric)
	assert.NotNil(t, m, "Got nil metric!")
	assert.Equal(t, standardMetric.Name, m.Name, "Name")
	assert.Equal(t, float64(standardMetric.Value), m.Value, "Value")
	assert.Equal(t, "counter", m.Type, "Type")
}

func TestParserSSFGauge(t *testing.T) {
	standardMetric := freshSSFMetric()
	standardMetric.Metric = ssf.SSFSample_GAUGE
	m, _ := samplers.ParseMetricSSF(standardMetric)
	assert.NotNil(t, m, "Got nil metric!")
	assert.Equal(t, standardMetric.Name, m.Name, "Name")
	assert.Equal(t, float64(standardMetric.Value), m.Value, "Value")
	assert.Equal(t, "gauge", m.Type, "Type")
}

func TestParserSSFSet(t *testing.T) {
	standardMetric := freshSSFMetric()
	standardMetric.Metric = ssf.SSFSample_SET
	m, _ := samplers.ParseMetricSSF(standardMetric)
	assert.NotNil(t, m, "Got nil metric!")
	assert.Equal(t, standardMetric.Name, m.Name, "Name")
	assert.Equal(t, standardMetric.Message, m.Value, "Value")
	assert.Equal(t, "set", m.Type, "Type")
}

func TestParserSSFWithTags(t *testing.T) {
	standardMetric := freshSSFMetric()
	m, _ := samplers.ParseMetricSSF(standardMetric)
	assert.NotNil(t, m, "Got nil metric!")
	assert.Equal(t, standardMetric.Name, m.Name, "Name")
	assert.Equal(t, float64(standardMetric.Value), m.Value, "Value")
	assert.Equal(t, "counter", m.Type, "Type")
	assert.Equal(t, 2, len(m.Tags), "# of Tags")
}

func TestParserSSFWithSampleRate(t *testing.T) {
	standardMetric := freshSSFMetric()
	standardMetric.SampleRate = 0.1
	m, _ := samplers.ParseMetricSSF(standardMetric)
	assert.NotNil(t, m, "Got nil metric!")
	assert.Equal(t, standardMetric.Name, m.Name, "Name")
	assert.Equal(t, float64(standardMetric.Value), m.Value, "Value")
	assert.Equal(t, "counter", m.Type, "Type")
	assert.Equal(t, float32(0.1), m.SampleRate, "Sample Rate")
}

func TestParserSSFWithSampleRateAndTags(t *testing.T) {
	standardMetric := freshSSFMetric()
	standardMetric.SampleRate = 0.1
	m, _ := samplers.ParseMetricSSF(standardMetric)
	assert.NotNil(t, m, "Got nil metric!")
	assert.Equal(t, standardMetric.Name, m.Name, "Name")
	assert.Equal(t, float64(standardMetric.Value), m.Value, "Value")
	assert.Equal(t, "counter", m.Type, "Type")
	assert.Equal(t, float32(0.1), m.SampleRate, "Sample Rate")
	assert.Len(t, m.Tags, 2, "Tags")
}

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
		"foo":                                "1 colon",
		"foo:1":                              "1 pipe",
		"foo:1||":                            "metric type",
		"foo:|c|":                            "metric value",
		"this_is_a_bad_metric:nan|g|#shell":  "metric value",
		"this_is_a_bad_metric:NaN|g|#shell":  "metric value",
		"this_is_a_bad_metric:-inf|g|#shell": "metric value",
		"this_is_a_bad_metric:+inf|g|#shell": "metric value",
		"foo:1|foo|":                         "Invalid type",
		"foo:1|c||":                          "pipes",
		"foo:1|c|foo":                        "unknown section",
		"foo:1|c|@-0.1":                      ">0",
		"foo:1|c|@1.1":                       "<=1",
		"foo:1|c|@0.5|@0.2":                  "multiple sample rates",
		"foo:1|c|#foo|#bar":                  "multiple tag sections",
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

func TestConsecutiveParseSSF(t *testing.T) {

	span := &ssf.SSFSpan{
		Id:             12345678,
		TraceId:        1,
		StartTimestamp: 123,
		EndTimestamp:   124,
		Tags: map[string]string{
			"foo": "bar",
		},
	}
	buff1, err := proto.Marshal(span)
	assert.Nil(t, err)

	otherSpan := &ssf.SSFSpan{
		Id:             12345679,
		TraceId:        2,
		StartTimestamp: 125,
		EndTimestamp:   126,
	}
	buff2, err := proto.Marshal(otherSpan)
	assert.Nil(t, err)

	// Because ParseSSF reuses buffers via a Pool when parsing we
	// want to ensure that subsequent calls properly reset and
	// don't bleed into each other, so "parse" each and check.
	span1, err := protocol.ParseSSF(buff1)
	assert.Nil(t, err)
	span2, err := protocol.ParseSSF(buff2)
	assert.Nil(t, err)

	assert.Equal(t, int64(12345678), span1.Id)
	assert.Equal(t, "bar", span1.Tags["foo"], "Tagful span has no tags!")

	assert.Equal(t, int64(12345679), span2.Id)
	assert.Equal(t, "", span2.Tags["foo"], "Tagless span has tags!")
}

func BenchmarkParseSSF(b *testing.B) {
	span := &ssf.SSFSpan{}

	buff, err := proto.Marshal(span)
	assert.Nil(b, err)

	for n := 0; n < b.N; n++ {
		protocol.ParseSSF(buff)
	}
}
