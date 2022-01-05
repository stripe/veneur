package veneur

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"reflect"
	"runtime"
	rtdebug "runtime/debug"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/DataDog/datadog-go/statsd"
	"github.com/getsentry/sentry-go"
	"github.com/sirupsen/logrus"
	"github.com/zenazn/goji/bind"
	"github.com/zenazn/goji/graceful"
	"google.golang.org/grpc"

	"github.com/pkg/profile"

	"github.com/stripe/veneur/v14/importsrv"
	"github.com/stripe/veneur/v14/protocol"
	"github.com/stripe/veneur/v14/samplers"
	"github.com/stripe/veneur/v14/scopedstatsd"
	"github.com/stripe/veneur/v14/sinks"
	"github.com/stripe/veneur/v14/sinks/falconer"
	"github.com/stripe/veneur/v14/sinks/lightstep"
	"github.com/stripe/veneur/v14/sinks/ssfmetrics"
	"github.com/stripe/veneur/v14/sources"
	"github.com/stripe/veneur/v14/ssf"
	"github.com/stripe/veneur/v14/tagging"
	"github.com/stripe/veneur/v14/trace"
	"github.com/stripe/veneur/v14/trace/metrics"
)

// VERSION stores the current veneur version.
// It must be a var so it can be set at link time.
var VERSION = defaultLinkValue

var BUILD_DATE = defaultLinkValue

const defaultLinkValue = "dirty"

var profileStartOnce = sync.Once{}

var log = logrus.StandardLogger()

var tracer = trace.GlobalTracer

const defaultTCPReadTimeout = 10 * time.Minute

const httpQuitEndpoint = "/quitquitquit"

type ParsedSourceConfig interface{}
type MetricSinkConfig interface{}
type SpanSinkConfig interface{}

type SourceTypes = map[string]struct {
	// Creates a new source intsance.
	Create func(
		*Server, string, *logrus.Entry, ParsedSourceConfig,
	) (sources.Source, error)
	// Parses the config for the source into a format that is validated and safe
	// to log.
	ParseConfig func(interface{}) (ParsedSourceConfig, error)
}

type SpanSinkTypes = map[string]struct {
	// Creates a new span sink intsance.
	Create func(
		*Server, string, *logrus.Entry, Config, SpanSinkConfig,
	) (sinks.SpanSink, error)
	// Parses the config for the sink into a format that is validated and safe to
	// log.
	ParseConfig func(string, interface{}) (SpanSinkConfig, error)
}

type MetricSinkTypes = map[string]struct {
	// Creates a new metric sink instance.
	Create func(
		*Server, string, *logrus.Entry, Config, MetricSinkConfig,
	) (sinks.MetricSink, error)
	// Parses the config for the sink into a format that is validated and safe to
	// log.
	ParseConfig func(string, interface{}) (MetricSinkConfig, error)
}

// Config used to create a new server.
type ServerConfig struct {
	Config          Config
	Logger          *logrus.Logger
	MetricSinkTypes MetricSinkTypes
	SourceTypes     SourceTypes
	SpanSinkTypes   SpanSinkTypes
}

// A Server is the actual veneur instance that will be run.
type Server struct {
	Config                Config
	Workers               []*Worker
	EventWorker           *EventWorker
	SpanChan              chan *ssf.SSFSpan
	SpanWorker            *SpanWorker
	SpanWorkerGoroutines  int
	CountUniqueTimeseries bool

	Statsd *scopedstatsd.ScopedClient

	Hostname  string
	Tags      []string
	TagsAsMap map[string]string

	HTTPClient *http.Client

	HTTPAddr         string
	numListeningHTTP *int32 // An atomic boolean for whether or not the HTTP server is running

	ForwardAddr    string
	forwardUseGRPC bool

	StatsdListenAddrs []net.Addr
	SSFListenAddrs    []net.Addr
	GRPCListenAddrs   []net.Addr
	RcvbufBytes       int

	Interval            time.Duration
	synchronizeInterval bool
	numReaders          int
	metricMaxLength     int
	traceMaxLengthBytes int

	tlsConfig      *tls.Config
	tcpReadTimeout time.Duration

	// closed when the server is shutting down gracefully
	shutdown chan struct{}
	httpQuit bool

	HistogramPercentiles []float64

	enableProfiling bool

	HistogramAggregates samplers.HistogramAggregates

	sources     []sources.Source
	spanSinks   []sinks.SpanSink
	metricSinks []sinks.MetricSink

	TraceClient *trace.Client

	ssfInternalMetrics          sync.Map
	listeningPerProtocolMetrics *GlobalListeningPerProtocolMetrics

	// gRPC server
	grpcListenAddress string
	grpcServer        *importsrv.Server

	// gRPC forward clients
	grpcForwardConn *grpc.ClientConn

	stuckIntervals int
	lastFlushUnix  int64

	parser samplers.Parser
}

type GlobalListeningPerProtocolMetrics struct {
	dogstatsdTcpReceivedTotal  int64
	dogstatsdUdpReceivedTotal  int64
	dogstatsdUnixReceivedTotal int64
	dogstatsdGrpcReceivedTotal int64

	ssfUnixReceivedTotal int64
	ssfUdpReceivedTotal  int64
	ssfGrpcReceivedTotal int64
}

type ProtocolType int

const (
	DOGSTATSD_TCP ProtocolType = iota
	DOGSTATSD_UDP
	DOGSTATSD_UNIX
	DOGSTATSD_GRPC
	SSF_UNIX
	SSF_UDP
	SSF_GRPC
)

func (p ProtocolType) String() string {
	return [...]string{
		"dogstatsd-tcp",
		"dogstatsd-udp",
		"dogstatsd-unix",
		"dogstatsd-grpc",
		"ssf-unix",
		"ssf-udp",
		"ssf-grpc",
	}[p]
}

// ssfServiceSpanMetrics refer to the span metrics that will
// be emitted for single (service,ssf_format) combination on each flush.
// It is expected the values on the struct will only be handled
// using atomic operations
type ssfServiceSpanMetrics struct {
	ssfSpansReceivedTotal     int64
	ssfRootSpansReceivedTotal int64
}

// SetLogger sets the default logger in veneur to the passed value.
func SetLogger(logger *logrus.Logger) {
	log = logger
}

func scopeFromName(name string) (ssf.SSFSample_Scope, error) {
	switch name {
	case "default":
		fallthrough
	case "":
		return ssf.SSFSample_DEFAULT, nil
	case "global":
		return ssf.SSFSample_GLOBAL, nil
	case "local":
		return ssf.SSFSample_LOCAL, nil
	default:
		return 0, fmt.Errorf("unknown metric scope option %q", name)
	}
}

