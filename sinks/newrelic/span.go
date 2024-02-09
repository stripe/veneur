package newrelic

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/newrelic/newrelic-telemetry-sdk-go/telemetry"
	"github.com/sirupsen/logrus"
	veneur "github.com/stripe/veneur/v14"
	"github.com/stripe/veneur/v14/protocol"
	"github.com/stripe/veneur/v14/sinks"
	"github.com/stripe/veneur/v14/ssf"
	"github.com/stripe/veneur/v14/trace"
	"github.com/stripe/veneur/v14/util"
)

type NewRelicSpanSinkConfig struct {
	CommonTags       []string          `yaml:"common_tags"`
	InsertKey        util.StringSecret `yaml:"insert_key"`
	TraceObserverURL string            `yaml:"trace_observer_url"`
}

type NewRelicSpanSink struct {
	harvester   *telemetry.Harvester
	log         *logrus.Entry
	name        string
	traceClient *trace.Client
}

// ParseSpanConfig decodes the map config for a New Relic span sink into a
// NewRelicSpanSinkConfig struct.
func ParseSpanConfig(
	name string, config interface{},
) (veneur.SpanSinkConfig, error) {
	newRelicConfig := NewRelicSpanSinkConfig{}
	err := util.DecodeConfig(name, config, &newRelicConfig)
	if err != nil {
		return nil, err
	}
	return newRelicConfig, nil
}

// CreateSpanSink creates a new New Relic sink for spans. This function
// should match the signature of a value in veneur.SpanSinkTypes, and is
// intended to be passed into veneur.NewFromConfig to be called based on the
// provided configuration.
func CreateSpanSink(
	server *veneur.Server, name string, logger *logrus.Entry,
	config veneur.Config, sinkConfig veneur.SpanSinkConfig,
) (sinks.SpanSink, error) {
	newRelicConfig := sinkConfig.(NewRelicSpanSinkConfig)

	sink := NewRelicSpanSink{
		log:  logger,
		name: name,
	}

	harvester, err := newHarvester(
		newRelicConfig.InsertKey.Value, logger, newRelicConfig.CommonTags,
		newRelicConfig.TraceObserverURL)
	if err != nil {
		logger.WithError(err).Error("unable to create NewRelicSpanSink")
		return nil, err
	}
	sink.harvester = harvester

	return &sink, nil
}

// Name returns the name of the sink
func (nr *NewRelicSpanSink) Name() string {
	return nr.name
}

// Start
func (nr *NewRelicSpanSink) Start(c *trace.Client) error {
	nr.traceClient = c
	return nil
}

// Ingest takes a span and records it in the New Relic telemetery
// format to be sent to New Relic on the next Flush()
func (nr *NewRelicSpanSink) Ingest(ssfSpan *ssf.SSFSpan) error {
	if err := protocol.ValidateTrace(ssfSpan); err != nil {
		return err
	}

	if nr.harvester == nil {
		err := errors.New("New Relic sink was not initialized")
		if nr.log != nil {
			nr.log.Error(err)
		}
		return err
	}

	startTime := time.Unix(ssfSpan.StartTimestamp/1e9, ssfSpan.StartTimestamp%1e9)
	endTime := time.Unix(ssfSpan.EndTimestamp/1e9, ssfSpan.EndTimestamp%1e9)
	duration := endTime.Sub(startTime)

	// copy out the tags
	tags := make(map[string]interface{}, len(ssfSpan.Tags))
	for k, v := range ssfSpan.Tags {
		tags[k] = v
	}

	err := nr.harvester.RecordSpan(telemetry.Span{
		ID:          strconv.FormatInt(ssfSpan.Id, 10),
		TraceID:     strconv.FormatInt(ssfSpan.TraceId, 10),
		Name:        ssfSpan.Name,
		ServiceName: ssfSpan.Service,
		Timestamp:   startTime,
		Duration:    duration,
		Attributes:  tags,
	})
	if err != nil {
		if nr.log != nil {
			nr.log.WithError(err).Error("failed to record span")
		}
		return err
	}

	return nil
}

// Flush signals for the sink to send data to New Relic
func (nr *NewRelicSpanSink) Flush() {
	if nr.harvester == nil {
		if nr.log != nil {
			nr.log.Error("New Relic sink was not initialized")
		}
		return
	}

	ctx := context.Background()
	nr.harvester.HarvestNow(ctx)

	return
}
