package veneur

import (
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/gogo/protobuf/proto"
	"github.com/segmentio/fasthash/fnv1a"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stripe/veneur/v14/protocol"
	"github.com/stripe/veneur/v14/protocol/dogstatsd"
	"github.com/stripe/veneur/v14/samplers"
	"github.com/stripe/veneur/v14/ssf"
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

	parser := samplers.Parser{}

	m, _ := parser.ParseMetricSSF(metric)
	assert.True(t, samplers.ValidMetric(m))

	metric.Name = ""
	m, _ = parser.ParseMetricSSF(metric)
	assert.False(t, samplers.ValidMetric(m))

	metric.SampleRate = 0
	m, _ = parser.ParseMetricSSF(metric)
	assert.False(t, samplers.ValidMetric(m))
}

func TestValidTrace(t *testing.T) {
	trace := &ssf.SSFSpan{}
	assert.False(t, protocol.ValidTrace(trace))

	trace.Id = 1
	trace.TraceId = 1
	trace.Name = "foo"
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
	trace.Name = "foo"
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

		metrics, err := (&samplers.Parser{}).ConvertMetrics(span)
		assert.NoError(t, err)
		assert.Empty(t, metrics)
	}
}

func TestParseSSFInvalidTraceValidMetric(t *testing.T) {
	metric := freshSSFMetric()

	trace := &ssf.SSFSpan{}
	trace.Tags = map[string]string{
		"foo": "bar",
	}
	trace.Metrics = make([]*ssf.SSFSample, 0)
	trace.Metrics = append(trace.Metrics, metric)

	buff, err := proto.Marshal(trace)
	assert.Nil(t, err)

	span, err := protocol.ParseSSF(buff)
	assert.NoError(t, err)
	if assert.NotNil(t, span) {
		metrics, err := (&samplers.Parser{}).ConvertMetrics(span)
		assert.NoError(t, err)
		if assert.Equal(t, 1, len(metrics)) {
			m := metrics[0]
			assert.Equal(t, metric.Name, m.Name, "Name")
			assert.Equal(t, float64(metric.Value), m.Value, "Value")
			assert.Equal(t, "counter", m.Type, "Type")
			assert.NotContains(t, "foo:bar", m.Tags, "Should not pick up tag from parent span")
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
	trace.Tags = map[string]string{}
	trace.Tags["foo"] = "bar"

	trace.Metrics = make([]*ssf.SSFSample, 0)
	trace.Metrics = append(trace.Metrics, metric)

	buff, err := proto.Marshal(trace)
	assert.Nil(t, err)

	msg, err := protocol.ParseSSF(buff)
	assert.NoError(t, err)
	if assert.NotNil(t, msg) {
		noTags := samplers.NewParser([]string{})
		yesTags := samplers.NewParser([]string{"implicit"})

		metrics, err := noTags.ConvertMetrics(msg)
		assert.NoError(t, err)
		if assert.Equal(t, 1, len(metrics)) {
			m := metrics[0]
			assert.Equal(t, metric.Name, m.Name, "Name")
			assert.Equal(t, float64(metric.Value), m.Value, "Value")
			assert.Equal(t, "counter", m.Type, "Type")
			assert.NotContains(t, m.Tags, "foo", "Metric should not inherit tags from its parent span")
			assert.NotContains(t, m.Tags, "implicit")
		}

		metrics, err = yesTags.ConvertMetrics(msg)
		assert.NoError(t, err)
		if assert.Equal(t, 1, len(metrics)) {
			m := metrics[0]
			assert.Equal(t, metric.Name, m.Name, "Name")
			assert.Equal(t, float64(metric.Value), m.Value, "Value")
			assert.Equal(t, "counter", m.Type, "Type")
			assert.NotContains(t, m.Tags, "foo", "Metric should not inherit tags from its parent span")
			assert.Contains(t, m.Tags, "implicit")
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

	noTags := samplers.NewParser([]string{})
	yesTags := samplers.NewParser([]string{"implicit"})

	metrics, err := noTags.ConvertIndicatorMetrics(inSpan, "timer_name", "")
	assert.NoError(t, err)
	if assert.Equal(t, 1, len(metrics)) {
		m := metrics[0]
		assert.Equal(t, "timer_name", m.Name)
		assert.Equal(t, "histogram", m.Type)
		assert.InEpsilon(t, float32(duration/time.Nanosecond), m.Value, 0.001)
		if assert.Equal(t, 2, len(m.Tags)) {
			var tags sort.StringSlice = m.Tags
			sort.Sort(tags)
			assert.Equal(t, sort.StringSlice{
				"error:false",
				fmt.Sprintf("service:%s", span.Service),
			}, tags)
		}
		assert.NotContains(t, m.Tags, "implicit")
	}

	metrics, err = yesTags.ConvertIndicatorMetrics(inSpan, "timer_name", "")
	assert.NoError(t, err)
	if assert.Equal(t, 1, len(metrics)) {
		m := metrics[0]
		assert.Equal(t, "timer_name", m.Name)
		assert.Equal(t, "histogram", m.Type)
		assert.InEpsilon(t, float32(duration/time.Nanosecond), m.Value, 0.001)
		if assert.Equal(t, 3, len(m.Tags)) {
			var tags sort.StringSlice = m.Tags
			sort.Sort(tags)
			assert.Equal(t, sort.StringSlice{
				"error:false",
				"implicit",
				fmt.Sprintf("service:%s", span.Service),
			}, tags)
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

	noTags := samplers.NewParser([]string{})
	yesTags := samplers.NewParser([]string{"implicit"})

	metrics, err := noTags.ConvertIndicatorMetrics(inSpan, "timer_name", "")
	assert.NoError(t, err)
	if assert.Equal(t, 1, len(metrics)) {
		m := metrics[0]
		assert.Equal(t, "timer_name", m.Name)
		assert.Equal(t, "histogram", m.Type)
		assert.InEpsilon(t, float32(duration/time.Nanosecond), m.Value, 0.001,
			"Duration seems incorrect: %f vs. %d", m.Value, duration/time.Nanosecond)
		if assert.Equal(t, 2, len(m.Tags)) {
			var tags sort.StringSlice = m.Tags
			sort.Sort(tags)
			assert.Equal(t, sort.StringSlice{
				"error:true",
				fmt.Sprintf("service:%s", span.Service),
			}, tags)
		}
	}

	metrics, err = yesTags.ConvertIndicatorMetrics(inSpan, "timer_name", "")
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
			assert.Equal(t, sort.StringSlice{
				"error:true",
				"implicit",
				fmt.Sprintf("service:%s", span.Service),
			}, tags)
		}
	}
}

func TestParseSSFIndicatorObjective(t *testing.T) {
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

	noTags := samplers.NewParser([]string{})
	yesTags := samplers.NewParser([]string{"implicit"})

	metrics, err := noTags.ConvertIndicatorMetrics(inSpan, "", "timer_name")
	assert.NoError(t, err)
	if assert.Equal(t, 1, len(metrics)) {
		m := metrics[0]
		assert.Equal(t, "timer_name", m.Name)
		assert.Equal(t, "histogram", m.Type)
		assert.InEpsilon(t, float32(duration/time.Nanosecond), m.Value, 0.001)
		if assert.Equal(t, 3, len(m.Tags)) {
			var tags sort.StringSlice = m.Tags
			sort.Sort(tags)
			assert.Equal(t, sort.StringSlice{
				"error:false",
				"objective:foo",
				fmt.Sprintf("service:%s", span.Service),
			}, tags)
		}
	}

	metrics, err = yesTags.ConvertIndicatorMetrics(inSpan, "", "timer_name")
	assert.NoError(t, err)
	if assert.Equal(t, 1, len(metrics)) {
		m := metrics[0]
		assert.Equal(t, "timer_name", m.Name)
		assert.Equal(t, "histogram", m.Type)
		assert.InEpsilon(t, float32(duration/time.Nanosecond), m.Value, 0.001)
		if assert.Equal(t, 4, len(m.Tags)) {
			var tags sort.StringSlice = m.Tags
			sort.Sort(tags)
			assert.Equal(t, sort.StringSlice{
				"error:false",
				"implicit",
				"objective:foo",
				fmt.Sprintf("service:%s", span.Service),
			}, tags)
		}
	}
}

func TestParseSSFIndicatorObjectiveTag(t *testing.T) {
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
		"ssf_objective":  "bar",
	}
	span.Metrics = make([]*ssf.SSFSample, 0)
	buff, err := proto.Marshal(span)
	assert.Nil(t, err)
	inSpan, err := protocol.ParseSSF(buff)
	assert.NoError(t, err)
	require.NotNil(t, inSpan)
	require.NoError(t, protocol.ValidateTrace(span))

	noTags := samplers.NewParser([]string{})
	yesTags := samplers.NewParser([]string{"implicit"})

	metrics, err := noTags.ConvertIndicatorMetrics(inSpan, "", "timer_name")
	assert.NoError(t, err)
	if assert.Equal(t, 1, len(metrics)) {
		m := metrics[0]
		assert.Equal(t, "timer_name", m.Name)
		assert.Equal(t, "histogram", m.Type)
		assert.InEpsilon(t, float32(duration/time.Nanosecond), m.Value, 0.001)
		if assert.Equal(t, 3, len(m.Tags)) {
			var tags sort.StringSlice = m.Tags
			sort.Sort(tags)
			assert.Equal(t, sort.StringSlice{
				"error:false",
				"objective:bar",
				fmt.Sprintf("service:%s", span.Service),
			}, tags)
		}
	}

	metrics, err = yesTags.ConvertIndicatorMetrics(inSpan, "", "timer_name")
	assert.NoError(t, err)
	if assert.Equal(t, 1, len(metrics)) {
		m := metrics[0]
		assert.Equal(t, "timer_name", m.Name)
		assert.Equal(t, "histogram", m.Type)
		assert.InEpsilon(t, float32(duration/time.Nanosecond), m.Value, 0.001)
		if assert.Equal(t, 4, len(m.Tags)) {
			var tags sort.StringSlice = m.Tags
			sort.Sort(tags)
			assert.Equal(t, sort.StringSlice{
				"error:false",
				"implicit",
				"objective:bar",
				fmt.Sprintf("service:%s", span.Service),
			}, tags)
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

	metrics, err := (&samplers.Parser{}).ConvertIndicatorMetrics(inSpan, "", "")
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

	metrics, err := (&samplers.Parser{}).ConvertIndicatorMetrics(inSpan, "timer_name", "objective_name")
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
		metrics, err := (&samplers.Parser{}).ConvertMetrics(span)
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
	m, _ := (&samplers.Parser{}).ParseMetricSSF(standardMetric)
	assert.NotNil(t, m, "Got nil metric!")
	assert.Equal(t, standardMetric.Name, m.Name, "Name")
	assert.Equal(t, float64(standardMetric.Value), m.Value, "Value")
	assert.Equal(t, "counter", m.Type, "Type")
}

func TestParserSSFGauge(t *testing.T) {
	standardMetric := freshSSFMetric()
	standardMetric.Metric = ssf.SSFSample_GAUGE
	m, _ := (&samplers.Parser{}).ParseMetricSSF(standardMetric)
	assert.NotNil(t, m, "Got nil metric!")
	assert.Equal(t, standardMetric.Name, m.Name, "Name")
	assert.Equal(t, float64(standardMetric.Value), m.Value, "Value")
	assert.Equal(t, "gauge", m.Type, "Type")
}

func TestParserSSFSet(t *testing.T) {
	standardMetric := freshSSFMetric()
	standardMetric.Metric = ssf.SSFSample_SET
	m, _ := (&samplers.Parser{}).ParseMetricSSF(standardMetric)
	assert.NotNil(t, m, "Got nil metric!")
	assert.Equal(t, standardMetric.Name, m.Name, "Name")
	assert.Equal(t, standardMetric.Message, m.Value, "Value")
	assert.Equal(t, "set", m.Type, "Type")
}

func TestParserSSFWithTags(t *testing.T) {
	standardMetric := freshSSFMetric()

	noTags := samplers.NewParser([]string{})
	yesTags := samplers.NewParser([]string{"implicit"})

	m, _ := noTags.ParseMetricSSF(standardMetric)
	assert.NotNil(t, m, "Got nil metric!")
	assert.Equal(t, standardMetric.Name, m.Name, "Name")
	assert.Equal(t, float64(standardMetric.Value), m.Value, "Value")
	assert.Equal(t, "counter", m.Type, "Type")
	assert.Equal(t, 2, len(m.Tags), "# of Tags")

	m, _ = yesTags.ParseMetricSSF(standardMetric)
	assert.Equal(t, 3, len(m.Tags), "# of Tags")
}

func TestParserSSFWithSampleRate(t *testing.T) {
	standardMetric := freshSSFMetric()
	standardMetric.SampleRate = 0.1
	m, _ := (&samplers.Parser{}).ParseMetricSSF(standardMetric)
	assert.NotNil(t, m, "Got nil metric!")
	assert.Equal(t, standardMetric.Name, m.Name, "Name")
	assert.Equal(t, float64(standardMetric.Value), m.Value, "Value")
	assert.Equal(t, "counter", m.Type, "Type")
	assert.Equal(t, float32(0.1), m.SampleRate, "Sample Rate")
}

func TestParserSSFWithSampleRateAndTags(t *testing.T) {
	standardMetric := freshSSFMetric()
	standardMetric.SampleRate = 0.1

	noTags := samplers.NewParser([]string{})
	yesTags := samplers.NewParser([]string{"implicit"})

	m, _ := noTags.ParseMetricSSF(standardMetric)
	assert.NotNil(t, m, "Got nil metric!")
	assert.Equal(t, standardMetric.Name, m.Name, "Name")
	assert.Equal(t, float64(standardMetric.Value), m.Value, "Value")
	assert.Equal(t, "counter", m.Type, "Type")
	assert.Equal(t, float32(0.1), m.SampleRate, "Sample Rate")
	assert.Len(t, m.Tags, 2, "Tags")
	m, _ = yesTags.ParseMetricSSF(standardMetric)
	assert.Len(t, m.Tags, 3, "Tags")
}

func TestParserSSFWithStatusCheck(t *testing.T) {
	standardMetric := freshSSFMetric()
	standardMetric.Metric = ssf.SSFSample_STATUS
	standardMetric.Status = ssf.SSFSample_UNKNOWN

	noTags := samplers.NewParser([]string{})
	yesTags := samplers.NewParser([]string{"implicit"})

	m, _ := noTags.ParseMetricSSF(standardMetric)
	assert.NotNil(t, m, "Got nil metric!")
	assert.Equal(t, standardMetric.Name, m.Name, "Name")
	assert.Equal(t, ssf.SSFSample_UNKNOWN, m.Value, "Value")
	assert.Equal(t, "status", m.Type, "Type")
	assert.Len(t, m.Tags, 2, "Tags")
	m, _ = yesTags.ParseMetricSSF(standardMetric)
	assert.Len(t, m.Tags, 3, "Tags")
}

func parseMetrics(t *testing.T, p *samplers.Parser, buf []byte) []*samplers.UDPMetric {
	var metrics []*samplers.UDPMetric
	err := p.ParseMetric(buf, func(m *samplers.UDPMetric) {
		metrics = append(metrics, m)
	})
	require.NoError(t, err)
	return metrics
}

func parseOneMetric(t *testing.T, p *samplers.Parser, buf []byte) *samplers.UDPMetric {
	metrics := parseMetrics(t, p, buf)
	require.Equal(t, 1, len(metrics))
	return metrics[0]
}

func TestParser(t *testing.T) {
	noTags := samplers.NewParser([]string{})
	yesTags := samplers.NewParser([]string{"implicit"})

	m := parseOneMetric(t, &noTags, []byte("a.b.c:1|c"))
	assert.NotNil(t, m, "Got nil metric!")
	assert.Equal(t, "a.b.c", m.Name, "Name")
	assert.Equal(t, float64(1), m.Value, "Value")
	assert.Equal(t, "counter", m.Type, "Type")
	assert.Equal(t, 0, len(m.Tags), "# of tags")

	m = parseOneMetric(t, &yesTags, []byte("a.b.c:1|c"))
	assert.Equal(t, 1, len(m.Tags), "# of tags")
}

func TestParserGauge(t *testing.T) {
	noTags := samplers.NewParser([]string{})
	yesTags := samplers.NewParser([]string{"implicit"})

	m := parseOneMetric(t, &noTags, []byte("a.b.c:1|g"))
	assert.NotNil(t, m, "Got nil metric!")
	assert.Equal(t, "a.b.c", m.Name, "Name")
	assert.Equal(t, float64(1), m.Value, "Value")
	assert.Equal(t, "gauge", m.Type, "Type")
	assert.Equal(t, 0, len(m.Tags), "# of tags")

	m = parseOneMetric(t, &yesTags, []byte("a.b.c:1|g"))
	assert.Equal(t, 1, len(m.Tags), "# of tags")
}

func TestParserHistogram(t *testing.T) {
	noTags := samplers.NewParser([]string{})
	yesTags := samplers.NewParser([]string{"implicit"})

	m := parseOneMetric(t, &noTags, []byte("a.b.c:1|h"))
	assert.NotNil(t, m, "Got nil metric!")
	assert.Equal(t, "a.b.c", m.Name, "Name")
	assert.Equal(t, float64(1), m.Value, "Value")
	assert.Equal(t, "histogram", m.Type, "Type")
	assert.Equal(t, 0, len(m.Tags), "# of tags")

	m = parseOneMetric(t, &yesTags, []byte("a.b.c:1|h"))
	assert.Equal(t, 1, len(m.Tags), "# of tags")
}

func TestParserHistogramFloat(t *testing.T) {
	noTags := samplers.NewParser([]string{})
	yesTags := samplers.NewParser([]string{"implicit"})

	m := parseOneMetric(t, &noTags, []byte("a.b.c:1.234|h"))
	assert.NotNil(t, m, "Got nil metric!")
	assert.Equal(t, "a.b.c", m.Name, "Name")
	assert.Equal(t, float64(1.234), m.Value, "Value")
	assert.Equal(t, "histogram", m.Type, "Type")
	assert.Equal(t, 0, len(m.Tags), "# of tags")

	m = parseOneMetric(t, &yesTags, []byte("a.b.c:1.234|h"))
	assert.Equal(t, 1, len(m.Tags), "# of tags")
}

func TestParserTimer(t *testing.T) {
	noTags := samplers.NewParser([]string{})
	yesTags := samplers.NewParser([]string{"implicit"})

	m := parseOneMetric(t, &noTags, []byte("a.b.c:1|ms"))
	assert.NotNil(t, m, "Got nil metric!")
	assert.Equal(t, "a.b.c", m.Name, "Name")
	assert.Equal(t, float64(1), m.Value, "Value")
	assert.Equal(t, "timer", m.Type, "Type")
	assert.Equal(t, 0, len(m.Tags), "# of tags")

	m = parseOneMetric(t, &yesTags, []byte("a.b.c:1|ms"))
	assert.Equal(t, 1, len(m.Tags), "# of tags")
}

func TestParserTimerAgg(t *testing.T) {
	parser := samplers.NewParser([]string{})
	ms := parseMetrics(t, &parser, []byte("a.b.c:1:2:3:4|ms|@0.1|#result:success,op:frob"))
	assert.Equal(t, 4, len(ms))
	expectedValues := []float64{1, 2, 3, 4}
	for i, m := range ms {
		assert.Equal(t, "a.b.c", m.Name, "Name for record %d", i)
		assert.Equal(t, expectedValues[i], m.Value, "Value for record %d", i)
		assert.Equal(t, "timer", m.Type, "Type for record %d", i)
		assert.Equal(t, []string{"op:frob", "result:success"}, m.Tags, "Tags for record %d", i)
		assert.Equal(t, "op:frob,result:success", m.JoinedTags, "JoinedTags for record %d", i)
		assert.Equal(t, float32(0.1), m.SampleRate, "SampleRate for record %d", i)
		// All records should have the same digest
		assert.Equal(t, ms[0].Digest, m.Digest, "Digest for record %d", i)
		assert.Equal(t, samplers.MixedScope, m.Scope, "Scope for record %d", i)
	}
}

func TestParserDistribution(t *testing.T) {
	noTags := samplers.NewParser([]string{})
	yesTags := samplers.NewParser([]string{"implicit"})

	m := parseOneMetric(t, &noTags, []byte("a.b.c:0.1716441474854946|d|#filter:flatulent"))
	assert.NotNil(t, m, "Got nil metric!")
	assert.Equal(t, "a.b.c", m.Name, "Name")
	assert.Equal(t, float64(0.1716441474854946), m.Value, "Value")
	assert.Equal(t, "histogram", m.Type, "Type")
	assert.Equal(t, 1, len(m.Tags), "# of tags")
	m = parseOneMetric(t, &yesTags, []byte("a.b.c:0.1716441474854946|d|#filter:flatulent"))
	assert.Equal(t, 2, len(m.Tags), "# of tags")
}

func TestParserTimerFloat(t *testing.T) {
	noTags := samplers.NewParser([]string{})
	yesTags := samplers.NewParser([]string{"implicit"})

	m := parseOneMetric(t, &noTags, []byte("a.b.c:1.234|ms"))
	assert.NotNil(t, m, "Got nil metric!")
	assert.Equal(t, "a.b.c", m.Name, "Name")
	assert.Equal(t, float64(1.234), m.Value, "Value")
	assert.Equal(t, "timer", m.Type, "Type")
	assert.Equal(t, 0, len(m.Tags), "# of tags")

	m = parseOneMetric(t, &yesTags, []byte("a.b.c:1.234|ms"))
	assert.Equal(t, 1, len(m.Tags), "# of tags")
}

func TestParserSet(t *testing.T) {
	noTags := samplers.NewParser([]string{})
	yesTags := samplers.NewParser([]string{"implicit"})

	m := parseOneMetric(t, &noTags, []byte("a.b.c:foo|s"))
	assert.NotNil(t, m, "Got nil metric!")
	assert.Equal(t, "a.b.c", m.Name, "Name")
	assert.Equal(t, "foo", m.Value, "Value")
	assert.Equal(t, "set", m.Type, "Type")
	assert.Equal(t, 0, len(m.Tags), "# of tags")

	m = parseOneMetric(t, &yesTags, []byte("a.b.c:foo|s"))
	assert.Equal(t, 1, len(m.Tags), "# of tags")
}

func TestParserWithTags(t *testing.T) {
	noTags := samplers.NewParser([]string{})
	yesTags := samplers.NewParser([]string{"implicit"})

	m := parseOneMetric(t, &noTags, []byte("a.b.c:1|c|#foo:bar,baz:gorch"))
	assert.NotNil(t, m, "Got nil metric!")
	assert.Equal(t, "a.b.c", m.Name, "Name")
	assert.Equal(t, float64(1), m.Value, "Value")
	assert.Equal(t, "counter", m.Type, "Type")
	assert.EqualValues(t, []string{"baz:gorch", "foo:bar"}, m.Tags, "Expected Tags")
	y := parseOneMetric(t, &yesTags, []byte("a.b.c:1|c|#foo:bar,baz:gorch"))
	assert.EqualValues(t, []string{"baz:gorch", "foo:bar", "implicit"}, y.Tags, "Expected Tags")

	// ensure that tag order doesn't matter: it must produce the same digest
	m2 := parseOneMetric(t, &noTags, []byte("a.b.c:1|c|#baz:gorch,foo:bar"))
	assert.EqualValues(t, []string{"baz:gorch", "foo:bar"}, m2.Tags, "Expected Tags")
	assert.Equal(t, m.Digest, m2.Digest, "Digest must not depend on tag order")
	assert.Equal(t, m.MetricKey, m2.MetricKey, "MetricKey must not depend on tag order")
	y2 := parseOneMetric(t, &yesTags, []byte("a.b.c:1|c|#baz:gorch,foo:bar"))
	assert.EqualValues(t, []string{"baz:gorch", "foo:bar", "implicit"}, y2.Tags, "Expected Tags")
	assert.Equal(t, y.Digest, y2.Digest, "Digest must not depend on tag order")
	assert.Equal(t, y.MetricKey, y2.MetricKey, "MetricKey must not depend on tag order")

	// treated as an empty tag
	m = parseOneMetric(t, &noTags, []byte("a.b.c:1|c|#"))
	assert.EqualValues(t, []string{""}, m.Tags, "expected empty tag")
	m = parseOneMetric(t, &yesTags, []byte("a.b.c:1|c|#"))
	assert.EqualValues(t, []string{"", "implicit"}, m.Tags, "expected empty tag")

	valueError := noTags.ParseMetric([]byte("a.b.c:fart|c"), nil)
	assert.NotNil(t, valueError, "No errors when parsing")
	assert.Contains(t, valueError.Error(), "Invalid number", "Invalid number error missing")
}

func TestParserWithSampleRate(t *testing.T) {
	noTags := samplers.NewParser([]string{})
	yesTags := samplers.NewParser([]string{"implicit"})

	m := parseOneMetric(t, &noTags, []byte("a.b.c:1|c|@0.1"))
	assert.NotNil(t, m, "Got nil metric!")
	assert.Equal(t, "a.b.c", m.Name, "Name")
	assert.Equal(t, float64(1), m.Value, "Value")
	assert.Equal(t, "counter", m.Type, "Type")
	assert.Equal(t, float32(0.1), m.SampleRate, "Sample Rate")
	assert.Equal(t, 0, len(m.Tags), "# of tags")
	m = parseOneMetric(t, &yesTags, []byte("a.b.c:1|c|@0.1"))
	assert.Equal(t, 1, len(m.Tags), "# of tags")

	valueError := (&samplers.Parser{}).ParseMetric([]byte("a.b.c:fart|c"), nil)
	assert.NotNil(t, valueError, "No errors when parsing")
	assert.Contains(t, valueError.Error(), "Invalid number", "Invalid number error missing")

	_ = parseOneMetric(t, &samplers.Parser{}, []byte("a.b.c:1|g|@0.1"))
}

func TestParserWithSampleRateAndTags(t *testing.T) {
	noTags := samplers.NewParser([]string{})
	yesTags := samplers.NewParser([]string{"implicit"})

	m := parseOneMetric(t, &noTags, []byte("a.b.c:1|c|@0.1|#foo:bar,baz:gorch"))
	assert.NotNil(t, m, "Got nil metric!")
	assert.Equal(t, "a.b.c", m.Name, "Name")
	assert.Equal(t, float64(1), m.Value, "Value")
	assert.Equal(t, "counter", m.Type, "Type")
	assert.Equal(t, float32(0.1), m.SampleRate, "Sample Rate")
	assert.Len(t, m.Tags, 2, "Tags")
	m = parseOneMetric(t, &yesTags, []byte("a.b.c:1|c|@0.1|#foo:bar,baz:gorch"))
	assert.Len(t, m.Tags, 3, "Tags")

	valueError := (&samplers.Parser{}).ParseMetric([]byte("a.b.c:fart|c"), nil)
	assert.NotNil(t, valueError, "No errors when parsing")
	assert.Contains(t, valueError.Error(), "Invalid number", "Invalid number error missing")
}

func TestInvalidPackets(t *testing.T) {
	table := map[string]string{
		"foo":                                "Invalid metric packet, need at least 1 pipe for type",
		"foo:1":                              "1 pipe",
		"foo:1||":                            "Invalid metric packet, metric type not specified",
		"foo:|c|":                            "Invalid metric packet, empty string after/between pipes",
		"this_is_a_bad_metric:nan|g|#shell":  "Invalid number for metric value",
		"this_is_a_bad_metric:NaN|g|#shell":  "Invalid number for metric value",
		"this_is_a_bad_metric:-inf|g|#shell": "Invalid number for metric value",
		"this_is_a_bad_metric:+inf|g|#shell": "Invalid number for metric value",
		"foo:1|foo|":                         "Invalid type",
		"foo:1|c||":                          "Invalid metric packet, empty string after/between pipes",
		"foo:1|c|foo":                        "unknown section",
		"foo:1|c|@-0.1":                      ">0",
		"foo:1|c|@1.1":                       "<=1",
		"foo:1|c|@0.5|@0.2":                  "multiple sample rates",
		"foo:1|c|#foo|#bar":                  "multiple tag sections",
	}

	for packet, errContent := range table {
		t.Run(packet, func(t *testing.T) {
			err := (&samplers.Parser{}).ParseMetric([]byte(packet), nil)
			assert.Error(t, err, "Should have gotten error parsing %q", packet)
			assert.Contains(t, err.Error(), errContent, "Error should have contained text")
		})
	}
}

func TestLocalOnlyEscape(t *testing.T) {
	m := parseOneMetric(t, &samplers.Parser{}, []byte("a.b.c:1|h|#veneurlocalonly,tag2:quacks"))
	assert.Equal(t, samplers.LocalOnly, m.Scope, "should have gotten local only metric")
	assert.NotContains(t, m.Tags, "veneurlocalonly", "veneurlocalonly should not actually be a tag")
	assert.Contains(t, m.Tags, "tag2:quacks", "tag2 should be preserved in the list of tags after removing magic tags")
}

func TestGlobalOnlyEscape(t *testing.T) {
	m := parseOneMetric(t, &samplers.Parser{}, []byte("a.b.c:1|h|#veneurglobalonly,tag2:quacks"))
	assert.Equal(t, samplers.GlobalOnly, m.Scope, "should have gotten local only metric")
	assert.NotContains(t, m.Tags, "veneurglobalonly", "veneurlocalonly should not actually be a tag")
	assert.Contains(t, m.Tags, "tag2:quacks", "tag2 should be preserved in the list of tags after removing magic tags")
}

func TestEvents(t *testing.T) {
	noTags := samplers.NewParser([]string{})
	yesTags := samplers.NewParser([]string{"implicit"})

	evt, err := noTags.ParseEvent([]byte("_e{3,3}:foo|bar|k:foos|s:test|t:success|p:low|#foo:bar,baz:qux|d:1136239445|h:example.com"))
	assert.NoError(t, err, "should have parsed correctly")
	assert.EqualValues(t, &ssf.SSFSample{
		Name:      "foo",
		Message:   "bar",
		Timestamp: 1136239445,
		Tags: map[string]string{
			dogstatsd.EventIdentifierKey:        "",
			dogstatsd.EventAggregationKeyTagKey: "foos",
			dogstatsd.EventSourceTypeTagKey:     "test",
			dogstatsd.EventAlertTypeTagKey:      "success",
			dogstatsd.EventPriorityTagKey:       "low",
			dogstatsd.EventHostnameTagKey:       "example.com",
			"foo":                               "bar",
			"baz":                               "qux",
		},
	}, evt, "should have parsed event")
	evt, err = yesTags.ParseEvent([]byte("_e{3,3}:foo|bar|k:foos|s:test|t:success|p:low|#foo:bar,baz:qux|d:1136239445|h:example.com"))
	assert.NoError(t, err, "should have parsed correctly")
	assert.Equal(t, map[string]string{
		dogstatsd.EventIdentifierKey:        "",
		dogstatsd.EventAggregationKeyTagKey: "foos",
		dogstatsd.EventSourceTypeTagKey:     "test",
		dogstatsd.EventAlertTypeTagKey:      "success",
		dogstatsd.EventPriorityTagKey:       "low",
		dogstatsd.EventHostnameTagKey:       "example.com",
		"foo":                               "bar",
		"baz":                               "qux",
		"implicit":                          "",
	}, evt.Tags)

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
		_, err := (&samplers.Parser{}).ParseEvent([]byte(packet))
		assert.NotNil(t, err, "Should have gotten error parsing %q", packet)
		assert.Contains(t, err.Error(), errContent, "Error should have contained text")
	}
}

func TestServiceChecks(t *testing.T) {
	noTags := samplers.NewParser([]string{})
	yesTags := samplers.NewParser([]string{"implicit"})

	svcCheck, err := noTags.ParseServiceCheck([]byte("_sc|foo.bar|0|#foo:bar,qux:dor|d:1136239445|h:example.com"))
	assert.NoError(t, err, "should have parsed correctly")
	expected := &samplers.UDPMetric{
		MetricKey: samplers.MetricKey{
			Name:       "foo.bar",
			Type:       "status",
			JoinedTags: "foo:bar,qux:dor",
		},
		SampleRate: 1.0,
		Value:      ssf.SSFSample_OK,
		Timestamp:  1136239445,
		HostName:   "example.com",
		Tags: []string{
			"foo:bar",
			"qux:dor",
		},
	}
	h := fnv1a.Init32
	h = fnv1a.AddString32(h, expected.Name)
	h = fnv1a.AddString32(h, expected.Type)
	h = fnv1a.AddString32(h, expected.JoinedTags)
	expected.Digest = h

	assert.EqualValues(t, expected, svcCheck, "should have parsed event")

	svcCheck, err = yesTags.ParseServiceCheck([]byte("_sc|foo.bar|0|#foo:bar,qux:dor|d:1136239445|h:example.com"))
	assert.NoError(t, err, "should have parsed correctly")
	expected = &samplers.UDPMetric{
		MetricKey: samplers.MetricKey{
			Name:       "foo.bar",
			Type:       "status",
			JoinedTags: "foo:bar,implicit,qux:dor",
		},
		SampleRate: 1.0,
		Value:      ssf.SSFSample_OK,
		Timestamp:  1136239445,
		HostName:   "example.com",
		Tags: []string{
			"foo:bar",
			"implicit",
			"qux:dor",
		},
	}
	h = fnv1a.Init32
	h = fnv1a.AddString32(h, expected.Name)
	h = fnv1a.AddString32(h, expected.Type)
	h = fnv1a.AddString32(h, expected.JoinedTags)
	expected.Digest = h

	assert.EqualValues(t, expected, svcCheck, "should have parsed event")

	table := map[string]string{
		"foo.bar|0":           "_sc",
		"_sc|foo.bar":         "status",
		"_sc|foo.bar|5":       "status",
		"_sc|foo.bar|0||":     "pipe",
		"_sc|foo.bar|0|d:abc": "date",
	}
	for packet, errContent := range table {
		_, err := (&samplers.Parser{}).ParseServiceCheck([]byte(packet))
		assert.NotNil(t, err, "Should have gotten error parsing %q", packet)
		assert.Contains(t, err.Error(), errContent, "Error should have contained text")
	}
}

func TestEventMessageUnescape(t *testing.T) {
	packet := []byte("_e{3,15}:foo|foo\\nbar\\nbaz\\n")
	evt, err := (&samplers.Parser{}).ParseEvent(packet)
	assert.NoError(t, err, "Should have parsed correctly")
	assert.Equal(t, "foo\nbar\nbaz\n", evt.Message, "Should contain newline")
}

func TestServiceCheckMessageUnescape(t *testing.T) {
	packet := []byte("_sc|foo|0|m:foo\\nbar\\nbaz\\n")
	svcheck, err := (&samplers.Parser{}).ParseServiceCheck(packet)
	assert.NoError(t, err, "Should have parsed correctly")
	assert.Equal(t, "foo\nbar\nbaz\n", svcheck.Message, "Should contain newline")
}

func TestServiceCheckMessageStatus(t *testing.T) {
	packet := []byte("_sc|foo|1|m:foo")
	svcheck, err := (&samplers.Parser{}).ParseServiceCheck(packet)
	assert.NoError(t, err, "Should have parsed correctly")
	assert.Equal(t, "foo", svcheck.Message, "Should contain newline")
	assert.Equal(t, ssf.SSFSample_WARNING, svcheck.Value, "Should parse status correctly")
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

var benchResult uint32

func BenchmarkParseMetric(b *testing.B) {
	var total uint32

	metricName := "a.b.c"
	metricValues := []struct {
		name        string
		metricValue string
		metricType  string
	}{
		{"IntCount", "1", "c"},
		{"FloatHistogram", "4.5", "h"},
	}
	explicitTags := []struct {
		name       string
		metricTags string
	}{
		{"ZeroTags", ""},
		{"OneTag", "baz:gorch"},
		{"TwoTags", "foo:bar,baz:gorch"},
	}
	implicitTags := []struct {
		name       string
		metricTags []string
	}{
		{"NoImplicit", []string{}},
		{"OneImplicit", []string{"six:6"}},
		{"TwoImplicit", []string{"three:three", "six:6", "nine:9", "ten:10", "eleven:11", "twelve:12"}},
	}

	for _, mv := range metricValues {
		for _, it := range implicitTags {
			for _, et := range explicitTags {
				var statsd string
				if et.metricTags == "" {
					statsd = fmt.Sprintf("%s:%s|%s", metricName, mv.metricValue, mv.metricType)
				} else {
					statsd = fmt.Sprintf("%s:%s|%s#%s", metricName, mv.metricValue, mv.metricType, et.metricTags)
				}
				benchName := fmt.Sprintf("%s%s%s", mv.name, it.name, et.name)
				metricBytes := []byte(statsd)
				parser := samplers.NewParser(it.metricTags)
				b.Run(benchName, func(b *testing.B) {
					b.ReportAllocs()
					for n := 0; n < b.N; n++ {
						err := parser.ParseMetric(metricBytes, func(m *samplers.UDPMetric) { total += m.Digest })
						if err != nil {
							b.Fatal(err)
						}
					}
				})
			}
		}
	}
	// Attempt to avoid compiler optimizations? Is this relevant?
	benchResult = total
}