func normalizeSpans(conf Config) trace.ClientParam {
	return func(cl *trace.Client) error {
		var err error
		typeScopes := map[ssf.SSFSample_Metric]ssf.SSFSample_Scope{}
		typeScopes[ssf.SSFSample_COUNTER], err = scopeFromName(conf.VeneurMetricsScopes.Counter)
		if err != nil {
			return err
		}
		typeScopes[ssf.SSFSample_GAUGE], err = scopeFromName(conf.VeneurMetricsScopes.Gauge)
		if err != nil {
			return err
		}
		typeScopes[ssf.SSFSample_HISTOGRAM], err = scopeFromName(conf.VeneurMetricsScopes.Histogram)
		if err != nil {
			return err
		}
		typeScopes[ssf.SSFSample_SET], err = scopeFromName(conf.VeneurMetricsScopes.Set)
		if err != nil {
			return err
		}
		typeScopes[ssf.SSFSample_STATUS], err = scopeFromName(conf.VeneurMetricsScopes.Status)
		if err != nil {
			return err
		}

		tags := map[string]string{}
		for _, elem := range conf.VeneurMetricsAdditionalTags {
			tag := strings.SplitN(elem, ":", 2)
			switch len(tag) {
			case 2:
				tags[tag[0]] = tag[1]
			case 1:
				tags[tag[0]] = ""
			}
		}

		normalizer := func(sample *ssf.SSFSample) {
			// adjust tags:
			if sample.Tags == nil {
				sample.Tags = map[string]string{}
			}
			for k, v := range tags {
				if _, ok := sample.Tags[k]; ok {
					// do not overwrite existing tags:
					continue
				}
				sample.Tags[k] = v
			}

			// adjust the scope:
			toScope := typeScopes[sample.Metric]
			if sample.Scope != ssf.SSFSample_DEFAULT || toScope == ssf.SSFSample_DEFAULT {
				return
			}
			sample.Scope = toScope
		}
		option := trace.NormalizeSamples(normalizer)
		return option(cl)
	}
}

func scopesFromConfig(conf Config) (scopedstatsd.MetricScopes, error) {
	var err error
	var ms scopedstatsd.MetricScopes
	ms.Gauge, err = scopeFromName(conf.VeneurMetricsScopes.Gauge)
	if err != nil {
		return ms, err
	}
	ms.Count, err = scopeFromName(conf.VeneurMetricsScopes.Counter)
	if err != nil {
		return ms, err
	}
	ms.Histogram, err = scopeFromName(conf.VeneurMetricsScopes.Histogram)
	if err != nil {
		return ms, err
	}
	return ms, nil
}

func (server *Server) createSources(
	logger *logrus.Logger, config *Config, sourceTypes SourceTypes,
) ([]sources.Source, error) {
	sources := []sources.Source{}
	for index, sourceConfig := range config.Sources {
		sourceFactory, ok := sourceTypes[sourceConfig.Kind]
		if !ok {
			logger.Warnf("Unknown source kind %s; skipping.", sourceConfig.Kind)
		}
		parsedSourceConfig, err := sourceFactory.ParseConfig(sourceConfig.Config)
		if err != nil {
			return nil, err
		}
		// Overwrite the map config with the parsed config. This prevents senstive
		// fields in the map from accidentally being logged.
		config.Sources[index].Config = parsedSourceConfig
		source, err := sourceFactory.Create(
			server, sourceConfig.Name, logger.WithField("source", sourceConfig.Name),
			parsedSourceConfig)
		if err != nil {
			return nil, err
		}
		sources = append(sources, source)
	}
	return sources, nil
}

func (server *Server) createSpanSinks(
	logger *logrus.Logger, config *Config, sinkTypes SpanSinkTypes,
) ([]sinks.SpanSink, error) {
	sinks := []sinks.SpanSink{}
	for index, sinkConfig := range config.SpanSinks {
		sinkFactory, ok := sinkTypes[sinkConfig.Kind]
		if !ok {
			logger.Warnf("Unknown sink kind %s; skipping.", sinkConfig.Kind)
		}
		parsedSinkConfig, err := sinkFactory.ParseConfig(
			sinkConfig.Name, sinkConfig.Config)
		if err != nil {
			return nil, err
		}
		// Overwrite the map config with the parsed config. This prevents senstive
		// fields in the map from accidentally being logged.
		config.SpanSinks[index].Config = parsedSinkConfig
		sink, err := sinkFactory.Create(
			server, sinkConfig.Name, logger.WithField("span_sink", sinkConfig.Name),
			*config, parsedSinkConfig)
		if err != nil {
			return nil, err
		}
		sinks = append(sinks, sink)
	}
	return sinks, nil
}

func (server *Server) createMetricSinks(
	logger *logrus.Logger, config *Config, sinkTypes MetricSinkTypes,
) ([]sinks.MetricSink, error) {
	sinks := []sinks.MetricSink{}
	for index, sinkConfig := range config.MetricSinks {
		sinkFactory, ok := sinkTypes[sinkConfig.Kind]
		if !ok {
			logger.Warnf("Unknown sink kind %s; skipping.", sinkConfig.Kind)
			continue
		}
		parsedSinkConfig, err := sinkFactory.ParseConfig(
			sinkConfig.Name, sinkConfig.Config)
		if err != nil {
			return nil, err
		}
		// Overwrite the map config with the parsed config. This prevents senstive
		// fields in the map from accidentally being logged.
		config.MetricSinks[index].Config = parsedSinkConfig
		sink, err := sinkFactory.Create(
			server, sinkConfig.Name, logger.WithField("metric_sink", sinkConfig.Name),
			*config, parsedSinkConfig)
		if err != nil {
			return nil, err
		}
		sinks = append(sinks, sink)
	}
	return sinks, nil
}

