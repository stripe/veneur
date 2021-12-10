package main

import (
	"flag"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/getsentry/sentry-go"
	"github.com/sirupsen/logrus"
	"github.com/stripe/veneur/v14"
	"github.com/stripe/veneur/v14/sinks/attribution"
	"github.com/stripe/veneur/v14/sinks/cortex"
	"github.com/stripe/veneur/v14/sinks/debug"
	"github.com/stripe/veneur/v14/sinks/kafka"
	"github.com/stripe/veneur/v14/sinks/localfile"
	"github.com/stripe/veneur/v14/sinks/newrelic"
	"github.com/stripe/veneur/v14/sinks/s3"
	"github.com/stripe/veneur/v14/sinks/signalfx"
	"github.com/stripe/veneur/v14/sinks/splunk"
	"github.com/stripe/veneur/v14/ssf"
	"github.com/stripe/veneur/v14/trace"
)

var (
	configFile           = flag.String("f", "", "The config file to read for settings.")
	configDir            = flag.String("config-dir", "", "The config dir to read config files.")
	validateConfig       = flag.Bool("validate-config", false, "Validate the config file is valid YAML with correct value types, then immediately exit.")
	validateConfigStrict = flag.Bool("validate-config-strict", false, "Validate as with -validate-config, but also fail if there are any unknown fields.")
)

func init() {
	trace.Service = "veneur"
}

func main() {
	flag.Parse()

	var configFiles []string

	if configFile != nil && *configFile != "" {
		configFiles = append(configFiles, *configFile)
	}

	if configDir != nil && *configDir != "" {
		files, err := ioutil.ReadDir(*configDir)
		if err != nil {
			logrus.Fatalf("Config dir specified but cannot be read (%s): %+v", *configDir, err)
		}

		for _, file := range files {
			configFiles = append(configFiles, filepath.Join(*configDir, file.Name()))
		}
	}

	if len(configFiles) == 0 {
		logrus.Fatal("You must specify a config file or config dir")
	}

	conf, err := veneur.ReadConfig(configFiles)
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
		debug.MigrateConfig(&conf)
		localfile.MigrateConfig(&conf)
		newrelic.MigrateConfig(&conf)
		s3.MigrateConfig(&conf)
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
	}

	logger := logrus.StandardLogger()
	server, err := veneur.NewFromConfig(veneur.ServerConfig{
		Config: conf,
		Logger: logger,
		MetricSinkTypes: veneur.MetricSinkTypes{
			// TODO(arnavdugar): Migrate metric sink types.
			"attribution": {
				Create:      attribution.Create,
				ParseConfig: attribution.ParseConfig,
			},
			"cortex": {
				Create:      cortex.Create,
				ParseConfig: cortex.ParseConfig,
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
			"debug": {
				Create:      debug.CreateSpanSink,
				ParseConfig: debug.ParseSpanConfig,
			},
			"kafka": {
				Create:      kafka.CreateSpanSink,
				ParseConfig: kafka.ParseSpanConfig,
			},
			"newrelic": {
				Create:      newrelic.CreateSpanSink,
				ParseConfig: newrelic.ParseSpanConfig,
			},
			"splunk": {
				Create:      splunk.Create,
				ParseConfig: splunk.ParseConfig,
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
