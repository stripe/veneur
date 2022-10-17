package veneur_test

import (
	"context"
	"testing"
	"time"

	"github.com/sirupsen/logrus/hooks/test"
	"github.com/stretchr/testify/assert"
	"github.com/stripe/veneur/v14"
	"github.com/stripe/veneur/v14/samplers"
)

type tcData struct {
	reason  string
	metrics []samplers.InterMetric
}

type tcExpected struct {
	reason string
	data   map[string]int64
}

var testCases = []struct {
	name     string
	data     []tcData
	expected []tcExpected
}{
	{
		name: "simple case",
		data: []tcData{
			{
				reason: "simple",
				metrics: []samplers.InterMetric{
					{
						Name: "foo",
					},
				},
			},
		},
		expected: []tcExpected{
			{
				reason: "simple",
				data: map[string]int64{
					"foo": 1,
				},
			},
		},
	},
	{
		name: "simple multiple same metric submitted",
		data: []tcData{
			{
				reason: "simple",
				metrics: []samplers.InterMetric{
					{
						Name: "foo",
					},
					{
						Name: "foo",
					},
					{
						Name: "foo",
					},
				},
			},
		},
		expected: []tcExpected{
			{
				reason: "simple",
				data: map[string]int64{
					"foo": 3,
				},
			},
		},
	},
	{
		name: "multiple metrics with different reasons",
		data: []tcData{
			{
				reason: "one",
				metrics: []samplers.InterMetric{
					{
						Name: "foo",
					},
				},
			},
			{
				reason: "two",
				metrics: []samplers.InterMetric{
					{
						Name: "bar",
					},
				},
			},
		},
		expected: []tcExpected{
			{
				reason: "one",
				data: map[string]int64{
					"foo": 1,
				},
			},
			{
				reason: "two",
				data: map[string]int64{
					"bar": 1,
				},
			},
		},
	},
	{
		name: "multiple overlapping metrics with different reasons",
		data: []tcData{
			{
				reason: "one",
				metrics: []samplers.InterMetric{
					{
						Name: "foo",
					},
					{
						Name: "foo",
					},
					{
						Name: "foo",
					},
					{
						Name: "bar",
					},
				},
			},
			{
				reason: "two",
				metrics: []samplers.InterMetric{
					{
						Name: "bar",
					},
					{
						Name: "bar",
					},
					{
						Name: "bar",
					},
					{
						Name: "foo",
					},
				},
			},
		},
		expected: []tcExpected{
			{
				reason: "one",
				data: map[string]int64{
					"foo": 3,
					"bar": 1,
				},
			},
			{
				reason: "two",
				data: map[string]int64{
					"bar": 3,
					"foo": 1,
				},
			},
		},
	},
}

func Test_DroppedMetrics(t *testing.T) {
	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tl, hook := test.NewNullLogger()
			l := tl.WithField("process", "dropped_metrics_tracker")
			dmt := veneur.NewDroppedMetricsTracker(l)
			dmt.Start(context.Background(), time.Second)
			for _, d := range tc.data {
				for _, m := range d.metrics {
					dmt.Track(d.reason, m)
				}
			}

			time.Sleep(time.Second * 2)

			entries := hook.AllEntries()
			entryMap := map[string]map[string]interface{}{}
			for _, entry := range entries {
				data := entry.Data
				if r, ok := data["reason"]; ok {
					r := r.(string)
					entryMap[r] = data
				}
			}

			for _, expected := range tc.expected {
				data, ok := entryMap[expected.reason]
				if !ok {
					assert.Failf(t, "could not find expected key '%s'", expected.reason)
					break
				}

				for k, v := range expected.data {
					res, ok := data[k]
					if !ok {
						assert.Failf(t, "could not find expected key in data '%s'", k)
					}

					assert.Equal(t, v, res.(int64))
				}
			}
		})
	}
}
