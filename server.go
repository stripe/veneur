package veneur

import (
	"net"
	"sync"

	"github.com/DataDog/datadog-go/statsd"
	"github.com/Sirupsen/logrus"
	"github.com/getsentry/raven-go"
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

	UDPAddr     *net.UDPAddr
	RcvbufBytes int
}

func NewFromConfig(conf Config) (ret Server, err error) {
	ret.Hostname = conf.Hostname
	ret.Tags = conf.Tags
	ret.DDHostname = conf.APIHostname
	ret.DDAPIKey = conf.Key

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
		ret.Workers[i] = NewWorker(i+1, ret.statsd, ret.logger, conf.Percentiles, conf.HistCounters)
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

	conf.Key = "REDACTED"
	conf.SentryDSN = "REDACTED"
	ret.logger.WithField("config", conf).Debug("Initialized server")

	return
}

func (s *Server) HandlePacket(packet []byte, packetPool *sync.Pool) {
	metric, err := ParseMetric(packet)
	if err != nil {
		s.logger.WithFields(logrus.Fields{
			logrus.ErrorKey: err,
			"packet":        string(packet),
		}).Error("Could not parse packet")
		s.statsd.Count("packet.error_total", 1, nil, 1.0)
	}

	if len(s.Tags) > 0 {
		metric.Tags = append(metric.Tags, s.Tags...)
	}

	s.Workers[metric.Digest%uint32(len(s.Workers))].WorkChan <- *metric

	// the Metric struct has no byte slices in it, only strings
	// therefore there are no outstanding references to this byte slice, and we
	// can return it to the pool
	packetPool.Put(packet[:cap(packet)])
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
		s.HandlePacket(buf[:n], packetPool)
	}
}
