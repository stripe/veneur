package samplers_test

import (
	"math/rand"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stripe/veneur/v14/samplers"
	"github.com/stripe/veneur/v14/samplers/metricpb"
	"github.com/stripe/veneur/v14/ssf"
	"github.com/stripe/veneur/v14/util/matcher"
)

func TestNewMetricKeyFromMetric(t *testing.T) {
	key := samplers.NewMetricKeyFromMetric(&metricpb.Metric{
		Name: "metric_name",
		Tags: []string{
			"key1:value1",
			"key2:value2",
			"le:5",
		},
		Type: metricpb.Type_Counter,
	}, []matcher.TagMatcher{})

	assert.Equal(t, key.JoinedTags, "key1:value1,key2:value2,le:5")
	assert.Equal(t, key.Type, "counter")
	assert.Equal(t, key.Name, "metric_name")
}

func TestNewMetricKeyFromMetricWithIgnoredTag(t *testing.T) {
	key := samplers.NewMetricKeyFromMetric(&metricpb.Metric{
		Name: "metric_name",
		Tags: []string{
			"key1:value1",
			"key2:value2",
			"le:5",
		},
		Type: metricpb.Type_Counter,
	}, []matcher.TagMatcher{
		matcher.CreateTagMatcher(&matcher.TagMatcherConfig{
			Kind:  "prefix",
			Value: "le:",
		}),
	})

	assert.Equal(t, key.JoinedTags, "key1:value1,key2:value2")
	assert.Equal(t, key.Type, "counter")
	assert.Equal(t, key.Name, "metric_name")
}

func BenchmarkConvertSpanUniquenessMetrics(b *testing.B) {
	p := samplers.Parser{}

	const LEN = 10000

	spans := make([]*ssf.SSFSpan, LEN)

	for i, _ := range spans {
		p := make([]byte, 10)
		_, err := rand.Read(p)
		if err != nil {
			b.Fatalf("Error generating data: %s", err)
		}
		span := &ssf.SSFSpan{
			Name:    "my.test.span." + string(p[:2]),
			Service: "bigmouth",
			Metrics: []*ssf.SSFSample{
				{
					Name:       "my.test.metric",
					Value:      rand.Float32(),
					Timestamp:  time.Now().Unix(),
					SampleRate: rand.Float32(),
					Tags: map[string]string{
						"keats":       "false",
						"yeats":       "false",
						"wilde":       "true",
						string(p[:5]): string(p[5:]),
					},
				},
				{
					Name:       string(p),
					Value:      rand.Float32(),
					Timestamp:  time.Now().Unix(),
					SampleRate: rand.Float32(),
					Tags: map[string]string{
						"keats":       "true",
						"yeats":       "true",
						"wilde":       "false",
						string(p[2:]): string(p[7:]),
					},
				},
				{
					Name:       string(p[1:]),
					Value:      rand.Float32(),
					Timestamp:  time.Now().Unix(),
					SampleRate: rand.Float32(),
					Tags: map[string]string{
						"keats":       "yeats",
						"wilde":       "weird",
						string(p[:3]): string(p[2:]),
					},
				},
			},
		}
		spans[i] = span
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		result, err := p.ConvertSpanUniquenessMetrics(spans[(i%LEN)], 1)
		if err != nil {
			b.Fatalf("Error converting span uniqueness metrics: %s", err)
		}
		if len(result) == 0 {
			b.Fatalf("Received zero-length uniqueness metric")
		}
	}
}
