package veneur

import (
	"bufio"
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"net"
	"net/http"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/DataDog/datadog-go/statsd"
	"github.com/Sirupsen/logrus"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3iface"
	"github.com/getsentry/raven-go"
	"github.com/golang/protobuf/proto"
	"github.com/stripe/veneur/ssf"
	"github.com/zenazn/goji/bind"
	"github.com/zenazn/goji/graceful"

	"github.com/pkg/profile"

	"github.com/stripe/veneur/plugins"
	"github.com/stripe/veneur/plugins/influxdb"
	s3p "github.com/stripe/veneur/plugins/s3"
	"github.com/stripe/veneur/samplers"
	"github.com/stripe/veneur/trace"
)

// VERSION stores the current veneur version.
// It must be a var so it can be set at link time.
var VERSION = "dirty"

var profileStartOnce = sync.Once{}

var log = logrus.New()

var tracer = trace.GlobalTracer

// A Server is the actual veneur instance that will be run.
type Server struct {
	Workers     []*Worker
	EventWorker *EventWorker
	TraceWorker *TraceWorker

	statsd *statsd.Client
	sentry *raven.Client

	Hostname string
	Tags     []string

	DDHostname     string
	DDAPIKey       string
	DDTraceAddress string
	HTTPClient     *http.Client

	HTTPAddr    string
	ForwardAddr string
	UDPAddr     *net.UDPAddr
	TraceAddr   *net.UDPAddr
	RcvbufBytes int

	interval            time.Duration
	numReaders          int
	metricMaxLength     int
	traceMaxLengthBytes int

	TCPAddr   *net.TCPAddr
	tlsConfig *tls.Config
	// closed when the TCP listener should exit
	tcpListener net.Listener

	HistogramPercentiles []float64
	FlushMaxPerBody      int

	plugins   []plugins.Plugin
	pluginMtx sync.Mutex

	enableProfiling bool

	HistogramAggregates samplers.HistogramAggregates
}

