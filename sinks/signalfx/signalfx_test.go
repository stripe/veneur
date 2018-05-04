package signalfx

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sort"
	"strconv"
	"testing"
	"time"

	"github.com/signalfx/golib/datapoint"
	"github.com/signalfx/golib/event"
	"github.com/signalfx/golib/sfxclient"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stripe/veneur/protocol/dogstatsd"
	"github.com/stripe/veneur/samplers"
	"github.com/stripe/veneur/ssf"
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

type testDerivedSink struct {
	samples []*ssf.SSFSample
}

func (d *testDerivedSink) SendSample(sample *ssf.SSFSample) error {
	d.samples = append(d.samples, sample)
	return nil
}

func newDerivedProcessor() *testDerivedSink {
	return &testDerivedSink{
		samples: []*ssf.SSFSample{},
	}
}

func TestNewSignalFxSink(t *testing.T) {
	// test the variables that have been renamed
	client := NewClient("http://www.example.com", "secret", *http.DefaultClient)
	derived := newDerivedProcessor()
	sink, err := NewSignalFxSink("host", "glooblestoots", map[string]string{"yay": "pie"}, logrus.New(), client, "", nil, derived)
	if err != nil {
		t.Fatal(err)
	}
	err = sink.Start(nil)
	if err != nil {
		t.Fatal(err)
	}

	httpsink, ok := client.(*sfxclient.HTTPSink)
	if !ok {
		assert.Fail(t, "SignalFx sink isn't the correct type")
	}
	assert.Equal(t, "http://www.example.com/v2/datapoint", httpsink.DatapointEndpoint)
	assert.Equal(t, "http://www.example.com/v2/event", httpsink.EventEndpoint)

	assert.Equal(t, "signalfx", sink.Name())
	assert.Equal(t, "host", sink.hostnameTag)
	assert.Equal(t, "glooblestoots", sink.hostname)
	assert.Equal(t, map[string]string{"yay": "pie"}, sink.commonDimensions)
}

func TestSignalFxFlushRouting(t *testing.T) {
	fakeSink := NewFakeSink()
	derived := newDerivedProcessor()
	sink, err := NewSignalFxSink("host", "glooblestoots", map[string]string{"yay": "pie"}, logrus.New(), fakeSink, "", nil, derived)

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
	fakeSink := NewFakeSink()
	derived := newDerivedProcessor()
	sink, err := NewSignalFxSink("host", "glooblestoots", map[string]string{"yay": "pie"}, logrus.New(), fakeSink, "", nil, derived)

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
	val, err := strconv.Atoi(point.Value.String())
	assert.Nil(t, err, "Failed to parse value as integer")
	assert.Equal(t, int(interMetrics[0].Value), val, "Status translates to gauge Value")
	dims := point.Dimensions
	assert.Equal(t, 4, len(dims), "Metric has incorrect tag count")
	assert.Equal(t, "bar", dims["foo"], "Metric has a busted tag")
	assert.Equal(t, "quz", dims["baz"], "Metric has a busted tag")
	assert.Equal(t, "pie", dims["yay"], "Metric is missing common tag")
	assert.Equal(t, "glooblestoots", dims["host"], "Metric is missing host tag")
	assert.Empty(t, derived.samples, "Gauges should not generated derived metrics")
}

func TestSignalFxFlushCounter(t *testing.T) {
	fakeSink := NewFakeSink()
	derived := newDerivedProcessor()
	sink, err := NewSignalFxSink("host", "glooblestoots", map[string]string{"yay": "pie"}, logrus.New(), fakeSink, "", nil, derived)
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
	val, err := strconv.Atoi(point.Value.String())
	assert.Nil(t, err, "Failed to parse value as integer")
	assert.Equal(t, int(interMetrics[0].Value), val, "Status translates to gauge Value")
	dims := point.Dimensions
	assert.Equal(t, 5, len(dims), "Metric has incorrect tag count")
	assert.Equal(t, "bar", dims["foo"], "Metric has a busted tag")
	assert.Equal(t, "quz", dims["baz"], "Metric has a busted tag")
	assert.Equal(t, "", dims["novalue"], "Metric has a busted tag")
	assert.Equal(t, "pie", dims["yay"], "Metric is missing a common tag")
	assert.Equal(t, "glooblestoots", dims["host"], "Metric is missing host tag")
	assert.Empty(t, derived.samples, "Counters should not generated derived metrics")
}

