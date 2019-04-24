package main

import (
	"flag"
	"os"
	"time"

	"github.com/getsentry/raven-go"
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

		logrus.WithError(e).Error("Error initializing server")
		var sentry *raven.Client
		if conf.SentryDsn != "" {
			sentry, err = raven.New(conf.SentryDsn)
			if err != nil {
				logrus.WithError(err).Error("Error initializing Sentry client")
			}
		}

		hostname, _ := os.Hostname()

		p := raven.NewPacket(e.Error())
		if hostname != "" {
			p.ServerName = hostname
		}

		_, ch := sentry.Capture(p, nil)
		select {
		case <-ch:
		case <-time.After(10 * time.Second):
		}

		logrus.WithError(e).Fatal("Could not initialize server")
	}
	ssf.NamePrefix = "veneur."

	defer func() {
		veneur.ConsumePanic(server.Sentry, server.TraceClient, server.Hostname, recover())
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