// NewFromConfig creates a new veneur server from a configuration
// specification and sets up the passed logger according to the
// configuration.
func NewFromConfig(config ServerConfig) (*Server, error) {
	logger := config.Logger
	conf := config.Config

	ret := &Server{
		Config:   conf,
		Interval: conf.Interval,
	}

	ret.Hostname = conf.Hostname
	ret.Tags = conf.Tags

	mappedTags := tagging.ParseTagSliceToMap(ret.Tags)

	ret.synchronizeInterval = conf.SynchronizeWithInterval

	ret.TagsAsMap = mappedTags

	ret.parser = samplers.NewParser(conf.ExtendTags)

	ret.HistogramPercentiles = conf.Percentiles
	ret.HistogramAggregates.Value = 0
	for _, agg := range conf.Aggregates {
		ret.HistogramAggregates.Value += samplers.AggregatesLookup[agg]
	}
	ret.HistogramAggregates.Count = len(conf.Aggregates)

	ret.stuckIntervals = conf.FlushWatchdogMissedFlushes

	transport := &http.Transport{
		IdleConnTimeout: ret.Interval * 2, // If we're idle more than one interval something is up
	}

	ret.HTTPClient = &http.Client{
		// make sure that POSTs to datadog do not overflow the flush interval
		Timeout:   ret.Interval * 9 / 10,
		Transport: transport,
	}

	stats, err := statsd.New(conf.StatsAddress, statsd.WithoutTelemetry(), statsd.WithMaxMessagesPerPayload(4096))
	if err != nil {
		return ret, err
	}
	stats.Namespace = "veneur."

	scopes, err := scopesFromConfig(conf)
	if err != nil {
		return ret, err
	}
	ret.Statsd = scopedstatsd.NewClient(stats, conf.VeneurMetricsAdditionalTags, scopes)

	ret.SpanChan = make(chan *ssf.SSFSpan, conf.SpanChannelCapacity)
	ret.TraceClient, err = trace.NewChannelClient(ret.SpanChan,
		trace.ReportStatistics(stats, 1*time.Second, []string{"ssf_format:internal"}),
		normalizeSpans(conf),
	)
	if err != nil {
		return ret, err
	}

	// Initialize Sentry if a Dsn is provided, else we leave it uninitalized
	// and no-op the methods.
	if conf.SentryDsn.Value != "" {
		err = sentry.Init(sentry.ClientOptions{
			Dsn: conf.SentryDsn.Value,
		})
		if err != nil {
			return ret, err
		}
	}

	if conf.Debug {
		logger.SetLevel(logrus.DebugLevel)
	}

	mpf := 0
	if conf.MutexProfileFraction > 0 {
		mpf = runtime.SetMutexProfileFraction(conf.MutexProfileFraction)
	}

	log.WithFields(logrus.Fields{
		"MutexProfileFraction":         conf.MutexProfileFraction,
		"previousMutexProfileFraction": mpf,
	}).Info("Set mutex profile fraction")

	if conf.BlockProfileRate > 0 {
		runtime.SetBlockProfileRate(conf.BlockProfileRate)
	}
	log.WithField("BlockProfileRate", conf.BlockProfileRate).Info("Set block profile rate (nanoseconds)")

	if conf.EnableProfiling {
		ret.enableProfiling = true
	}

	// This is a check to ensure that we don't repeatedly add a hook
	// to the "global" log instance on repeated calls to `NewFromConfig`
	// such as those made in testing. By skipping this we avoid a race
	// condition in logrus discussed here:
	// https://github.com/sirupsen/logrus/issues/295
	if _, ok := logger.Hooks[logrus.FatalLevel]; !ok {
		logger.AddHook(sentryHook{
			hostname: ret.Hostname,
			lv: []logrus.Level{
				logrus.ErrorLevel,
				logrus.FatalLevel,
				logrus.PanicLevel,
			},
		})
	}

	// After log hooks are configured, if any further errors are
	// found during setup we should use logger.Fatal or
	// logger.Panic, because that will give us breakage monitoring
	// through Sentry.
	numWorkers := 1
	if conf.NumWorkers > 1 {
		numWorkers = conf.NumWorkers
	}
	logger.WithField("number", numWorkers).Info("Preparing workers")
	// Allocate the slice, we'll fill it with workers later.
	ret.Workers = make([]*Worker, numWorkers)
	ret.numReaders = conf.NumReaders

	// This must come before worker initialization. We need to
	// initialize workers with state from *Server.IsWorker.
	ret.ForwardAddr = conf.ForwardAddress

	// Control whether Veneur should emit metric
	// "veneur.flush.unique_timeseries_total", which may come at a
	// slight performance hit to workers.
	ret.CountUniqueTimeseries = conf.CountUniqueTimeseries

	// Use the pre-allocated Workers slice to know how many to start.
	for i := range ret.Workers {
		ret.Workers[i] = NewWorker(i+1, ret.IsLocal(), ret.CountUniqueTimeseries, ret.TraceClient, log, ret.Statsd)
		// do not close over loop index
		go func(w *Worker) {
			defer func() {
				ConsumePanic(ret.TraceClient, ret.Hostname, recover())
			}()
			w.Work()
		}(ret.Workers[i])
	}

	ret.EventWorker = NewEventWorker(ret.TraceClient, ret.Statsd)

	// Set up a span sink that extracts metrics from SSF spans and
	// reports them via the metric workers:
	processors := make([]ssfmetrics.Processor, len(ret.Workers))
	for i, w := range ret.Workers {
		processors[i] = w
	}
	metricSink, err := ssfmetrics.NewMetricExtractionSink(processors, conf.IndicatorSpanTimerName, conf.ObjectiveSpanTimerName, ret.TraceClient, log, &ret.parser)
	if err != nil {
		return ret, err
	}
	ret.spanSinks = append(ret.spanSinks, metricSink)

	for _, addrStr := range conf.StatsdListenAddresses {
		addr, err := protocol.ResolveAddr(addrStr.Value)
		if err != nil {
			return ret, err
		}
		ret.StatsdListenAddrs = append(ret.StatsdListenAddrs, addr)
	}

	for _, addrStr := range conf.SsfListenAddresses {
		addr, err := protocol.ResolveAddr(addrStr.Value)
		if err != nil {
			return ret, err
		}
		ret.SSFListenAddrs = append(ret.SSFListenAddrs, addr)
	}

	for _, addrStr := range conf.GrpcListenAddresses {
		addr, err := protocol.ResolveAddr(addrStr.Value)
		if err != nil {
			return ret, err
		}
		ret.GRPCListenAddrs = append(ret.GRPCListenAddrs, addr)
	}

	ret.metricMaxLength = conf.MetricMaxLength
	ret.traceMaxLengthBytes = conf.TraceMaxLengthBytes
	ret.RcvbufBytes = conf.ReadBufferSizeBytes
	ret.HTTPAddr = conf.HTTPAddress
	ret.numListeningHTTP = new(int32)

	if conf.TLSKey.Value != "" {
		if conf.TLSCertificate == "" {
			err = errors.New("tls_key is set; must set tls_certificate")
			logger.WithError(err).Error("Improper TLS configuration")
			return ret, err
		}

		// load the TLS key and certificate
		var cert tls.Certificate
		cert, err = tls.X509KeyPair([]byte(conf.TLSCertificate), []byte(conf.TLSKey.Value))
		if err != nil {
			logger.WithError(err).Error("Improper TLS configuration")
			return ret, err
		}

		clientAuthMode := tls.NoClientCert
		var clientCAs *x509.CertPool
		if conf.TLSAuthorityCertificate != "" {
			// load the authority; require clients to present certificated signed by this authority
			clientAuthMode = tls.RequireAndVerifyClientCert
			clientCAs = x509.NewCertPool()
			ok := clientCAs.AppendCertsFromPEM([]byte(conf.TLSAuthorityCertificate))
			if !ok {
				err = errors.New("tls_authority_certificate: Could not load any certificates")
				logger.WithError(err).Error("Improper TLS configuration")
				return ret, err
			}
		}

		ret.tlsConfig = &tls.Config{
			Certificates: []tls.Certificate{cert},
			ClientAuth:   clientAuthMode,
			ClientCAs:    clientCAs,
		}
	}

	// Configure tracing sinks if we are listening for ssf
	if len(conf.SsfListenAddresses) > 0 || len(conf.GrpcListenAddresses) > 0 {

		trace.Enable()

		// configure Lightstep as a Span Sink
		if conf.LightstepAccessToken.Value != "" {

			var lsSink sinks.SpanSink
			lsSink, err = lightstep.NewLightStepSpanSink(
				conf.LightstepCollectorHost, conf.LightstepReconnectPeriod,
				conf.LightstepMaximumSpans, conf.LightstepNumClients,
				conf.LightstepAccessToken.Value, log,
			)
			if err != nil {
				return ret, err
			}
			ret.spanSinks = append(ret.spanSinks, lsSink)

			logger.Info("Configured Lightstep span sink")
		}

		if conf.FalconerAddress != "" {
			falsink, err := falconer.NewSpanSink(context.Background(), conf.FalconerAddress, log, grpc.WithInsecure())
			if err != nil {
				return ret, err
			}

			ret.spanSinks = append(ret.spanSinks, falsink)
			logger.Info("Configured Falconer trace sink")
		}

		// Set up as many span workers as we need:
		ret.SpanWorkerGoroutines = 1
		if conf.NumSpanWorkers > 0 {
			ret.SpanWorkerGoroutines = conf.NumSpanWorkers
		}
	}

	customMetricSinks, err :=
		ret.createMetricSinks(logger, &conf, config.MetricSinkTypes)
	if err != nil {
		return nil, err
	}
	if conf.Features.MigrateMetricSinks {
		ret.metricSinks = customMetricSinks
	} else {
		ret.metricSinks = append(ret.metricSinks, customMetricSinks...)
	}
	customSpanSinks, err :=
		ret.createSpanSinks(logger, &conf, config.SpanSinkTypes)
	if err != nil {
		return nil, err
	}
	if conf.Features.MigrateSpanSinks {
		ret.spanSinks = customSpanSinks
	} else {
		ret.spanSinks = append(ret.spanSinks, customSpanSinks...)
	}

	// After all sinks are initialized, set the list of tags to exclude
	setSinkExcludedTags(conf.TagsExclude, ret.metricSinks, ret.spanSinks)

	// closed in Shutdown; Same approach and http.Shutdown
	ret.shutdown = make(chan struct{})
	if conf.HTTPQuit {
		logger.WithField("endpoint", httpQuitEndpoint).Info("Enabling graceful shutdown endpoint (via HTTP POST request)")
		ret.httpQuit = true
	}

	ret.forwardUseGRPC = conf.ForwardUseGrpc

	// Setup the grpc server if it was configured
	ret.grpcListenAddress = conf.GrpcAddress
	if ret.grpcListenAddress != "" {
		// convert all the workers to the proper interface
		ingesters := make([]importsrv.MetricIngester, len(ret.Workers))
		for i, worker := range ret.Workers {
			ingesters[i] = worker
		}

		ret.grpcServer = importsrv.New(ingesters,
			importsrv.WithTraceClient(ret.TraceClient))
	}

	ret.sources, err = ret.createSources(logger, &conf, config.SourceTypes)
	if err != nil {
		return nil, err
	}

	// If this is a global veneur then initialize the listening per protocol metrics
	if !ret.IsLocal() {
		ret.listeningPerProtocolMetrics = &GlobalListeningPerProtocolMetrics{
			dogstatsdTcpReceivedTotal:  0,
			dogstatsdUdpReceivedTotal:  0,
			dogstatsdUnixReceivedTotal: 0,
			dogstatsdGrpcReceivedTotal: 0,
			ssfUdpReceivedTotal:        0,
			ssfUnixReceivedTotal:       0,
			ssfGrpcReceivedTotal:       0,
		}
		log.Info("Tracking listening per protocol metrics on global instance")
	}

	logger.WithField("config", conf).Debug("Initialized server")

	return ret, err
}

