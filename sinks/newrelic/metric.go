package newrelic

import (
	"context"
	"errors"
	"time"

	"github.com/newrelic/newrelic-client-go/newrelic"
	"github.com/newrelic/newrelic-telemetry-sdk-go/telemetry"
	"github.com/sirupsen/logrus"

	veneur "github.com/stripe/veneur/v14"
	"github.com/stripe/veneur/v14/samplers"
	"github.com/stripe/veneur/v14/sinks"
	"github.com/stripe/veneur/v14/ssf"
	"github.com/stripe/veneur/v14/trace"
	"github.com/stripe/veneur/v14/util"
)

type NewRelicMetricSinkConfig struct {
	AccountID             int               `yaml:"account_id"`
	CommonTags            []string          `yaml:"common_tags"`
	EventType             string            `yaml:"event_type"`
	InsertKey             util.StringSecret `yaml:"insert_key"`
	Region                string            `yaml:"region"`
	ServiceCheckEventType string            `yaml:"service_check_event_type"`
}

type NewRelicMetricSink struct {
	accountID         int
	client            *newrelic.NewRelic
	eventType         string
	harvester         *telemetry.Harvester
	log               *logrus.Entry
	name              string
	serviceCheckEvent string
	traceClient       *trace.Client
}

// ParseConfig decodes the map config for a NewRelic sink into a
// NewRelicSinkConfig struct.
func ParseMetricConfig(
	name string, config interface{},
) (veneur.MetricSinkConfig, error) {
	newRelicConfig := NewRelicMetricSinkConfig{}
	err := util.DecodeConfig(name, config, &newRelicConfig)
	if err != nil {
		return nil, err
	}
	return newRelicConfig, nil
}

// CreateMetricSink creates a new NewRelic sink for metrics. This function
// should match the signature of a value in veneur.MetricSinkTypes, and is
// intended to be passed into veneur.NewFromConfig to be called based on the
// provided configuration.
func CreateMetricSink(
	server *veneur.Server, name string, logger *logrus.Entry,
	config veneur.Config, sinkConfig veneur.MetricSinkConfig,
) (sinks.MetricSink, error) {
	newRelicConfig, ok := sinkConfig.(NewRelicMetricSinkConfig)
	if !ok {
		return nil, errors.New("invalid sink config type")
	}

	// For storing metrics
	h, err := newHarvester(
		newRelicConfig.InsertKey.Value, logger, newRelicConfig.CommonTags, "") // No Span Override
	if err != nil {
		logger.WithError(err).Error("unable to create NewRelicMetricSink")
		return nil, err
	}

	if newRelicConfig.EventType == "" {
		newRelicConfig.EventType = DefaultEventType
	}
	if newRelicConfig.ServiceCheckEventType == "" {
		newRelicConfig.ServiceCheckEventType = DefaultServiceCheckEventType
	}

	nr, err := newrelic.New(
		newrelic.ConfigInsightsInsertKey(newRelicConfig.InsertKey.Value),
		newrelic.ConfigRegion(newRelicConfig.Region),
		newrelic.ConfigServiceName("veneur"),
	)
	if err != nil {
		logger.WithError(err).Error("unable to create NewRelicMetricSink")
		return nil, err
	}

	// Start the batch workers
	err = nr.Events.BatchMode(context.Background(), newRelicConfig.AccountID)
	if err != nil {
		logger.WithError(err).Error("unable to create NewRelicMetricSink")
		return nil, err
	}
	logger.WithFields(logrus.Fields{
		"accountID":             newRelicConfig.AccountID,
		"eventType":             newRelicConfig.EventType,
		"serviceCheckEventType": newRelicConfig.ServiceCheckEventType,
	}).Info("started New Relic sink")

	return &NewRelicMetricSink{
		accountID:         newRelicConfig.AccountID,
		client:            nr,
		eventType:         newRelicConfig.EventType,
		harvester:         h,
		log:               logger,
		name:              name,
		serviceCheckEvent: newRelicConfig.ServiceCheckEventType,
	}, nil
}

// Name returns the name of the sink
func (nr *NewRelicMetricSink) Name() string {
	return nr.name
}

// Kin returns the kind of the sink
func (nr *NewRelicMetricSink) Kind() string {
	return "newrelic"
}

func (nr *NewRelicMetricSink) Start(c *trace.Client) error {
	nr.traceClient = c

	return nil
}

func (nr *NewRelicMetricSink) Flush(ctx context.Context, interMetrics []samplers.InterMetric) (sinks.MetricFlushResult, error) {
	span, _ := trace.StartSpanFromContext(ctx, "")
	defer span.ClientFinish(nr.traceClient)

	if nr.harvester == nil {
		err := errors.New("New Relic sink was not initialized")
		nr.log.Error(err)
		return sinks.MetricFlushResult{}, err
	}

	if len(interMetrics) <= 0 {
		return sinks.MetricFlushResult{}, nil
	}

	for _, m := range interMetrics {
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

	return sinks.MetricFlushResult{}, nil
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
