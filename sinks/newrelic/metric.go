package newrelic

import (
	"context"
	"errors"
	"time"

	"github.com/newrelic/newrelic-client-go/newrelic"
	"github.com/newrelic/newrelic-telemetry-sdk-go/telemetry"
	"github.com/sirupsen/logrus"

	"github.com/stripe/veneur/v14/samplers"
	"github.com/stripe/veneur/v14/sinks"
	"github.com/stripe/veneur/v14/ssf"
	"github.com/stripe/veneur/v14/trace"
)

type NewRelicMetricSink struct {
	accountID         int
	client            *newrelic.NewRelic
	eventType         string
	serviceCheckEvent string
	harvester         *telemetry.Harvester
	log               *logrus.Logger
	traceClient       *trace.Client
}

var _ sinks.MetricSink = &NewRelicMetricSink{}

// NewNewRelicMetricSink creates a new NewRelicMetricSink. This sink sends
// data to the New Relic platform
func NewNewRelicMetricSink(insertKey string, accountID int, region string, eventType string, tags []string, log *logrus.Logger, serviceCheckEvent string) (*NewRelicMetricSink, error) {
	if log == nil {
		log = logrus.StandardLogger()
	}

	// For storing metrics
	h, err := newHarvester(insertKey, log, tags, "") // No Span Override
	if err != nil {
		log.WithError(err).Error("unable to create NewRelicMetricSink")
		return nil, err
	}

	if eventType == "" {
		eventType = DefaultEventType
	}
	if serviceCheckEvent == "" {
		serviceCheckEvent = DefaultServiceCheckEventType
	}

	nr, err := newrelic.New(
		newrelic.ConfigInsightsInsertKey(insertKey),
		newrelic.ConfigRegion(region),
		newrelic.ConfigServiceName("veneur"),
	)
	if err != nil {
		log.WithError(err).Error("unable to create NewRelicMetricSink")
		return nil, err
	}

	// Start the batch workers
	err = nr.Events.BatchMode(context.Background(), accountID)
	if err != nil {
		log.WithError(err).Error("unable to create NewRelicMetricSink")
		return nil, err
	}
	log.WithFields(logrus.Fields{
		"accountID":             accountID,
		"eventType":             eventType,
		"serviceCheckEventType": serviceCheckEvent,
	}).Info("started New Relic sink")

	return &NewRelicMetricSink{
		accountID:         accountID,
		client:            nr,
		eventType:         eventType,
		harvester:         h,
		log:               log,
		serviceCheckEvent: serviceCheckEvent,
	}, nil

}

// Name returns the name of the sink
func (nr *NewRelicMetricSink) Name() string {
	return "newrelic"
}

func (nr *NewRelicMetricSink) Start(c *trace.Client) error {
	nr.traceClient = c

	return nil
}

func (nr *NewRelicMetricSink) Flush(ctx context.Context, interMetrics []samplers.InterMetric) error {
	span, _ := trace.StartSpanFromContext(ctx, "")
	defer span.ClientFinish(nr.traceClient)

	if nr.harvester == nil {
		err := errors.New("New Relic sink was not initialized")
		nr.log.Error(err)
		return err
	}

	if len(interMetrics) <= 0 {
		return nil
	}

	for _, m := range interMetrics {
		if !sinks.IsAcceptableMetric(m, nr) {
			continue
		}

		// defined as Now().Unix() in samplers/samplers.go#L152
		timestamp := time.Unix(m.Timestamp, 0)
		attrs := tagsToKeyValue(m.Tags)
		if m.HostName != "" {
			attrs["hostname"] = m.HostName
		}
		if m.Message != "" {
			attrs["message"] = m.Message
		}

		switch m.Type {
		case samplers.CounterMetric:
			nr.harvester.RecordMetric(telemetry.Count{
				Name:       m.Name,
				Value:      m.Value,
				Timestamp:  timestamp,
				Attributes: attrs,
			})

		case samplers.GaugeMetric:
			nr.harvester.RecordMetric(telemetry.Gauge{
				Name:       m.Name,
				Value:      m.Value,
				Timestamp:  timestamp,
				Attributes: attrs,
				//Interval:       5 * time.Second,
			})

		case samplers.StatusMetric:
			// already have a map, flush it out
			attrs["eventType"] = nr.serviceCheckEvent
			attrs["name"] = m.Name
			attrs["timestamp"] = timestamp
			attrs["statusCode"] = int(m.Value)

			// (OK = 0, WARNING = 1, CRITICAL = 2, UNKNOWN = 3)*
			switch int(m.Value) {
			case 0:
				attrs["status"] = "OK"
			case 1:
				attrs["status"] = "WARNING"
			case 2:
				attrs["status"] = "CRITICAL"
			default: // also 3
				attrs["status"] = "UNKNOWN"
			}

			err := nr.client.Events.EnqueueEvent(ctx, attrs)
			if err != nil {
				nr.log.WithError(err).Error("failed to enqueue service check")
				continue
			}

		default:
			nr.log.WithField("metric", m).Error("unknown metric type")
			continue
		}
	}

	// Send the data off to New Relic
	nr.harvester.HarvestNow(ctx)

	return nil
}

func (nr *NewRelicMetricSink) FlushOtherSamples(ctx context.Context, samples []ssf.SSFSample) {
	if len(samples) == 0 {
		return
	}

	span, _ := trace.StartSpanFromContext(ctx, "")
	defer span.ClientFinish(nr.traceClient)

	if nr.client == nil {
		nr.log.WithFields(logrus.Fields{
			"sampleCount": len(samples),
		}).Warn("event queued but New Relic event client disabled, dropping")

		return
	}

	count := 0
	for _, sample := range samples {
		// Fields from: `type SSFSample struct` /ssf/sample.pb.go
		evt := map[string]interface{}{
			"eventType": nr.eventType,
			"type":      sample.Metric,
			"name":      sample.Name,
			"value":     sample.Value,
			"timestamp": sample.Timestamp,
			"message":   sample.Message,
			"status":    sample.Status,
			"rate":      sample.SampleRate,
			"unit":      sample.Unit,
			"scope":     sample.Scope,
		}

		// flatten tags
		for key, val := range sample.Tags {
			evt[key] = val
		}

		err := nr.client.Events.EnqueueEvent(ctx, evt)
		if err != nil {
			nr.log.WithError(err).Error("failed to enqueue event to New Relic")
			continue
		}

		count++
	}

	nr.log.WithFields(logrus.Fields{
		"sampleCount": len(samples),
		"eventCount":  count,
	}).Info("queued events to New Relic")

	return
}
