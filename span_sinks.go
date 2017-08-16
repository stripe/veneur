package veneur

import (
	"container/ring"
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"

	"github.com/DataDog/datadog-go/statsd"
	"github.com/Sirupsen/logrus"
	lightstep "github.com/lightstep/lightstep-tracer-go"
	opentracing "github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"
	"github.com/stripe/veneur/ssf"
	"github.com/stripe/veneur/trace"
)

const datadogResourceKey = "resource"
const datadogNameKey = "name"

const lightStepOperationKey = "name"

const totalSpansFlushedMetricKey = "worker.trace.sink.flushed_total"

// SpanSink is a receiver of spans that handles sending those spans to some
// downstream sink. Calls to `Ingest(span)` are meant to give the sink control
// of the span, with periodic calls to flush as a signal for sinks that don't
// handle their own flushing in a separate goroutine, etc.
type SpanSink interface {
	Name() string
	Ingest(ssf.SSFSpan)
	Flush()
}

// DatadogSpanSink is a sink for sending spans to a Datadog trace agent.
type DatadogSpanSink struct {
	HTTPClient   *http.Client
	buffer       *ring.Ring
	bufferSize   int
	mutex        *sync.Mutex
	stats        *statsd.Client
	commonTags   map[string]string
	traceAddress string
}

// NewDatadogSpanSink creates a new Datadog sink for trace spans.
func NewDatadogSpanSink(config *Config, stats *statsd.Client, httpClient *http.Client, commonTags map[string]string) (*DatadogSpanSink, error) {
	return &DatadogSpanSink{
		HTTPClient:   httpClient,
		bufferSize:   config.SsfBufferSize,
		buffer:       ring.New(config.SsfBufferSize),
		mutex:        &sync.Mutex{},
		stats:        stats,
		commonTags:   commonTags,
		traceAddress: config.DatadogTraceAPIAddress,
	}, nil
}

// Name returns the name of this sink.
func (dd *DatadogSpanSink) Name() string {
	return "datadog"
}

// Ingest takes the span and adds it to the ringbuffer.
func (dd *DatadogSpanSink) Ingest(span ssf.SSFSpan) {
	dd.mutex.Lock()
	defer dd.mutex.Unlock()

	dd.buffer.Value = span
	dd.buffer = dd.buffer.Next()
}

// Flush signals the sink to send it's spans to their destination. For this
// sync it means we'll be making an HTTP request to send them along. We assume
// it's beneficial to performance to defer these until the normal 10s flush.
func (dd *DatadogSpanSink) Flush() {
	dd.mutex.Lock()

	ssfSpans := make([]ssf.SSFSpan, 0, dd.buffer.Len())

	dd.buffer.Do(func(t interface{}) {
		const tooEarly = 1497
		const tooLate = 1497629343000000

		if t != nil {
			ssfSpan, ok := t.(ssf.SSFSpan)
			if !ok {
				log.Error("Got an unknown object in tracing ring!")
				// We'll just skip this one so we don't poison pill or anything.
			}

			var timeErr string
			if ssfSpan.StartTimestamp < tooEarly {
				timeErr = "type:tooEarly"
			}
			if ssfSpan.StartTimestamp > tooLate {
				timeErr = "type:tooLate"
			}
			if timeErr != "" {
				dd.stats.Incr("worker.trace.sink.timestamp_error", []string{timeErr}, 1) // TODO tag as dd?
			}

			if ssfSpan.Tags == nil {
				ssfSpan.Tags = make(map[string]string)
			}

			// Add common tags from veneur's config
			// this will overwrite tags already present on the span
			for k, v := range dd.commonTags {
				ssfSpan.Tags[k] = v
			}
			ssfSpans = append(ssfSpans, ssfSpan)
		}
	})

	// Reset the ring.
	dd.buffer = ring.New(dd.bufferSize)

	// We're done manipulating stuff, let Ingest loose again.
	dd.mutex.Unlock()

	serviceCount := make(map[string]int64)
	var finalTraces []*DatadogTraceSpan
	// Conver the SSFSpans into Datadog Spans
	for _, span := range ssfSpans {
		// -1 is a canonical way of passing in invalid info in Go
		// so we should support that too
		parentID := span.ParentId

		// check if this is the root span
		if parentID <= 0 {
			// we need parentId to be zero for json:omitempty to work
			parentID = 0
		}

		resource := span.Tags[datadogResourceKey]
		name := span.Tags[datadogNameKey]

		tags := map[string]string{}
		// Get the span's existing tags
		for k, v := range span.Tags {
			tags[k] = v
		}

		delete(tags, datadogNameKey)
		delete(tags, datadogResourceKey)

		// TODO implement additional metrics
		var metrics map[string]float64

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
			// TODO don't hardcode
			Type:    "http",
			Error:   errorCode,
			Metrics: metrics,
			Meta:    tags,
		}
		serviceCount[span.Service]++
		finalTraces = append(finalTraces, ddspan)
	}

	if len(finalTraces) != 0 {
		// this endpoint is not documented to take an array... but it does
		// another curious constraint of this endpoint is that it does not
		// support "Content-Encoding: deflate"

		err := postHelper(context.TODO(), dd.HTTPClient, dd.stats, fmt.Sprintf("%s/spans", dd.traceAddress), finalTraces, "flush_traces", false)

		if err == nil {
			log.WithField("traces", len(finalTraces)).Info("Completed flushing traces to Datadog")
			dd.stats.Count(totalSpansFlushedMetricKey, int64(len(ssfSpans)), []string{"sink:datadog"}, 1)
			// TODO: Per service counters?
		} else {
			log.WithFields(logrus.Fields{
				"traces":        len(finalTraces),
				logrus.ErrorKey: err}).Warn("Error flushing traces to Datadog")
		}
		for service, count := range serviceCount {
			dd.stats.Count(totalSpansFlushedMetricKey, count, []string{"sink:datadog", fmt.Sprintf("service:%s", service)}, 1)
		}
	} else {
		log.Info("No traces to flush to Datadog, skipping.")
	}
}