// Start spins up the Server to do actual work, firing off goroutines for
// various workers and utilities.
func (s *Server) Start() {
	log.WithField("version", VERSION).Info("Starting server")

	// Set up the processors for spans:

	// Use the pre-allocated Workers slice to know how many to start.
	s.SpanWorker = NewSpanWorker(s.spanSinks, s.TraceClient, s.Statsd, s.SpanChan, s.TagsAsMap)

	go func() {
		log.Info("Starting Event worker")
		defer func() {
			ConsumePanic(s.TraceClient, s.Hostname, recover())
		}()
		s.EventWorker.Work()
	}()

	log.WithField("n", s.SpanWorkerGoroutines).Info("Starting span workers")
	for i := 0; i < s.SpanWorkerGoroutines; i++ {
		go func() {
			defer func() {
				ConsumePanic(s.TraceClient, s.Hostname, recover())
			}()
			s.SpanWorker.Work()
		}()
	}

	statsdPool := &sync.Pool{
		// We +1 this so we an "detect" when someone sends us too long of a metric!
		New: func() interface{} {
			return make([]byte, s.metricMaxLength+1)
		},
	}

	tracePool := &sync.Pool{
		New: func() interface{} {
			return make([]byte, s.traceMaxLengthBytes)
		},
	}

	for _, sink := range s.spanSinks {
		logrus.WithField("sink", sink.Name()).Info("Starting span sink")
		if err := sink.Start(s.TraceClient); err != nil {
			logrus.WithError(err).WithField("sink", sink).Panic("Error starting span sink")
		}
	}

	for _, sink := range s.metricSinks {
		logrus.WithField("sink", sink.Name()).Info("Starting metric sink")
		if err := sink.Start(s.TraceClient); err != nil {
			logrus.WithError(err).WithField("sink", sink).Fatal("Error starting metric sink")
		}
	}

	// Read Metrics Forever!
	concreteAddrs := make([]net.Addr, 0, len(s.StatsdListenAddrs))
	for _, addr := range s.StatsdListenAddrs {
		concreteAddrs = append(concreteAddrs, StartStatsd(s, addr, statsdPool))
	}
	s.StatsdListenAddrs = concreteAddrs

	// Read Traces Forever!
	if len(s.SSFListenAddrs) > 0 {
		concreteAddrs := make([]net.Addr, 0, len(s.StatsdListenAddrs))
		for _, addr := range s.SSFListenAddrs {
			concreteAddrs = append(concreteAddrs, StartSSF(s, addr, tracePool))
		}
		s.SSFListenAddrs = concreteAddrs
	} else {
		logrus.Info("Tracing sockets are not configured - not reading trace socket")
	}

	// Read grpc traces forever!
	if len(s.GRPCListenAddrs) > 0 {
		concreteAddrs := make([]net.Addr, 0, len(s.GRPCListenAddrs))
		for _, addr := range s.GRPCListenAddrs {
			concreteAddrs = append(concreteAddrs, StartGRPC(s, addr))
		}
		//If there are already ssf listen addresses then append the grpc ones otherwise just use the grpc ones
		if len(s.SSFListenAddrs) > 0 {
			s.SSFListenAddrs = append(s.SSFListenAddrs, concreteAddrs...)
		} else {
			s.SSFListenAddrs = concreteAddrs
		}

		//If there are already statsd listen addresses then append the grpc ones otherwise just use the grpc ones
		if len(s.StatsdListenAddrs) > 0 {
			s.StatsdListenAddrs = append(s.StatsdListenAddrs, concreteAddrs...)
		} else {
			s.StatsdListenAddrs = concreteAddrs
		}
	} else {
		logrus.Info("GRPC tracing sockets are not configured - not reading trace socket")
	}

	// Initialize a gRPC connection for forwarding
	if s.forwardUseGRPC {
		var err error
		s.grpcForwardConn, err = grpc.Dial(s.ForwardAddr, grpc.WithInsecure())
		if err != nil {
			log.WithError(err).WithFields(logrus.Fields{
				"forwardAddr": s.ForwardAddr,
			}).Fatal("Failed to initialize a gRPC connection for forwarding")
		}
	}

	// Flush every Interval forever!
	go func() {
		defer func() {
			ConsumePanic(s.TraceClient, s.Hostname, recover())
		}()

		ctx, cancel := context.WithCancel(context.Background())
		go func() {
			// If the server is shutting down, cancel any in-flight flush:
			<-s.shutdown
			cancel()
		}()

		if s.synchronizeInterval {
			// We want to align our ticker to a multiple of its duration for
			// convenience of bucketing.
			<-time.After(CalculateTickDelay(s.Interval, time.Now()))
		}

		// We aligned the ticker to our interval above. It's worth noting that just
		// because we aligned once we're not guaranteed to be perfect on each
		// subsequent tick. This code is small, however, and should service the
		// incoming tick signal fast enough that the amount we are "off" is
		// negligible.
		ticker := time.NewTicker(s.Interval)
		for {
			select {
			case <-s.shutdown:
				// stop flushing on graceful shutdown
				ticker.Stop()
				return
			case triggered := <-ticker.C:
				ctx, cancel := context.WithDeadline(ctx, triggered.Add(s.Interval))
				s.Flush(ctx)
				cancel()
			}
		}
	}()
}