func TestSignalFxFlushStatus(t *testing.T) {
	fakeSink := NewFakeSink()
	derived := newDerivedProcessor()
	sink, err := NewSignalFxSink("host", "glooblestoots", map[string]string{"yay": "pie"}, logrus.New(), fakeSink, "", nil, derived)
	assert.NoError(t, err)

	interMetrics := []samplers.InterMetric{samplers.InterMetric{
		Name:      "a.b.c",
		Timestamp: 1476119058,
		Value:     float64(ssf.SSFSample_UNKNOWN),
		Tags: []string{
			"foo:bar",
			"baz:quz",
			"novalue",
			"veneursinkonly:signalfx", // should not be present in the reported metric
		},
		Type: samplers.StatusMetric,
	}}

	sink.Flush(context.TODO(), interMetrics)

	assert.Equal(t, 1, len(fakeSink.points))
	point := fakeSink.points[0]
	assert.Equal(t, "a.b.c", point.Metric, "Metric has wrong name")
	assert.Equal(t, datapoint.Gauge, point.MetricType, "Metric has wrong type")
	val, err := strconv.Atoi(point.Value.String())
	assert.Nil(t, err, "Failed to parse value as integer")
	assert.Equal(t, int(ssf.SSFSample_UNKNOWN), val, "Status translates to gauge Value")
	dims := point.Dimensions
	assert.Equal(t, 5, len(dims), "Metric has incorrect tag count")
	assert.Equal(t, "bar", dims["foo"], "Metric has a busted tag")
	assert.Equal(t, "quz", dims["baz"], "Metric has a busted tag")
	assert.Equal(t, "", dims["novalue"], "Metric has a busted tag")
	assert.Equal(t, "pie", dims["yay"], "Metric is missing a common tag")
	assert.Equal(t, "glooblestoots", dims["host"], "Metric is missing host tag")
	assert.Empty(t, derived.samples, "Counters should not generated derived metrics")
}

func TestSignalFxServiceCheckFlushOther(t *testing.T) {
	fakeSink := NewFakeSink()
	derived := newDerivedProcessor()
	sink, err := NewSignalFxSink("host", "glooblestoots", map[string]string{"yay": "pie"}, logrus.New(), fakeSink, "", nil, derived)
	assert.NoError(t, err)

	serviceCheckMsg := "Service Farts starting[an example link](http://catchpoint.com/session_id \"Title\")"
	ev := ssf.SSFSample{
		Name: "Farts farts farts",
		// Include the markdown bits DD expects, we'll trim it out hopefully!
		Message:   "%%% \n " + serviceCheckMsg + " \n %%%",
		Timestamp: time.Now().Unix(),
		Tags:      map[string]string{"foo": "bar", "baz": "gorch", "novalue": ""},
		Status:    ssf.SSFSample_CRITICAL,
	}
	sink.FlushOtherSamples(context.TODO(), []ssf.SSFSample{ev})

	assert.Empty(t, fakeSink.events)
	assert.Empty(t, derived.samples, "Should ignore any service check")
}

