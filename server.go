package veneur

import (
	"bufio"
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"fmt"
	"io"
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
	"github.com/zenazn/goji/bind"
	"github.com/zenazn/goji/graceful"

	"github.com/pkg/profile"

	"github.com/stripe/veneur/plugins"
	"github.com/stripe/veneur/plugins/influxdb"
	localfilep "github.com/stripe/veneur/plugins/localfile"
	s3p "github.com/stripe/veneur/plugins/s3"
	"github.com/stripe/veneur/protocol"
	"github.com/stripe/veneur/samplers"
	"github.com/stripe/veneur/trace"
)

// VERSION stores the current veneur version.
// It must be a var so it can be set at link time.
var VERSION = "dirty"

// REDACTED is used to replace values that we don't want to leak into loglines (e.g., credentials)
const REDACTED = "REDACTED"

var profileStartOnce = sync.Once{}

var log = logrus.StandardLogger()

var tracer = trace.GlobalTracer

const defaultTCPReadTimeout = 10 * time.Minute

// defaultSpanBufferSize is the default maximum number of spans that
// we can flush per flush-interval
const defaultSpanBufferSize = 1 << 14

const lightstepDefaultPort = 8080
const lightstepDefaultInterval = 5 * time.Minute

// A Server is the actual veneur instance that will be run.
type Server struct {
	Workers     []*Worker
	EventWorker *EventWorker
	SpanWorker  *SpanWorker

	Statsd *statsd.Client
	Sentry *raven.Client

	Hostname  string
	Tags      []string
	TagsAsMap map[string]string

	HTTPClient *http.Client

	HTTPAddr string

	ForwardAddr string

	StatsdListenAddrs []net.Addr
	SSFListenAddrs    []net.Addr
	RcvbufBytes       int

	interval            time.Duration
	numReaders          int
	metricMaxLength     int
	traceMaxLengthBytes int

	tlsConfig      *tls.Config
	tcpReadTimeout time.Duration

	// closed when the server is shutting down gracefully
	shutdown chan struct{}

	HistogramPercentiles []float64

	plugins   []plugins.Plugin
	pluginMtx sync.Mutex

	enableProfiling bool

	HistogramAggregates samplers.HistogramAggregates

	spanSinks   []spanSink
	metricSinks []metricSink

	traceLightstepAccessToken string
}

