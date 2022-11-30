package main

import (
	"context"
	"flag"
	"net/http"
	"os"

	"github.com/getsentry/sentry-go"
	"github.com/sirupsen/logrus"
	"github.com/stripe/veneur/v14"
	"github.com/stripe/veneur/v14/diagnostics"
	"github.com/stripe/veneur/v14/sinks/cortex"
	"github.com/stripe/veneur/v14/sinks/datadog"
	"github.com/stripe/veneur/v14/sinks/debug"
	"github.com/stripe/veneur/v14/sinks/falconer"
	"github.com/stripe/veneur/v14/sinks/kafka"
	"github.com/stripe/veneur/v14/sinks/lightstep"
	"github.com/stripe/veneur/v14/sinks/localfile"
	"github.com/stripe/veneur/v14/sinks/newrelic"
	"github.com/stripe/veneur/v14/sinks/prometheus"
	"github.com/stripe/veneur/v14/sinks/s3"
	"github.com/stripe/veneur/v14/sinks/signalfx"
	"github.com/stripe/veneur/v14/sinks/splunk"
	"github.com/stripe/veneur/v14/sinks/xray"
	"github.com/stripe/veneur/v14/sources/openmetrics"
	"github.com/stripe/veneur/v14/ssf"
	"github.com/stripe/veneur/v14/trace"
	"github.com/stripe/veneur/v14/util/build"
	utilConfig "github.com/stripe/veneur/v14/util/config"
)

var (
	configFile           = flag.String("f", "", "The config file to read for settings.")
	validateConfig       = flag.Bool("validate-config", false, "Validate the config file is valid YAML with correct value types, then immediately exit.")
	validateConfigStrict = flag.Bool("validate-config-strict", false, "Validate as with -validate-config, but also fail if there are any unknown fields.")
)

func init() {
	trace.Service = "veneur"
}

func main() {
	flag.Parse()
	logger := logrus.StandardLogger()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if configFile == nil || *configFile == "" {
		logrus.Fatal("missing required config file")
	}

	config, err :=
		utilConfig.ReadConfig[veneur.Config](
			*configFile, nil, *validateConfigStrict, "veneur")
	if err != nil {
		logger.WithError(err).Fatal("failed to load config file")
	}
	config.ApplyDefaults()

	if config.SentryDsn.Value != "" {
		err = sentry.Init(sentry.ClientOptions{
			Dsn:        config.SentryDsn.Value,
			ServerName: config.Hostname,
			Release:    build.VERSION,
		})
		if err != nil {
			logger.WithError(err).Fatal("failed to initialzie Sentry")
		}
		logger.AddHook(veneur.SentryHook{
			Level: []logrus.Level{
				logrus.ErrorLevel,
				logrus.FatalLevel,
				logrus.PanicLevel,
			},
		})
	}

	if *validateConfig {
		os.Exit(0)
	}

	server, err := veneur.NewFromConfig(veneur.ServerConfig{
		Config: *config,
		Logger: logger,
		SourceTypes: veneur.SourceTypes{
			"openmetrics": {
				Create:      openmetrics.Create,
				ParseConfig: openmetrics.ParseConfig,
			},
		},
		MetricSinkTypes: veneur.MetricSinkTypes{
			"cortex": {
				Create:      cortex.Create,
				ParseConfig: cortex.ParseConfig,
			},
			"datadog": {
				Create:      datadog.CreateMetricSink,
				ParseConfig: datadog.ParseMetricConfig,
			},
			"debug": {
				Create:      debug.CreateMetricSink,
				ParseConfig: debug.ParseMetricConfig,
			},
			"kafka": {
				Create:      kafka.CreateMetricSink,
				ParseConfig: kafka.ParseMetricConfig,
			},
			"localfile": {
				Create:      localfile.Create,
				ParseConfig: localfile.ParseConfig,
			},
			"newrelic": {
				Create:      newrelic.CreateMetricSink,
				ParseConfig: newrelic.ParseMetricConfig,
			},
			"prometheus": {
				Create:      prometheus.CreateMetricSink,
				ParseConfig: prometheus.ParseMetricConfig,
			},
			"s3": {
				Create:      s3.Create,
				ParseConfig: s3.ParseConfig,
			},
			"signalfx": {
				Create:      signalfx.Create,
				ParseConfig: signalfx.ParseConfig,
			},
		},
		SpanSinkTypes: veneur.SpanSinkTypes{
			"datadog": {
				Create:      datadog.CreateSpanSink,
				ParseConfig: datadog.ParseSpanConfig,
			},
			"debug": {
				Create:      debug.CreateSpanSink,
				ParseConfig: debug.ParseSpanConfig,
			},
			"falconer": {
				Create:      falconer.Create,
				ParseConfig: falconer.ParseConfig,
			},
			"kafka": {
				Create:      kafka.CreateSpanSink,
				ParseConfig: kafka.ParseSpanConfig,
			},
			"lightstep": {
				Create:      lightstep.CreateSpanSink,
				ParseConfig: lightstep.ParseSpanConfig,
			},
			"newrelic": {
				Create:      newrelic.CreateSpanSink,
				ParseConfig: newrelic.ParseSpanConfig,
			},
			"splunk": {
				Create:      splunk.Create,
				ParseConfig: splunk.ParseConfig,
			},
			"xray": {
				Create:      xray.Create,
				ParseConfig: xray.ParseConfig,
			},
		},
		HttpCustomHandlers: veneur.HttpCustomHandlers{
			"/echo": func(w http.ResponseWriter, r *http.Request) {
				w.Write([]byte("hello world!\n"))
			},
		},
	})
	if err != nil {
		logger.WithError(err).Fatal("Could not initialize server")
	}

	if config.Features.DiagnosticsMetricsEnabled {
		go diagnostics.CollectDiagnosticsMetrics(
			ctx, server.Statsd, server.Interval,
			[]string{"git_sha:" + build.VERSION})
	}

	ssf.NamePrefix = "veneur."

	defer func() {
		veneur.ConsumePanic(server.TraceClient, server.Hostname, recover())
	}()

	if server.TraceClient != nil {
		if trace.DefaultClient != nil {
			trace.DefaultClient.Close()
		}
		trace.DefaultClient = server.TraceClient
	}
	go server.FlushWatchdog()
	server.Start()

	if config.HTTPAddress != "" || config.GrpcAddress != "" {
		server.Serve()
	}
}