// FlushWatchdog periodically checks that at most
// `flush_watchdog_missed_flushes` were skipped in a Server. If more
// than that number was skipped, it panics (assuming that flushing is
// stuck) with a full level of detail on that panic's backtraces.
//
// It never terminates, so is ideally run from a goroutine in a
// program's main function.
func (s *Server) FlushWatchdog() {
	defer func() {
		ConsumePanic(s.TraceClient, s.Hostname, recover())
	}()

	if s.stuckIntervals == 0 {
		// No watchdog needed:
		return
	}
	atomic.StoreInt64(&s.lastFlushUnix, time.Now().UnixNano())

	ticker := time.NewTicker(s.Interval)
	for {
		select {
		case <-s.shutdown:
			ticker.Stop()
			return
		case <-ticker.C:
			last := time.Unix(0, atomic.LoadInt64(&s.lastFlushUnix))
			since := time.Since(last)

			// If no flush was kicked off in the last N
			// times, we're stuck - panic because that's a
			// bug.
			if since > time.Duration(s.stuckIntervals)*s.Interval {
				rtdebug.SetTraceback("all")
				log.WithFields(logrus.Fields{
					"last_flush":       last,
					"missed_intervals": s.stuckIntervals,
					"time_since":       since,
				}).
					Panic("Flushing seems to be stuck. Terminating.")
			}
		}
	}
}

// Increment internal metrics keeping track of protocols received when listening
func incrementListeningProtocol(s *Server, protocol ProtocolType) {
	metricsStruct := s.listeningPerProtocolMetrics
	if metricsStruct != nil {
		switch protocol {
		case DOGSTATSD_TCP:
			atomic.AddInt64(&metricsStruct.dogstatsdTcpReceivedTotal, 1)
		case DOGSTATSD_UDP:
			atomic.AddInt64(&metricsStruct.dogstatsdUdpReceivedTotal, 1)
		case DOGSTATSD_UNIX:
			atomic.AddInt64(&metricsStruct.dogstatsdUnixReceivedTotal, 1)
		case DOGSTATSD_GRPC:
			atomic.AddInt64(&metricsStruct.dogstatsdGrpcReceivedTotal, 1)
		case SSF_UDP:
			atomic.AddInt64(&metricsStruct.ssfUdpReceivedTotal, 1)
		case SSF_UNIX:
			atomic.AddInt64(&metricsStruct.ssfUnixReceivedTotal, 1)
		case SSF_GRPC:
			atomic.AddInt64(&metricsStruct.ssfGrpcReceivedTotal, 1)
		default: //If it is an unrecognized protocol then don't increment anything
			logrus.WithField("protocol", protocol).
				Warning("Attempted to increment metrics for unrecognized protocol")
		}
	}
}