// NewFromConfig creates a new veneur server from a configuration specification.
func NewFromConfig(conf Config) (ret Server, err error) {
	ret.Hostname = conf.Hostname
	ret.Tags = conf.Tags

	mappedTags := make(map[string]string)
	for _, tag := range ret.Tags {
		splt := strings.SplitN(tag, ":", 2)
		if len(splt) < 2 {
			mappedTags[splt[0]] = ""
		} else {
			mappedTags[splt[0]] = splt[1]
		}
	}

	ret.TagsAsMap = mappedTags
	ret.traceLightstepAccessToken = conf.TraceLightstepAccessToken
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

	ret.interval, err = time.ParseDuration(conf.Interval)
	if err != nil {
		return
	}
	ret.HTTPClient = &http.Client{
		// make sure that POSTs to datadog do not overflow the flush interval
		Timeout: ret.interval * 9 / 10,
		// we're fine with using the default transport and redirect behavior
	}

	ret.Statsd, err = statsd.NewBuffered(conf.StatsAddress, 1024)
	if err != nil {
		return
	}
	ret.Statsd.Namespace = "veneur."
	ret.Statsd.Tags = append(ret.Tags, "veneurlocalonly")

	// nil is a valid sentry client that noops all methods, if there is no DSN
	// we can just leave it as nil
	if conf.SentryDsn != "" {
		ret.Sentry, err = raven.New(conf.SentryDsn)
		if err != nil {
			return
		}
	}

	if conf.Debug {
		log.SetLevel(logrus.DebugLevel)
	}

	if conf.EnableProfiling {
		ret.enableProfiling = true
	}

	// This is a check to ensure that we don't repeatedly add a hook
	// to the "global" log instance on repeated calls to `NewFromConfig`
	// such as those made in testing. By skipping this we avoid a race
	// condition in logrus discussed here:
	// https://github.com/sirupsen/logrus/issues/295
	if _, ok := log.Hooks[logrus.FatalLevel]; !ok {
		log.AddHook(sentryHook{
			c:        ret.Sentry,
			hostname: ret.Hostname,
			lv: []logrus.Level{
				logrus.ErrorLevel,
				logrus.FatalLevel,
				logrus.PanicLevel,
			},
		})
	}

	log.WithField("number", conf.NumWorkers).Info("Preparing workers")
	// Allocate the slice, we'll fill it with workers later.
	ret.Workers = make([]*Worker, conf.NumWorkers)
	ret.numReaders = conf.NumReaders

	// Use the pre-allocated Workers slice to know how many to start.
	for i := range ret.Workers {
		ret.Workers[i] = NewWorker(i+1, ret.Statsd, log)
		// do not close over loop index
		go func(w *Worker) {
			defer func() {
				ConsumePanic(ret.Sentry, ret.Statsd, ret.Hostname, recover())
			}()
			w.Work()
		}(ret.Workers[i])
	}

	ret.EventWorker = NewEventWorker(ret.Statsd)

	for _, addrStr := range conf.StatsdListenAddresses {
		addr, err := protocol.ResolveAddr(addrStr)
		if err != nil {
			return ret, err
		}
		ret.StatsdListenAddrs = append(ret.StatsdListenAddrs, addr)
	}
	for _, addrStr := range conf.SsfListenAddresses {
		addr, err := protocol.ResolveAddr(addrStr)
		if err != nil {
			return ret, err
		}
		ret.SSFListenAddrs = append(ret.SSFListenAddrs, addr)
	}

	ret.metricMaxLength = conf.MetricMaxLength
	ret.traceMaxLengthBytes = conf.TraceMaxLengthBytes
	ret.RcvbufBytes = conf.ReadBufferSizeBytes
	ret.HTTPAddr = conf.HTTPAddress
	ret.ForwardAddr = conf.ForwardAddress

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
		var clientCAs *x509.CertPool
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

	conf.Key = REDACTED
	conf.SentryDsn = REDACTED
	conf.TLSKey = REDACTED
	log.WithField("config", conf).Debug("Initialized server")

	ddSink, err := NewDatadogMetricSink(&conf, ret.interval.Seconds(), ret.HTTPClient, ret.Statsd)
	if err != nil {
		return
	}
	ret.metricSinks = append(ret.metricSinks, ddSink)

	// Configure tracing sinks
	if len(conf.SsfListenAddresses) > 0 && (conf.DatadogTraceAPIAddress != "" || conf.TraceLightstepAccessToken != "") {
		// Set a sane default
		if conf.SsfBufferSize == 0 {
			conf.SsfBufferSize = defaultSpanBufferSize
		}

		trace.Enable()

		// configure Datadog as a Span sink
		if conf.DatadogTraceAPIAddress != "" {

			ddSink, err := NewDatadogSpanSink(&conf, ret.Statsd, ret.HTTPClient, ret.TagsAsMap)
			if err != nil {
				return ret, err
			}

			ret.spanSinks = append(ret.spanSinks, ddSink)
			log.Info("Configured Datadog trace sink")
		}

		// configure Lightstep as a Span Sink
		if ret.traceLightstepAccessToken != "" {

			var lsSink spanSink
			lsSink, err = NewLightStepSpanSink(&conf, ret.Statsd, ret.TagsAsMap)
			if err != nil {
				return
			}
			ret.spanSinks = append(ret.spanSinks, lsSink)

			// only set this if the original token was non-empty
			ret.traceLightstepAccessToken = REDACTED
			log.Info("Configured Lightstep trace sink")
		}

		ret.SpanWorker = NewSpanWorker(ret.spanSinks, ret.Statsd)

	} else {
		trace.Disable()
	}

	var svc s3iface.S3API
	awsID := conf.AwsAccessKeyID
	awsSecret := conf.AwsSecretAccessKey

	conf.AwsAccessKeyID = REDACTED
	conf.AwsSecretAccessKey = REDACTED

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
			log, conf.InfluxAddress, conf.InfluxConsistency, conf.InfluxDBName, ret.HTTPClient, ret.Statsd,
		)
		ret.registerPlugin(plugin)
	}

	if conf.FlushFile != "" {
		localFilePlugin := &localfilep.Plugin{
			FilePath: conf.FlushFile,
			Logger:   log,
		}
		ret.registerPlugin(localFilePlugin)
		log.Info(fmt.Sprintf("Local file logging to %s", conf.FlushFile))
	}

	// closed in Shutdown; Same approach and http.Shutdown
	ret.shutdown = make(chan struct{})

	return
}

