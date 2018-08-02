package veneur

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/pprof"
	"os"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"golang.org/x/net/context"

	"github.com/DataDog/datadog-go/statsd"
	raven "github.com/getsentry/raven-go"
	"github.com/hashicorp/consul/api"
	"github.com/pkg/profile"
	"github.com/sirupsen/logrus"
	vhttp "github.com/stripe/veneur/http"
	"github.com/stripe/veneur/samplers"
	"github.com/stripe/veneur/ssf"
	"github.com/stripe/veneur/trace"
	"github.com/stripe/veneur/trace/metrics"
	"github.com/zenazn/goji/bind"
	"github.com/zenazn/goji/graceful"
	"stathat.com/c/consistent"

	"goji.io"
	"goji.io/pat"
)

type Proxy struct {
	Sentry                 *raven.Client
	Hostname               string
	ForwardDestinations    *consistent.Consistent
	TraceDestinations      *consistent.Consistent
	Discoverer             Discoverer
	ConsulForwardService   string
	ConsulTraceService     string
	ConsulInterval         time.Duration
	MetricsInterval        time.Duration
	ForwardDestinationsMtx sync.Mutex
	TraceDestinationsMtx   sync.Mutex
	HTTPAddr               string
	HTTPClient             *http.Client
	AcceptingForwards      bool
	AcceptingTraces        bool
	ForwardTimeout         time.Duration

	usingConsul     bool
	usingKubernetes bool
	enableProfiling bool
	shutdown        chan struct{}
	TraceClient     *trace.Client
}

