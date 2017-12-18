package main

import (
	"flag"

	"github.com/sirupsen/logrus"
	"github.com/stripe/veneur"
	"github.com/stripe/veneur/ssf"
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

	logger := logrus.StandardLogger()
	proxy, err := veneur.NewProxyFromConfig(logger, conf)
	veneur.SetLogger(logger)

	ssf.NamePrefix = "veneur_proxy."

	if err != nil {
		logrus.WithError(err).Fatal("Could not initialize proxy")
	}
	defer func() {
		veneur.ConsumePanic(proxy.Sentry, proxy.TraceClient, proxy.Hostname, recover())
	}()
	proxy.Start()

	proxy.HTTPServe()
}
