package datadog

import (
	"container/ring"
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stripe/veneur/v14"
	vhttp "github.com/stripe/veneur/v14/http"
	"github.com/stripe/veneur/v14/protocol"
	"github.com/stripe/veneur/v14/protocol/dogstatsd"
	"github.com/stripe/veneur/v14/samplers"
	"github.com/stripe/veneur/v14/sinks"
	"github.com/stripe/veneur/v14/ssf"
	"github.com/stripe/veneur/v14/trace"
	"github.com/stripe/veneur/v14/trace/metrics"
	"github.com/stripe/veneur/v14/util"
)

const datadogNameKey = "name"
const datadogResourceKey = "resource"

// At present Veneur has no way to differentiate between types. This could likely
// be changed to a tag conversion (e.g. tag type is removed and used for this value)
const datadogSpanType = "web"

// datadogSpanBufferSize is the default maximum number of spans that
// we can flush per flush-interval
const datadogSpanBufferSize = 1 << 14

type DatadogMetricSinkConfig struct {
	APIKey                          string   `yaml:"api_key"`
	APIHostname                     string   `yaml:"api_hostname"`
	FlushMaxPerBody                 int      `yaml:"flush_max_per_body"`
	MetricNamePrefixDrops           []string `yaml:"metric_name_prefix_drops"`
	ExcludeTagsPrefixByPrefixMetric []struct {
		MetricPrefix string   `yaml:"metric_prefix"`
		Tags         []string `yaml:"tags"`
	} `yaml:"exclude_tags_prefix_by_prefix_metric"`
}

type DatadogMetricSink struct {
	HTTPClient                      *http.Client
	APIKey                          string
	DDHostname                      string
	hostname                        string
	flushMaxPerBody                 int
	tags                            []string
	interval                        float64
	traceClient                     *trace.Client
	log                             *logrus.Entry
	name                            string
	metricNamePrefixDrops           []string
	excludedTags                    []string
	excludeTagsPrefixByPrefixMetric map[string][]string
}

// ParseMetricConfig decodes the map config for a Datadog metric sink into a
// DatadogMetricSinkConfig struct.
func ParseMetricConfig(name string, config interface{}) (veneur.MetricSinkConfig, error) {
	datadogConfig := DatadogMetricSinkConfig{}
	err := util.DecodeConfig(name, config, &datadogConfig)
	if err != nil {
		return nil, err
	}
	return datadogConfig, nil
}

// CreateMetricSink creates a new Datadog sink for metrics. This function
// should match the signature of a value in veneur.MetricSinkTypes, and is
// intended to be passed into veneur.NewFromConfig to be called based on the
// provided configuration.
func CreateMetricSink(
	server *veneur.Server, name string, logger *logrus.Entry,
	config veneur.Config, sinkConfig veneur.MetricSinkConfig,
) (sinks.MetricSink, error) {
	datadogConfig, ok := sinkConfig.(DatadogMetricSinkConfig)
	if !ok {
		return nil, errors.New("invalid sink config type")
	}

	excludeTagsPrefixByPrefixMetric := map[string][]string{}
	for _, m := range datadogConfig.ExcludeTagsPrefixByPrefixMetric {
		excludeTagsPrefixByPrefixMetric[m.MetricPrefix] = m.Tags
	}

	return &DatadogMetricSink{
		HTTPClient:                      server.HTTPClient,
		APIKey:                          datadogConfig.APIKey,
		DDHostname:                      datadogConfig.APIHostname,
		interval:                        server.Interval.Seconds(),
		flushMaxPerBody:                 datadogConfig.FlushMaxPerBody,
		hostname:                        config.Hostname,
		tags:                            server.Tags,
		name:                            name,
		metricNamePrefixDrops:           datadogConfig.MetricNamePrefixDrops,
		excludeTagsPrefixByPrefixMetric: excludeTagsPrefixByPrefixMetric,
		log:                             logger,
	}, nil
}

