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

type SignalFxSink struct {
	client           dpsink.Sink
	endpoint         string
	hostnameTag      string
	hostname         string
	commonDimensions map[string]string
	log              *logrus.Logger
	traceClient      *trace.Client
	excludedTags     map[string]struct{}
	chunkMetricCount int
}

// NewSignalFxSink creates a new SignalFx sink for metrics.
func NewSignalFxSink(apiKey string, endpoint string, hostnameTag string, hostname string, commonDimensions map[string]string, chunkMetricCount int, log *logrus.Logger, client dpsink.Sink) (*SignalFxSink, error) {
	if client == nil {
		httpSink := sfxclient.NewHTTPSink()
		httpSink.AuthToken = apiKey
		httpSink.DatapointEndpoint = fmt.Sprintf("%s/v2/datapoint", endpoint)
		httpSink.EventEndpoint = fmt.Sprintf("%s/v2/event", endpoint)
		client = httpSink
	}

	return &SignalFxSink{
		client:           client,
		endpoint:         endpoint,
		hostnameTag:      hostnameTag,
		hostname:         hostname,
		commonDimensions: commonDimensions,
		log:              log,
		chunkMetricCount: chunkMetricCount,
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

	sizeWanted := 1000
	sent := 0

	var wg sync.WaitGroup
	flushStart := time.Now()
	points := []*datapoint.Datapoint{}
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

		if metric.Type == samplers.GaugeMetric {
			points = append(points, sfxclient.GaugeF(metric.Name, dims, metric.Value))
		} else if metric.Type == samplers.CounterMetric {
			points = append(points, sfxclient.Counter(metric.Name, dims, int64(metric.Value)))
		}
		if len(points) >= sizeWanted {
			// Copy the points so we can continue with a new one
			toFlush := make([]*datapoint.Datapoint, len(points))
			copy(toFlush, points)
			sent += len(points)

			wg.Add(1)
			go func() {
				sfx.flushPoints(ctx, &wg, toFlush)
			}()
			// Empty out the points for the next chunk
			points = []*datapoint.Datapoint{}
		}
	}
	if len(points) > 0 {
		sent += len(points)
		wg.Add(1)
		sfx.flushPoints(ctx, &wg, points)
	}
	wg.Wait()

	tags := map[string]string{"sink": "signalfx"}
	span.Add(ssf.Timing(sinks.MetricKeyMetricFlushDuration, time.Since(flushStart), time.Nanosecond, tags))
	span.Add(ssf.Count(sinks.MetricKeyTotalMetricsFlushed, float32(sent), tags))
	sfx.log.WithField("metrics", sent).Info("Completed flush to SignalFx")

	return nil
}

func (sfx *SignalFxSink) flushPoints(ctx context.Context, wg *sync.WaitGroup, points []*datapoint.Datapoint) error {
	span, _ := trace.StartSpanFromContext(ctx, "")
	defer span.ClientFinish(sfx.traceClient)
	defer wg.Done()

	sfx.log.WithField("num_points", len(points)).Debug("Flushing a chunk from SignalFx")
	err := sfx.client.AddDatapoints(ctx, points)
	if err != nil {
		sfx.log.WithField("num_points", len(points)).WithError(err).Warn("Failed to send metrics")
		span.Error(err)
	}

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
		sfx.client.AddEvents(ctx, []*event.Event{&ev})
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