// Start spins up the Server to do actual work, firing off goroutines for
// various workers and utilities.
func (s *Server) Start() {
	log.WithField("version", VERSION).Info("Starting server")

	go func() {
		log.Info("Starting Event worker")
		defer func() {
			ConsumePanic(s.Sentry, s.Statsd, s.Hostname, recover())
		}()
		s.EventWorker.Work()
	}()

	if s.TracingEnabled() {
		log.Info("Starting Trace worker")
		go func() {
			defer func() {
				ConsumePanic(s.Sentry, s.Statsd, s.Hostname, recover())
			}()
			s.SpanWorker.Work()
		}()
	}

	statsdPool := &sync.Pool{
		// We +1 this so we an "detect" when someone sends us too long of a metric!
		New: func() interface{} {
			return make([]byte, s.metricMaxLength+1)
		},
	}

	tracePool := &sync.Pool{
		New: func() interface{} {
			return make([]byte, s.traceMaxLengthBytes)
		},
	}

	// Read Metrics Forever!
	for _, addr := range s.StatsdListenAddrs {
		StartStatsd(s, addr, statsdPool)
	}

	// Read Traces Forever!
	if s.TracingEnabled() && len(s.SSFListenAddrs) > 0 {
		for _, addr := range s.SSFListenAddrs {
			StartSSF(s, addr, tracePool)
		}
	} else {
		logrus.Info("Tracing sockets are configured - not reading trace socket")
	}

	// Flush every Interval forever!
	go func() {
		defer func() {
			ConsumePanic(s.Sentry, s.Statsd, s.Hostname, recover())
		}()
		ticker := time.NewTicker(s.interval)
		for {
			select {
			case <-s.shutdown:
				// stop flushing on graceful shutdown
				ticker.Stop()
				return
			case <-ticker.C:
				s.Flush()
			}
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
			}).Warn("Could not parse packet")
			s.Statsd.Count("packet.error_total", 1, []string{"packet_type:event", "reason:parse"}, 1.0)
			return err
		}
		s.EventWorker.EventChan <- *event
	} else if bytes.HasPrefix(packet, []byte{'_', 's', 'c'}) {
		svcheck, err := samplers.ParseServiceCheck(packet)
		if err != nil {
			log.WithFields(logrus.Fields{
				logrus.ErrorKey: err,
				"packet":        string(packet),
			}).Warn("Could not parse packet")
			s.Statsd.Count("packet.error_total", 1, []string{"packet_type:service_check", "reason:parse"}, 1.0)
			return err
		}
		s.EventWorker.ServiceCheckChan <- *svcheck
	} else {
		metric, err := samplers.ParseMetric(packet)
		if err != nil {
			log.WithFields(logrus.Fields{
				logrus.ErrorKey: err,
				"packet":        string(packet),
			}).Warn("Could not parse packet")
			s.Statsd.Count("packet.error_total", 1, []string{"packet_type:metric", "reason:parse"}, 1.0)
			return err
		}
		s.Workers[metric.Digest%uint32(len(s.Workers))].PacketChan <- *metric
	}
	return nil
}