// NewFromConfig creates a new veneur server from a configuration specification.
func NewFromConfig(conf Config) (ret Server, err error) {
	ret.Hostname = conf.Hostname
	ret.Tags = conf.Tags
	ret.DDHostname = conf.APIHostname
	ret.DDAPIKey = conf.Key
	ret.DDTraceAddress = conf.TraceAPIAddress
	ret.HistogramPercentiles = conf.Percentiles
	if len(conf.Aggregates) == 0 {
		ret.HistogramAggregates.Value = samplers.AggregateMin + samplers.AggregateMax + samplers.AggregateCount
		ret.HistogramAggregates.Count = 3
	} else {
		ret.HistogramAggregates.Value = 0
		for _, agg := range conf.Aggregates {
			ret.HistogramAggregates.Value += samplers.AggregatesLookup[agg]
		}
		ret.HistogramAggregates.Count = len(conf.Aggregates)
	}

	interval, err := time.ParseDuration(conf.Interval)
	if err != nil {
		return
	}
	ret.interval = interval
	ret.HTTPClient = &http.Client{
		// make sure that POSTs to datadog do not overflow the flush interval
		Timeout: interval * 9 / 10,
		// we're fine with using the default transport and redirect behavior
	}
	ret.FlushMaxPerBody = conf.FlushMaxPerBody

	ret.statsd, err = statsd.NewBuffered(conf.StatsAddress, 1024)
	if err != nil {
		return
	}
	ret.statsd.Namespace = "veneur."
	ret.statsd.Tags = append(ret.Tags, "veneurlocalonly")

	// nil is a valid sentry client that noops all methods, if there is no DSN
	// we can just leave it as nil
	if conf.SentryDsn != "" {
		ret.sentry, err = raven.New(conf.SentryDsn)
		if err != nil {
			return
		}
	}

	if conf.Debug {
		log.Level = logrus.DebugLevel
	}

	if conf.EnableProfiling {
		ret.enableProfiling = true
	}

	log.Hooks.Add(sentryHook{
		c:        ret.sentry,
		hostname: ret.Hostname,
		lv: []logrus.Level{
			logrus.ErrorLevel,
			logrus.FatalLevel,
			logrus.PanicLevel,
		},
	})

	log.WithField("number", conf.NumWorkers).Info("Preparing workers")
	// Allocate the slice, we'll fill it with workers later.
	ret.Workers = make([]*Worker, conf.NumWorkers)
	ret.numReaders = conf.NumReaders

	// Use the pre-allocated Workers slice to know how many to start.
	for i := range ret.Workers {
		ret.Workers[i] = NewWorker(i+1, ret.statsd, log)
		// do not close over loop index
		go func(w *Worker) {
			defer func() {
				ret.ConsumePanic(recover())
			}()
			w.Work()
		}(ret.Workers[i])
	}

	ret.EventWorker = NewEventWorker(ret.statsd)

	ret.UDPAddr, err = net.ResolveUDPAddr("udp", conf.UdpAddress)
	if err != nil {
		return
	}

	ret.metricMaxLength = conf.MetricMaxLength
	ret.traceMaxLengthBytes = conf.TraceMaxLengthBytes
	ret.RcvbufBytes = conf.ReadBufferSizeBytes
	ret.HTTPAddr = conf.HTTPAddress
	ret.ForwardAddr = conf.ForwardAddress

	if conf.TcpAddress != "" {
		ret.TCPAddr, err = net.ResolveTCPAddr("tcp", conf.TcpAddress)
		if err != nil {
			return
		}
		if conf.TLSKey != "" {
			if conf.TLSCertificate == "" {
				err = errors.New("tls_key is set; must set tls_certificate")
				return
			}

			// load the TLS key and certificate
			var cert tls.Certificate
			cert, err = tls.X509KeyPair([]byte(conf.TLSCertificate), []byte(conf.TLSKey))
			if err != nil {
				return
			}

			clientAuthMode := tls.NoClientCert
			var clientCAs *x509.CertPool = nil
			if conf.TLSAuthorityCertificate != "" {
				// load the authority; require clients to present certificated signed by this authority
				clientAuthMode = tls.RequireAndVerifyClientCert
				clientCAs = x509.NewCertPool()
				ok := clientCAs.AppendCertsFromPEM([]byte(conf.TLSAuthorityCertificate))
				if !ok {
					err = errors.New("tls_authority_certificate: Could not load any certificates")
					return
				}
			}

			ret.tlsConfig = &tls.Config{
				Certificates: []tls.Certificate{cert},
				ClientAuth:   clientAuthMode,
				ClientCAs:    clientCAs,
			}
		}
	}

	conf.Key = "REDACTED"
	conf.SentryDsn = "REDACTED"
	conf.TLSKey = "REDACTED"
	log.WithField("config", conf).Debug("Initialized server")

	if len(conf.TraceAddress) > 0 && len(conf.TraceAPIAddress) > 0 {

		ret.TraceWorker = NewTraceWorker(ret.statsd)

		ret.TraceAddr, err = net.ResolveUDPAddr("udp", conf.TraceAddress)
		log.WithField("traceaddr", ret.TraceAddr).Info("Set trace address")
		if err == nil && ret.TraceAddr == nil {
			err = errors.New("resolved nil UDP address")
		}
		if err != nil {
			return
		}
	} else {
		trace.Disabled = true
	}

	var svc s3iface.S3API
	awsID := conf.AwsAccessKeyID
	awsSecret := conf.AwsSecretAccessKey

	conf.AwsAccessKeyID = "REDACTED"
	conf.AwsSecretAccessKey = "REDACTED"

	if len(awsID) > 0 && len(awsSecret) > 0 {
		sess, err := session.NewSession(&aws.Config{
			Region:      aws.String(conf.AwsRegion),
			Credentials: credentials.NewStaticCredentials(awsID, awsSecret, ""),
		})

		if err != nil {
			log.Info("error getting AWS session: %s", err)
			svc = nil
		} else {
			log.Info("Successfully created AWS session")
			svc = s3.New(sess)

			plugin := &s3p.S3Plugin{
				Logger:   log,
				Svc:      svc,
				S3Bucket: conf.AwsS3Bucket,
				Hostname: ret.Hostname,
			}
			ret.registerPlugin(plugin)
		}
	} else {
		log.Info("AWS credentials not found")
	}

	if svc == nil {
		log.Info("S3 archives are disabled")
	} else {
		log.Info("S3 archives are enabled")
	}

	if conf.InfluxAddress != "" {
		plugin := influxdb.NewInfluxDBPlugin(
			log, conf.InfluxAddress, conf.InfluxConsistency, conf.InfluxDBName, ret.HTTPClient, ret.statsd,
		)
		ret.registerPlugin(plugin)
	}

	return
}

