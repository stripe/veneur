package signalfx

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/signalfx/golib/datapoint"
	"github.com/signalfx/golib/datapoint/dpsink"
	"github.com/signalfx/golib/event"
	"github.com/signalfx/golib/sfxclient"
	"github.com/sirupsen/logrus"
	"github.com/stripe/veneur/protocol/dogstatsd"
	"github.com/stripe/veneur/samplers"
	"github.com/stripe/veneur/sinks"
	"github.com/stripe/veneur/ssf"
	"github.com/stripe/veneur/trace"
)

const EventNameMaxLength = 256
const EventDescriptionMaxLength = 256

// collection is a structure that aggregates signalfx data points
// per-endpoint. It takes care of collecting the metrics by the tag
// values that identify where to send them, and
type collection struct {
	sink        *SignalFxSink
	points      []*datapoint.Datapoint
	pointsByKey map[string][]*datapoint.Datapoint
}

func (c *collection) addPoint(key string, point *datapoint.Datapoint) {
	if c.sink.clientsByTagValue != nil {
		if _, ok := c.sink.clientsByTagValue[key]; ok {
			c.pointsByKey[key] = append(c.pointsByKey[key], point)
			return
		}
	}
	c.points = append(c.points, point)
}

func (c *collection) submit(ctx context.Context, cl *trace.Client) error {
	wg := &sync.WaitGroup{}
	errorCh := make(chan error, len(c.pointsByKey)+1)

	submitOne := func(client dpsink.Sink, points []*datapoint.Datapoint) {
		span, _ := trace.StartSpanFromContext(ctx, "")
		defer span.ClientFinish(cl)
		err := client.AddDatapoints(ctx, points)
		if err != nil {
			span.Error(err)
			span.Add(ssf.Count("flush.error_total", 1, map[string]string{"cause": "io", "sink": "signalfx"}))
			errorCh <- err
		}
		wg.Done()
	}

	wg.Add(1)
	go submitOne(c.sink.defaultClient, c.points)
	for key, points := range c.pointsByKey {
		wg.Add(1)
		go submitOne(c.sink.client(key), points)
	}
	wg.Wait()
	close(errorCh)
	errors := []error{}
	for err := range errorCh {
		errors = append(errors, err)
	}
	if len(errors) > 0 {
		return fmt.Errorf("Could not submit to all sfx sinks: %v", errors)
	}
	return nil
}

// SignalFxSink is a MetricsSink implementation.
type SignalFxSink struct {
	defaultClient     DPClient
	clientsByTagValue map[string]DPClient
	keyClients        map[string]dpsink.Sink
	varyBy            string
	hostnameTag       string
	hostname          string
	commonDimensions  map[string]string
	log               *logrus.Logger
	traceClient       *trace.Client
	excludedTags      map[string]struct{}
	derivedMetrics    samplers.DerivedMetricsProcessor
}

// A DPClient is a client that can be used to submit signalfx data
// points to an upstream consumer. It wraps the dpsink.Sink interface.
type DPClient dpsink.Sink

// NewClient constructs a new signalfx HTTP client for the given
// endpoint and API token.
func NewClient(endpoint, apiKey string, client http.Client) DPClient {
	httpSink := sfxclient.NewHTTPSink()
	httpSink.AuthToken = apiKey
	httpSink.DatapointEndpoint = fmt.Sprintf("%s/v2/datapoint", endpoint)
	httpSink.EventEndpoint = fmt.Sprintf("%s/v2/event", endpoint)
	httpSink.Client = client
	return httpSink
}

// NewSignalFxSink creates a new SignalFx sink for metrics.
func NewSignalFxSink(hostnameTag string, hostname string, commonDimensions map[string]string, log *logrus.Logger, client DPClient, varyBy string, perTagClients map[string]DPClient, derivedMetrics samplers.DerivedMetricsProcessor) (*SignalFxSink, error) {
	return &SignalFxSink{
		defaultClient:     client,
		clientsByTagValue: perTagClients,
		hostnameTag:       hostnameTag,
		hostname:          hostname,
		commonDimensions:  commonDimensions,
		log:               log,
		varyBy:            varyBy,
		derivedMetrics:    derivedMetrics,
	}, nil
}

// Name returns the name of this sink.
func (sfx *SignalFxSink) Name() string {
	return "signalfx"
}

// Start begins the sink. For SignalFx this is a noop.
func (sfx *SignalFxSink) Start(traceClient *trace.Client) error {
	sfx.traceClient = traceClient
	return nil
}

// client returns a client that can be used to submit to vary-by tag's
// value. If no client is specified for that tag value, the default
// client is returned.
func (sfx *SignalFxSink) client(key string) DPClient {
	if cl, ok := sfx.clientsByTagValue[key]; ok {
		return cl
	}
	return sfx.defaultClient
}

// newPointCollection creates an empty collection object and returns it
func (sfx *SignalFxSink) newPointCollection() *collection {
	return &collection{
		sink:        sfx,
		points:      []*datapoint.Datapoint{},
		pointsByKey: map[string][]*datapoint.Datapoint{},
	}
}

