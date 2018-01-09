package signalfx

import (
	"context"
	"sort"
	"testing"
	"time"

	"github.com/DataDog/datadog-go/statsd"
	"github.com/signalfx/golib/datapoint"
	"github.com/signalfx/golib/event"
	"github.com/signalfx/golib/sfxclient"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stripe/veneur/samplers"
)

type FakeSink struct {
	points []*datapoint.Datapoint
	events []*event.Event
}

func NewFakeSink() *FakeSink {
	return &FakeSink{
		points: []*datapoint.Datapoint{},
	}
}

func (fs *FakeSink) AddDatapoints(ctx context.Context, points []*datapoint.Datapoint) (err error) {
	fs.points = append(fs.points, points...)
	return nil
}

func (fs *FakeSink) AddEvents(ctx context.Context, events []*event.Event) (err error) {
	fs.events = append(fs.events, events...)
	return nil
}

func TestNewSignalFxSink(t *testing.T) {
	// test the variables that have been renamed
	stats, _ := statsd.NewBuffered("localhost:1235", 1024)
	sink, err := NewSignalFXSink("secret", "http://www.example.com", "host", "glooblestoots", map[string]string{"yay": "pie"}, stats, logrus.New(), nil)
	if err != nil {
		t.Fatal(err)
	}
	err = sink.Start(nil)
	if err != nil {
		t.Fatal(err)
	}

	httpsink, ok := sink.client.(*sfxclient.HTTPSink)
	if !ok {
		assert.Fail(t, "SignalFX sink isn't the correct type")
	}
	assert.Equal(t, "http://www.example.com/v2/datapoint", httpsink.DatapointEndpoint)
	assert.Equal(t, "http://www.example.com/v2/event", httpsink.EventEndpoint)

	assert.Equal(t, "http://www.example.com", sink.endpoint)
	assert.Equal(t, "signalfx", sink.Name())
	assert.Equal(t, "host", sink.hostnameTag)
	assert.Equal(t, "glooblestoots", sink.hostname)
	assert.Equal(t, map[string]string{"yay": "pie"}, sink.commonDimensions)
}

func TestSignalFxFlushRouting(t *testing.T) {
	stats, _ := statsd.NewBuffered("localhost:1235", 1024)
	fakeSink := NewFakeSink()
	sink, err := NewSignalFXSink("secret", "http://www.example.com", "host", "glooblestoots", map[string]string{"yay": "pie"}, stats, logrus.New(), fakeSink)

	assert.NoError(t, err)

	interMetrics := []samplers.InterMetric{samplers.InterMetric{
		Name:      "any",
		Timestamp: 1476119058,
		Value:     float64(100),
		Tags: []string{
			"foo:bar",
			"baz:quz",
		},
		Type: samplers.GaugeMetric,
	},
		samplers.InterMetric{
			Name:      "sfx",
			Timestamp: 1476119058,
			Value:     float64(100),
			Tags: []string{
				"foo:bar",
				"baz:quz",
				"veneursinkonly:signalfx",
			},
			Type:  samplers.GaugeMetric,
			Sinks: samplers.RouteInformation{"signalfx": struct{}{}},
		},
		samplers.InterMetric{
			Name:      "not.us",
			Timestamp: 1476119058,
			Value:     float64(100),
			Tags: []string{
				"foo:bar",
				"baz:quz",
				"veneursinkonly:anyone_else",
			},
			Type:  samplers.GaugeMetric,
			Sinks: samplers.RouteInformation{"anyone_else": struct{}{}},
		},
	}

	sink.Flush(context.TODO(), interMetrics)

	assert.Equal(t, 2, len(fakeSink.points))
	metrics := make([]string, 0, len(fakeSink.points))
	for _, pt := range fakeSink.points {
		metrics = append(metrics, pt.Metric)
	}
	sort.Strings(metrics)
	assert.Equal(t, []string{"any", "sfx"}, metrics)
}

