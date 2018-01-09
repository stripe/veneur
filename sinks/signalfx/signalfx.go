package signalfx

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/DataDog/datadog-go/statsd"
	"github.com/signalfx/golib/datapoint"
	"github.com/signalfx/golib/datapoint/dpsink"
	"github.com/signalfx/golib/event"
	"github.com/signalfx/golib/sfxclient"
	"github.com/sirupsen/logrus"
	"github.com/stripe/veneur/samplers"
	"github.com/stripe/veneur/sinks"
	"github.com/stripe/veneur/trace"
)

type SignalFXSink struct {
	client           dpsink.Sink
	endpoint         string
	hostnameTag      string
	hostname         string
	commonDimensions map[string]string
	statsd           *statsd.Client
	log              *logrus.Logger
	traceClient      *trace.Client
}

// NewSignalFXSink creates a new SignalFX sink for metrics.
func NewSignalFXSink(apiKey string, endpoint string, hostnameTag string, hostname string, commonDimensions map[string]string, stats *statsd.Client, log *logrus.Logger, client dpsink.Sink) (*SignalFXSink, error) {
	if client == nil {
		httpSink := sfxclient.NewHTTPSink()
		httpSink.AuthToken = apiKey
		httpSink.DatapointEndpoint = fmt.Sprintf("%s/v2/datapoint", endpoint)
		httpSink.EventEndpoint = fmt.Sprintf("%s/v2/event", endpoint)
		client = httpSink
	}

	return &SignalFXSink{
		client:           client,
		endpoint:         endpoint,
		hostnameTag:      hostnameTag,
		hostname:         hostname,
		commonDimensions: commonDimensions,
		statsd:           stats,
		log:              log,
	}, nil
}

// Name returns the name of this sink.
func (sfx *SignalFXSink) Name() string {
	return "signalfx"
}

// Start begins the sink. For SignalFx this is a noop.
func (sfx *SignalFXSink) Start(traceClient *trace.Client) error {
	sfx.traceClient = traceClient
	return nil
}

// Flush sends metrics to SignalFx
func (sfx *SignalFXSink) Flush(ctx context.Context, interMetrics []samplers.InterMetric) error {
	span, _ := trace.StartSpanFromContext(ctx, "")
	defer span.ClientFinish(sfx.traceClient)

	flushStart := time.Now()
	points := []*datapoint.Datapoint{}
	for _, metric := range interMetrics {
		if !sinks.IsAcceptableMetric(metric, sfx) {
			continue
		}
		dims := map[string]string{}
		for _, tag := range metric.Tags {
			kv := strings.SplitN(tag, ":", 2)
			if len(kv) == 1 {
				dims[kv[0]] = ""
			} else {
				dims[kv[0]] = kv[1]
			}
		}
		// Copy common dimensions
		for k, v := range sfx.commonDimensions {
			dims[k] = v
		}

		if metric.Type == samplers.GaugeMetric {
			points = append(points, sfxclient.GaugeF(metric.Name, dims, metric.Value))
		} else if metric.Type == samplers.CounterMetric {
			// TODO I am not certain if this should be a Counter or a Cumulative
			points = append(points, sfxclient.Counter(metric.Name, dims, int64(metric.Value)))
		}
	}
	err := sfx.client.AddDatapoints(ctx, points)
	if err != nil {
		span.Error(err)
	}
	sfx.statsd.TimeInMilliseconds("flush.total_duration_ns", float64(time.Since(flushStart).Nanoseconds()), []string{"plugin:signalfx"}, 1.0)
	// TODO Fix these metrics to be per-metric sink
	sfx.log.WithField("metrics", len(interMetrics)).Info("Completed flush to SignalFX")

	return err
}

// FlushEventsChecks sends events to SignalFx. It does not support checks. It is also currently disabled.
func (sfx *SignalFXSink) FlushEventsChecks(ctx context.Context, events []samplers.UDPEvent, checks []samplers.UDPServiceCheck) {
	span, _ := trace.StartSpanFromContext(ctx, "")
	defer span.ClientFinish(sfx.traceClient)

	for _, udpEvent := range events {

		// TODO: SignalFx wants `map[string]string` for tags but at this point we're
		// getting []string. We should fix this, as it feels less icky for sinks to
		// get `map[string]string`.
		dims := map[string]string{}
		for _, tag := range udpEvent.Tags {
			parts := strings.SplitN(tag, ":", 2)
			if len(parts) == 1 {
				dims[parts[0]] = ""
			} else {
				dims[parts[0]] = parts[1]
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