// Start spins up the Server to do actual work, firing off goroutines for
// various workers and utilities.
func (s *Server) Start() {
	log.WithField("version", VERSION).Info("Starting server")

	go func() {
		log.Info("Starting Event worker")
		defer func() {
			s.ConsumePanic(recover())
		}()
		s.EventWorker.Work()
	}()

	if s.TraceWorker != nil {
		log.Info("Starting Trace worker")
		go func() {
			defer func() {
				s.ConsumePanic(recover())
			}()
			s.TraceWorker.Work()
		}()
	}

	packetPool := &sync.Pool{
		New: func() interface{} {
			return make([]byte, s.metricMaxLength)
		},
	}

	tracePool := &sync.Pool{
		New: func() interface{} {
			return make([]byte, s.traceMaxLengthBytes)
		},
	}

	// Read Metrics Forever!
	for i := 0; i < s.numReaders; i++ {
		go func() {
			defer func() {
				s.ConsumePanic(recover())
			}()
			s.ReadMetricSocket(packetPool, s.numReaders != 1)
		}()
	}

	// Read Metrics from TCP Forever!
	if s.TCPAddr != nil {
		// allow shutdown to stop the accept goroutine
		var err error
		s.tcpListener, err = net.ListenTCP("tcp", s.TCPAddr)
		if err != nil {
			logrus.WithError(err).Fatal("Error listening for TCP connections")
		}

		go func() {
			defer func() {
				s.ConsumePanic(recover())
			}()
			s.ReadTCPSocket()
		}()
	} else {
		logrus.Info("TCP not configured - not reading TCP socket")
	}

	// Read Traces Forever!
	go func() {
		defer func() {
			s.ConsumePanic(recover())
		}()
		if s.TraceAddr != nil {
			// If we ever use multiple readers, pass in the appropriate reuseport option
			s.ReadTraceSocket(tracePool)
		} else {
			logrus.Info("Tracing not configured - not reading trace socket")
		}
	}()

	// Flush every Interval forever!
	go func() {
		defer func() {
			s.ConsumePanic(recover())
		}()
		ticker := time.NewTicker(s.interval)
		for range ticker.C {
			s.Flush()
		}
	}()

}

// HandleMetricPacket processes each packet that is sent to the server, and sends to an
// appropriate worker (EventWorker or Worker).
func (s *Server) HandleMetricPacket(packet []byte) error {
	// This is a very performance-sensitive function
	// and packets may be dropped if it gets slowed down.
	// Keep that in mind when modifying!

	if len(packet) == 0 {
		// a lot of clients send packets that accidentally have a trailing
		// newline, it's easier to just let them be
		return nil
	}

	if bytes.HasPrefix(packet, []byte{'_', 'e', '{'}) {
		event, err := samplers.ParseEvent(packet)
		if err != nil {
			log.WithFields(logrus.Fields{
				logrus.ErrorKey: err,
				"packet":        string(packet),
			}).Error("Could not parse packet")
			s.statsd.Count("packet.error_total", 1, []string{"packet_type:event"}, 1.0)
			return err
		}
		s.EventWorker.EventChan <- *event
	} else if bytes.HasPrefix(packet, []byte{'_', 's', 'c'}) {
		svcheck, err := samplers.ParseServiceCheck(packet)
		if err != nil {
			log.WithFields(logrus.Fields{
				logrus.ErrorKey: err,
				"packet":        string(packet),
			}).Error("Could not parse packet")
			s.statsd.Count("packet.error_total", 1, []string{"packet_type:service_check"}, 1.0)
			return err
		}
		s.EventWorker.ServiceCheckChan <- *svcheck
	} else {
		metric, err := samplers.ParseMetric(packet)
		if err != nil {
			log.WithFields(logrus.Fields{
				logrus.ErrorKey: err,
				"packet":        string(packet),
			}).Error("Could not parse packet")
			s.statsd.Count("packet.error_total", 1, []string{"packet_type:metric"}, 1.0)
			return err
		}
		s.Workers[metric.Digest%uint32(len(s.Workers))].PacketChan <- *metric
	}
	return nil
}

// HandleTracePacket accepts an incoming packet as bytes and sends it to the
// appropriate worker.
func (s *Server) HandleTracePacket(packet []byte) {
	// Unlike metrics, protobuf shouldn't have an issue with 0-length packets
	if len(packet) == 0 {
		log.Error("received zero-length trace packet")
		return
	}

	// Technically this could be anything, but we're only consuming trace spans
	// for now.
	newSample := &ssf.SSFSample{}
	err := proto.Unmarshal(packet, newSample)
	if err != nil {
		log.WithError(err).Error("Trace unmarshaling error")
		return
	}

	s.TraceWorker.TraceChan <- *newSample
}