func TestSignalFxFlushGauge(t *testing.T) {
	stats, _ := statsd.NewBuffered("localhost:1235", 1024)
	fakeSink := NewFakeSink()
	sink, err := NewSignalFXSink("secret", "http://www.example.com", "host", "glooblestoots", map[string]string{"yay": "pie"}, stats, logrus.New(), fakeSink)

	assert.NoError(t, err)

	interMetrics := []samplers.InterMetric{samplers.InterMetric{
		Name:      "a.b.c",
		Timestamp: 1476119058,
		Value:     float64(100),
		Tags: []string{
			"foo:bar",
			"baz:quz",
		},
		Type: samplers.GaugeMetric,
	}}

	sink.Flush(context.TODO(), interMetrics)

	assert.Equal(t, 1, len(fakeSink.points))
	point := fakeSink.points[0]
	assert.Equal(t, "a.b.c", point.Metric, "Metric has wrong name")
	assert.Equal(t, datapoint.Gauge, point.MetricType, "Metric has wrong type")
	dims := point.Dimensions
	assert.Equal(t, 3, len(dims), "Metric has incorrect tag count")
	assert.Equal(t, "bar", dims["foo"], "Metric has a busted tag")
	assert.Equal(t, "quz", dims["baz"], "Metric has a busted tag")
	assert.Equal(t, "pie", dims["yay"], "Metric is missing common tag")
}

func TestSignalFxFlushCounter(t *testing.T) {
	stats, _ := statsd.NewBuffered("localhost:1235", 1024)
	fakeSink := NewFakeSink()
	sink, err := NewSignalFXSink("secret", "http://www.example.com", "host", "glooblestoots", map[string]string{"yay": "pie"}, stats, logrus.New(), fakeSink)

	assert.NoError(t, err)

	interMetrics := []samplers.InterMetric{samplers.InterMetric{
		Name:      "a.b.c",
		Timestamp: 1476119058,
		Value:     10,
		Tags: []string{
			"foo:bar",
			"baz:quz",
			"novalue",
		},
		Type: samplers.CounterMetric,
	}}

	sink.Flush(context.TODO(), interMetrics)

	assert.Equal(t, 1, len(fakeSink.points))
	point := fakeSink.points[0]
	assert.Equal(t, "a.b.c", point.Metric, "Metric has wrong name")
	assert.Equal(t, datapoint.Count, point.MetricType, "Metric has wrong type")
	dims := point.Dimensions
	assert.Equal(t, 4, len(dims), "Metric has incorrect tag count")
	assert.Equal(t, "bar", dims["foo"], "Metric has a busted tag")
	assert.Equal(t, "quz", dims["baz"], "Metric has a busted tag")
	assert.Equal(t, "", dims["novalue"], "Metric has a busted tag")
	assert.Equal(t, "pie", dims["yay"], "Metric is missing a common tag")
}

func TestSignalFxEventFlush(t *testing.T) {
	stats, _ := statsd.NewBuffered("localhost:1235", 1024)
	fakeSink := NewFakeSink()
	sink, err := NewSignalFXSink("secret", "http://www.example.com", "host", "glooblestoots", map[string]string{"yay": "pie"}, stats, logrus.New(), fakeSink)

	assert.NoError(t, err)

	ev := samplers.UDPEvent{
		Title:     "Farts farts farts",
		Timestamp: time.Now().Unix(),
		Tags:      []string{"foo:bar", "baz:gorch", "novalue"},
	}
	sink.FlushEventsChecks(context.TODO(), []samplers.UDPEvent{ev}, nil)

	assert.Equal(t, 1, len(fakeSink.events))
	event := fakeSink.events[0]
	assert.Equal(t, ev.Title, event.EventType)
	dims := event.Dimensions
	assert.Equal(t, 4, len(dims), "Event has incorrect tag count")
	assert.Equal(t, "bar", dims["foo"], "Event has a busted tag")
	assert.Equal(t, "gorch", dims["baz"], "Event has a busted tag")
	assert.Equal(t, "pie", dims["yay"], "Event missing a common tag")
	assert.Equal(t, "", dims["novalue"], "Event has a busted tag")
}
