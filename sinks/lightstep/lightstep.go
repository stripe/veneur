package lightstep

import (
	"fmt"
	"net/url"
	"strconv"
	"sync"
	"time"

	"github.com/DataDog/datadog-go/statsd"
	lightstep "github.com/lightstep/lightstep-tracer-go"
	opentracing "github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"
	"github.com/sirupsen/logrus"
	"github.com/stripe/veneur/protocol"
	"github.com/stripe/veneur/sinks"
	"github.com/stripe/veneur/ssf"
	"github.com/stripe/veneur/trace"
)

const indicatorSpanTagName = "indicator"

const lightstepDefaultPort = 8080
const lightstepDefaultInterval = 5 * time.Minute

const lightStepOperationKey = "name"

// LightStepSpanSink is a sink for spans to be sent to the LightStep client.
type LightStepSpanSink struct {
	tracers      []opentracing.Tracer
	stats        *statsd.Client
	commonTags   map[string]string
	mutex        *sync.Mutex
	serviceCount map[string]int64
	traceClient  *trace.Client
	log          *logrus.Logger
}

var _ sinks.SpanSink = &LightStepSpanSink{}

// NewLightStepSpanSink creates a new instance of a LightStepSpanSink.
func NewLightStepSpanSink(collector string, reconnectPeriod string, maximumSpans int, numClients int, accessToken string, stats *statsd.Client, commonTags map[string]string, log *logrus.Logger) (*LightStepSpanSink, error) {

	var host *url.URL
	host, err := url.Parse(collector)
	if err != nil {
		log.WithError(err).WithField(
			"host", collector,
		).Error("Error parsing LightStep collector URL")
		return &LightStepSpanSink{}, err
	}

	port, err := strconv.Atoi(host.Port())
	if err != nil {
		log.WithError(err).WithFields(logrus.Fields{
			"port":         port,
			"default_port": lightstepDefaultPort,
		}).Warn("Error parsing LightStep port, using default")
		port = lightstepDefaultPort
	}

	reconPeriod := lightstepDefaultInterval
	if reconnectPeriod != "" {
		reconPeriod, err = time.ParseDuration(reconnectPeriod)
		if err != nil {
			log.WithError(err).WithFields(logrus.Fields{
				"interval":         reconnectPeriod,
				"default_interval": lightstepDefaultInterval,
			}).Warn("Failed to parse reconnect duration, using default.")
			reconPeriod = lightstepDefaultInterval
		}
	}

	log.WithFields(logrus.Fields{
		"Host": host.Hostname(),
		"Port": port,
	}).Info("Dialing lightstep host")

	lightstepMultiplexTracerNum := numClients
	// If config value is missing, this value should default to one client
	if lightstepMultiplexTracerNum <= 0 {
		lightstepMultiplexTracerNum = 1
	}

	tracers := make([]opentracing.Tracer, 0, lightstepMultiplexTracerNum)

	for i := 0; i < lightstepMultiplexTracerNum; i++ {
		tracers = append(tracers, lightstep.NewTracer(lightstep.Options{
			AccessToken:     accessToken,
			ReconnectPeriod: reconPeriod,
			Collector: lightstep.Endpoint{
				Host:      host.Hostname(),
				Port:      port,
				Plaintext: true,
			},
			UseGRPC:          true,
			MaxBufferedSpans: maximumSpans,
		}))
	}

	return &LightStepSpanSink{
		tracers:      tracers,
		stats:        stats,
		serviceCount: make(map[string]int64),
		mutex:        &sync.Mutex{},
		log:          log,
	}, nil
}

func (ls *LightStepSpanSink) Start(cl *trace.Client) error {
	ls.traceClient = cl
	return nil
}

// Name returns this sink's name.
func (ls *LightStepSpanSink) Name() string {
	return "lightstep"
}

// Ingest takes in a span and passed it along to the LS client after
// some sanity checks and improvements are made.
func (ls *LightStepSpanSink) Ingest(ssfSpan *ssf.SSFSpan) error {
	if err := protocol.ValidateTrace(ssfSpan); err != nil {
		return err
	}

	parentID := ssfSpan.ParentId
	if parentID <= 0 {
		parentID = 0
	}

	var errorCode int64
	if ssfSpan.Error {
		errorCode = 1
	}

	timestamp := time.Unix(ssfSpan.StartTimestamp/1e9, ssfSpan.StartTimestamp%1e9)

	if len(ls.tracers) == 0 {
		err := fmt.Errorf("No lightstep tracer clients initialized")
		ls.log.Error(err)
		return err
	}
	// pick the tracer to use
	tracerIndex := ssfSpan.TraceId % int64(len(ls.tracers))
	tracer := ls.tracers[tracerIndex]

	sp := tracer.StartSpan(
		ssfSpan.Name,
		opentracing.StartTime(timestamp),
		lightstep.SetTraceID(uint64(ssfSpan.TraceId)),
		lightstep.SetSpanID(uint64(ssfSpan.Id)),
		lightstep.SetParentSpanID(uint64(parentID)))

	sp.SetTag(trace.ResourceKey, ssfSpan.Tags[trace.ResourceKey]) // TODO Why is this here?
	sp.SetTag(lightstep.ComponentNameKey, ssfSpan.Service)
	sp.SetTag(indicatorSpanTagName, strconv.FormatBool(ssfSpan.Indicator))
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
	finishOpts := opentracing.FinishOptions{FinishTime: endTime}
	sp.FinishWithOptions(finishOpts)

	service := ssfSpan.Service
	if service == "" {
		service = "unknown"
	}

	// Protect mutating the service count with a mutex
	ls.mutex.Lock()
	defer ls.mutex.Unlock()
	ls.serviceCount[service]++

	return nil
}

// Flush doesn't need to do anything to the LS tracer, so we emit metrics
// instead.
func (ls *LightStepSpanSink) Flush() {
	ls.mutex.Lock()
	defer ls.mutex.Unlock()

	totalCount := int64(0)
	for service, count := range ls.serviceCount {
		totalCount += count
		ls.stats.Count(sinks.MetricKeyTotalSpansFlushed, count, []string{fmt.Sprintf("sink:%s", ls.Name()), fmt.Sprintf("service:%s", service)}, 1)
	}
	ls.serviceCount = make(map[string]int64)
	ls.log.WithField("total_spans", totalCount).Debug("Checkpointing flushed spans for Lightstep")
}