// TODO(arnavdugar): Remove this once the old configuration format has been
// removed.
func MigrateConfig(conf *veneur.Config) {
	if conf.DatadogAPIKey.Value != "" && conf.DatadogAPIHostname != "" {
		conf.MetricSinks = append(conf.MetricSinks, veneur.SinkConfig{
			Kind: "datadog",
			Name: "datadog",
			Config: DatadogMetricSinkConfig{
				APIKey:                          conf.DatadogAPIKey.Value,
				APIHostname:                     conf.DatadogAPIHostname,
				FlushMaxPerBody:                 conf.DatadogFlushMaxPerBody,
				MetricNamePrefixDrops:           conf.DatadogMetricNamePrefixDrops,
				ExcludeTagsPrefixByPrefixMetric: conf.DatadogExcludeTagsPrefixByPrefixMetric,
			},
		})
	}

	// configure Datadog as a Span sink
	if conf.DatadogAPIKey.Value != "" && conf.DatadogTraceAPIAddress != "" {
		conf.SpanSinks = append(conf.SpanSinks, veneur.SinkConfig{
			Kind: "datadog",
			Name: "datadog",
			Config: DatadogSpanSinkConfig{
				SpanBufferSize:  conf.DatadogSpanBufferSize,
				TraceAPIAddress: conf.DatadogTraceAPIAddress,
			},
		})
	}
}

// DDEvent represents the structure of datadog's undocumented /intake endpoint
type DDEvent struct {
	Title       string   `json:"msg_title"`
	Text        string   `json:"msg_text"`
	Timestamp   int64    `json:"timestamp,omitempty"` // represented as a unix epoch
	Hostname    string   `json:"host,omitempty"`
	Aggregation string   `json:"aggregation_key,omitempty"`
	Priority    string   `json:"priority,omitempty"`
	Source      string   `json:"source_type_name,omitempty"`
	AlertType   string   `json:"alert_type,omitempty"`
	Tags        []string `json:"tags,omitempty"`
}

// DDMetric is a data structure that represents the JSON that Datadog
// wants when posting to the API
type DDMetric struct {
	Name       string        `json:"metric"`
	Value      [1][2]float64 `json:"points"`
	Tags       []string      `json:"tags,omitempty"`
	MetricType string        `json:"type"`
	Hostname   string        `json:"host,omitempty"`
	DeviceName string        `json:"device_name,omitempty"`
	Interval   int32         `json:"interval,omitempty"`
}

// DDServiceCheck is a representation of the service check.
type DDServiceCheck struct {
	Name      string   `json:"check"`
	Status    int      `json:"status"`
	Hostname  string   `json:"host_name"`
	Timestamp int64    `json:"timestamp,omitempty"` // represented as a unix epoch
	Tags      []string `json:"tags,omitempty"`
	Message   string   `json:"message,omitempty"`
}

// Name returns the name of this sink.
func (dd *DatadogMetricSink) Name() string {
	return dd.name
}

// Start sets the sink up.
func (dd *DatadogMetricSink) Start(cl *trace.Client) error {
	dd.traceClient = cl
	return nil
}

// Flush sends metrics to Datadog
func (dd *DatadogMetricSink) Flush(ctx context.Context, interMetrics []samplers.InterMetric) error {
	span, _ := trace.StartSpanFromContext(ctx, "")
	defer span.ClientFinish(dd.traceClient)

	ddmetrics, checks := dd.finalizeMetrics(interMetrics)

	if len(checks) != 0 {
		// this endpoint is not documented to take an array... but it does
		// another curious constraint of this endpoint is that it does not
		// support "Content-Encoding: deflate"
		err := vhttp.PostHelper(
			context.TODO(), dd.HTTPClient, dd.traceClient, http.MethodPost,
			fmt.Sprintf("%s/api/v1/check_run?api_key=%s", dd.DDHostname, dd.APIKey),
			checks, "flush_checks", false, map[string]string{"sink": "datadog"},
			dd.log)
		if err == nil {
			dd.log.WithField("checks", len(checks)).Info("Completed flushing service checks to Datadog")
		} else {
			dd.log.WithFields(logrus.Fields{
				"checks":        len(checks),
				logrus.ErrorKey: err}).Warn("Error flushing checks to Datadog")
		}
	}

	// break the metrics into chunks of approximately equal size, such that
	// each chunk is less than the limit
	// we compute the chunks using rounding-up integer division
	workers := ((len(ddmetrics) - 1) / dd.flushMaxPerBody) + 1
	chunkSize := ((len(ddmetrics) - 1) / workers) + 1
	dd.log.WithField("workers", workers).Debug("Worker count chosen")
	dd.log.WithField("chunkSize", chunkSize).Debug("Chunk size chosen")
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		chunk := ddmetrics[i*chunkSize:]
		if i < workers-1 {
			// trim to chunk size unless this is the last one
			chunk = chunk[:chunkSize]
		}
		wg.Add(1)
		go dd.flushPart(span.Attach(ctx), chunk, &wg)
	}
	wg.Wait()
	tags := map[string]string{"sink": dd.Name()}
	span.Add(
		ssf.Count(sinks.MetricKeyTotalMetricsFlushed, float32(len(ddmetrics)), tags),
	)
	dd.log.WithField("metrics", len(ddmetrics)).Info("flushed")
	return nil
}