// Flush sends metrics to SignalFx
func (sfx *SignalFxSink) Flush(ctx context.Context, interMetrics []samplers.InterMetric) error {
	span, _ := trace.StartSpanFromContext(ctx, "")
	defer span.ClientFinish(sfx.traceClient)

	flushStart := time.Now()
	coll := sfx.newPointCollection()
	numPoints := 0
	countSkipped := 0
	countStatusMetrics := 0

	for _, metric := range interMetrics {
		if !sinks.IsAcceptableMetric(metric, sfx) {
			countSkipped++
			continue
		}
		dims := map[string]string{}
		// Set the hostname as a tag, since SFx doesn't have a first-class hostname field
		dims[sfx.hostnameTag] = sfx.hostname
		for _, tag := range metric.Tags {
			kv := strings.SplitN(tag, ":", 2)
			key := kv[0]

			if len(kv) == 1 {
				dims[key] = ""
			} else {
				dims[key] = kv[1]
			}
		}
		// Copy common dimensions
		for k, v := range sfx.commonDimensions {
			dims[k] = v
		}
		metricKey := ""
		if sfx.varyBy != "" {
			if val, ok := dims[sfx.varyBy]; ok {
				metricKey = val
			}
		}

		for k := range sfx.excludedTags {
			delete(dims, k)
		}
		delete(dims, "veneursinkonly")

		var point *datapoint.Datapoint
		switch metric.Type {
		case samplers.GaugeMetric:
			point = sfxclient.GaugeF(metric.Name, dims, metric.Value)
		case samplers.CounterMetric:
			// TODO I am not certain if this should be a Counter or a Cumulative
			point = sfxclient.Counter(metric.Name, dims, int64(metric.Value))
		case samplers.StatusMetric:
			countStatusMetrics++
			point = sfxclient.GaugeF(metric.Name, dims, metric.Value)
		}
		coll.addPoint(metricKey, point)
		numPoints++
	}
	tags := map[string]string{"sink": "signalfx"}
	span.Add(ssf.Count(sinks.MetricKeyTotalMetricsSkipped, float32(countSkipped), tags))
	err := coll.submit(ctx, sfx.traceClient)
	if err != nil {
		span.Error(err)
	}
	span.Add(ssf.Timing(sinks.MetricKeyMetricFlushDuration, time.Since(flushStart), time.Nanosecond, tags))
	span.Add(ssf.Count(sinks.MetricKeyTotalMetricsFlushed, float32(numPoints), tags))
	sfx.log.WithField("metrics", len(interMetrics)).Info("Completed flush to SignalFx")

	return err
}

var successSpanTags = map[string]string{"sink": "signalfx", "results": "success"}
var failureSpanTags = map[string]string{"sink": "signalfx", "results": "failure"}

// FlushOtherSamples sends events to SignalFx. Event type samples will be serialized as SFX
// Events directly. All other metric types are ignored
func (sfx *SignalFxSink) FlushOtherSamples(ctx context.Context, samples []ssf.SSFSample) {
	span, _ := trace.StartSpanFromContext(ctx, "")
	defer span.ClientFinish(sfx.traceClient)
	var countFailed = 0
	var countSuccess = 0
	for _, sample := range samples {
		if _, ok := sample.Tags[dogstatsd.EventIdentifierKey]; ok {
			err := sfx.reportEvent(ctx, &sample)
			if err != nil {
				countFailed++
			} else {
				countSuccess++
			}
		}
	}
	if countSuccess > 0 {
		span.Add(ssf.Count(sinks.EventReportedCount, float32(countSuccess), successSpanTags))
	}
	if countFailed > 0 {
		span.Add(ssf.Count(sinks.EventReportedCount, float32(countFailed), failureSpanTags))
	}
}

// SetExcludedTags sets the excluded tag names. Any tags with the
// provided key (name) will be excluded.
func (sfx *SignalFxSink) SetExcludedTags(excludes []string) {

	tagsSet := map[string]struct{}{}
	for _, tag := range excludes {
		tagsSet[tag] = struct{}{}
	}
	sfx.excludedTags = tagsSet
}

type ddSampleKind int

const (
	ddSampleUnknown ddSampleKind = iota
	ddSampleEvent
	ddSampleServiceCheck
)

func (sfx *SignalFxSink) reportEvent(ctx context.Context, sample *ssf.SSFSample) error {
	// Copy common dimensions in
	dims := map[string]string{}
	for k, v := range sfx.commonDimensions {
		dims[k] = v
	}
	// And hostname
	dims[sfx.hostnameTag] = sfx.hostname

	for k, v := range sample.Tags {
		if k == dogstatsd.EventIdentifierKey {
			// Don't copy this tag
			continue
		}
		dims[k] = v
	}

	for k := range sfx.excludedTags {
		delete(dims, k)
	}
	name := sample.Name
	if len(name) > EventNameMaxLength {
		name = name[0:EventNameMaxLength]
	}
	message := sample.Message
	if len(message) > EventDescriptionMaxLength {
		message = message[0:EventDescriptionMaxLength]
	}
	// Datadog requires some weird chars to do markdown… SignalFx does not so
	// we'll chop those out
	// https://docs.datadoghq.com/graphing/event_stream/#markdown-events
	message = strings.Replace(message, "%%% \n", "", 1)
	message = strings.Replace(message, "\n %%%", "", 1)
	// Sometimes there are leading and trailing spaces
	message = strings.TrimSpace(message)

	ev := event.Event{
		EventType:  name,
		Category:   event.USERDEFINED,
		Dimensions: dims,
		Timestamp:  time.Unix(sample.Timestamp, 0),
		Properties: map[string]interface{}{
			"description": message,
		},
	}

	// TODO: Split events out the same way as points
	// report evt
	return sfx.defaultClient.AddEvents(ctx, []*event.Event{&ev})
}
