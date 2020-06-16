package main

import (
	"flag"
	"os"

	"github.com/getsentry/sentry-go"
	"github.com/sirupsen/logrus"
	"github.com/stripe/veneur"
	"github.com/stripe/veneur/ssf"
	"github.com/stripe/veneur/trace"
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

	logger := logrus.StandardLogger()
	server, err := veneur.NewFromConfig(logger, conf)
	veneur.SetLogger(logger)
	if err != nil {
		e := err
		if conf.SentryDsn != "" {
			err = sentry.Init(sentry.ClientOptions{
				Dsn: conf.SentryDsn,
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