// ReadMetricSocket listens for available packets to handle.
func (s *Server) ReadMetricSocket(packetPool *sync.Pool, reuseport bool) {
	// each goroutine gets its own socket
	// if the sockets support SO_REUSEPORT, then this will cause the
	// kernel to distribute datagrams across them, for better read
	// performance
	serverConn, err := NewSocket(s.UDPAddr, s.RcvbufBytes, reuseport)
	if err != nil {
		// if any goroutine fails to create the socket, we can't really
		// recover, so we just blow up
		// this probably indicates a systemic issue, eg lack of
		// SO_REUSEPORT support
		log.WithError(err).Fatal("Error listening for UDP metrics")
	}
	log.WithField("address", s.UDPAddr).Info("Listening for UDP metrics")

	for {
		buf := packetPool.Get().([]byte)
		n, _, err := serverConn.ReadFrom(buf)
		if err != nil {
			log.WithError(err).Error("Error reading from UDP metrics socket")
			continue
		}

		// statsd allows multiple packets to be joined by newlines and sent as
		// one larger packet
		// note that spurious newlines are not allowed in this format, it has
		// to be exactly one newline between each packet, with no leading or
		// trailing newlines
		splitPacket := samplers.NewSplitBytes(buf[:n], '\n')
		for splitPacket.Next() {
			s.HandleMetricPacket(splitPacket.Chunk())
		}

		// the Metric struct created by HandleMetricPacket has no byte slices in it,
		// only strings
		// therefore there are no outstanding references to this byte slice, we
		// can return it to the pool
		packetPool.Put(buf)
	}
}

// ReadTraceSocket listens for available packets to handle.
func (s *Server) ReadTraceSocket(packetPool *sync.Pool) {
	// TODO This is duplicated from ReadMetricSocket and feels like it could be it's
	// own function?

	if s.TraceAddr == nil {
		log.WithField("s.TraceAddr", s.TraceAddr).Fatal("Cannot listen on nil trace address")
	}
	p := packetPool.Get().([]byte)
	if len(p) == 0 {
		log.WithField("len", len(p)).Fatal(
			"packetPool making empty slices: trace_max_length_bytes must be >= 0")
	}
	packetPool.Put(p)

	// if we want to use multiple readers, make reuseport a parameter, like ReadMetricSocket.
	serverConn, err := NewSocket(s.TraceAddr, s.RcvbufBytes, false)
	if err != nil {
		// if any goroutine fails to create the socket, we can't really
		// recover, so we just blow up
		// this probably indicates a systemic issue, eg lack of
		// SO_REUSEPORT support
		log.WithError(err).Fatal("Error listening for UDP traces")
	}
	log.WithField("address", s.TraceAddr).Info("Listening for UDP traces")

	for {
		buf := packetPool.Get().([]byte)
		n, _, err := serverConn.ReadFrom(buf)
		if err != nil {
			log.WithError(err).Error("Error reading from UDP trace socket")
			continue
		}

		s.HandleTracePacket(buf[:n])
		packetPool.Put(buf)
	}
}

func (s *Server) handleTCPGoroutine(conn net.Conn) {
	defer func() {
		s.ConsumePanic(recover())
	}()

	defer func() {
		log.WithField("peer", conn.RemoteAddr()).Debug("Closing TCP connection")
		err := conn.Close()
		s.statsd.Count("tcp.disconnects", 1, nil, 1.0)
		if err != nil {
			// most often "write: broken pipe"; not really an error
			log.WithFields(logrus.Fields{
				logrus.ErrorKey: err,
				"peer":          conn.RemoteAddr(),
			}).Info("TCP close failed")
		}
	}()
	s.statsd.Count("tcp.connects", 1, nil, 1.0)

	if tlsConn, ok := conn.(*tls.Conn); ok {
		// complete the handshake to verify the certificate
		if !tlsConn.ConnectionState().HandshakeComplete {
			err := tlsConn.Handshake()
			if err != nil {
				// usually io.EOF or "read: connection reset by peer"; not really errors
				// it can also be caused by certificate authentication problems
				s.statsd.Count("tcp.tls_handshake_failures", 1, nil, 1.0)
				log.WithFields(logrus.Fields{
					logrus.ErrorKey: err,
					"peer":          conn.RemoteAddr(),
				}).Info("TLS Handshake failed")

				return
			}
		}

		state := tlsConn.ConnectionState()
		var clientCert pkix.RDNSequence
		if len(state.PeerCertificates) > 0 {
			clientCert = state.PeerCertificates[0].Subject.ToRDNSequence()
		}
		log.WithFields(logrus.Fields{
			"peer":        conn.RemoteAddr(),
			"client_cert": clientCert,
		}).Debug("Starting TLS connection")
	} else {
		log.WithFields(logrus.Fields{
			"peer": conn.RemoteAddr(),
		}).Debug("Starting TCP connection")
	}

	// Scanner is nearly the same performance as a custom implementation
	buf := bufio.NewScanner(conn)
	for buf.Scan() {
		// treat each line as a separate packet
		err := s.HandleMetricPacket(buf.Bytes())
		if err != nil {
			// don't consume bad data from a client indefinitely
			// HandleMetricPacket logs the err and packet, and increments error counters
			log.WithField("peer", conn.RemoteAddr()).Error(
				"Error parsing packet; closing TCP connection")
			return
		}
	}
	if buf.Err() != nil {
		// usually "read: connection reset by peer"
		log.WithFields(logrus.Fields{
			logrus.ErrorKey: buf.Err(),
			"peer":          conn.RemoteAddr(),
		}).Info("Error reading from TCP client")
	}
}