// FlushOtherSamples serializes Events or Service Checks directly to datadog.
// May make 2 external calls to the datadog client.
func (dd *DatadogMetricSink) FlushOtherSamples(ctx context.Context, samples []ssf.SSFSample) {

	events := []DDEvent{}

	span, _ := trace.StartSpanFromContext(ctx, "")
	defer span.ClientFinish(dd.traceClient)

	// fill in the default hostname for packets that didn't set it
	for _, sample := range samples {

		if _, ok := sample.Tags[dogstatsd.EventIdentifierKey]; ok {
			// This is an event!
			ret := DDEvent{
				Title:     sample.Name,
				Text:      sample.Message,
				Timestamp: sample.Timestamp,
				Priority:  "normal",
				AlertType: "info",
			}

			// Defensively copy the tags that came in
			tags := map[string]string{}
			for k, v := range sample.Tags {
				tags[k] = v
			}
			// Remove the tag that flagged this as an event
			delete(tags, dogstatsd.EventIdentifierKey)

			// The parser uses special tags to encode the fields for us from DogStatsD
			// that don't fit into a normal SSF Sample. We'll hunt for each one and
			// delete the tag if we find it.
			if v, ok := tags[dogstatsd.EventAggregationKeyTagKey]; ok {
				ret.Aggregation = v
				delete(tags, dogstatsd.EventAggregationKeyTagKey)
			}
			if v, ok := tags[dogstatsd.EventPriorityTagKey]; ok {
				ret.Priority = v
				delete(tags, dogstatsd.EventPriorityTagKey)
			}
			if v, ok := tags[dogstatsd.EventSourceTypeTagKey]; ok {
				ret.Source = v
				delete(tags, dogstatsd.EventSourceTypeTagKey)
			}
			if v, ok := tags[dogstatsd.EventAlertTypeTagKey]; ok {
				ret.AlertType = v
				delete(tags, dogstatsd.EventAlertTypeTagKey)
			}
			if v, ok := tags[dogstatsd.EventHostnameTagKey]; ok {
				ret.Hostname = v
				delete(tags, dogstatsd.EventHostnameTagKey)
			} else {
				// Default hostname since there isn't one
				ret.Hostname = dd.hostname
			}
			// Do our last bit of tag housekeeping
			finalTags := []string{}
			for k, v := range tags {
				finalTags = append(finalTags, fmt.Sprintf("%s:%s", k, v))
			}

			ret.Tags = append(finalTags, dd.tags...)
			events = append(events, ret)
		} else {
			dd.log.Warn("Received an SSF Sample that wasn't an event or service check, ack!")
		}
	}

	if len(events) != 0 {
		// this endpoint is not documented at all, its existence is only known from
		// the official dd-agent
		// we don't actually pass all the body keys that dd-agent passes here... but
		// it still works
		err := vhttp.PostHelper(
			context.Background(), dd.HTTPClient, dd.traceClient, http.MethodPost,
			fmt.Sprintf("%s/intake?api_key=%s", dd.DDHostname, dd.APIKey),
			map[string]map[string][]DDEvent{
				"events": {
					"api": events,
				},
			}, "flush_events", true, map[string]string{"sink": "datadog"}, dd.log)

		if err == nil {
			dd.log.WithField("events", len(events)).Info("Completed flushing events to Datadog")
		} else {
			dd.log.WithFields(logrus.Fields{
				"events":        len(events),
				logrus.ErrorKey: err}).Warn("Error flushing events to Datadog")
		}
	}
}

