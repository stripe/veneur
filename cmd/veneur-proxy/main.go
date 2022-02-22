package main

import (
	"flag"

	"github.com/sirupsen/logrus"
	"github.com/stripe/veneur/v14"
	"github.com/stripe/veneur/v14/ssf"
	"github.com/stripe/veneur/v14/trace"
)

var (
	configFile = flag.String("f", "", "The config file to read for settings.")
)

func init() {
	trace.Service = "veneur-proxy"
}

func main() {
	flag.Parse()
	logger := logrus.StandardLogger()

	if configFile == nil || *configFile == "" {
		logrus.Fatal("You must specify a config file")
	}

	conf, err := veneur.ReadProxyConfig(logrus.NewEntry(logger), *configFile)
	if err != nil {
		if _, ok := err.(*veneur.UnknownConfigKeys); ok {
			logrus.WithError(err).Warn("Config contains invalid or deprecated keys")
		} else {
			logrus.WithError(err).Fatal("Error reading config file")
		}
	}

	proxy, err := veneur.NewProxyFromConfig(logger, conf)

	ssf.NamePrefix = "veneur_proxy."

	if err != nil {
		logrus.WithError(err).Fatal("Could not initialize proxy")
	}
	defer func() {
		veneur.ConsumePanic(proxy.TraceClient, proxy.Hostname, recover())
	}()

	if proxy.TraceClient != trace.DefaultClient && proxy.TraceClient != nil {
		if trace.DefaultClient != nil {
			trace.DefaultClient.Close()
		}
		trace.DefaultClient = proxy.TraceClient
	}
	proxy.Start()

	proxy.Serve()
}
