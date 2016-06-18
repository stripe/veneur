package veneur

import (
	"net"
	"sync"

	"github.com/DataDog/datadog-go/statsd"
	"github.com/Sirupsen/logrus"
)

type Server struct {
	Workers []*Worker

	Stats *statsd.Client

	Hostname string
	Tags     []string

	DDHostname string
	DDAPIKey   string

	UDPAddr     *net.UDPAddr
	RcvbufBytes int
}

func NewFromConfig(conf *VeneurConfig) (ret Server, err error) {
	ret.Hostname = conf.Hostname
	ret.Tags = conf.Tags
	ret.DDHostname = conf.APIHostname
	ret.DDAPIKey = conf.Key

	ret.Stats = Stats // TODO: take ownership of this value and initialize this here

	logrus.WithField("number", conf.NumWorkers).Info("Starting workers")
	ret.Workers = make([]*Worker, conf.NumWorkers)
	for i := range ret.Workers {
		ret.Workers[i] = NewWorker(i+1, conf.Percentiles, conf.HistCounters, conf.SetSize, conf.SetAccuracy)
		ret.Workers[i].Start()
	}

	ret.UDPAddr, err = net.ResolveUDPAddr("udp", conf.UDPAddr)
	if err != nil {
		return
	}
	ret.RcvbufBytes = conf.ReadBufferSizeBytes

	return
}

func (s *Server) HandlePacket(packet []byte, packetPool *sync.Pool) {
	metric, err := ParseMetric(packet)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			logrus.ErrorKey: err,
			"packet":        string(packet),
		}).Error("Could not parse packet")
		s.Stats.Count("packet.error_total", 1, nil, 1.0)
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

func (s *Server) ReadSocket(packetPool *sync.Pool, parserChan chan<- []byte) {
	// each goroutine gets its own socket
	// if the sockets support SO_REUSEPORT, then this will cause the
	// kernel to distribute datagrams across them, for better read
	// performance
	logrus.WithField("address", s.UDPAddr).Info("UDP server listening")
	serverConn, err := NewSocket(s.UDPAddr, s.RcvbufBytes)
	if err != nil {
		// if any goroutine fails to create the socket, we can't really
		// recover, so we just blow up
		// this probably indicates a systemic issue, eg lack of
		// SO_REUSEPORT support
		logrus.WithError(err).Fatal("Error listening for UDP")
	}

	for {
		buf := packetPool.Get().([]byte)
		n, _, err := serverConn.ReadFrom(buf)
		if err != nil {
			logrus.WithError(err).Error("Error reading from UDP")
			continue
		}
		parserChan <- buf[:n]
	}
}
