package proxy

import (
	"context"
	"errors"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stripe/veneur/v14/discovery"
	"github.com/stripe/veneur/v14/forwardrpc"
	"github.com/stripe/veneur/v14/proxy/destinations"
	"github.com/stripe/veneur/v14/proxy/grpcstats"
	"github.com/stripe/veneur/v14/proxy/handlers"
	"github.com/stripe/veneur/v14/scopedstatsd"
	"google.golang.org/grpc"
)

type CreateParams struct {
	Config             *Config
	Destinations       destinations.Destinations
	Discoverer         discovery.Discoverer
	HealthcheckContext context.Context
	HttpHandler        *http.ServeMux
	Logger             *logrus.Entry
	Statsd             scopedstatsd.Client
}

type Proxy struct {
	destinations      destinations.Destinations
	dialTimeout       time.Duration
	discoverer        discovery.Discoverer
	discoveryInterval time.Duration
	forwardAddresses  []string
	forwardService    string
	grpcAddress       string
	grpcListener      net.Listener
	grpcServer        *grpc.Server
	handlers          *handlers.Handlers
	httpAddress       string
	httpListener      net.Listener
	httpServer        http.Server
	logger            *logrus.Entry
	ready             chan struct{}
	shutdownTimeout   time.Duration
	statsd            scopedstatsd.Client
}

// Creates a new proxy server.
func Create(params *CreateParams) *Proxy {
	proxy := &Proxy{
		destinations:      params.Destinations,
		dialTimeout:       params.Config.DialTimeout,
		discoverer:        params.Discoverer,
		discoveryInterval: params.Config.DiscoveryInterval,
		forwardAddresses:  params.Config.ForwardAddresses,
		forwardService:    params.Config.ForwardService,
		grpcAddress:       params.Config.GrpcAddress,
		grpcServer: grpc.NewServer(grpc.StatsHandler(&grpcstats.StatsHandler{
			IsClient: false,
			Statsd:   params.Statsd,
		})),
		handlers: &handlers.Handlers{
			Destinations:       params.Destinations,
			HealthcheckContext: params.HealthcheckContext,
			IgnoreTags:         params.Config.IgnoreTags,
			Logger:             params.Logger,
			Statsd:             params.Statsd,
		},
		httpAddress: params.Config.HttpAddress,
		httpServer: http.Server{
			Handler: params.HttpHandler,
		},
		logger:          params.Logger,
		ready:           make(chan struct{}),
		shutdownTimeout: params.Config.ShutdownTimeout,
		statsd:          params.Statsd,
	}

	params.HttpHandler.HandleFunc(
		"/healthcheck", proxy.handlers.HandleHealthcheck)
	params.HttpHandler.HandleFunc("/import", proxy.handlers.HandleJsonMetrics)
	forwardrpc.RegisterForwardServer(proxy.grpcServer, proxy.handlers)

	return proxy
}