// SetExcludedTags sets the excluded tag names. Any tags with the
// provided key (name) will be excluded.
func (dd *DatadogMetricSink) SetExcludedTags(excludes []string) {
	dd.excludedTags = excludes
}

func (dd *DatadogMetricSink) finalizeMetrics(metrics []samplers.InterMetric) ([]DDMetric, []DDServiceCheck) {
	ddMetrics := make([]DDMetric, 0, len(metrics))
	checks := []DDServiceCheck{}

METRICLOOP:
	for _, m := range metrics {
		if !sinks.IsAcceptableMetric(m, dd) {
			continue
		}

		for _, dropMetricPrefix := range dd.metricNamePrefixDrops {
			if strings.HasPrefix(m.Name, dropMetricPrefix) {
				continue METRICLOOP
			}
		}

		// Defensively copy tags since we're gonna mutate it
		tags := make([]string, 0, len(dd.tags))

		// Prepare exclude tags by specific prefix metric
		var excludeTagsPrefixByPrefixMetric []string
		if len(dd.excludeTagsPrefixByPrefixMetric) > 0 {
			for prefixMetric, tags := range dd.excludeTagsPrefixByPrefixMetric {
				if strings.HasPrefix(m.Name, prefixMetric) {
					excludeTagsPrefixByPrefixMetric = tags
					break
				}
			}
		}

		for i := range dd.tags {
			exclude := false
			for j := range dd.excludedTags {
				if strings.HasPrefix(dd.tags[i], dd.excludedTags[j]) {
					exclude = true
					break
				}
			}
			if !exclude {
				tags = append(tags, dd.tags[i])
			}

		}
		var hostname, devicename string
		// Let's look for "magic tags" that override metric fields host and device.
		for _, tag := range m.Tags {
			// This overrides hostname
			if strings.HasPrefix(tag, "host:") {
				// Override the hostname with the tag, trimming off the prefix.
				hostname = tag[5:]
			} else if strings.HasPrefix(tag, "device:") {
				// Same as above, but device this time
				devicename = tag[7:]
			} else {
				exclude := false
				for i := range dd.excludedTags {
					// access excluded tags by index to avoid a string copy
					if strings.HasPrefix(tag, dd.excludedTags[i]) {
						exclude = true
						break
					}

				}

				for i := range excludeTagsPrefixByPrefixMetric {
					if strings.HasPrefix(tag, excludeTagsPrefixByPrefixMetric[i]) {
						exclude = true
						break
					}
				}
				if !exclude {
					tags = append(tags, tag)
				}
			}
		}

		if hostname == "" {
			// No magic tag, set the hostname
			hostname = dd.hostname
		}

		if m.Type == samplers.StatusMetric {
			// This is a service check!
			ret := DDServiceCheck{
				Name:      m.Name,
				Message:   m.Message,
				Timestamp: m.Timestamp,
				Tags:      tags,
				Status:    int(m.Value),
				Hostname:  hostname,
			}
			// Do our last bit of tag housekeeping
			checks = append(checks, ret)
			continue
		}
		metricType := ""
		value := m.Value

		switch m.Type {
		case samplers.CounterMetric:
			// We convert counters into rates for Datadog
			metricType = "rate"
			value = m.Value / dd.interval
		case samplers.GaugeMetric:
			metricType = "gauge"
		default:
			dd.log.WithField("metric_type", m.Type).Warn("Encountered an unknown metric type")
			continue
		}

		ddMetric := DDMetric{
			Name: m.Name,
			Value: [1][2]float64{
				[2]float64{
					float64(m.Timestamp), value,
				},
			},
			Tags:       tags,
			MetricType: metricType,
			Interval:   int32(dd.interval),
			Hostname:   hostname,
			DeviceName: devicename,
		}
		ddMetrics = append(ddMetrics, ddMetric)
	}

	return ddMetrics, checks
}

