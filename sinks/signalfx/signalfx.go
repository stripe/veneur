package signalfx

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/signalfx/golib/datapoint"
	"github.com/signalfx/golib/datapoint/dpsink"
	"github.com/signalfx/golib/event"
	"github.com/signalfx/golib/sfxclient"
	"github.com/sirupsen/logrus"
	"github.com/stripe/veneur/samplers"
	"github.com/stripe/veneur/sinks"
	"github.com/stripe/veneur/ssf"
	"github.com/stripe/veneur/trace"
)

type destinations struct {
	fallback dpsink.Sink
	byKey    map[string]dpsink.Sink
}

func (d *destinations) getClient(key string) dpsink.Sink {
	if cl, ok := d.byKey[key]; ok {
		return cl
	}
	return d.fallback
}

type collection struct {
	points       []*datapoint.Datapoint
	pointsByKey  map[string][]*datapoint.Datapoint
	destinations *destinations
}

func (d *destinations) pointCollection() *collection {
	return &collection{
		destinations: d,
		points:       []*datapoint.Datapoint{},
		pointsByKey:  map[string][]*datapoint.Datapoint{},
	}
}

func (c *collection) addPoint(key string, point *datapoint.Datapoint) {
	if c.destinations.byKey != nil {
		if _, ok := c.destinations.byKey[key]; ok {
			c.pointsByKey[key] = append(c.pointsByKey[key], point)
			return
		}
	}
	logrus.WithField("key", key).WithField("pbk", c.pointsByKey).Info("I have a fallback")

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
			errorCh <- err
		}
		wg.Done()
	}

	wg.Add(1)
	go submitOne(c.destinations.fallback, c.points)
	for key, points := range c.pointsByKey {
		wg.Add(1)
		go submitOne(c.destinations.getClient(key), points)
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

type SignalFxSink struct {
	clients          *destinations
	keyClients       map[string]dpsink.Sink
	varyBy           string
	hostnameTag      string
	hostname         string
	commonDimensions map[string]string
	log              *logrus.Logger
	traceClient      *trace.Client
	excludedTags     map[string]struct{}
}

// NewClient constructs a new signalfx HTTP client for the given
// endpoint and API token.
func NewClient(endpoint, apiKey string) dpsink.Sink {
	httpSink := sfxclient.NewHTTPSink()
	httpSink.AuthToken = apiKey
	httpSink.DatapointEndpoint = fmt.Sprintf("%s/v2/datapoint", endpoint)
	httpSink.EventEndpoint = fmt.Sprintf("%s/v2/event", endpoint)
	return httpSink
}

// NewSignalFxSink creates a new SignalFx sink for metrics.
func NewSignalFxSink(hostnameTag string, hostname string, commonDimensions map[string]string, log *logrus.Logger, client dpsink.Sink, varyBy string, perTagClients map[string]dpsink.Sink) (*SignalFxSink, error) {
	return &SignalFxSink{
		clients:          &destinations{fallback: client, byKey: perTagClients},
		hostnameTag:      hostnameTag,
		hostname:         hostname,
		commonDimensions: commonDimensions,
		log:              log,
		varyBy:           varyBy,
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

// Flush sends metrics to SignalFx
func (sfx *SignalFxSink) Flush(ctx context.Context, interMetrics []samplers.InterMetric) error {
	span, _ := trace.StartSpanFromContext(ctx, "")
	defer span.ClientFinish(sfx.traceClient)

	flushStart := time.Now()
	coll := sfx.clients.pointCollection()
	numPoints := 0

	for _, metric := range interMetrics {
		if !sinks.IsAcceptableMetric(metric, sfx) {
			continue
		}
		dims := map[string]string{}
		// Set the hostname as a tag, since SFx doesn't have a first-class hostname field
		if _, ok := sfx.excludedTags[sfx.hostnameTag]; !ok {
			dims[sfx.hostnameTag] = sfx.hostname
		}
		for _, tag := range metric.Tags {
			kv := strings.SplitN(tag, ":", 2)
			key := kv[0]

			// skip excluded tags
			if _, ok := sfx.excludedTags[key]; ok {
				continue
			}

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

		var point *datapoint.Datapoint
		if metric.Type == samplers.GaugeMetric {
			point = sfxclient.GaugeF(metric.Name, dims, metric.Value)
		} else if metric.Type == samplers.CounterMetric {
			// TODO I am not certain if this should be a Counter or a Cumulative
			point = sfxclient.Counter(metric.Name, dims, int64(metric.Value))
		}
		coll.addPoint(metricKey, point)
		numPoints++
	}
	err := coll.submit(ctx, sfx.traceClient)
	if err != nil {
		span.Error(err)
	}
	tags := map[string]string{"sink": "signalfx"}
	span.Add(ssf.Timing(sinks.MetricKeyMetricFlushDuration, time.Since(flushStart), time.Nanosecond, tags))
	span.Add(ssf.Count(sinks.MetricKeyTotalMetricsFlushed, float32(numPoints), tags))
	sfx.log.WithField("metrics", len(interMetrics)).Info("Completed flush to SignalFx")

	return err
}

// FlushEventsChecks sends events to SignalFx. It does not support checks. It is also currently disabled.
func (sfx *SignalFxSink) FlushEventsChecks(ctx context.Context, events []samplers.UDPEvent, checks []samplers.UDPServiceCheck) {
	span, _ := trace.StartSpanFromContext(ctx, "")
	defer span.ClientFinish(sfx.traceClient)

	for _, udpEvent := range events {

		// TODO: SignalFx wants `map[string]string` for tags but at this point we're
		// getting []string. We should fix this, as it feels less icky for sinks to
		// get `map[string]string`.
		dims := map[string]string{}

		// Set the hostname as a tag, since SFx doesn't have a first-class hostname field
		if _, ok := sfx.excludedTags[sfx.hostnameTag]; !ok {
			dims[sfx.hostnameTag] = sfx.hostname
		}

		for _, tag := range udpEvent.Tags {
			parts := strings.SplitN(tag, ":", 2)
			key := parts[0]

			// skip excluded tags
			if _, ok := sfx.excludedTags[key]; ok {
				continue
			}

			if len(parts) == 1 {
				dims[key] = ""
			} else {
				dims[key] = parts[1]
			}
		}
		// Copy common dimensions in
		for k, v := range sfx.commonDimensions {
			dims[k] = v
		}

		ev := event.Event{
			EventType:  udpEvent.Title,
			Category:   event.USERDEFINED,
			Dimensions: dims,
			Timestamp:  time.Unix(udpEvent.Timestamp, 0),
		}
		// TODO: Split events out the same way as points
		sfx.clients.fallback.AddEvents(ctx, []*event.Event{&ev})
	}
}

// SetTagExcludes sets the excluded tag names. Any tags with the
// provided key (name) will be excluded.
func (sfx *SignalFxSink) SetExcludedTags(excludes []string) {

	tagsSet := map[string]struct{}{}
	for _, tag := range excludes {
		tagsSet[tag] = struct{}{}
	}
	sfx.excludedTags = tagsSet
}