func TestSignalFxEventFlush(t *testing.T) {
	fakeSink := NewFakeSink()
	derived := newDerivedProcessor()
	sink, err := NewSignalFxSink("host", "glooblestoots", map[string]string{"yay": "pie"}, logrus.New(), fakeSink, "", nil, derived)
	assert.NoError(t, err)

	evMessage := "[an example link](http://catchpoint.com/session_id \"Title\")"
	ev := ssf.SSFSample{
		Name: "Farts farts farts",
		// Include the markdown bits DD expects, we'll trim it out hopefully!
		Message:   "%%% \n " + evMessage + " \n %%%",
		Timestamp: time.Now().Unix(),
		Tags:      map[string]string{"foo": "bar", "baz": "gorch", "novalue": "", dogstatsd.EventIdentifierKey: ""},
	}
	sink.FlushOtherSamples(context.TODO(), []ssf.SSFSample{ev})

	assert.Equal(t, 1, len(fakeSink.events))
	event := fakeSink.events[0]
	assert.Equal(t, ev.Name, event.EventType)
	// We're checking this to ensure the above markdown is also gone!
	assert.Equal(t, event.Properties["description"], evMessage)
	dims := event.Dimensions
	// 5 because 5 passed in, 1 eliminated (event identifier) and 1 added (host!)
	assert.Equal(t, 5, len(dims), "Event has incorrect tag count")
	assert.Equal(t, "bar", dims["foo"], "Event has a busted tag")
	assert.Equal(t, "gorch", dims["baz"], "Event has a busted tag")
	assert.Equal(t, "pie", dims["yay"], "Event missing a common tag")
	assert.Equal(t, "", dims["novalue"], "Event has a busted tag")
	assert.Equal(t, "glooblestoots", dims["host"], "Event is missing host tag")
}

func TestSignalFxSetExcludeTags(t *testing.T) {
	fakeSink := NewFakeSink()
	derived := newDerivedProcessor()
	sink, err := NewSignalFxSink("host", "glooblestoots", map[string]string{"yay": "pie", "boo": "snakes"}, logrus.New(), fakeSink, "", nil, derived)

	sink.SetExcludedTags([]string{"foo", "boo", "host"})
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
	sink.Flush(context.Background(), interMetrics)

	ev := ssf.SSFSample{
		Name:      "Test Event",
		Timestamp: time.Now().Unix(),
		Tags: map[string]string{
			dogstatsd.EventIdentifierKey: "",
			"foo":     "bar",
			"baz":     "gorch",
			"novalue": "",
		},
	}

	sink.FlushOtherSamples(context.Background(), []ssf.SSFSample{ev})

	assert.Equal(t, 1, len(fakeSink.points))
	point := fakeSink.points[0]
	assert.Equal(t, "a.b.c", point.Metric, "Metric has wrong name")
	assert.Equal(t, datapoint.Count, point.MetricType, "Metric has wrong type")
	val, err := strconv.Atoi(point.Value.String())
	assert.Nil(t, err, "Failed to parse value as integer")
	assert.Equal(t, int(interMetrics[0].Value), val, "Status translates to gauge Value")
	dims := point.Dimensions
	assert.Equal(t, 3, len(dims), "Metric has incorrect tag count")
	assert.Equal(t, "", dims["foo"], "Metric has a foo tag despite exclude rule")
	assert.Equal(t, "quz", dims["baz"], "Metric has a busted tag")
	assert.Equal(t, "", dims["novalue"], "Metric has a busted tag")
	assert.Equal(t, "pie", dims["yay"], "Metric is missing a common tag")
	assert.Equal(t, "", dims["boo"], "Metric has host tag despite exclude rule")

	assert.Equal(t, 1, len(fakeSink.events))
	event := fakeSink.events[0]
	assert.Equal(t, ev.Name, event.EventType)
	dims = event.Dimensions
	assert.Equal(t, 3, len(dims), "Event has incorrect tag count")
	assert.Equal(t, "", dims["foo"], "Event has a foo tag despite exclude rule")
	assert.Equal(t, "gorch", dims["baz"], "Event has a busted tag")
	assert.Equal(t, "pie", dims["yay"], "Event missing a common tag")
	assert.Equal(t, "", dims["novalue"], "Event has a busted tag")
	assert.Equal(t, "", dims["boo"], "Event has host tag despite exclude rule")
	assert.Empty(t, derived.samples, "Events should not generated derived metrics")
}