func (dd *DatadogMetricSink) flushPart(ctx context.Context, metricSlice []DDMetric, wg *sync.WaitGroup) {
	defer wg.Done()
	vhttp.PostHelper(ctx, dd.HTTPClient, dd.traceClient, http.MethodPost,
		fmt.Sprintf("%s/api/v1/series?api_key=%s", dd.DDHostname, dd.APIKey),
		map[string][]DDMetric{
			"series": metricSlice,
		}, "flush", true, map[string]string{"sink": "datadog"}, dd.log)
}

// DatadogTraceSpan represents a trace span as JSON for the
// Datadog tracing API.
type DatadogTraceSpan struct {
	Duration int64              `json:"duration"`
	Error    int64              `json:"error"`
	Meta     map[string]string  `json:"meta"`
	Metrics  map[string]float64 `json:"metrics"`
	Name     string             `json:"name"`
	ParentID int64              `json:"parent_id,omitempty"`
	Resource string             `json:"resource,omitempty"`
	Service  string             `json:"service"`
	SpanID   int64              `json:"span_id"`
	Start    int64              `json:"start"`
	TraceID  int64              `json:"trace_id"`
	Type     string             `json:"type"`
}

type DatadogSpanSinkConfig struct {
	SpanBufferSize  int    `yaml:"datadog_span_buffer_size"`
	TraceAPIAddress string `yaml:"datadog_trace_api_address"`
}

// DatadogSpanSink is a sink for sending spans to a Datadog trace agent.
type DatadogSpanSink struct {
	HTTPClient   *http.Client
	buffer       *ring.Ring
	bufferSize   int
	mutex        *sync.Mutex
	traceAddress string
	traceClient  *trace.Client
	log          *logrus.Entry
	name         string
}

// ParseSpanConfig decodes the map config for a Datadog span sink into a
// DatadogSpanSinkConfig struct.
func ParseSpanConfig(name string, config interface{}) (veneur.SpanSinkConfig, error) {
	datadogConfig := DatadogSpanSinkConfig{}
	err := util.DecodeConfig(name, config, &datadogConfig)
	if err != nil {
		return nil, err
	}
	return datadogConfig, nil
}

// CreateSpanSink creates a new Datadog sink for spans. This function
// should match the signature of a value in veneur.SpanSinkTypes, and is
// intended to be passed into veneur.NewFromConfig to be called based on the
// provided configuration.
func CreateSpanSink(
	server *veneur.Server, name string, logger *logrus.Entry,
	config veneur.Config, sinkConfig veneur.SpanSinkConfig,
) (sinks.SpanSink, error) {
	datadogConfig, ok := sinkConfig.(DatadogSpanSinkConfig)
	if !ok {
		return nil, errors.New("invalid sink config type")
	}

	bufferSize := datadogConfig.SpanBufferSize
	if bufferSize == 0 {
		bufferSize = datadogSpanBufferSize
	}

	return &DatadogSpanSink{
		HTTPClient:   server.HTTPClient,
		bufferSize:   bufferSize,
		buffer:       ring.New(bufferSize),
		mutex:        &sync.Mutex{},
		traceAddress: datadogConfig.TraceAPIAddress,
		log:          logger,
		name:         name,
	}, nil
}

// Name returns the name of this sink.
func (dd *DatadogSpanSink) Name() string {
	return dd.name
}

// Start performs final adjustments on the sink.
func (dd *DatadogSpanSink) Start(cl *trace.Client) error {
	dd.traceClient = cl
	return nil
}

// Ingest takes the span and adds it to the ringbuffer.
func (dd *DatadogSpanSink) Ingest(span *ssf.SSFSpan) error {
	if err := protocol.ValidateTrace(span); err != nil {
		return err
	}
	dd.mutex.Lock()
	defer dd.mutex.Unlock()

	dd.buffer.Value = span
	dd.buffer = dd.buffer.Next()
	return nil
}