// HandleTracePacket accepts an incoming packet as bytes and sends it to the
// appropriate worker.
func (s *Server) HandleTracePacket(packet []byte) {
	// TODO: don't use statsd for this in favor of an in-memory counter
	s.Statsd.Incr("ssf.received_total", nil, .1)
	// Unlike metrics, protobuf shouldn't have an issue with 0-length packets
	if len(packet) == 0 {
		s.Statsd.Count("ssf.error_total", 1, []string{"ssf_format:packet", "packet_type:unknown", "reason:zerolength"}, 1.0)
		log.Warn("received zero-length trace packet")
		return
	}

	s.Statsd.Histogram("ssf.packet_size", float64(len(packet)), nil, .1)

	msg, err := samplers.ParseSSF(packet)
	if err != nil {
		reason := fmt.Sprintf("reason:%s", err.Error())
		s.Statsd.Count("ssf.error_total", 1, []string{"ssf_format:packet", "packet_type:ssf_metric", reason}, 1.0)
		log.WithError(err).Warn("ParseSSF")
		return
	}
	s.handleSSF(msg, []string{"ssf_format:packet"})
}

func (s *Server) handleSSF(msg *samplers.Message, tags []string) {
	metrics, err := msg.Metrics()
	if err != nil {
		if _, ok := err.(samplers.InvalidMetrics); ok {
			log.WithError(err).Warn("Could not parse metrics from SSF Message")
			s.Statsd.Count("ssf.error_total", 1, append([]string{
				"packet_type:ssf_metric",
				"step:extract_metrics",
				"reason:invalid_metrics",
			}, tags...), 1.0)
		} else {
			log.WithError(err).Error("Unexpected error extracting metrics from SSF Message")
			errorTag := fmt.Sprintf("error:%s", err.Error())
			s.Statsd.Count("ssf.error_total", 1, append([]string{
				"packet_type:ssf_metric",
				"step:extract_metrics",
				"reason:unexpected_error",
				errorTag,
			}, tags...), 1.0)
			return
		}
	}
	for _, metric := range metrics {
		s.Workers[metric.Digest%uint32(len(s.Workers))].PacketChan <- metric
	}

	span, err := msg.TraceSpan()
	if err != nil {
		if _, ok := err.(*samplers.InvalidTrace); !ok {
			log.WithError(err).Warn("Unexpected error extracting trace span from SSF Message")
		}
		return
	}

	tags = append([]string{fmt.Sprintf("service:%s", span.Service)}, tags...)

	s.Statsd.Incr("ssf.spans.received_total", tags, .1)
	s.Statsd.Histogram("ssf.spans.tags_per_span", float64(len(span.Tags)), tags, .1)
	s.SpanWorker.SpanChan <- *span
}