// HandleMetricPacket processes each packet that is sent to the server, and sends to an
// appropriate worker (EventWorker or Worker).
func (s *Server) HandleMetricPacket(packet []byte, protocolType ProtocolType) error {
	// This is a very performance-sensitive function
	// and packets may be dropped if it gets slowed down.
	// Keep that in mind when modifying!

	if len(packet) == 0 {
		// a lot of clients send packets that accidentally have a trailing
		// newline, it's easier to just let them be
		return nil
	}
	samples := &ssf.Samples{}
	defer metrics.Report(s.TraceClient, samples)

	if !s.IsLocal() {
		incrementListeningProtocol(s, protocolType)
	}

	if bytes.HasPrefix(packet, []byte{'_', 'e', '{'}) {
		event, err := s.parser.ParseEvent(packet)
		if err != nil {
			log.WithFields(logrus.Fields{
				logrus.ErrorKey: err,
				"packet":        string(packet),
			}).Debug("Could not parse packet")
			samples.Add(ssf.Count("packet.error_total", 1, map[string]string{"packet_type": "event", "reason": "parse"}))
			return err
		}
		s.EventWorker.sampleChan <- *event
	} else if bytes.HasPrefix(packet, []byte{'_', 's', 'c'}) {
		svcheck, err := s.parser.ParseServiceCheck(packet)
		if err != nil {
			log.WithFields(logrus.Fields{
				logrus.ErrorKey: err,
				"packet":        string(packet),
			}).Debug("Could not parse packet")
			samples.Add(ssf.Count("packet.error_total", 1, map[string]string{"packet_type": "service_check", "reason": "parse"}))
			return err
		}
		s.Workers[svcheck.Digest%uint32(len(s.Workers))].PacketChan <- *svcheck
	} else {
		metric, err := s.parser.ParseMetric(packet)
		if err != nil {
			log.WithFields(logrus.Fields{
				logrus.ErrorKey: err,
				"packet":        string(packet),
			}).Debug("Could not parse packet")
			samples.Add(ssf.Count("packet.error_total", 1, map[string]string{"packet_type": "metric", "reason": "parse"}))
			return err
		}
		s.Workers[metric.Digest%uint32(len(s.Workers))].PacketChan <- *metric
	}
	return nil
}

// HandleTracePacket accepts an incoming packet as bytes and sends it to the
// appropriate worker.
func (s *Server) HandleTracePacket(packet []byte, protocolType ProtocolType) {
	samples := &ssf.Samples{}
	defer metrics.Report(s.TraceClient, samples)

	// Unlike metrics, protobuf shouldn't have an issue with 0-length packets
	if len(packet) == 0 {
		s.Statsd.Count("ssf.error_total", 1, []string{"ssf_format:packet", "packet_type:unknown", "reason:zerolength"}, 1.0)
		log.Warn("received zero-length trace packet")
		return
	}

	s.Statsd.Histogram("ssf.packet_size", float64(len(packet)), nil, .1)

	span, err := protocol.ParseSSF(packet)
	if err != nil {
		reason := "reason:" + err.Error()
		s.Statsd.Count("ssf.error_total", 1, []string{"ssf_format:packet", "packet_type:ssf_metric", reason}, 1.0)
		log.WithError(err).Warn("ParseSSF")
		return
	}
	// we want to keep track of this, because it's a client problem, but still
	// handle the span normally
	if span.Id == 0 {
		reason := "reason:" + "empty_id"
		s.Statsd.Count("ssf.error_total", 1, []string{"ssf_format:packet", "packet_type:ssf_metric", reason}, 1.0)
		log.Warn("HandleTracePacket: Span ID is zero")
	}

	s.handleSSF(span, "packet", protocolType)
}

func (s *Server) handleSSF(span *ssf.SSFSpan, ssfFormat string, protocolType ProtocolType) {
	// 1/internalMetricSampleRate packets will be chosen
	const internalMetricSampleRate = 1000

	key := "service:" + span.Service + "," + "ssf_format:" + ssfFormat

	if (span.Id % internalMetricSampleRate) == 1 {
		// we can't avoid emitting this metric synchronously by aggregating in-memory, but that's okay
		s.Statsd.Histogram("ssf.spans.tags_per_span", float64(len(span.Tags)), []string{"service:" + span.Service, "ssf_format:" + ssfFormat}, 1)
	}

	metrics, ok := s.ssfInternalMetrics.Load(key)

	if !ok {
		// ensure that the value is in the map
		// we only do this if the value was not found in the map once already, to save an
		// allocation and more expensive operation in the typical case
		metrics, ok = s.ssfInternalMetrics.LoadOrStore(key, &ssfServiceSpanMetrics{})
		if metrics == nil {
			log.WithFields(logrus.Fields{
				"service":   span.Service,
				"ssfFormat": ssfFormat,
			}).Error("found nil value in ssfInternalMetrics map")
		}
	}

	metricsStruct, ok := metrics.(*ssfServiceSpanMetrics)
	if !ok {
		log.WithField("type", reflect.TypeOf(metrics)).Error()
		log.WithFields(logrus.Fields{
			"type":      reflect.TypeOf(metrics),
			"service":   span.Service,
			"ssfFormat": ssfFormat,
		}).Error("Unexpected type in ssfInternalMetrics map")
	}

	atomic.AddInt64(&metricsStruct.ssfSpansReceivedTotal, 1)

	if span.Id == span.TraceId {
		atomic.AddInt64(&metricsStruct.ssfRootSpansReceivedTotal, 1)
	}

	if !s.IsLocal() {
		incrementListeningProtocol(s, protocolType)
	}

	s.SpanChan <- span
}

// ReadMetricSocket listens for available packets to handle.
func (s *Server) ReadMetricSocket(serverConn net.PacketConn, packetPool *sync.Pool) {
	for {
		buf := packetPool.Get().([]byte)
		n, _, err := serverConn.ReadFrom(buf)
		if err != nil {
			log.WithError(err).Error("Error reading from UDP metrics socket")
			continue
		}
		s.processMetricPacket(n, buf, packetPool, DOGSTATSD_UDP)
	}
}

// Splits the read metric packet into multiple metrics and handles them
func (s *Server) processMetricPacket(numBytes int, buf []byte, packetPool *sync.Pool, protocolType ProtocolType) {
	if numBytes > s.metricMaxLength {
		metrics.ReportOne(s.TraceClient, ssf.Count("packet.error_total", 1, map[string]string{"packet_type": "unknown", "reason": "toolong"}))
		return
	}

	// statsd allows multiple packets to be joined by newlines and sent as
	// one larger packet
	// note that spurious newlines are not allowed in this format, it has
	// to be exactly one newline between each packet, with no leading or
	// trailing newlines
	splitPacket := samplers.NewSplitBytes(buf[:numBytes], '\n')
	for splitPacket.Next() {
		s.HandleMetricPacket(splitPacket.Chunk(), protocolType)
	}

	//Only return to the pool if there is a pool
	if packetPool != nil {
		// the Metric struct created by HandleMetricPacket has no byte slices in it,
		// only strings
		// therefore there are no outstanding references to this byte slice, we
		// can return it to the pool
		packetPool.Put(buf)
	}
}