// Flush signals the sink to send it's spans to their destination. For this
// sync it means we'll be making an HTTP request to send them along. We assume
// it's beneficial to performance to defer these until the normal 10s flush.
func (dd *DatadogSpanSink) Flush() {
	samples := &ssf.Samples{}
	defer metrics.Report(dd.traceClient, samples)
	dd.mutex.Lock()

	flushStart := time.Now()
	ssfSpans := make([]*ssf.SSFSpan, 0, dd.buffer.Len())

	dd.buffer.Do(func(t interface{}) {
		const tooEarly = 1497
		const tooLate = 1497629343000000

		if t != nil {
			ssfSpan, ok := t.(*ssf.SSFSpan)
			if !ok {
				dd.log.Error("Got an unknown object in tracing ring!")
				dd.mutex.Unlock()
				// We'll just skip this one so we don't poison pill or anything.
				return
			}

			var timeErr string

			tags := map[string]string{} // TODO: tag as dd?
			if ssfSpan.StartTimestamp < tooEarly {
				tags["type"] = "tooEarly"
			}
			if ssfSpan.StartTimestamp > tooLate {
				tags["type"] = "tooLate"
			}
			if timeErr != "" {
				samples.Add(ssf.Count("worker.trace.sink.timestamp_error", 1, tags))
			}

			ssfSpans = append(ssfSpans, ssfSpan)
		}
	})

	// Reset the ring.
	dd.buffer = ring.New(dd.bufferSize)

	// We're done manipulating stuff, let Ingest loose again.
	dd.mutex.Unlock()

	serviceCount := make(map[string]int64)
	// Datadog wants the spans for each trace in an array, so make a map.
	traceMap := map[int64][]*DatadogTraceSpan{}
	// Convert the SSFSpans into Datadog Spans
	for _, span := range ssfSpans {
		// -1 is a canonical way of passing in invalid info in Go
		// so we should support that too
		parentID := span.ParentId

		// check if this is the root span
		if parentID <= 0 {
			// we need parentId to be zero for json:omitempty to work
			parentID = 0
		}

		tags := map[string]string{}
		// Get the span's existing tags
		for k, v := range span.Tags {
			tags[k] = v
		}

		resource := span.Tags[datadogResourceKey]
		if resource == "" {
			resource = "unknown"
		}
		delete(tags, datadogResourceKey)

		name := span.Name
		if name == "" {
			name = "unknown"
		}

		var errorCode int64
		if span.Error {
			errorCode = 2
		}

		ddspan := &DatadogTraceSpan{
			TraceID:  span.TraceId,
			SpanID:   span.Id,
			ParentID: parentID,
			Service:  span.Service,
			Name:     name,
			Resource: resource,
			Start:    span.StartTimestamp,
			Duration: span.EndTimestamp - span.StartTimestamp,
			Type:     datadogSpanType,
			Error:    errorCode,
			Meta:     tags,
		}
		serviceCount[span.Service]++
		if _, ok := traceMap[span.TraceId]; !ok {
			traceMap[span.TraceId] = []*DatadogTraceSpan{}
		}
		traceMap[span.TraceId] = append(traceMap[span.TraceId], ddspan)
	}
	// Smush the spans into a two-dimensional array now that they are grouped by trace id.
	finalTraces := make([][]*DatadogTraceSpan, len(traceMap))
	idx := 0
	for _, val := range traceMap {
		finalTraces[idx] = val
		idx++
	}

	if len(finalTraces) != 0 {
		// this endpoint is not documented to take an array... but it does
		// another curious constraint of this endpoint is that it does not
		// support "Content-Encoding: deflate"

		err := vhttp.PostHelper(
			context.TODO(), dd.HTTPClient, dd.traceClient, http.MethodPut,
			fmt.Sprintf("%s/v0.3/traces", dd.traceAddress), finalTraces,
			"flush_traces", false, map[string]string{"sink": "datadog"}, dd.log)
		if err == nil {
			dd.log.WithField("traces", len(finalTraces)).Info("Completed flushing traces to Datadog")
		} else {
			dd.log.WithFields(logrus.Fields{
				"traces":        len(finalTraces),
				logrus.ErrorKey: err}).Warn("Error flushing traces to Datadog")
		}
		for service, count := range serviceCount {
			samples.Add(ssf.Count(sinks.MetricKeyTotalSpansFlushed, float32(count), map[string]string{"sink": dd.Name(), "service": service}))
		}
		samples.Add(ssf.Timing(sinks.MetricKeySpanFlushDuration, time.Since(flushStart), time.Nanosecond, map[string]string{"sink": dd.Name()}))
	} else {
		dd.log.Info("No traces to flush to Datadog, skipping.")
	}
}