// Starts discovery, and the HTTP and gRPC servers. This method stops polling
// discovery and shuts down the servers when the provided context is cancelled
// or if either server stops due to an error. This method attempts to shut down
// the servers gracefully, but shuts them down immediately if this takes longer
// than `shutdownTimeout`.
func (proxy *Proxy) Start(ctx context.Context) error {
	var err error

	// Listen on HTTP address
	proxy.httpListener, err = net.Listen("tcp", proxy.httpAddress)
	if err != nil {
		return err
	}

	// Listen on gRPC address
	proxy.grpcListener, err = net.Listen("tcp", proxy.grpcAddress)
	if err != nil {
		return err
	}

	// Add static destinations
	proxy.destinations.Add(ctx, proxy.forwardAddresses)

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Poll discovery for available destinations.
	if proxy.forwardService != "" {
		go proxy.pollDiscovery(ctx)
	}

	waitGroup := sync.WaitGroup{}

	// Start HTTP server
	httpExit := make(chan error, 1)
	proxy.logger.WithField("address", proxy.GetHttpAddress()).
		Debug("serving http")
	waitGroup.Add(1)
	go func() {
		defer waitGroup.Done()

		err := proxy.httpServer.Serve(proxy.httpListener)
		if err != http.ErrServerClosed {
			proxy.logger.WithError(err).Error("http server closed")
		} else {
			proxy.logger.Debug("http server closed")
		}
		httpExit <- err
	}()

	// Handle HTTP shutdown
	httpError := make(chan error, 1)
	waitGroup.Add(1)
	go func() {
		defer func() {
			proxy.httpListener.Close()
			close(httpError)
			waitGroup.Done()
		}()

		select {
		case err := <-httpExit:
			cancel()
			httpError <- err
			return
		case <-ctx.Done():
			proxy.logger.Info("shutting down http server")
			ctx, shutdownCancel :=
				context.WithTimeout(context.Background(), proxy.shutdownTimeout)
			defer shutdownCancel()

			err := proxy.httpServer.Shutdown(ctx)
			if err != nil {
				proxy.logger.WithError(err).Error("error shuting down http server")
				httpError <- err
			}
			return
		}
	}()

	// Start gRPC server
	grpcExit := make(chan error, 1)
	proxy.logger.WithField("address", proxy.GetGrpcAddress()).
		Debug("serving grpc")
	waitGroup.Add(1)
	go func() {
		defer waitGroup.Done()

		err := proxy.grpcServer.Serve(proxy.grpcListener)
		if err != nil {
			proxy.logger.WithError(err).Error("grpc server closed")
		} else {
			proxy.logger.Debug("grpc server closed")
		}
		grpcExit <- err
	}()

	// Handle gRPC shutdown
	grpcError := make(chan error, 1)
	waitGroup.Add(1)
	go func() {
		defer func() {
			proxy.grpcListener.Close()
			close(grpcError)
			waitGroup.Done()
		}()

		select {
		case err := <-grpcExit:
			cancel()
			grpcError <- err
			return
		case <-ctx.Done():
			proxy.logger.Info("shutting down grpc server")
			shutdownContext, shutdownCancel :=
				context.WithTimeout(context.Background(), proxy.shutdownTimeout)
			defer shutdownCancel()

			done := make(chan struct{})
			go func() {
				proxy.grpcServer.GracefulStop()
				close(done)
			}()

			select {
			case <-done:
				return
			case <-shutdownContext.Done():
				proxy.grpcServer.Stop()
				err := shutdownContext.Err()
				proxy.logger.WithError(err).Error("error shuting down grpc server")
				grpcError <- err
				return
			}
		}
	}()

	close(proxy.ready)

	// Wait for shut down.
	waitGroup.Wait()
	proxy.destinations.Clear()
	proxy.destinations.Wait()

	httpErr := <-httpError
	grpcErr := <-grpcError

	if httpErr != nil || grpcErr != nil {
		return errors.New("error shutting down")
	}
	return nil
}

// A channel that is closed once the server is ready.
func (proxy *Proxy) Ready() <-chan struct{} {
	return proxy.ready
}

// The address at which the HTTP server is listening.
func (proxy *Proxy) GetHttpAddress() net.Addr {
	return proxy.httpListener.Addr()
}

// Closes the HTTP listener.
func (proxy *Proxy) CloseHttpListener() error {
	return proxy.httpListener.Close()
}

// The address at which the gRPC server is listening.
func (proxy *Proxy) GetGrpcAddress() net.Addr {
	return proxy.grpcListener.Addr()
}

// Closes the gRPC listener.
func (proxy *Proxy) CloseGrpcListener() error {
	return proxy.grpcListener.Close()
}

// Poll discovery immediately, and every `discoveryInterval`. This method stops
// polling and exits when the provided context is cancelled.
func (proxy *Proxy) pollDiscovery(ctx context.Context) {
	proxy.HandleDiscovery(ctx)
	discoveryTicker := time.NewTicker(proxy.discoveryInterval)
	for {
		select {
		case <-discoveryTicker.C:
			proxy.HandleDiscovery(ctx)
		case <-ctx.Done():
			discoveryTicker.Stop()
			return
		}
	}
}

// Handles a single discovery query and updates the consistent hash.
func (proxy *Proxy) HandleDiscovery(ctx context.Context) {
	proxy.logger.Debug("discovering destinations")
	startTime := time.Now()

	// Query the discovery service.
	proxy.logger.WithField("service", proxy.forwardService).
		Debug("discovering service")
	newDestinations, err :=
		proxy.discoverer.GetDestinationsForService(proxy.forwardService)
	if err != nil {
		proxy.logger.WithField("error", err).Error("failed discover destinations")
		proxy.statsd.Count(
			"veneur_proxy.discovery.count", 1, []string{"status:fail"}, 1.0)
		return
	}
	proxy.statsd.Gauge(
		"veneur_proxy.discovery.destinations", float64(len(newDestinations)),
		[]string{}, 1.0)

	// Update the consistent hash.
	proxy.destinations.Add(ctx, newDestinations)

	proxy.statsd.Count(
		"veneur_proxy.discovery.duration_total_ms",
		time.Since(startTime).Milliseconds(), []string{}, 1.0)
	proxy.statsd.Count(
		"veneur_proxy.discovery.count", 1, []string{"status:success"}, 1.0)
}