func NewProxyFromConfig(logger *logrus.Logger, conf ProxyConfig) (p Proxy, err error) {

	hostname, err := os.Hostname()
	if err != nil {
		logger.WithError(err).Error("Error finding hostname")
		return
	}
	p.Hostname = hostname
	p.shutdown = make(chan struct{})

	logger.AddHook(sentryHook{
		c:        p.Sentry,
		hostname: hostname,
		lv: []logrus.Level{
			logrus.ErrorLevel,
			logrus.FatalLevel,
			logrus.PanicLevel,
		},
	})

	p.HTTPAddr = conf.HTTPAddress

	var idleTimeout time.Duration
	if conf.IdleConnectionTimeout != "" {
		idleTimeout, err = time.ParseDuration(conf.IdleConnectionTimeout)
		if err != nil {
			return p, err
		}
	}
	transport := &http.Transport{
		IdleConnTimeout: idleTimeout,
		// Each of these properties DTRT (according to Go docs) when supplied with
		// zero values as of Go 0.10.3
		MaxIdleConns:        conf.MaxIdleConns,
		MaxIdleConnsPerHost: conf.MaxIdleConnsPerHost,
	}

	p.HTTPClient = &http.Client{
		Transport: transport,
	}

	p.enableProfiling = conf.EnableProfiling

	p.ConsulForwardService = conf.ConsulForwardServiceName
	p.ConsulTraceService = conf.ConsulTraceServiceName

	if p.ConsulForwardService != "" || conf.ForwardAddress != "" {
		p.AcceptingForwards = true
	}
	if p.ConsulTraceService != "" || conf.TraceAddress != "" {
		p.AcceptingTraces = true
	}

	// We need a convenient way to know if we're even using Consul later
	if p.ConsulForwardService != "" || p.ConsulTraceService != "" {
		log.WithFields(logrus.Fields{
			"consulForwardService": p.ConsulForwardService,
			"consulTraceService":   p.ConsulTraceService,
		}).Info("Using consul for service discovery")
		p.usingConsul = true
	}

	// check if we are running on Kubernetes
	if _, err := os.Stat("/var/run/secrets/kubernetes.io/serviceaccount"); !os.IsNotExist(err) {
		log.Info("Using Kubernetes for service discovery")
		p.usingKubernetes = true

		//TODO don't overload this
		if conf.ConsulForwardServiceName != "" {
			p.AcceptingForwards = true
		}
	}

	p.ForwardDestinations = consistent.New()
	p.TraceDestinations = consistent.New()

	if conf.ForwardTimeout != "" {
		p.ForwardTimeout, err = time.ParseDuration(conf.ForwardTimeout)
		if err != nil {
			logger.WithError(err).
				WithField("value", conf.ForwardTimeout).
				Error("Could not parse forward timeout")
			return
		}
	}

	// We got a static forward address, stick it in the destination!
	if p.ConsulForwardService == "" && conf.ForwardAddress != "" {
		p.ForwardDestinations.Add(conf.ForwardAddress)
	}
	if p.ConsulTraceService == "" && conf.TraceAddress != "" {
		p.TraceDestinations.Add(conf.TraceAddress)
	}

	if !p.AcceptingForwards && !p.AcceptingTraces {
		err = errors.New("refusing to start with no Consul service names or static addresses in config")
		logger.WithError(err).WithFields(logrus.Fields{
			"consul_forward_service_name": p.ConsulForwardService,
			"consul_trace_service_name":   p.ConsulTraceService,
			"forward_address":             conf.ForwardAddress,
			"trace_address":               conf.TraceAddress,
		}).Error("Oops")
		return
	}

	if p.usingConsul {
		p.ConsulInterval, err = time.ParseDuration(conf.ConsulRefreshInterval)
		if err != nil {
			logger.WithError(err).Error("Error parsing Consul refresh interval")
			return
		}
		logger.WithField("interval", conf.ConsulRefreshInterval).Info("Will use Consul for service discovery")
	}

	p.MetricsInterval = time.Second * 10
	if conf.RuntimeMetricsInterval != "" {
		p.MetricsInterval, err = time.ParseDuration(conf.RuntimeMetricsInterval)
		if err != nil {
			logger.WithError(err).Error("Error parsing metric refresh interval")
			return
		}
	}

	p.TraceClient = trace.DefaultClient
	if conf.SsfDestinationAddress != "" {
		stats, err := statsd.NewBuffered(conf.StatsAddress, 4096)
		if err != nil {
			return p, err
		}
		stats.Namespace = "veneur_proxy."
		format := "ssf_format:packet"
		if strings.HasPrefix(conf.SsfDestinationAddress, "unix://") {
			format = "ssf_format:framed"
		}

		traceFlushInterval, err := time.ParseDuration(conf.TracingClientFlushInterval)
		if err != nil {
			logger.WithError(err).Error("Error parsing tracing flush interval")
			return p, err
		}
		traceMetricsInterval, err := time.ParseDuration(conf.TracingClientMetricsInterval)
		if err != nil {
			logger.WithError(err).Error("Error parsing tracing metrics interval")
			return p, err
		}

		p.TraceClient, err = trace.NewClient(conf.SsfDestinationAddress,
			trace.Buffered,
			trace.Capacity(uint(conf.TracingClientCapacity)),
			trace.FlushInterval(traceFlushInterval),
			trace.ReportStatistics(stats, traceMetricsInterval, []string{format}),
		)
		if err != nil {
			logger.WithField("ssf_destination_address", conf.SsfDestinationAddress).
				WithError(err).
				Fatal("Error using SSF destination address")
		}
	}

	// TODO Size of replicas in config?
	//ret.ForwardDestinations.NumberOfReplicas = ???

	if conf.Debug {
		logger.SetLevel(logrus.DebugLevel)
	}

	logger.WithField("config", conf).Debug("Initialized server")

	return
}