// ReadMetricSocket listens for available packets to handle.
func (s *Server) ReadMetricSocket(serverConn net.PacketConn, packetPool *sync.Pool) {
	for {
		buf := packetPool.Get().([]byte)
		n, _, err := serverConn.ReadFrom(buf)
		if err != nil {
			log.WithError(err).Error("Error reading from UDP metrics socket")
			continue
		}
		if n > s.metricMaxLength {
			s.Statsd.Count("packet.error_total", 1, []string{"packet_type:unknown", "reason:toolong"}, 1.0)
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

// ReadSSFPacketSocket reads SSF packets off a packet connection.
func (s *Server) ReadSSFPacketSocket(serverConn net.PacketConn, packetPool *sync.Pool) {
	// TODO This is duplicated from ReadMetricSocket and feels like it could be it's
	// own function?
	p := packetPool.Get().([]byte)
	if len(p) == 0 {
		log.WithField("len", len(p)).Fatal(
			"packetPool making empty slices: trace_max_length_bytes must be >= 0")
	}
	packetPool.Put(p)

	for {
		buf := packetPool.Get().([]byte)
		n, _, err := serverConn.ReadFrom(buf)
		if err != nil {
			// In tests, the probably-best way to
			// terminate this reader is to issue a shutdown and close the listening
			// socket, which returns an error, so let's handle it here:
			select {
			case <-s.shutdown:
				log.WithError(err).Info("Ignoring ReadFrom error while shutting down")
				return
			default:
				log.WithError(err).Error("Error reading from UDP trace socket")
				continue
			}
		}

		s.HandleTracePacket(buf[:n])
		packetPool.Put(buf)
	}
}

// ReadSSFStreamSocket reads a streaming connection in framed wire format
// off a streaming socket. See package
// github.com/stripe/veneur/protocol for details.
func (s *Server) ReadSSFStreamSocket(serverConn net.Conn) {
	defer func() {
		serverConn.Close()
	}()
	tags := []string{"ssf_format:framed"}
	for {
		msg, err := protocol.ReadSSF(serverConn)
		if err != nil {
			if err == io.EOF {
				// Client hangup, close this
				s.Statsd.Incr("frames.disconnects", nil, 1.0)
				return
			}
			if protocol.IsFramingError(err) {
				log.WithError(err).
					WithField("remote", serverConn.RemoteAddr()).
					Error("Frame error reading from SSF connection. Closing.")
				s.Statsd.Incr("ssf.error_total",
					append([]string{"packet_type:unknown", "reason:framing"}, tags...),
					1.0)
				return
			}
			// Non-frame errors means we can continue reading:
			log.WithError(err).
				WithField("remote", serverConn.RemoteAddr()).
				Error("Error processing an SSF frame")
			s.Statsd.Incr("ssf.error_total",
				append([]string{"packet_type:unknown", "reason:processing"}, tags...),
				1.0)
			continue
		}
		// TODO: don't use statsd for this metric, use an in-memory counter.
		s.Statsd.Incr("ssf.received_total", tags, .1)
		s.handleSSF(msg, tags)
	}
}

func (s *Server) handleTCPGoroutine(conn net.Conn) {
	defer func() {
		ConsumePanic(s.Sentry, s.Statsd, s.Hostname, recover())
	}()

	defer func() {
		log.WithField("peer", conn.RemoteAddr()).Debug("Closing TCP connection")
		err := conn.Close()
		s.Statsd.Count("tcp.disconnects", 1, nil, 1.0)
		if err != nil {
			// most often "write: broken pipe"; not really an error
			log.WithFields(logrus.Fields{
				logrus.ErrorKey: err,
				"peer":          conn.RemoteAddr(),
			}).Info("TCP close failed")
		}
	}()
	s.Statsd.Count("tcp.connects", 1, nil, 1.0)

	// time out idle connections to prevent leaking memory/goroutines
	timeout := defaultTCPReadTimeout
	if s.tcpReadTimeout != 0 {
		timeout = s.tcpReadTimeout
	}

	if tlsConn, ok := conn.(*tls.Conn); ok {
		// complete the handshake to verify the certificate
		conn.SetReadDeadline(time.Now().Add(timeout))
		err := tlsConn.Handshake()
		if err != nil {
			// usually io.EOF or "read: connection reset by peer"; not really errors
			// it can also be caused by certificate authentication problems
			s.Statsd.Count("tcp.tls_handshake_failures", 1, nil, 1.0)
			log.WithFields(logrus.Fields{
				logrus.ErrorKey: err,
				"peer":          conn.RemoteAddr(),
			}).Info("TLS Handshake failed")

			return
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

	scanWithDeadline := func() bool {
		conn.SetReadDeadline(time.Now().Add(timeout))
		return buf.Scan()
	}
	for scanWithDeadline() {
		// treat each line as a separate packet
		err := s.HandleMetricPacket(buf.Bytes())
		if err != nil {
			// don't consume bad data from a client indefinitely
			// HandleMetricPacket logs the err and packet, and increments error counters
			log.WithField("peer", conn.RemoteAddr()).Warn(
				"Error parsing packet; closing TCP connection")
			return
		}
	}
	if buf.Err() != nil {
		// usually "read: connection reset by peer" or "i/o timeout"
		log.WithFields(logrus.Fields{
			logrus.ErrorKey: buf.Err(),
			"peer":          conn.RemoteAddr(),
		}).Info("Error reading from TCP client")
	}
}

// ReadTCPSocket listens on Server.TCPAddr for new connections, starting a goroutine for each.
func (s *Server) ReadTCPSocket(listener net.Listener) {
	for {
		conn, err := listener.Accept()
		if err != nil {
			select {
			case <-s.shutdown:
				// occurs when cleanly shutting down the server e.g. in tests; ignore errors
				log.WithError(err).Info("Ignoring Accept error while shutting down")
				return
			default:
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
	close(s.shutdown)
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
	//TODO we now need to check that the backends are flushing the data too
	return s.SpanWorker != nil
}
