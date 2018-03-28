package samplers

import (
	"math/rand"
	"testing"
	"time"

	"github.com/stripe/veneur/ssf"
)

func BenchmarkConvertSpanUniquenessMetrics(b *testing.B) {
	const LEN = 10000

	spans := make([]*ssf.SSFSpan, LEN)

	for i, _ := range spans {
		p := make([]byte, 10)
		_, err := rand.Read(p)
		if err != nil {
			b.Fatalf("Error generating data: %s", err)
		}
		span := &ssf.SSFSpan{
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
		_, err := ConvertSpanUniquenessMetrics(spans[(i%LEN)], .01)
		if err != nil {
			b.Fatalf("Error converting span uniqueness metrics: %s", err)
		}
	}
}