// Start fires up the various goroutines that run on behalf of the server.
// This is separated from the constructor for testing convenience.
func (p *Proxy) Start() {
	log.WithField("version", VERSION).Info("Starting server")

	config := api.DefaultConfig()
	// Use the same HTTP Client we're using for other things, so we can leverage
	// it for testing.
	config.HttpClient = p.HTTPClient

	if p.usingKubernetes {
		disc, err := NewKubernetesDiscoverer()
		if err != nil {
			log.WithError(err).Error("Error creating KubernetesDiscoverer")
			return
		}
		p.Discoverer = disc
		log.Info("Set Kubernetes discoverer")
	} else if p.usingConsul {
		disc, consulErr := NewConsul(config)
		if consulErr != nil {
			log.WithError(consulErr).Error("Error creating Consul discoverer")
			return
		}
		p.Discoverer = disc
		log.Info("Set Consul discoverer")
	}

	if p.AcceptingForwards && p.ConsulForwardService != "" {
		p.RefreshDestinations(p.ConsulForwardService, p.ForwardDestinations, &p.ForwardDestinationsMtx)
		if len(p.ForwardDestinations.Members()) == 0 {
			log.WithField("serviceName", p.ConsulForwardService).Fatal("Refusing to start with zero destinations for forwarding.")
		}
	}

	if p.AcceptingTraces && p.ConsulTraceService != "" {
		p.RefreshDestinations(p.ConsulTraceService, p.TraceDestinations, &p.TraceDestinationsMtx)
		if len(p.ForwardDestinations.Members()) == 0 {
			log.WithField("serviceName", p.ConsulTraceService).Fatal("Refusing to start with zero destinations for tracing.")
		}
	}

	if p.usingConsul || p.usingKubernetes {
		log.Info("Creating service discovery goroutine")
		go func() {
			defer func() {
				ConsumePanic(p.Sentry, p.TraceClient, p.Hostname, recover())
			}()
			ticker := time.NewTicker(p.ConsulInterval)
			for range ticker.C {
				log.WithFields(logrus.Fields{
					"acceptingForwards":    p.AcceptingForwards,
					"consulForwardService": p.ConsulForwardService,
					"consulTraceService":   p.ConsulTraceService,
				}).Debug("About to refresh destinations")
				if p.AcceptingForwards && p.ConsulForwardService != "" {
					p.RefreshDestinations(p.ConsulForwardService, p.ForwardDestinations, &p.ForwardDestinationsMtx)
				}
				if p.AcceptingTraces && p.ConsulTraceService != "" {
					p.RefreshDestinations(p.ConsulTraceService, p.TraceDestinations, &p.TraceDestinationsMtx)
				}
			}
		}()
	}

	go func() {
		hostname, _ := os.Hostname()
		defer func() {
			ConsumePanic(p.Sentry, p.TraceClient, hostname, recover())
		}()
		ticker := time.NewTicker(p.MetricsInterval)
		for {
			select {
			case <-p.shutdown:
				// stop flushing on graceful shutdown
				ticker.Stop()
				return
			case <-ticker.C:
				p.ReportRuntimeMetrics()
			}
		}
	}()

}

// HTTPServe starts the HTTP server and listens perpetually until it encounters an unrecoverable error.
func (p *Proxy) HTTPServe() {
	var prf interface {
		Stop()
	}

	// We want to make sure the profile is stopped
	// exactly once (and only once), even if the
	// shutdown pre-hook does not run (which it may not)
	profileStopOnce := sync.Once{}

	if p.enableProfiling {
		profileStartOnce.Do(func() {
			prf = profile.Start()
		})

		defer func() {
			profileStopOnce.Do(prf.Stop)
		}()
	}
	httpSocket := bind.Socket(p.HTTPAddr)
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
	log.WithField("address", p.HTTPAddr).Info("HTTP server listening")
	bind.Ready()

	if err := graceful.Serve(httpSocket, p.Handler()); err != nil {
		log.WithError(err).Error("HTTP server shut down due to error")
	}

	graceful.Shutdown()
}

