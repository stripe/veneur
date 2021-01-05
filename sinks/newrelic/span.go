package newrelic

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/newrelic/newrelic-telemetry-sdk-go/telemetry"
	"github.com/sirupsen/logrus"
	"github.com/stripe/veneur/v14/protocol"
	"github.com/stripe/veneur/v14/sinks"
	"github.com/stripe/veneur/v14/ssf"
	"github.com/stripe/veneur/v14/trace"
)

type NewRelicSpanSink struct {
	harvester   *telemetry.Harvester
	log         *logrus.Logger
	traceClient *trace.Client
}

var _ sinks.SpanSink = &NewRelicSpanSink{}

// NewNewRelicSpanSink creates a new NewRelicSpanSink. This sink sends
// span data to the New Relic Platform
func NewNewRelicSpanSink(insertKey string, accountID int, reg string, tags []string, spanURL string, log *logrus.Logger) (*NewRelicSpanSink, error) {
	var ret NewRelicSpanSink

	h, err := newHarvester(insertKey, log, tags, spanURL)
	if err != nil {
		log.WithError(err).Error("unable to create NewRelicSpanSink")
		return &ret, err
	}
	ret.harvester = h
	ret.log = log

	return &ret, nil
}

// Name returns the name of the sink
func (nr *NewRelicSpanSink) Name() string {
	return "newrelic"
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