func TestSignalFxFlushMultiKey(t *testing.T) {
	fallback := NewFakeSink()
	specialized := NewFakeSink()

	derived := newDerivedProcessor()
	sink, err := NewSignalFxSink("host", "glooblestoots", map[string]string{"yay": "pie"}, logrus.New(), fallback, "test_by", map[string]DPClient{"available": specialized}, derived)

	assert.NoError(t, err)

	interMetrics := []samplers.InterMetric{
		samplers.InterMetric{
			Name:      "a.b.c",
			Timestamp: 1476119058,
			Value:     float64(100),
			Tags: []string{
				"foo:bar",
				"baz:quz",
				"test_by:needs_fallback",
			},
			Type: samplers.GaugeMetric,
		},
		samplers.InterMetric{
			Name:      "a.b.c",
			Timestamp: 1476119058,
			Value:     float64(99),
			Tags: []string{
				"foo:bar",
				"baz:quz",
				"test_by:available",
			},
			Type: samplers.GaugeMetric,
		},
	}

	sink.Flush(context.TODO(), interMetrics)

	assert.Equal(t, 1, len(fallback.points))
	assert.Equal(t, 1, len(specialized.points))
	{
		point := fallback.points[0]
		assert.Equal(t, "a.b.c", point.Metric, "Metric has wrong name")
		assert.Equal(t, datapoint.Gauge, point.MetricType, "Metric has wrong type")
		val, err := strconv.Atoi(point.Value.String())
		assert.Nil(t, err, "Failed to parse value as integer")
		assert.Equal(t, int(interMetrics[0].Value), val, "Status translates to gauge Value")
		dims := point.Dimensions
		assert.Equal(t, 5, len(dims), "Metric has incorrect tag count")
		assert.Equal(t, "bar", dims["foo"], "Metric has a busted tag")
		assert.Equal(t, "quz", dims["baz"], "Metric has a busted tag")
		assert.Equal(t, "pie", dims["yay"], "Metric is missing common tag")
		assert.Equal(t, "glooblestoots", dims["host"], "Metric is missing host tag")
		assert.Equal(t, "needs_fallback", dims["test_by"], "Metric should have the right test_by tag")
	}
	{
		point := specialized.points[0]
		assert.Equal(t, "a.b.c", point.Metric, "Metric has wrong name")
		assert.Equal(t, datapoint.Gauge, point.MetricType, "Metric has wrong type")
		val, err := strconv.Atoi(point.Value.String())
		assert.Nil(t, err, "Failed to parse value as integer")
		assert.Equal(t, int(interMetrics[1].Value), val, "Status translates to gauge Value")
		dims := point.Dimensions
		assert.Equal(t, 5, len(dims), "Metric has incorrect tag count")
		assert.Equal(t, "bar", dims["foo"], "Metric has a busted tag")
		assert.Equal(t, "quz", dims["baz"], "Metric has a busted tag")
		assert.Equal(t, "pie", dims["yay"], "Metric is missing common tag")
		assert.Equal(t, "glooblestoots", dims["host"], "Metric is missing host tag")
		assert.Equal(t, "available", dims["test_by"], "Metric should have the right test_by tag")
	}
	assert.Empty(t, derived.samples, "Gauges should not generated derived metrics")
}

func TestSignalFxMetricsTimeout(t *testing.T) {
	never := make(chan struct{})
	defer close(never)

	derived := newDerivedProcessor()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-never
	}))
	fallback := NewClient(srv.URL, "fakekey", *http.DefaultClient)
	defer srv.Close()
	sink, err := NewSignalFxSink("host", srv.URL, map[string]string{"yay": "pie"}, logrus.New(), fallback, "", nil, derived)
	require.NoError(t, err)

	ctx := context.Background()
	ctx, cancel := context.WithTimeout(ctx, 1*time.Nanosecond)
	defer cancel()

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
	// Failure to account for the timeout will cause a test
	// timeout here:
	require.Error(t, sink.Flush(ctx, interMetrics))
}
