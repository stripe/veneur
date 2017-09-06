package main

import (
	"flag"

	"github.com/Sirupsen/logrus"
	"github.com/stripe/veneur"
	"github.com/stripe/veneur/trace"
)

var (
	configFile = flag.String("f", "", "The config file to read for settings.")
)

func init() {
	trace.Service = "veneur-proxy"
}

func main() {
	flag.Parse()

	if configFile == nil || *configFile == "" {
		logrus.Fatal("You must specify a config file")
	}

	conf, err := veneur.ReadProxyConfig(*configFile)
	if err != nil {
		logrus.WithError(err).Fatal("Error reading config file")
	}

	proxy, err := veneur.NewProxyFromConfig(conf)

	if err != nil {
		logrus.WithError(err).Fatal("Could not initialize proxy")
	}
	defer func() {
		veneur.ConsumePanic(proxy.Sentry, proxy.Recorder, proxy.Hostname, recover())
	}()
	proxy.Start()

	proxy.HTTPServe()
}