// ReadStatsdDatagramSocket reads statsd metrics packets from connection off a unix datagram socket.
func (s *Server) ReadStatsdDatagramSocket(serverConn *net.UnixConn, packetPool *sync.Pool) {
	for {
		buf := packetPool.Get().([]byte)
		n, _, err := serverConn.ReadFromUnix(buf)
		if err != nil {
			select {
			case <-s.shutdown:
				log.WithError(err).Info("Ignoring ReadFrom error while shutting down")
				return
			default:
				log.WithError(err).Error("Error reading packet from Unix domain socket")
				continue
			}
		}

		s.processMetricPacket(n, buf, packetPool, DOGSTATSD_UNIX)
	}
}

// ReadSSFPacketSocket reads SSF packets off a packet connection.
func (s *Server) ReadSSFPacketSocket(serverConn net.PacketConn, packetPool *sync.Pool) {
	// TODO This is duplicated from ReadMetricSocket and feels like it could be it's
	// own function?
	p := packetPool.Get().([]byte)
	if len(p) == 0 {
		log.WithField("len", len(p)).Fatal(
			"packetPool making empty slices: trace_max_length_bytes must be >= 0")
	}
	packetPool.Put(p)

	for {
		buf := packetPool.Get().([]byte)
		n, _, err := serverConn.ReadFrom(buf)
		if err != nil {
			// In tests, the probably-best way to
			// terminate this reader is to issue a shutdown and close the listening
			// socket, which returns an error, so let's handle it here:
			select {
			case <-s.shutdown:
				log.WithError(err).Info("Ignoring ReadFrom error while shutting down")
				return
			default:
				log.WithError(err).Error("Error reading from UDP trace socket")
				continue
			}
		}

		s.HandleTracePacket(buf[:n], SSF_UDP)
		packetPool.Put(buf)
	}
}

// ReadSSFStreamSocket reads a streaming connection in framed wire format
// off a streaming socket. See package
// github.com/stripe/veneur/v14/protocol for details.
func (s *Server) ReadSSFStreamSocket(serverConn net.Conn) {
	defer func() {
		serverConn.Close()
	}()

	// initialize the capacity to the max size
	// based on the number of tags we add later
	tags := make([]string, 1, 3)
	tags[0] = "ssf_format:framed"

	for {
		msg, err := protocol.ReadSSF(serverConn)
		if err != nil {
			if err == io.EOF {
				// Client hangup, close this
				s.Statsd.Count("frames.disconnects", 1, nil, 1.0)
				return
			}
			if protocol.IsFramingError(err) {
				log.WithError(err).
					WithField("remote", serverConn.RemoteAddr()).
					Info("Frame error reading from SSF connection. Closing.")
				tags = append(tags, []string{"packet_type:unknown", "reason:framing"}...)
				s.Statsd.Incr("ssf.error_total", tags, 1.0)
				return
			}
			// Non-frame errors means we can continue reading:
			log.WithError(err).
				WithField("remote", serverConn.RemoteAddr()).
				Error("Error processing an SSF frame")
			tags = append(tags, []string{"packet_type:unknown", "reason:processing"}...)
			s.Statsd.Incr("ssf.error_total", tags, 1.0)
			tags = tags[:1]
			continue
		}
		s.handleSSF(msg, "framed", SSF_UNIX)
	}
}

func (s *Server) handleTCPGoroutine(conn net.Conn) {
	defer func() {
		ConsumePanic(s.TraceClient, s.Hostname, recover())
	}()

	defer func() {
		log.WithField("peer", conn.RemoteAddr()).Debug("Closing TCP connection")
		err := conn.Close()
		metrics.ReportOne(s.TraceClient, ssf.Count("tcp.disconnects", 1, nil))
		if err != nil {
			// most often "write: broken pipe"; not really an error
			log.WithFields(logrus.Fields{
				logrus.ErrorKey: err,
				"peer":          conn.RemoteAddr(),
			}).Info("TCP close failed")
		}
	}()
	metrics.ReportOne(s.TraceClient, ssf.Count("tcp.connects", 1, nil))

	// time out idle connections to prevent leaking memory/goroutines
	timeout := defaultTCPReadTimeout
	if s.tcpReadTimeout != 0 {
		timeout = s.tcpReadTimeout
	}

	if tlsConn, ok := conn.(*tls.Conn); ok {
		// complete the handshake to verify the certificate
		conn.SetReadDeadline(time.Now().Add(timeout))
		err := tlsConn.Handshake()
		if err != nil {
			// usually io.EOF or "read: connection reset by peer"; not really errors
			// it can also be caused by certificate authentication problems
			metrics.ReportOne(s.TraceClient, ssf.Count("tcp.tls_handshake_failures", 1, nil))
			log.WithFields(logrus.Fields{
				logrus.ErrorKey: err,
				"peer":          conn.RemoteAddr(),
			}).Info("TLS Handshake failed")

			return
		}

		state := tlsConn.ConnectionState()
		var clientCert pkix.RDNSequence
		if len(state.PeerCertificates) > 0 {
			clientCert = state.PeerCertificates[0].Subject.ToRDNSequence()
		}
		log.WithFields(logrus.Fields{
			"peer":        conn.RemoteAddr(),
			"client_cert": clientCert,
		}).Debug("Starting TLS connection")
	} else {
		log.WithFields(logrus.Fields{
			"peer": conn.RemoteAddr(),
		}).Debug("Starting TCP connection")
	}

	// Scanner is nearly the same performance as a custom implementation
	buf := bufio.NewScanner(conn)

	scanWithDeadline := func() bool {
		conn.SetReadDeadline(time.Now().Add(timeout))
		return buf.Scan()
	}
	for scanWithDeadline() {
		// treat each line as a separate packet
		err := s.HandleMetricPacket(buf.Bytes(), DOGSTATSD_TCP)
		if err != nil {
			// don't consume bad data from a client indefinitely
			// HandleMetricPacket logs the err and packet, and increments error counters
			log.WithField("peer", conn.RemoteAddr()).Warn(
				"Error parsing packet; closing TCP connection")
			return
		}
	}
	if buf.Err() != nil {
		// usually "read: connection reset by peer" or "i/o timeout"
		log.WithFields(logrus.Fields{
			logrus.ErrorKey: buf.Err(),
			"peer":          conn.RemoteAddr(),
		}).Info("Error reading from TCP client")
	}
}

