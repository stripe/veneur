package main

import (
	"context"
	"flag"

	"github.com/DataDog/datadog-go/statsd"
	"github.com/hashicorp/consul/api"
	"github.com/sirupsen/logrus"
	"github.com/stripe/veneur/v14"
	"github.com/stripe/veneur/v14/diagnostics"
	"github.com/stripe/veneur/v14/discovery/consul"
	"github.com/stripe/veneur/v14/ssf"
	"github.com/stripe/veneur/v14/trace"
	"github.com/stripe/veneur/v14/util/build"
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

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if configFile == nil || *configFile == "" {
		logrus.Fatal("You must specify a config file")
	}

	config, err := veneur.ReadProxyConfig(logrus.NewEntry(logger), *configFile)
	if err != nil {
		if _, ok := err.(*veneur.UnknownConfigKeys); ok {
			logrus.WithError(err).Warn("Config contains invalid or deprecated keys")
		} else {
			logrus.WithError(err).Fatal("Error reading config file")
		}
	}

	if config.Debug {
		logger.SetLevel(logrus.DebugLevel)
	}

	statsClient, err := statsd.New(
		config.StatsAddress, statsd.WithoutTelemetry(),
		statsd.WithMaxMessagesPerPayload(4096))
	if err != nil {
		logger.WithError(err).Fatal("failed to create statsd client")
	}

	go diagnostics.CollectDiagnosticsMetrics(
		ctx, statsClient, config.RuntimeMetricsInterval, []string{
			"git_sha:" + build.VERSION,
			"service:veneur-proxy",
		})

	discoverer, err := consul.NewConsul(api.DefaultConfig())
	if err != nil {
		logger.WithError(err).Fatal("failed to create discoverer")
	}

	proxy, err := veneur.NewProxyFromConfig(logger, config, discoverer)

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
