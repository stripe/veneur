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
	strict     = flag.Bool("strict", false, "Fails if unknown config fields are present")
)

func main() {
	flag.Parse()
	logger := logrus.StandardLogger()
	logger.WithField("version", build.VERSION).Info("starting server")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	signalChannel := make(chan os.Signal, 1)
	signal.Notify(signalChannel, os.Interrupt, syscall.SIGUSR2, syscall.SIGHUP)
	go func() {
		select {
		case signal := <-signalChannel:
			logger.WithField("signal", signal).Info("handling signal")
			cancel()
		case <-ctx.Done():
			break
		}
		signal.Stop(signalChannel)
		close(signalChannel)
	}()

	if configFile == nil || *configFile == "" {
		logrus.Fatal("missing required config file")
	}

	config, err :=
		utilConfig.ReadConfig[proxy.Config](*configFile, nil, *strict, "veneur_proxy")
	if err != nil {
		logger.WithError(err).Fatal("failed to load config file")
	}

	if config.Debug {
		logger.SetLevel(logrus.DebugLevel)
	}

	statsClient, err := statsd.New(
		config.Statsd.Address,
		statsd.WithAggregationInterval(config.Statsd.AggregationInterval),
		statsd.WithChannelMode(),
		statsd.WithChannelModeBufferSize(config.Statsd.ChannelBufferSize),
		statsd.WithClientSideAggregation(),
		statsd.WithMaxMessagesPerPayload(config.Statsd.MessagesPerPayload),
		statsd.WithoutTelemetry())
	if err != nil {
		logger.WithError(err).Fatal("failed to create statsd client")
	}
	defer statsClient.Flush()

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
		serveMux.Handle("/debug/pprof/", http.HandlerFunc(pprof.Index))
		serveMux.Handle("/debug/pprof/allocs", pprof.Handler("allocs"))
		serveMux.Handle("/debug/pprof/block", pprof.Handler("block"))
		serveMux.Handle("/debug/pprof/cmdline", http.HandlerFunc(pprof.Cmdline))
		serveMux.Handle("/debug/pprof/goroutine", pprof.Handler("goroutine"))
		serveMux.Handle("/debug/pprof/heap", pprof.Handler("heap"))
		serveMux.Handle("/debug/pprof/mutex", pprof.Handler("mutex"))
		serveMux.Handle("/debug/pprof/profile", http.HandlerFunc(pprof.Profile))
		serveMux.Handle("/debug/pprof/threadcreate", pprof.Handler("threadcreate"))
		serveMux.Handle("/debug/pprof/symbol", http.HandlerFunc(pprof.Symbol))
		serveMux.Handle("/debug/pprof/trace", http.HandlerFunc(pprof.Trace))
	}

	discoverer, err := consul.NewConsul(api.DefaultConfig())
	if err != nil {
		statsClient.Incr("exit", []string{"error:true"}, 1.0)
		logger.WithError(err).Fatal("failed to create discoverer")
	}

	loggerEntry := logrus.NewEntry(logger)
	proxy, err := proxy.Create(&proxy.CreateParams{
		Config: config,
		Destinations: destinations.Create(
			connect.Create(
				config.DialTimeout, loggerEntry, config.SendBufferSize, statsClient,
				config.Statsd.AggregationInterval),
			loggerEntry),
		Discoverer:         discoverer,
		HealthcheckContext: ctx,
		HttpHandler:        serveMux,
		Logger:             loggerEntry,
		Statsd:             statsClient,
	})
	if err != nil {
		statsClient.Incr("exit", []string{"error:true"}, 1.0)
		logger.WithError(err).Fatal("failed to create proxy server")
	}

	err = proxy.Start(ctx)
	if err != nil {
		statsClient.Incr("exit", []string{"error:true"}, 1.0)
		logger.WithError(err).Fatal("exited with error")
	}

	statsClient.Incr("exit", []string{"error:false"}, 1.0)
}
