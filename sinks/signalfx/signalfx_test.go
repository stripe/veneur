package signalfx

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/signalfx/golib/v3/datapoint"
	"github.com/signalfx/golib/v3/event"
	"github.com/signalfx/golib/v3/sfxclient"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	veneur "github.com/stripe/veneur/v14"
	"github.com/stripe/veneur/v14/protocol/dogstatsd"
	"github.com/stripe/veneur/v14/samplers"
	"github.com/stripe/veneur/v14/ssf"
	"github.com/stripe/veneur/v14/util"
)

type FakeSink struct {
	mtx       sync.Mutex
	points    []*datapoint.Datapoint
	pointAdds int
	eventAdds int
	events    []*event.Event
}

func NewFakeSink() *FakeSink {
	return &FakeSink{
		points: []*datapoint.Datapoint{},
	}
}

func (fs *FakeSink) AddDatapoints(ctx context.Context, points []*datapoint.Datapoint) (err error) {
	fs.mtx.Lock()
	defer fs.mtx.Unlock()

	fs.points = append(fs.points, points...)
	fs.pointAdds += 1
	return nil
}

func (fs *FakeSink) AddEvents(ctx context.Context, events []*event.Event) (err error) {
	fs.mtx.Lock()
	defer fs.mtx.Unlock()

	fs.events = append(fs.events, events...)
	fs.eventAdds += 1
	return nil
}

type failSink struct{}

func (fs failSink) AddDatapoints(ctx context.Context, points []*datapoint.Datapoint) (err error) {
	return errors.New("simulated failure to send")
}

