package main

import (
	"flag"
	"os"
	"strings"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/getsentry/raven-go"
	"github.com/stripe/veneur"
	"github.com/stripe/veneur/trace"
)

var (
	configFile = flag.String("f", "", "The config file to read for settings.")
)

func init() {
	trace.Service = "veneur"
}

func main() {
	flag.Parse()

	if configFile == nil || *configFile == "" {
		logrus.Fatal("You must specify a config file")
	}

	conf, err, warnings := veneur.ReadConfig(*configFile)
	if err != nil {
		logrus.WithError(err).Fatal("Error reading config file")
	}
	if len(warnings) > 0 {
		logrus.Warn("Warnings when parsing the config file:\n%s", strings.Join(warnings, "\n  "))
	}

	server, err := veneur.NewFromConfig(conf)
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
	defer func() {
		veneur.ConsumePanic(server.Sentry, server.Statsd, server.Hostname, recover())
	}()
	server.Start()

	if conf.HTTPAddress != "" {
		server.HTTPServe()
	} else {
		select {}
	}
}