// ReadTCPSocket listens on Server.TCPAddr for new connections, starting a goroutine for each.
func (s *Server) ReadTCPSocket(listener net.Listener) {
	for {
		conn, err := listener.Accept()
		if err != nil {
			select {
			case <-s.shutdown:
				// occurs when cleanly shutting down the server e.g. in tests; ignore errors
				log.WithError(err).Info("Ignoring Accept error while shutting down")
				return
			default:
				log.WithError(err).Fatal("TCP accept failed")
			}
		}

		go s.handleTCPGoroutine(conn)
	}
}

// Start all of the the configured servers (gRPC or HTTP) and block until
// one of them exist.  At that point, stop them both.
func (s *Server) Serve() {
	done := make(chan struct{})

	if s.HTTPAddr != "" {
		go func() {
			s.HTTPServe()
			done <- struct{}{}
		}()
	}

	for _, source := range s.sources {
		go func(source sources.Source) {
			source.Start()
			done <- struct{}{}
		}(source)
	}

	if s.grpcListenAddress != "" {
		go func() {
			s.gRPCServe()
			done <- struct{}{}
		}()
	}

	// wait until at least one of the servers has shut down
	<-done
	graceful.Shutdown()
	for _, source := range s.sources {
		source.Stop()
	}
	s.gRPCStop()
}

// HTTPServe starts the HTTP server and listens perpetually until it encounters an unrecoverable error.
func (s *Server) HTTPServe() {
	var prf interface {
		Stop()
	}

	// We want to make sure the profile is stopped
	// exactly once (and only once), even if the
	// shutdown pre-hook does not run (which it may not)
	profileStopOnce := sync.Once{}

	if s.enableProfiling {
		profileStartOnce.Do(func() {
			prf = profile.Start()
		})

		defer func() {
			profileStopOnce.Do(prf.Stop)
		}()
	}
	httpSocket := bind.Socket(s.HTTPAddr)
	graceful.Timeout(10 * time.Second)
	graceful.PreHook(func() {

		if prf != nil {
			profileStopOnce.Do(prf.Stop)
		}

		log.Info("Terminating HTTP listener")
	})

	// Ensure that the server responds to SIGUSR2 even
	// when *not* running under einhorn.
	graceful.AddSignal(syscall.SIGUSR2, syscall.SIGHUP)
	graceful.HandleSignals()
	gracefulSocket := graceful.WrapListener(httpSocket)
	log.WithField("address", s.HTTPAddr).Info("HTTP server listening")

	// Signal that the HTTP server is starting
	atomic.AddInt32(s.numListeningHTTP, 1)
	defer atomic.AddInt32(s.numListeningHTTP, -1)
	bind.Ready()

	if err := http.Serve(gracefulSocket, s.Handler()); err != nil {
		log.WithError(err).Error("HTTP server shut down due to error")
	}
	log.Info("Stopped HTTP server")

	graceful.Shutdown()
}

// gRPCServe starts the gRPC server and blocks until an error is encountered,
// or the server is shutdown.
//
// TODO this doesn't handle SIGUSR2 and SIGHUP on it's own, unlike HTTPServe
// As long as both are running this is actually fine, as Serve will stop
// the gRPC server when the HTTP one exits.  When running just gRPC however,
// the signal handling won't work.
func (s *Server) gRPCServe() {
	entry := log.WithFields(logrus.Fields{"address": s.grpcListenAddress})
	entry.Info("Starting gRPC server")
	if err := s.grpcServer.Serve(s.grpcListenAddress); err != nil {
		entry.WithError(err).Error("gRPC server was not shut down cleanly")
	}

	entry.Info("Stopped gRPC server")
}

// Try to perform a graceful stop of the gRPC server.  If it takes more than
// 10 seconds, timeout and force-stop.
func (s *Server) gRPCStop() {
	if s.grpcServer == nil {
		return
	}

	done := make(chan struct{})
	defer close(done)
	go func() {
		s.grpcServer.GracefulStop()
		done <- struct{}{}
	}()

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		log.Info("Force-stopping the gRPC server after waiting for a graceful shutdown")
		s.grpcServer.Stop()
	}
}

// Shutdown signals the server to shut down after closing all
// current connections.
func (s *Server) Shutdown() {
	// TODO(aditya) shut down workers and socket readers
	log.Info("Shutting down server gracefully")
	close(s.shutdown)
	graceful.Shutdown()
	for _, source := range s.sources {
		source.Stop()
	}
	s.gRPCStop()

	// Close the gRPC connection for forwarding
	if s.grpcForwardConn != nil {
		s.grpcForwardConn.Close()
	}
}

// IsLocal indicates whether veneur is running as a local instance
// (forwarding non-local data to a global veneur instance) or is running as a global
// instance (sending all data directly to the final destination).
func (s *Server) IsLocal() bool {
	return s.ForwardAddr != ""
}

// IsListeningHTTP returns if the Server is currently listening over HTTP
func (s *Server) IsListeningHTTP() bool {
	return atomic.LoadInt32(s.numListeningHTTP) > 0
}

// CalculateTickDelay takes the provided time, `Truncate`s it a rounded-down
// multiple of `interval`, then adds `interval` back to find the "next" tick.
func CalculateTickDelay(interval time.Duration, t time.Time) time.Duration {
	return t.Truncate(interval).Add(interval).Sub(t)
}

// Set the list of tags to exclude on each sink
func setSinkExcludedTags(excludeRules []string, metricSinks []sinks.MetricSink, spanSinks []sinks.SpanSink) {
	type excludableSink interface {
		SetExcludedTags([]string)
	}

	for _, sink := range metricSinks {
		if s, ok := sink.(excludableSink); ok {
			excludedTags := generateExcludedTags(excludeRules, sink.Name())
			log.WithFields(logrus.Fields{
				"sink":         sink.Name(),
				"excludedTags": excludedTags,
			}).Info("Setting excluded tags on metric sink")
			s.SetExcludedTags(excludedTags)
		}
	}

	for _, sink := range spanSinks {
		if s, ok := sink.(excludableSink); ok {
			excludedTags := generateExcludedTags(excludeRules, sink.Name())
			log.WithFields(logrus.Fields{
				"sink":         sink.Name(),
				"excludedTags": excludedTags,
			}).Info("Setting excluded tags on span sink")
			s.SetExcludedTags(excludedTags)
		}
	}
}

func generateExcludedTags(excludeRules []string, sinkName string) []string {
	excludedTags := make([]string, 0, len(excludeRules))
	for _, rule := range excludeRules {
		splt := strings.Split(rule, "|")
		if len(splt) == 1 {
			excludedTags = append(excludedTags, splt[0])
			continue
		}
		for _, name := range splt[1:] {
			if name == sinkName {
				excludedTags = append(excludedTags, splt[0])
			}
		}
	}
	return excludedTags
}