func (fs failSink) AddEvents(ctx context.Context, events []*event.Event) (err error) {
	return errors.New("simulated failure to send")
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

func TestMigrateConfig(t *testing.T) {
	config := veneur.Config{
		SignalfxAPIKey: util.StringSecret{Value: "signalfx-api-key"},
	}
	MigrateConfig(&config)
	assert.Len(t, config.MetricSinks, 1)
	parsedConfig, err := ParseConfig("singalfx", config.MetricSinks[0].Config)
	assert.Nil(t, err)
	signalFxConfig, ok := parsedConfig.(SignalFxSinkConfig)
	assert.True(t, ok)
	assert.Equal(t, "signalfx-api-key", signalFxConfig.APIKey.Value)
}

func TestNewSignalFxSink(t *testing.T) {
	// test the variables that have been renamed
	client := NewClient("http://www.example.com", "secret", http.DefaultClient)
	sink, err := newSignalFxSink("signalfx", SignalFxSinkConfig{
		APIKey:                            util.StringSecret{Value: ""},
		DynamicPerTagAPIKeysEnable:        false,
		DynamicPerTagAPIKeysRefreshPeriod: time.Second,
		EndpointAPI:                       "",
		EndpointBase:                      "",
		FlushMaxPerBody:                   0,
		HostnameTag:                       "host",
		MetricNamePrefixDrops:             nil,
		MetricTagPrefixDrops:              nil,
		VaryKeyBy:                         "",
	}, "signalfx-hostname", map[string]string{"yay": "pie"}, logrus.NewEntry(logrus.New()), nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	err = sink.Start(nil)
	if err != nil {
		t.Fatal(err)
	}

	httpsink, ok := client.(*sfxclient.HTTPSink)
	assert.True(t, ok, "SignalFx sink isn't the correct type")
	assert.Equal(t, "http://www.example.com/v2/datapoint", httpsink.DatapointEndpoint)
	assert.Equal(t, "http://www.example.com/v2/event", httpsink.EventEndpoint)

	assert.Equal(t, "signalfx", sink.Name())
	assert.Equal(t, "host", sink.hostnameTag)
	assert.Equal(t, "signalfx-hostname", sink.hostname)
	assert.Equal(t, map[string]string{"yay": "pie"}, sink.commonDimensions)
}

func TestSignalFxFlushRouting(t *testing.T) {
	fakeSink := NewFakeSink()
	sink, err := newSignalFxSink("signalfx", SignalFxSinkConfig{
		APIKey:                            util.StringSecret{Value: ""},
		DynamicPerTagAPIKeysEnable:        false,
		DynamicPerTagAPIKeysRefreshPeriod: time.Second,
		EndpointAPI:                       "",
		EndpointBase:                      "",
		FlushMaxPerBody:                   0,
		HostnameTag:                       "host",
		MetricNamePrefixDrops:             nil,
		MetricTagPrefixDrops:              nil,
		VaryKeyBy:                         "",
	}, "signalfx-hostname", map[string]string{"yay": "pie"}, logrus.NewEntry(logrus.New()), fakeSink, nil, nil)

	assert.NoError(t, err)

	interMetrics := []samplers.InterMetric{{
		Name:      "any",
		Timestamp: 1476119058,
		Value:     float64(100),
		Tags: []string{
			"foo:bar",
			"baz:quz",
		},
		Type: samplers.GaugeMetric,
	}, {
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
	}, {
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
	}}

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

	sink, err := newSignalFxSink("signalfx", SignalFxSinkConfig{
		APIKey:                            util.StringSecret{Value: ""},
		DynamicPerTagAPIKeysEnable:        false,
		DynamicPerTagAPIKeysRefreshPeriod: time.Second,
		EndpointAPI:                       "",
		EndpointBase:                      "",
		FlushMaxPerBody:                   0,
		HostnameTag:                       "host",
		MetricNamePrefixDrops:             nil,
		MetricTagPrefixDrops:              nil,
		VaryKeyBy:                         "",
	}, "signalfx-hostname", map[string]string{"yay": "pie"}, logrus.NewEntry(logrus.New()), fakeSink, nil, nil)

	assert.NoError(t, err)

	interMetrics := []samplers.InterMetric{{
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
	assert.Equal(t, "signalfx-hostname", dims["host"], "Metric is missing host tag")
	assert.Empty(t, derived.samples, "Gauges should not generated derived metrics")
}

func TestSignalFxFlushCounter(t *testing.T) {
	fakeSink := NewFakeSink()
	derived := newDerivedProcessor()
	sink, err := newSignalFxSink("signalfx", SignalFxSinkConfig{
		APIKey:                            util.StringSecret{Value: ""},
		DynamicPerTagAPIKeysEnable:        false,
		DynamicPerTagAPIKeysRefreshPeriod: time.Second,
		EndpointAPI:                       "",
		EndpointBase:                      "",
		FlushMaxPerBody:                   0,
		HostnameTag:                       "host",
		MetricNamePrefixDrops:             nil,
		MetricTagPrefixDrops:              nil,
		VaryKeyBy:                         "",
	}, "signalfx-hostname", map[string]string{"yay": "pie"}, logrus.NewEntry(logrus.New()), fakeSink, nil, nil)
	assert.NoError(t, err)

	interMetrics := []samplers.InterMetric{{
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
	assert.Equal(t, "signalfx-hostname", dims["host"], "Metric is missing host tag")
	assert.Empty(t, derived.samples, "Counters should not generated derived metrics")
}

func TestSignalFxFlushWithDrops(t *testing.T) {
	fakeSink := NewFakeSink()
	sink, err := newSignalFxSink("signalfx", SignalFxSinkConfig{
		APIKey:                            util.StringSecret{Value: ""},
		DynamicPerTagAPIKeysEnable:        false,
		DynamicPerTagAPIKeysRefreshPeriod: time.Second,
		EndpointAPI:                       "",
		EndpointBase:                      "",
		FlushMaxPerBody:                   0,
		HostnameTag:                       "host",
		MetricNamePrefixDrops:             []string{"foo.bar"},
		MetricTagPrefixDrops:              []string{"baz:gorch"},
		VaryKeyBy:                         "",
	}, "signalfx-hostname", map[string]string{"yay": "pie"}, logrus.NewEntry(logrus.New()), fakeSink, nil, nil)
	assert.NoError(t, err)

	interMetrics := []samplers.InterMetric{{
		Name:      "foo.bar.baz", // tag prefix drop
		Timestamp: 1476119058,
		Value:     10,
		Tags: []string{
			"foo:bar",
			"baz:quz",
			"novalue",
		},
		Type: samplers.CounterMetric,
	}, {
		Name:      "fart.farts",
		Timestamp: 1476119058,
		Value:     10,
		Tags: []string{
			"foo:bar",
			"baz:quz",
			"novalue",
		},
		Type: samplers.CounterMetric,
	}, {
		Name:      "fart.farts2",
		Timestamp: 1476119058,
		Value:     10,
		Tags: []string{
			"baz:gorch", // literal tag drop
			"baz:quz",
			"novalue",
		},
		Type: samplers.CounterMetric,
	}}

	sink.Flush(context.TODO(), interMetrics)

	assert.Equal(t, 1, len(fakeSink.points))
	point := fakeSink.points[0]
	assert.Equal(t, "fart.farts", point.Metric, "Metric has wrong name")
}

func TestSignalFxFlushStatus(t *testing.T) {
	fakeSink := NewFakeSink()
	derived := newDerivedProcessor()
	sink, err := newSignalFxSink("signalfx", SignalFxSinkConfig{
		APIKey:                            util.StringSecret{Value: ""},
		DynamicPerTagAPIKeysEnable:        false,
		DynamicPerTagAPIKeysRefreshPeriod: time.Second,
		EndpointAPI:                       "",
		EndpointBase:                      "",
		FlushMaxPerBody:                   0,
		HostnameTag:                       "host",
		MetricNamePrefixDrops:             nil,
		MetricTagPrefixDrops:              nil,
		VaryKeyBy:                         "",
	}, "signalfx-hostname", map[string]string{"yay": "pie"}, logrus.NewEntry(logrus.New()), fakeSink, nil, nil)
	assert.NoError(t, err)

	interMetrics := []samplers.InterMetric{{
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
	assert.Equal(t, "signalfx-hostname", dims["host"], "Metric is missing host tag")
	assert.Empty(t, derived.samples, "Counters should not generated derived metrics")
}

func TestSignalFxServiceCheckFlushOther(t *testing.T) {
	fakeSink := NewFakeSink()
	derived := newDerivedProcessor()
	sink, err := newSignalFxSink("signalfx", SignalFxSinkConfig{
		APIKey:                            util.StringSecret{Value: ""},
		DynamicPerTagAPIKeysEnable:        false,
		DynamicPerTagAPIKeysRefreshPeriod: time.Second,
		EndpointAPI:                       "",
		EndpointBase:                      "",
		FlushMaxPerBody:                   0,
		HostnameTag:                       "host",
		MetricNamePrefixDrops:             nil,
		MetricTagPrefixDrops:              nil,
		VaryKeyBy:                         "",
	}, "signalfx-hostname", map[string]string{"yay": "pie"}, logrus.NewEntry(logrus.New()), fakeSink, nil, nil)
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
	sink, err := newSignalFxSink("signalfx", SignalFxSinkConfig{
		APIKey:                            util.StringSecret{Value: ""},
		DynamicPerTagAPIKeysEnable:        false,
		DynamicPerTagAPIKeysRefreshPeriod: time.Second,
		EndpointAPI:                       "",
		EndpointBase:                      "",
		FlushMaxPerBody:                   0,
		HostnameTag:                       "host",
		MetricNamePrefixDrops:             nil,
		MetricTagPrefixDrops:              nil,
		VaryKeyBy:                         "",
	}, "signalfx-hostname", map[string]string{"yay": "pie"}, logrus.NewEntry(logrus.New()), fakeSink, nil, nil)
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
	assert.Equal(t, "signalfx-hostname", dims["host"], "Event is missing host tag")
}

func TestSignalFxSetExcludeTags(t *testing.T) {
	fakeSink := NewFakeSink()
	derived := newDerivedProcessor()
	sink, err := newSignalFxSink("signalfx", SignalFxSinkConfig{
		APIKey:                            util.StringSecret{Value: ""},
		DynamicPerTagAPIKeysEnable:        false,
		DynamicPerTagAPIKeysRefreshPeriod: time.Second,
		EndpointAPI:                       "",
		EndpointBase:                      "",
		FlushMaxPerBody:                   0,
		HostnameTag:                       "host",
		MetricNamePrefixDrops:             nil,
		MetricTagPrefixDrops:              nil,
		VaryKeyBy:                         "",
	}, "signalfx-hostname", map[string]string{"yay": "pie"}, logrus.NewEntry(logrus.New()), fakeSink, nil, nil)
	sink.SetExcludedTags([]string{"foo", "boo", "host"})
	assert.NoError(t, err)

	interMetrics := []samplers.InterMetric{{
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
			"foo":                        "bar",
			"baz":                        "gorch",
			"novalue":                    "",
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
	sink, err := newSignalFxSink("signalfx", SignalFxSinkConfig{
		APIKey:                            util.StringSecret{Value: ""},
		DynamicPerTagAPIKeysEnable:        false,
		DynamicPerTagAPIKeysRefreshPeriod: time.Second,
		EndpointAPI:                       "",
		EndpointBase:                      "",
		FlushMaxPerBody:                   0,
		HostnameTag:                       "host",
		MetricNamePrefixDrops:             nil,
		MetricTagPrefixDrops:              nil,
		VaryKeyBy:                         "test_by",
	}, "signalfx-hostname", map[string]string{"yay": "pie"}, logrus.NewEntry(logrus.New()), fallback, map[string]DPClient{"available": specialized}, nil)
	assert.NoError(t, err)

	interMetrics := []samplers.InterMetric{{
		Name:      "a.b.c",
		Timestamp: 1476119058,
		Value:     float64(100),
		Tags: []string{
			"foo:bar",
			"baz:quz",
			"test_by:needs_fallback",
		},
		Type: samplers.GaugeMetric,
	}, {
		Name:      "a.b.c",
		Timestamp: 1476119058,
		Value:     float64(99),
		Tags: []string{
			"foo:bar",
			"baz:quz",
			"test_by:available",
		},
		Type: samplers.GaugeMetric,
	}}

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
		assert.Equal(t, "signalfx-hostname", dims["host"], "Metric is missing host tag")
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
		assert.Equal(t, "signalfx-hostname", dims["host"], "Metric is missing host tag")
		assert.Equal(t, "available", dims["test_by"], "Metric should have the right test_by tag")
	}
	assert.Empty(t, derived.samples, "Gauges should not generated derived metrics")
}

func TestSignalFxFlushBatches(t *testing.T) {
	fallback := NewFakeSink()

	perBatch := 1
	sink, err := newSignalFxSink("signalfx", SignalFxSinkConfig{
		APIKey:                            util.StringSecret{Value: ""},
		DynamicPerTagAPIKeysEnable:        false,
		DynamicPerTagAPIKeysRefreshPeriod: time.Second,
		EndpointAPI:                       "",
		EndpointBase:                      "",
		FlushMaxPerBody:                   perBatch,
		HostnameTag:                       "host",
		MetricNamePrefixDrops:             nil,
		MetricTagPrefixDrops:              nil,
		VaryKeyBy:                         "test_by",
	}, "signalfx-hostname", map[string]string{"yay": "pie"}, logrus.NewEntry(logrus.New()), fallback, map[string]DPClient{}, nil)

	assert.NoError(t, err)

	interMetrics := []samplers.InterMetric{{
		Name:      "a.b.c",
		Timestamp: 1476119058,
		Value:     float64(100),
		Tags: []string{
			"foo:bar",
			"baz:quz",
			"test_by:first",
		},
		Type: samplers.GaugeMetric,
	}, {
		Name:      "a.b.c",
		Timestamp: 1476119058,
		Value:     float64(99),
		Tags: []string{
			"foo:bar",
			"baz:quz",
			"test_by:second",
		},
		Type: samplers.GaugeMetric,
	}}

	require.NoError(t, sink.Flush(context.TODO(), interMetrics))

	assert.Equal(t, 2, len(fallback.points))
	assert.Equal(t, 2, fallback.pointAdds)
	found := map[string]bool{}

	for _, point := range fallback.points {
		assert.Equal(t, "a.b.c", point.Metric, "Metric has wrong name")
		found[point.Dimensions["test_by"]] = true
	}
	assert.True(t, found["first"])
	assert.True(t, found["second"])
}

func TestSignalFxFlushBatchHang(t *testing.T) {
	fallback := failSink{}

	perBatch := 1
	sink, err := newSignalFxSink("signalfx", SignalFxSinkConfig{
		APIKey:                            util.StringSecret{Value: ""},
		DynamicPerTagAPIKeysEnable:        false,
		DynamicPerTagAPIKeysRefreshPeriod: time.Second,
		EndpointAPI:                       "",
		EndpointBase:                      "",
		FlushMaxPerBody:                   perBatch,
		HostnameTag:                       "host",
		MetricNamePrefixDrops:             nil,
		MetricTagPrefixDrops:              nil,
		VaryKeyBy:                         "test_by",
	}, "signalfx-hostname", map[string]string{"yay": "pie"}, logrus.NewEntry(logrus.New()), fallback, map[string]DPClient{}, nil)

	assert.NoError(t, err)

	interMetrics := []samplers.InterMetric{{
		Name:      "a.b.c",
		Timestamp: 1476119058,
		Value:     float64(100),
		Tags: []string{
			"foo:bar",
			"baz:quz",
			"test_by:first",
		},
		Type: samplers.GaugeMetric,
	}, {
		Name:      "a.b.c.d",
		Timestamp: 1476119058,
		Value:     float64(100),
		Tags: []string{
			"foo:bar",
			"baz:quz",
			"test_by:first",
		},
		Type: samplers.GaugeMetric,
	}}

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel()
	require.Error(t, sink.Flush(ctx, interMetrics))
}

func TestNewSinkDoubleSlashes(t *testing.T) {
	cl := NewClient("http://example.com/", "foo", nil).(*sfxclient.HTTPSink)
	assert.Equal(t, "http://example.com/v2/datapoint", cl.DatapointEndpoint)
	assert.Equal(t, "http://example.com/v2/event", cl.EventEndpoint)
}

const (
	response1 = `{
  "results": [
    {
      "name": "service",
      "secret": "accessToken"
    },
    {
      "name": "differentService",
      "secret": "differentAccessToken"
    }
  ]
}`
	response2 = `{
  "results": [
  ]
}`
	response3 = `{
  "results": [
    {
      "name": "thirdService",
      "secret": "thirdAccessToken"
    }
  ]
}`
)

func TestSignalFxExtractTokensFromResponse(t *testing.T) {
	tests := []struct {
		input         string
		output        map[string]string
		expectedCount int
	}{
		{
			input: response1,
			output: map[string]string{
				"service":          "accessToken",
				"differentService": "differentAccessToken",
			},
			expectedCount: 2,
		},
		{
			input:         response2,
			output:        map[string]string{},
			expectedCount: 0,
		},
	}

	for _, test := range tests {
		result := make(map[string]string)
		b := bytes.NewBufferString(test.input)

		count, err := extractTokensFromResponse(result, b)

		require.NoError(t, err)
		require.Equal(t, test.expectedCount, count)
		require.Equal(t, test.output, result)
	}
}

// mockHandler cycles through a slice of predefined HTTP responses
type mockHandler struct {
	responses []string
	index     int64
}

func (m *mockHandler) ServeHTTP(resp http.ResponseWriter, req *http.Request) {
	resp.WriteHeader(http.StatusOK)
	response := m.responses[atomic.LoadInt64(&m.index)%int64(len(m.responses))]
	resp.Write([]byte(response))
	atomic.AddInt64(&m.index, 1)
}

func TestSignalFxFetchAPITokens(t *testing.T) {
	m := &mockHandler{
		responses: []string{
			response1,
			response3,
			response2,
		},
	}
	expectedParams := []string{offsetQueryParam, limitQueryParam}
	respCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		if r.Method != "GET" {
			t.Errorf("Expected ‘GET’ request, got ‘%s’", r.Method)
		}
		q := r.URL.Query()

		for _, param := range expectedParams {
			if q.Get(param) == "" {
				t.Errorf("Expected %s parameter to be present in the request URL", param)
			}
		}

		w.Write([]byte(m.responses[respCount]))
		respCount += 1
	}))

	expected := map[string]string{
		"service":          "accessToken",
		"differentService": "differentAccessToken",
		"thirdService":     "thirdAccessToken",
	}

	result, err := fetchAPIKeys(server.Client(), server.URL, "")
	require.NoError(t, err)
	require.Equal(t, expected, result)
}

func TestSignalFxClientByTagUpdater(t *testing.T) {
	const dynamicKeyRefreshPeriod = 1 * time.Millisecond
	m := &mockHandler{
		responses: []string{
			response1,
			response3,
			response2,
		},
	}

	server := httptest.NewServer(m)
	fallback := NewFakeSink()
	perBatch := 1
	sink, err := newSignalFxSink("signalfx", SignalFxSinkConfig{
		APIKey:                            util.StringSecret{Value: ""},
		DynamicPerTagAPIKeysEnable:        true,
		DynamicPerTagAPIKeysRefreshPeriod: dynamicKeyRefreshPeriod,
		EndpointAPI:                       server.URL,
		EndpointBase:                      "",
		FlushMaxPerBody:                   perBatch,
		HostnameTag:                       "host",
		MetricNamePrefixDrops:             nil,
		MetricTagPrefixDrops:              nil,
		VaryKeyBy:                         "test_by",
	}, "signalfx-hostname", map[string]string{"yay": "pie"}, logrus.NewEntry(logrus.New()), fallback, map[string]DPClient{}, server.Client())

	require.NoError(t, err)

	err = sink.Start(nil)
	require.NoError(t, err)

	timeout := time.After(100 * dynamicKeyRefreshPeriod)
	interval := time.NewTicker(2 * dynamicKeyRefreshPeriod)

LOOP:
	for {
		select {
		case <-timeout:
			require.FailNow(t, "Timed out waiting for SignalFX sink to synchronize clients")
		case <-interval.C:
			// The sink polls and updates serially, so by the time it has made the fourth
			// request, it is guaranteed to have finished processing the third.
			if atomic.LoadInt64(&m.index) >= 4 {
				break LOOP
			}
		}
	}

	sink.clientsByTagValueMu.Lock()
	defer sink.clientsByTagValueMu.Unlock()

	expectedPerTagClients := []string{
		"service",
		"differentService",
		"thirdService",
	}

	actualPerTagClients := make([]string, 0)

	mindex := atomic.LoadInt64(&m.index)
	assert.True(t, mindex >= 3, "m.index should be at least 3, not %d", mindex)
	assert.Equal(t, len(expectedPerTagClients), len(sink.clientsByTagValue), "Sink should have %d clients", len(expectedPerTagClients))
	for tag, client := range sink.clientsByTagValue {
		actualPerTagClients = append(actualPerTagClients, tag)
		require.NotNil(t, client)
	}

	sort.Strings(expectedPerTagClients)
	sort.Strings(actualPerTagClients)

	// Check that both sets are equal (subsets of each other)
	// The order might be different due to modular division, but we should always receive the same
	// three responses
	assert.Subset(t, expectedPerTagClients, actualPerTagClients, "The actual values should be a subset of the expected values")
	assert.Subset(t, actualPerTagClients, expectedPerTagClients, "The expected values should be a subset of the actual values")
}

func TestSignalFxVaryByOverride(t *testing.T) {
	varyByTagKey := "vary_by"
	commonDimensions := map[string]string{"vary_by": "bar"}
	defaultFakeSink := NewFakeSink()
	customFakeSinkFoo := NewFakeSink()
	customFakeSinkBar := NewFakeSink()
	perTagClients := make(map[string]DPClient)
	perTagClients["foo"] = customFakeSinkFoo
	perTagClients["bar"] = customFakeSinkBar

	sink, err := newSignalFxSink("signalfx", SignalFxSinkConfig{
		APIKey:                            util.StringSecret{Value: ""},
		DynamicPerTagAPIKeysEnable:        false,
		DynamicPerTagAPIKeysRefreshPeriod: time.Second,
		EndpointAPI:                       "",
		EndpointBase:                      "",
		FlushMaxPerBody:                   0,
		HostnameTag:                       "host",
		MetricNamePrefixDrops:             nil,
		MetricTagPrefixDrops:              nil,
		VaryKeyBy:                         varyByTagKey,
	}, "signalfx-hostname", commonDimensions, logrus.NewEntry(logrus.New()), defaultFakeSink, perTagClients, nil)
	assert.NoError(t, err)

	interMetrics := []samplers.InterMetric{{
		Name:      "a.b.c",
		Timestamp: 1476119058,
		Value:     float64(100),
		Tags: []string{
			"vary_by:foo",
		},
		Type: samplers.GaugeMetric,
	}, {
		Name:      "a.b.d",
		Timestamp: 1476119059,
		Value:     float64(100),
		Tags:      []string{},
		Type:      samplers.GaugeMetric,
	}}

	sink.Flush(context.TODO(), interMetrics)

	assert.Equal(t, 0, len(defaultFakeSink.points))
	assert.Equal(t, 1, len(customFakeSinkFoo.points))
	assert.Equal(t, 1, len(customFakeSinkBar.points))

	assert.Equal(t, "foo", customFakeSinkFoo.points[0].Dimensions["vary_by"])
	assert.Equal(t, "bar", customFakeSinkBar.points[0].Dimensions["vary_by"])
}

func TestSignalFxVaryByOverridePreferringCommonDimensions(t *testing.T) {
	varyByTagKey := "vary_by"
	commonDimensions := map[string]string{"vary_by": "bar"}
	defaultFakeSink := NewFakeSink()
	customFakeSinkFoo := NewFakeSink()
	customFakeSinkBar := NewFakeSink()
	perTagClients := make(map[string]DPClient)
	perTagClients["foo"] = customFakeSinkFoo
	perTagClients["bar"] = customFakeSinkBar

	sink, err := newSignalFxSink("signalfx", SignalFxSinkConfig{
		APIKey:                            util.StringSecret{Value: ""},
		DynamicPerTagAPIKeysEnable:        false,
		DynamicPerTagAPIKeysRefreshPeriod: time.Second,
		EndpointAPI:                       "",
		EndpointBase:                      "",
		FlushMaxPerBody:                   0,
		HostnameTag:                       "host",
		MetricNamePrefixDrops:             nil,
		MetricTagPrefixDrops:              nil,
		VaryKeyBy:                         varyByTagKey,
		VaryKeyByFavorCommonDimensions:    true,
	}, "signalfx-hostname", commonDimensions, logrus.NewEntry(logrus.New()), defaultFakeSink, perTagClients, nil)
	assert.NoError(t, err)

	interMetrics := []samplers.InterMetric{{
		Name:      "a.b.c",
		Timestamp: 1476119058,
		Value:     float64(100),
		Tags: []string{
			"vary_by:foo",
		},
		Type: samplers.GaugeMetric,
	}, {
		Name:      "a.b.d",
		Timestamp: 1476119059,
		Value:     float64(100),
		Tags:      []string{},
		Type:      samplers.GaugeMetric,
	}}

	sink.Flush(context.TODO(), interMetrics)

	assert.Equal(t, 0, len(defaultFakeSink.points))
	assert.Equal(t, 1, len(customFakeSinkFoo.points))
	assert.Equal(t, 1, len(customFakeSinkBar.points))

	assert.Equal(t, "bar", customFakeSinkFoo.points[0].Dimensions["vary_by"])
	assert.Equal(t, "bar", customFakeSinkBar.points[0].Dimensions["vary_by"])
}
