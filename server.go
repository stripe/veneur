package veneur

import (
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/DataDog/datadog-go/statsd"
	"github.com/Sirupsen/logrus"
	"github.com/getsentry/raven-go"
	"github.com/zenazn/goji/bind"
	"github.com/zenazn/goji/graceful"
)

type Server struct {
	Workers []*Worker

	statsd *statsd.Client
	logger *logrus.Logger
	sentry *raven.Client

	Hostname string
	Tags     []string

	DDHostname string
	DDAPIKey   string
	HTTPClient *http.Client

	HTTPAddr    string
	UDPAddr     *net.UDPAddr
	RcvbufBytes int

	HistogramPercentiles []float64
	HistogramCounter     bool
}

func NewFromConfig(conf Config) (ret Server, err error) {
	ret.Hostname = conf.Hostname
	ret.Tags = conf.Tags
	ret.DDHostname = conf.APIHostname
	ret.DDAPIKey = conf.Key
	ret.HistogramCounter = conf.HistCounters
	ret.HistogramPercentiles = conf.Percentiles

	ret.HTTPClient = &http.Client{
		// make sure that POSTs to datadog do not overflow the flush interval
		Timeout: conf.Interval * 9 / 10,
		// we're fine with using the default transport and redirect behavior
	}

	ret.statsd, err = statsd.NewBuffered(conf.StatsAddr, 1024)
	if err != nil {
		return
	}
	ret.statsd.Namespace = "veneur."
	ret.statsd.Tags = ret.Tags

	// nil is a valid sentry client that noops all methods, if there is no DSN
	// we can just leave it as nil
	if conf.SentryDSN != "" {
		ret.sentry, err = raven.New(conf.SentryDSN)
		if err != nil {
			return
		}
	}

	ret.logger = logrus.New()
	if conf.Debug {
		ret.logger.Level = logrus.DebugLevel
	}
	ret.logger.Hooks.Add(sentryHook{
		c:        ret.sentry,
		hostname: ret.Hostname,
		lv: []logrus.Level{
			logrus.ErrorLevel,
			logrus.FatalLevel,
			logrus.PanicLevel,
		},
	})

	ret.logger.WithField("number", conf.NumWorkers).Info("Starting workers")
	ret.Workers = make([]*Worker, conf.NumWorkers)
	for i := range ret.Workers {
		ret.Workers[i] = NewWorker(i+1, ret.statsd, ret.logger)
		// do not close over loop index
		go func(w *Worker) {
			defer func() {
				ret.ConsumePanic(recover())
			}()
			w.Work()
		}(ret.Workers[i])
	}

	ret.UDPAddr, err = net.ResolveUDPAddr("udp", conf.UDPAddr)
	if err != nil {
		return
	}
	ret.RcvbufBytes = conf.ReadBufferSizeBytes
	ret.HTTPAddr = conf.HTTPAddr

	conf.Key = "REDACTED"
	conf.SentryDSN = "REDACTED"
	ret.logger.WithField("config", conf).Debug("Initialized server")

	return
}

func (s *Server) HandlePacket(packet []byte) {
	metric, err := ParseMetric(packet)
	if err != nil {
		s.logger.WithFields(logrus.Fields{
			logrus.ErrorKey: err,
			"packet":        string(packet),
		}).Error("Could not parse packet")
		s.statsd.Count("packet.error_total", 1, nil, 1.0)
		return
	}

	if len(s.Tags) > 0 {
		metric.Tags = append(metric.Tags, s.Tags...)
	}

	s.Workers[metric.Digest%uint32(len(s.Workers))].WorkChan <- *metric
}

func (s *Server) ReadSocket(packetPool *sync.Pool) {
	// each goroutine gets its own socket
	// if the sockets support SO_REUSEPORT, then this will cause the
	// kernel to distribute datagrams across them, for better read
	// performance
	s.logger.WithField("address", s.UDPAddr).Info("UDP server listening")
	serverConn, err := NewSocket(s.UDPAddr, s.RcvbufBytes)
	if err != nil {
		// if any goroutine fails to create the socket, we can't really
		// recover, so we just blow up
		// this probably indicates a systemic issue, eg lack of
		// SO_REUSEPORT support
		s.logger.WithError(err).Fatal("Error listening for UDP")
	}

	for {
		buf := packetPool.Get().([]byte)
		n, _, err := serverConn.ReadFrom(buf)
		if err != nil {
			s.logger.WithError(err).Error("Error reading from UDP")
			continue
		}
		s.HandlePacket(buf[:n])
		// the Metric struct created by HandlePacket has no byte slices in it,
		// only strings
		// therefore there are no outstanding references to this byte slice, we
		// can return it to the pool
		packetPool.Put(buf)
	}
}

func (s *Server) HTTPServe() {
	httpSocket := bind.Socket(s.HTTPAddr)
	graceful.Timeout(10 * time.Second)
	graceful.PreHook(func() {
		s.logger.Info("Terminating HTTP listener")
	})
	graceful.HandleSignals()
	s.logger.WithField("address", s.HTTPAddr).Info("HTTP server listening")
	bind.Ready()

	if err := graceful.Serve(httpSocket, s.Handler()); err != nil {
		s.logger.WithError(err).Error("HTTP server shut down due to error")
	}
	graceful.Wait()
}