// LightStepSpanSink is a sink for spans to be sent to the LightStep client.
type LightStepSpanSink struct {
	tracer       opentracing.Tracer
	stats        *statsd.Client
	commonTags   map[string]string
	mutex        *sync.Mutex
	serviceCount map[string]int64
}

// NewLightStepSpanSink creates a new instance of a LightStepSpanSink.
func NewLightStepSpanSink(config *Config, stats *statsd.Client, commonTags map[string]string) (*LightStepSpanSink, error) {

	var host *url.URL
	host, err := url.Parse(config.TraceLightstepCollectorHost)
	if err != nil {
		log.WithError(err).WithField(
			"host", config.TraceLightstepCollectorHost,
		).Error("Error parsing LightStep collector URL")
		return &LightStepSpanSink{}, err
	}

	port, err := strconv.Atoi(host.Port())
	if err != nil {
		port = lightstepDefaultPort
	} else {
		log.WithError(err).WithFields(logrus.Fields{
			"port":         port,
			"default_port": lightstepDefaultPort,
		}).Warn("Error parsing LightStep port, using default")
	}

	reconPeriod, err := time.ParseDuration(config.TraceLightstepReconnectPeriod)
	if err != nil {
		log.WithError(err).WithFields(logrus.Fields{
			"interval":         config.TraceLightstepReconnectPeriod,
			"default_interval": lightstepDefaultInterval,
		}).Warn("Failed to parse reconnect duration, using default.")
		reconPeriod = lightstepDefaultInterval
	}

	log.WithFields(logrus.Fields{
		"Host": host.Hostname(),
		"Port": port,
	}).Info("Dialing lightstep host")

	maxSpans := config.TraceLightstepMaximumSpans
	if maxSpans == 0 {
		maxSpans = config.SsfBufferSize
		log.WithField("max spans", maxSpans).Info("Using default maximum spans — ssf_buffer_size — for LightStep")
	}

	lightstepTracer := lightstep.NewTracer(lightstep.Options{
		AccessToken:     config.TraceLightstepAccessToken,
		ReconnectPeriod: reconPeriod,
		Collector: lightstep.Endpoint{
			Host:      host.Hostname(),
			Port:      port,
			Plaintext: true,
		},
		UseGRPC:          true,
		MaxBufferedSpans: maxSpans,
	})

	return &LightStepSpanSink{
		tracer:       lightstepTracer,
		stats:        stats,
		serviceCount: make(map[string]int64),
		mutex:        &sync.Mutex{},
	}, nil
}

// Name returns this sink's name.
func (ls *LightStepSpanSink) Name() string {
	return "lightstep"
}

// Ingest takes in a span and passed it along to the LS client after
// some sanity checks and improvements are made.
func (ls *LightStepSpanSink) Ingest(ssfSpan ssf.SSFSpan) {
	ls.mutex.Lock()
	defer ls.mutex.Unlock()

	parentID := ssfSpan.ParentId
	if parentID <= 0 {
		parentID = 0
	}

	var errorCode int64
	if ssfSpan.Error {
		errorCode = 1
	}

	timestamp := time.Unix(ssfSpan.StartTimestamp/1e9, ssfSpan.StartTimestamp%1e9)
	sp := ls.tracer.StartSpan(
		ssfSpan.Tags[lightStepOperationKey],
		opentracing.StartTime(timestamp),
		lightstep.SetTraceID(uint64(ssfSpan.TraceId)),
		lightstep.SetSpanID(uint64(ssfSpan.Id)),
		lightstep.SetParentSpanID(uint64(parentID)))

	sp.SetTag(trace.ResourceKey, ssfSpan.Tags[trace.ResourceKey]) // TODO Why is this here?
	sp.SetTag(lightstep.ComponentNameKey, ssfSpan.Service)
	// TODO don't hardcode
	sp.SetTag("type", "http")
	sp.SetTag("error-code", errorCode)
	for k, v := range ssfSpan.Tags {
		sp.SetTag(k, v)
	}
	// And now set any veneur common tags
	for k, v := range ls.commonTags {
		sp.SetTag(k, v)
	}
	// TODO add metrics as tags to the span as well?

	if errorCode > 0 {
		// Note: this sets the OT-standard "error" tag, which
		// LightStep uses to flag error spans.
		ext.Error.Set(sp, true)
	}

	endTime := time.Unix(ssfSpan.EndTimestamp/1e9, ssfSpan.EndTimestamp%1e9)
	sp.FinishWithOptions(opentracing.FinishOptions{
		FinishTime: endTime,
	})

	service := ssfSpan.Service
	if service == "" {
		service = "unknown"
	}
	ls.serviceCount[service]++
}

// Flush doesn't need to do anything to the LS tracer, so we emit metrics
// instead.
func (ls *LightStepSpanSink) Flush() {
	ls.mutex.Lock()
	defer ls.mutex.Unlock()

	totalCount := int64(0)
	for service, count := range ls.serviceCount {
		totalCount += count
		ls.stats.Count(totalSpansFlushedMetricKey, count, []string{"sink:lightstep", fmt.Sprintf("service:%s", service)}, 1)
	}
	ls.serviceCount = make(map[string]int64)
	log.WithField("total_spans", totalCount).Debug("Checkpointing flushed spans for Lightstep")
}