// RefreshDestinations updates the server's list of valid destinations
// for flushing. This should be called periodically to ensure we have
// the latest data.
func (p *Proxy) RefreshDestinations(serviceName string, ring *consistent.Consistent, mtx *sync.Mutex) {
	samples := &ssf.Samples{}
	defer metrics.Report(p.TraceClient, samples)
	srvTags := map[string]string{"service": serviceName}

	start := time.Now()
	destinations, err := p.Discoverer.GetDestinationsForService(serviceName)
	samples.Add(ssf.Timing("discoverer.update_duration_ns", time.Since(start), time.Nanosecond, srvTags))
	log.WithFields(logrus.Fields{
		"destinations": destinations,
		"service":      serviceName,
	}).Debug("Got destinations")

	samples.Add(ssf.Timing("discoverer.update_duration_ns", time.Since(start), time.Nanosecond, srvTags))
	if err != nil || len(destinations) == 0 {
		log.WithError(err).WithFields(logrus.Fields{
			"service":         serviceName,
			"errorType":       reflect.TypeOf(err),
			"numDestinations": len(destinations),
		}).Error("Discoverer found zero destinations and/or returned an error. Destinations may be stale!")
		samples.Add(ssf.Count("discoverer.errors", 1, srvTags))
		// Return since we got no hosts. We don't want to zero out the list. This
		// should result in us leaving the "last good" values in the ring.
		return
	}

	mtx.Lock()
	ring.Set(destinations)
	mtx.Unlock()
	samples.Add(ssf.Gauge("discoverer.destination_number", float32(len(destinations)), srvTags))
}

// Handler returns the Handler responsible for routing request processing.
func (p *Proxy) Handler() http.Handler {
	mux := goji.NewMux()

	mux.HandleFuncC(pat.Get("/healthcheck"), func(c context.Context, w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok\n"))
	})

	mux.Handle(pat.Post("/import"), handleProxy(p))

	mux.Handle(pat.Get("/debug/pprof/cmdline"), http.HandlerFunc(pprof.Cmdline))
	mux.Handle(pat.Get("/debug/pprof/profile"), http.HandlerFunc(pprof.Profile))
	mux.Handle(pat.Get("/debug/pprof/symbol"), http.HandlerFunc(pprof.Symbol))
	mux.Handle(pat.Get("/debug/pprof/trace"), http.HandlerFunc(pprof.Trace))
	// TODO match without trailing slash as well
	mux.Handle(pat.Get("/debug/pprof/*"), http.HandlerFunc(pprof.Index))

	return mux
}

func (p *Proxy) ProxyTraces(ctx context.Context, traces []DatadogTraceSpan) {
	span, _ := trace.StartSpanFromContext(ctx, "veneur.opentracing.proxy.proxy_traces")
	defer span.ClientFinish(p.TraceClient)
	if p.ForwardTimeout > 0 {
		var cancel func()
		ctx, cancel = context.WithTimeout(ctx, p.ForwardTimeout)
		defer cancel()
	}

	tracesByDestination := make(map[string][]*DatadogTraceSpan)
	for _, h := range p.TraceDestinations.Members() {
		tracesByDestination[h] = make([]*DatadogTraceSpan, 0)
	}

	for _, t := range traces {
		dest, _ := p.TraceDestinations.Get(strconv.FormatInt(t.TraceID, 10))
		tracesByDestination[dest] = append(tracesByDestination[dest], &t)
	}

	for dest, batch := range tracesByDestination {
		if len(batch) != 0 {
			// this endpoint is not documented to take an array... but it does
			// another curious constraint of this endpoint is that it does not
			// support "Content-Encoding: deflate"
			err := vhttp.PostHelper(span.Attach(ctx), p.HTTPClient, p.TraceClient, http.MethodPost, fmt.Sprintf("%s/spans", dest), batch, "flush_traces", false, nil, log)
			if err == nil {
				log.WithFields(logrus.Fields{
					"traces":      len(batch),
					"destination": dest,
				}).Debug("Completed flushing traces to Datadog")
			} else {
				log.WithFields(
					logrus.Fields{
						"traces":        len(batch),
						logrus.ErrorKey: err}).Error("Error flushing traces to Datadog")
			}
		} else {
			log.WithField("destination", dest).Info("No traces to flush, skipping.")
		}
	}
}

