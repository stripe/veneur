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
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/keepalive"
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
	grpcTlsAddress    string
	grpcTlsListener   net.Listener
	grpcTlsServer     *grpc.Server
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
func Create(params *CreateParams) (*Proxy, error) {
	tlsConfig, err := params.Config.Tls.GetTlsConfig()
	if err != nil {
		return nil, err
	}

	proxy := &Proxy{
		destinations:      params.Destinations,
		dialTimeout:       params.Config.DialTimeout,
		discoverer:        params.Discoverer,
		discoveryInterval: params.Config.DiscoveryInterval,
		forwardAddresses:  params.Config.ForwardAddresses,
		forwardService:    params.Config.ForwardService,
		grpcAddress:       params.Config.GrpcAddress,
		grpcServer: grpc.NewServer(
			grpc.ConnectionTimeout(params.Config.GrpcServer.ConnectionTimeout),
			grpc.KeepaliveParams(keepalive.ServerParameters{
				MaxConnectionIdle:     params.Config.GrpcServer.MaxConnectionIdle,
				MaxConnectionAge:      params.Config.GrpcServer.MaxConnectionAge,
				MaxConnectionAgeGrace: params.Config.GrpcServer.MaxConnectionAgeGrace,
				Time:                  params.Config.GrpcServer.PingTimeout,
				Timeout:               params.Config.GrpcServer.KeepaliveTimeout,
			}),
			grpc.StatsHandler(&grpcstats.StatsHandler{
				IsClient: false,
				Statsd:   params.Statsd,
			})),
		grpcTlsAddress: params.Config.GrpcTlsAddress,
		grpcTlsServer: grpc.NewServer(
			grpc.Creds(credentials.NewTLS(tlsConfig)),
			grpc.ConnectionTimeout(params.Config.GrpcServer.ConnectionTimeout),
			grpc.KeepaliveParams(keepalive.ServerParameters{
				MaxConnectionIdle:     params.Config.GrpcServer.MaxConnectionIdle,
				MaxConnectionAge:      params.Config.GrpcServer.MaxConnectionAge,
				MaxConnectionAgeGrace: params.Config.GrpcServer.MaxConnectionAgeGrace,
				Time:                  params.Config.GrpcServer.PingTimeout,
				Timeout:               params.Config.GrpcServer.KeepaliveTimeout,
			}),
			grpc.StatsHandler(&grpcstats.StatsHandler{
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
	forwardrpc.RegisterForwardServer(proxy.grpcTlsServer, proxy.handlers)

	return proxy, nil
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

	// Listen on gRPC TLS address
	proxy.grpcTlsListener, err = net.Listen("tcp", proxy.grpcTlsAddress)
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

	httpError := startHttpServer(
		ctx, cancel, proxy.logger, &waitGroup,
		&proxy.httpServer, proxy.httpListener, proxy.shutdownTimeout)
	grpcError := startGrpcServer(
		ctx, cancel, proxy.logger.WithField("tls", false), &waitGroup,
		proxy.grpcServer, proxy.grpcListener, proxy.shutdownTimeout)
	grpcTlsError := startGrpcServer(
		ctx, cancel, proxy.logger.WithField("tls", true), &waitGroup,
		proxy.grpcTlsServer, proxy.grpcTlsListener, proxy.shutdownTimeout)

	close(proxy.ready)

	// Wait for shut down.
	waitGroup.Wait()
	proxy.destinations.Clear()
	proxy.destinations.Wait()

	httpErr := <-httpError
	grpcErr := <-grpcError
	grpcTlsErr := <-grpcTlsError

	if httpErr != nil || grpcErr != nil || grpcTlsErr != nil {
		return errors.New("error shutting down")
	}
	return nil
}

func startHttpServer(
	ctx context.Context, cancel func(), logger *logrus.Entry,
	waitGroup *sync.WaitGroup, server *http.Server, listener net.Listener,
	shutdownTimeout time.Duration,
) <-chan (error) {
	// Start server
	httpExit := make(chan error, 1)
	logger.WithField("address", listener.Addr()).Debug("serving http")
	waitGroup.Add(1)
	go func() {
		defer waitGroup.Done()

		err := server.Serve(listener)
		if err != http.ErrServerClosed {
			logger.WithError(err).Error("http server closed")
		} else {
			logger.Debug("http server closed")
		}
		httpExit <- err
	}()

	// Handle shutdown
	httpError := make(chan error, 1)
	waitGroup.Add(1)
	go func() {
		defer func() {
			listener.Close()
			close(httpError)
			waitGroup.Done()
		}()

		select {
		case err := <-httpExit:
			cancel()
			httpError <- err
			return
		case <-ctx.Done():
			logger.Info("shutting down http server")
			ctx, shutdownCancel :=
				context.WithTimeout(context.Background(), shutdownTimeout)
			defer shutdownCancel()

			err := server.Shutdown(ctx)
			if err != nil {
				logger.WithError(err).Error("error shuting down http server")
				httpError <- err
			}
			return
		}
	}()

	return httpError
}

func startGrpcServer(
	ctx context.Context, cancel func(), logger *logrus.Entry,
	waitGroup *sync.WaitGroup, server *grpc.Server, listener net.Listener,
	shutdownTimeout time.Duration,
) <-chan (error) {
	// Start server
	grpcTlsExit := make(chan error, 1)
	logger.WithField("address", listener.Addr()).Debug("serving grpc")
	waitGroup.Add(1)
	go func() {
		defer waitGroup.Done()

		err := server.Serve(listener)
		if err != nil {
			logger.WithError(err).Error("grpc server closed")
		} else {
			logger.Debug("grpc server closed")
		}
		grpcTlsExit <- err
	}()

	// Handle shutdown
	grpcTlsError := make(chan error, 1)
	waitGroup.Add(1)
	go func() {
		defer func() {
			listener.Close()
			close(grpcTlsError)
			waitGroup.Done()
		}()

		select {
		case err := <-grpcTlsExit:
			cancel()
			grpcTlsError <- err
			return
		case <-ctx.Done():
			logger.Info("shutting down grpc server")
			shutdownContext, shutdownCancel :=
				context.WithTimeout(context.Background(), shutdownTimeout)
			defer shutdownCancel()

			done := make(chan struct{})
			go func() {
				server.GracefulStop()
				close(done)
			}()

			select {
			case <-done:
				return
			case <-shutdownContext.Done():
				server.Stop()
				err := shutdownContext.Err()
				logger.WithError(err).Error("error shutting down grpc server")
				grpcTlsError <- err
				return
			}
		}
	}()

	return grpcTlsError
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

// The address at which the gRPC TLS server is listening.
func (proxy *Proxy) GetGrpcTlsAddress() net.Addr {
	return proxy.grpcTlsListener.Addr()
}

// Closes the gRPC TLS listener.
func (proxy *Proxy) CloseGrpcTlsListener() error {
	return proxy.grpcTlsListener.Close()
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
