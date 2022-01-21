package lightstep

import (
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	lightstep "github.com/lightstep/lightstep-tracer-go"
	opentracing "github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"
	"github.com/sirupsen/logrus"
	veneur "github.com/stripe/veneur/v14"
	"github.com/stripe/veneur/v14/protocol"
	"github.com/stripe/veneur/v14/sinks"
	"github.com/stripe/veneur/v14/ssf"
	"github.com/stripe/veneur/v14/trace"
	"github.com/stripe/veneur/v14/trace/metrics"
	"github.com/stripe/veneur/v14/util"
)

const indicatorSpanTagName = "indicator"

const lightstepDefaultPort = 8080
const lightstepDefaultInterval = 5 * time.Minute

var unexpectedCountTypeErr = fmt.Errorf("Received unexpected count type")

type LightStepSpanSinkConfig struct {
	AccessToken     util.StringSecret `yaml:"lightstep_access_token"`
	CollectorHost   util.Url          `yaml:"lightstep_collector_host"`
	MaximumSpans    int               `yaml:"lightstep_maximum_spans"`
	NumClients      int               `yaml:"lightstep_num_clients"`
	ReconnectPeriod time.Duration     `yaml:"lightstep_reconnect_period"`
}

// LightStepSpanSink is a sink for spans to be sent to the LightStep client.
type LightStepSpanSink struct {
	tracers      []opentracing.Tracer
	mutex        *sync.Mutex
	serviceCount sync.Map
	traceClient  *trace.Client
	log          *logrus.Entry
	name         string
}

var _ sinks.SpanSink = &LightStepSpanSink{}

func ParseSpanConfig(
	name string, config interface{},
) (veneur.SpanSinkConfig, error) {
	lightstepConfig := LightStepSpanSinkConfig{}
	err := util.DecodeConfig(name, config, &lightstepConfig)
	if err != nil {
		return nil, err
	}

	if lightstepConfig.ReconnectPeriod == 0 {
		lightstepConfig.ReconnectPeriod = lightstepDefaultInterval
	}

	return lightstepConfig, nil
}

// NewLightStepSpanSink creates a new instance of a LightStepSpanSink.
func CreateSpanSink(
	server *veneur.Server, name string, logger *logrus.Entry,
	config veneur.Config, sinkConfig veneur.SpanSinkConfig,
) (sinks.SpanSink, error) {
	lightstepConfig, ok := sinkConfig.(LightStepSpanSinkConfig)
	if !ok {
		return nil, errors.New("invalid sink config type")
	}

	host := lightstepConfig.CollectorHost.Value
	port, err := strconv.Atoi(host.Port())
	if err != nil {
		logger.WithError(err).WithFields(logrus.Fields{
			"port":         port,
			"default_port": lightstepDefaultPort,
		}).Warn("Error parsing LightStep port, using default")
		port = lightstepDefaultPort
	}

	logger.WithFields(logrus.Fields{
		"Host": host.Hostname(),
		"Port": port,
	}).Info("Dialing lightstep host")

	lightstepMultiplexTracerNum := lightstepConfig.NumClients
	// If config value is missing, this value should default to one client
	if lightstepMultiplexTracerNum <= 0 {
		lightstepMultiplexTracerNum = 1
	}

	tracers := make([]opentracing.Tracer, 0, lightstepMultiplexTracerNum)

	plaintext := false
	if host.Scheme == "http" {
		plaintext = true
	}

	for i := 0; i < lightstepMultiplexTracerNum; i++ {
		tracers = append(tracers, lightstep.NewTracer(lightstep.Options{
			// Unsure whether this should be Token.Value or Token.String
			AccessToken:     lightstepConfig.AccessToken.Value,
			ReconnectPeriod: lightstepConfig.ReconnectPeriod,
			Collector: lightstep.Endpoint{
				Host:      host.Hostname(),
				Port:      port,
				Plaintext: plaintext,
			},
			UseGRPC:          true,
			MaxBufferedSpans: lightstepConfig.MaximumSpans,
		}))
	}

	return &LightStepSpanSink{
		tracers:      tracers,
		serviceCount: sync.Map{},
		mutex:        &sync.Mutex{},
		log:          logger,
		name:         name,
	}, nil
}

// TODO(awb): Remove this once the old configuration format has been
// removed.
func MigrateConfig(conf *veneur.Config) {
	if conf.LightstepAccessToken.Value == "" {
		return
	}
	conf.SpanSinks = append(conf.SpanSinks, veneur.SinkConfig{
		Kind: "lightstep",
		Name: "lightstep",
		Config: LightStepSpanSinkConfig{
			AccessToken:     conf.LightstepAccessToken,
			CollectorHost:   conf.LightstepCollectorHost,
			MaximumSpans:    conf.LightstepMaximumSpans,
			NumClients:      conf.LightstepNumClients,
			ReconnectPeriod: conf.LightstepReconnectPeriod,
		},
	})
}

// Start performs final adjustments on the sink.
func (ls *LightStepSpanSink) Start(cl *trace.Client) error {
	ls.traceClient = cl
	return nil
}

// Name returns this sink's name.
func (ls *LightStepSpanSink) Name() string {
	return ls.name
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

	count, ok := ls.serviceCount.Load(service)
	if !ok {
		// ensure the value is in the map
		// we only do this if the value was not found in the map once already, to save an
		// allocation and more expensive operation in the typical case
		var c int64 = 0
		count, _ = ls.serviceCount.LoadOrStore(service, &c)
	}

	c, ok := count.(*int64)
	if !ok {
		ls.log.WithField("type", reflect.TypeOf(count)).Debug(unexpectedCountTypeErr.Error())
		return unexpectedCountTypeErr
	}
	atomic.AddInt64(c, 1)
	return nil
}

// Flush doesn't need to do anything to the LS tracer, so we emit metrics
// instead.
func (ls *LightStepSpanSink) Flush() {
	ls.mutex.Lock()
	defer ls.mutex.Unlock()

	samples := &ssf.Samples{}
	defer metrics.Report(ls.traceClient, samples)

	totalCount := int64(0)

	ls.serviceCount.Range(func(keyI, valueI interface{}) bool {
		service, ok := keyI.(string)
		if !ok {
			ls.log.WithFields(logrus.Fields{
				"key":  keyI,
				"type": reflect.TypeOf(keyI),
			}).Error("Invalid key type in map when flushing Lightstep client")
			return true
		}

		value, ok := valueI.(*int64)
		if !ok {
			ls.log.WithFields(logrus.Fields{
				"value": valueI,
				"type":  reflect.TypeOf(valueI),
			}).Error("Invalid value type in map when flushing Lightstep client")
			return true
		}

		count := atomic.SwapInt64(value, 0)
		totalCount += count
		samples.Add(ssf.Count(sinks.MetricKeyTotalSpansFlushed, float32(count),
			map[string]string{"sink": ls.Name(), "service": service}))

		return true
	})

	ls.log.WithField("total_spans", totalCount).Debug("Checkpointing flushed spans for Lightstep")
}