// ProxyMetrics takes a slice of JSONMetrics and breaks them up into
// multiple HTTP requests by MetricKey using the hash ring.
func (p *Proxy) ProxyMetrics(ctx context.Context, jsonMetrics []samplers.JSONMetric, origin string) {
	span, _ := trace.StartSpanFromContext(ctx, "veneur.opentracing.proxy.proxy_metrics")
	defer span.ClientFinish(p.TraceClient)

	if p.ForwardTimeout > 0 {
		var cancel func()
		ctx, cancel = context.WithTimeout(ctx, p.ForwardTimeout)
		defer cancel()
	}
	metricCount := len(jsonMetrics)
	span.Add(ssf.RandomlySample(0.1,
		ssf.Count("import.metrics_total", float32(metricCount), map[string]string{
			"remote_addr":      origin,
			"veneurglobalonly": "",
		}),
	)...)

	jsonMetricsByDestination := make(map[string][]samplers.JSONMetric)
	for _, h := range p.ForwardDestinations.Members() {
		jsonMetricsByDestination[h] = make([]samplers.JSONMetric, 0)
	}

	for _, jm := range jsonMetrics {
		dest, _ := p.ForwardDestinations.Get(jm.MetricKey.String())
		jsonMetricsByDestination[dest] = append(jsonMetricsByDestination[dest], jm)
	}

	// nb The response has already been returned at this point, because we
	wg := sync.WaitGroup{}
	wg.Add(len(jsonMetricsByDestination)) // Make our waitgroup the size of our destinations

	for dest, batch := range jsonMetricsByDestination {
		go p.doPost(ctx, &wg, dest, batch)
	}
	wg.Wait() // Wait for all the above goroutines to complete
	log.WithField("count", metricCount).Debug("Completed forward")

	span.Add(ssf.RandomlySample(0.1,
		ssf.Timing("proxy.duration_ns", time.Since(span.Start), time.Nanosecond, nil),
		ssf.Count("proxy.proxied_metrics_total", float32(len(jsonMetrics)), nil),
	)...)
}

func (p *Proxy) doPost(ctx context.Context, wg *sync.WaitGroup, destination string, batch []samplers.JSONMetric) {
	defer wg.Done()

	samples := &ssf.Samples{}
	defer metrics.Report(p.TraceClient, samples)

	batchSize := len(batch)
	if batchSize < 1 {
		return
	}

	endpoint := fmt.Sprintf("%s/import", destination)
	err := vhttp.PostHelper(ctx, p.HTTPClient, p.TraceClient, http.MethodPost, endpoint, batch, "forward", true, nil, log)
	if err == nil {
		log.WithField("metrics", batchSize).Debug("Completed forward to Veneur")
	} else {
		samples.Add(ssf.Count("forward.error_total", 1, map[string]string{"cause": "post"}))
		log.WithError(err).WithFields(logrus.Fields{
			"endpoint":  endpoint,
			"batchSize": batchSize,
		}).Warn("Failed to POST metrics to destination")
	}
	samples.Add(ssf.RandomlySample(0.1,
		ssf.Count("metrics_by_destination", float32(batchSize), map[string]string{"destination": destination}),
	)...)
}

func (p *Proxy) ReportRuntimeMetrics() {
	mem := &runtime.MemStats{}
	runtime.ReadMemStats(mem)
	metrics.ReportBatch(p.TraceClient, []*ssf.SSFSample{
		ssf.Gauge("mem.heap_alloc_bytes", float32(mem.HeapAlloc), nil),
		ssf.Gauge("gc.number", float32(mem.NumGC), nil),
		ssf.Gauge("gc.pause_total_ns", float32(mem.PauseTotalNs), nil),
		ssf.Gauge("gc.alloc_heap_bytes", float32(mem.HeapAlloc), nil),
		ssf.Gauge("gc.alloc_heap_bytes_total", float32(mem.TotalAlloc), nil),
		ssf.Gauge("gc.mallocs_objects_total", float32(mem.Mallocs), nil),
		ssf.Gauge("gc.GCCPUFraction", float32(mem.GCCPUFraction), nil),
	})
}

// Shutdown signals the server to shut down after closing all
// current connections.
func (p *Proxy) Shutdown() {
	log.Info("Shutting down server gracefully")
	close(p.shutdown)
	graceful.Shutdown()
}
