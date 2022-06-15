package main

import (
	"context"
	"flag"
	"net/http"
	"net/http/pprof"
	"os"
	"os/signal"
	"syscall"

	"github.com/DataDog/datadog-go/statsd"
	"github.com/hashicorp/consul/api"
	"github.com/sirupsen/logrus"
	"github.com/stripe/veneur/v14/diagnostics"
	"github.com/stripe/veneur/v14/discovery/consul"
	"github.com/stripe/veneur/v14/proxy"
	"github.com/stripe/veneur/v14/proxy/connect"
	"github.com/stripe/veneur/v14/proxy/destinations"
	"github.com/stripe/veneur/v14/util/build"
	utilConfig "github.com/stripe/veneur/v14/util/config"
)

var (
	configFile = flag.String("f", "", "The config file to read for settings.")
)

func main() {
	flag.Parse()
	logger := logrus.StandardLogger()
	logger.WithField("version", build.VERSION).Info("starting server")

	ctx, cancel := signal.NotifyContext(
		context.Background(), os.Interrupt, syscall.SIGUSR2, syscall.SIGHUP)
	defer cancel()

	if configFile == nil || *configFile == "" {
		logrus.Fatal("missing required config file")
	}

	config, err :=
		utilConfig.ReadConfig[proxy.Config](*configFile, "veneur_proxy")
	if err != nil {
		logger.WithError(err).Fatal("failed to load config file")
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

	serveMux := http.NewServeMux()
	serveMux.HandleFunc("/builddate", build.HandleBuildDate)
	serveMux.HandleFunc("/version", build.HandleVersion)
	if config.Http.EnableConfig {
		serveMux.HandleFunc("/config/json", utilConfig.HandleConfigJson(config))
		serveMux.HandleFunc("/config/yaml", utilConfig.HandleConfigYaml(config))
	}
	if config.Http.EnableProfiling {
		serveMux.HandleFunc("/debug/pprof/", pprof.Index)
		serveMux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
		serveMux.HandleFunc("/debug/pprof/profile", pprof.Profile)
		serveMux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
		serveMux.HandleFunc("/debug/pprof/trace", pprof.Trace)
	}

	discoverer, err := consul.NewConsul(api.DefaultConfig())
	if err != nil {
		statsClient.Incr("exit", []string{"error:true"}, 1.0)
		logger.WithError(err).Fatal("failed to create discoverer")
	}

	loggerEntry := logrus.NewEntry(logger)
	proxy := proxy.Create(&proxy.CreateParams{
		Config: config,
		Destinations: destinations.Create(
			connect.Create(
				config.DialTimeout, loggerEntry, statsClient),
			loggerEntry),
		Discoverer:  discoverer,
		HttpHandler: serveMux,
		Logger:      loggerEntry,
		Statsd:      statsClient,
	})

	err = proxy.Start(ctx)
	if err != nil {
		statsClient.Incr("exit", []string{"error:true"}, 1.0)
		logger.WithError(err).Fatal("exited with error")
	}

	statsClient.Incr("exit", []string{"error:false"}, 1.0)
}
