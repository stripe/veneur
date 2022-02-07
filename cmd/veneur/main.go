package main

import (
	"flag"
	"os"

	"github.com/getsentry/sentry-go"
	"github.com/sirupsen/logrus"
	"github.com/stripe/veneur/v14"
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
	"github.com/stripe/veneur/v14/ssf"
	"github.com/stripe/veneur/v14/trace"
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

	if configFile == nil || *configFile == "" {
		logrus.Fatal("You must specify a config file")
	}

	conf, err := veneur.ReadConfig(*configFile)
	if err != nil {
		if _, ok := err.(*veneur.UnknownConfigKeys); ok {
			if *validateConfigStrict {
				logrus.WithError(err).Fatal("Config contains invalid or deprecated keys")
			} else {
				logrus.WithError(err).Warn("Config contains invalid or deprecated keys")
			}
		} else {
			logrus.WithError(err).Fatal("Error reading config file")
		}
	}

	if *validateConfig {
		os.Exit(0)
	}
	if !conf.Features.MigrateMetricSinks {
		datadog.MigrateConfig(&conf)
		debug.MigrateConfig(&conf)
		falconer.MigrateConfig(&conf)
		localfile.MigrateConfig(&conf)
		lightstep.MigrateConfig(&conf)
		newrelic.MigrateConfig(&conf)
		s3.MigrateConfig(&conf)
		prometheus.MigrateConfig(&conf)
		err = signalfx.MigrateConfig(&conf)
		if err != nil {
			logrus.WithError(err).Fatal("error migrating signalfx config")
		}
		err = kafka.MigrateConfig(&conf)
		if err != nil {
			logrus.WithError(err).Fatal("error migrating kafka config")
		}
		err = splunk.MigrateConfig(&conf)
		if err != nil {
			logrus.WithError(err).Fatal("error migrating splunk config")
		}
		xray.MigrateConfig(&conf)
	}

	logger := logrus.StandardLogger()
	server, err := veneur.NewFromConfig(veneur.ServerConfig{
		Config: conf,
		Logger: logger,
		MetricSinkTypes: veneur.MetricSinkTypes{
			// TODO(arnavdugar): Migrate metric sink types.
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
			// TODO(arnavdugar): Migrate span sink types.
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
	})
	veneur.SetLogger(logger)
	if err != nil {
		e := err
		if conf.SentryDsn.Value != "" {
			err = sentry.Init(sentry.ClientOptions{
				Dsn: conf.SentryDsn.Value,
			})
			if err != nil {
				logrus.WithError(err).Error("Error initializing Sentry client")
			}

			event := sentry.NewEvent()
			event.Message = e.Error()
			hostname, _ := os.Hostname()
			if hostname != "" {
				event.ServerName = hostname
			}

			sentry.CaptureEvent(event)
			sentry.Flush(veneur.SentryFlushTimeout)
		}

		logrus.WithError(e).Fatal("Could not initialize server")
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

	if conf.HTTPAddress != "" || conf.GrpcAddress != "" {
		server.Serve()
	} else {
		select {}
	}
}