// ReadTCPSocket listens on Server.TCPAddr for new connections, starting a goroutine for each.
func (s *Server) ReadTCPSocket() {
	mode := "unencrypted"
	if s.tlsConfig != nil {
		// wrap the listener with TLS
		s.tcpListener = tls.NewListener(s.tcpListener, s.tlsConfig)
		if s.tlsConfig.ClientAuth == tls.RequireAndVerifyClientCert {
			mode = "authenticated"
		} else {
			mode = "encrypted"
		}
	}

	log.WithFields(logrus.Fields{
		"address": s.TCPAddr, "mode": mode,
	}).Info("Listening for TCP connections")

	for {
		defer func() {
			s.ConsumePanic(recover())
		}()
		conn, err := s.tcpListener.Accept()
		if err != nil {
			// TODO: better way of detecting this error? net.errClosing is private
			if strings.HasSuffix(err.Error(), "use of closed network connection") {
				// occurs when cleanly shutting down the server e.g. in testss
				log.WithError(err).Info("Accept on closed listener; shutting down")
				return
			} else {
				log.WithError(err).Fatal("TCP accept failed")
			}
		}

		go s.handleTCPGoroutine(conn)
	}
}

// HTTPServe starts the HTTP server and listens perpetually until it encounters an unrecoverable error.
func (s *Server) HTTPServe() {
	var prf interface {
		Stop()
	}

	// We want to make sure the profile is stopped
	// exactly once (and only once), even if the
	// shutdown pre-hook does not run (which it may not)
	profileStopOnce := sync.Once{}

	if s.enableProfiling {
		profileStartOnce.Do(func() {
			prf = profile.Start()
		})

		defer func() {
			profileStopOnce.Do(prf.Stop)
		}()
	}
	httpSocket := bind.Socket(s.HTTPAddr)
	graceful.Timeout(10 * time.Second)
	graceful.PreHook(func() {

		if prf != nil {
			profileStopOnce.Do(prf.Stop)
		}

		log.Info("Terminating HTTP listener")
	})

	// Ensure that the server responds to SIGUSR2 even
	// when *not* running under einhorn.
	graceful.AddSignal(syscall.SIGUSR2, syscall.SIGHUP)
	graceful.HandleSignals()
	log.WithField("address", s.HTTPAddr).Info("HTTP server listening")
	bind.Ready()

	if err := graceful.Serve(httpSocket, s.Handler()); err != nil {
		log.WithError(err).Error("HTTP server shut down due to error")
	}

	graceful.Shutdown()
}

// Shutdown signals the server to shut down after closing all
// current connections.
func (s *Server) Shutdown() {
	// TODO(aditya) shut down workers and socket readers
	log.Info("Shutting down server gracefully")
	if s.tcpListener != nil {
		// TODO: the socket is in use until there are no goroutines blocked in Accept
		// we should wait until the accepting goroutine exits
		err := s.tcpListener.Close()
		if err != nil {
			log.WithError(err).Warn("Ignoring error closing TCP listener")
		}
	}
	graceful.Shutdown()
}

// IsLocal indicates whether veneur is running as a local instance
// (forwarding non-local data to a global veneur instance) or is running as a global
// instance (sending all data directly to the final destination).
func (s *Server) IsLocal() bool {
	return s.ForwardAddr != ""
}

// registerPlugin registers a plugin for use
// on the veneur server. It is blocking
// and not threadsafe.
func (s *Server) registerPlugin(p plugins.Plugin) {
	s.pluginMtx.Lock()
	defer s.pluginMtx.Unlock()
	s.plugins = append(s.plugins, p)
}

func (s *Server) getPlugins() []plugins.Plugin {
	s.pluginMtx.Lock()
	plugins := make([]plugins.Plugin, len(s.plugins))
	copy(plugins, s.plugins)
	s.pluginMtx.Unlock()
	return plugins
}

// TracingEnabled returns true if tracing is enabled.
func (s *Server) TracingEnabled() bool {
	return s.TraceWorker != nil
}
