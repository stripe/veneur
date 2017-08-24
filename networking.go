package veneur

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/url"
	"strings"
	"sync"

	"github.com/Sirupsen/logrus"
)

// ListeningAddr implements the net.Addr interface and gets
// deserialized from the YAML config file by interpreting it as a URL,
// where the Scheme corresponds to the "net" argument to net.Listen,
// and the host&port or path are the "laddr" arg.
//
// Valid address examples are:
//   - udp6://127.0.0.1:8000
//   - unix:///tmp/foo.sock
//   - tcp://127.0.0.1:9002
type ListeningAddr struct {
	address     string
	network     string
	startStatsd StatsdStarter
}

var _ net.Addr = &ListeningAddr{}

func (a *ListeningAddr) Network() string {
	return a.network
}

func (a *ListeningAddr) String() string {
	return a.address
}

func (a *ListeningAddr) FromURL(u *url.URL) error {
	switch u.Scheme {
	case "unix":
		a.address = u.Path
	case "unixgram", "unixpacket":
		a.address = u.Path
	case "tcp6", "tcp4", "tcp":
		a.address = u.Host
		a.startStatsd = startStatsdTCP
	case "udp6", "udp4", "udp":
		a.address = u.Host
		a.startStatsd = startStatsdUDP
	default:
		return fmt.Errorf("unknown address family %q on address %q", u.Scheme, u.String())
	}
	a.network = u.Scheme
	return nil
}

func (a ListeningAddr) StartStatsd(s *Server, packetPool *sync.Pool) {
	if a.startStatsd == nil {
		log.WithFields(logrus.Fields{
			"address": a.address,
			"network": a.network,
		}).Fatal("I do not know how to listen for statsd packets on this kind of socket")
	}
	a.startStatsd(s, a, packetPool)
}

func (a ListeningAddr) Resolve() (net.Addr, error) {
	switch {
	case strings.HasPrefix(a.network, "udp"):
		return net.ResolveUDPAddr(a.network, a.address)
	case strings.HasPrefix(a.network, "tcp"):
		return net.ResolveTCPAddr(a.network, a.address)
	case strings.HasPrefix(a.network, "unix"):
		return net.ResolveUnixAddr(a.network, a.address)
	}
	return nil, fmt.Errorf("can't tell how to resolve address for network %q", a.network)
}

// StatsdStarter is a function (to be called in a goroutine) that will
// loop forever and read statsd packets from a socket that it creates
// from the address passed.  Errors in listening are fatal, so the
// function is expected to either loop forever, handling its own
// errors, or trigger a fatal error.
type StatsdStarter func(s *Server, addr ListeningAddr, packetPool *sync.Pool)

func startStatsdUDP(s *Server, laddr ListeningAddr, packetPool *sync.Pool) {
	addr, err := laddr.Resolve()
	if err != nil {
		log.WithError(err).WithField("address", addr.String()).
			Fatal("Couldn't resolve UDP listening address")
	}
	for i := 0; i < s.numReaders; i++ {
		go func() {
			defer func() {
				ConsumePanic(s.Sentry, s.Statsd, s.Hostname, recover())
			}()
			// each goroutine gets its own socket
			// if the sockets support SO_REUSEPORT, then this will cause the
			// kernel to distribute datagrams across them, for better read
			// performance
			sock, err := NewSocket(addr.(*net.UDPAddr), s.RcvbufBytes, s.numReaders != 1)
			if err != nil {
				// if any goroutine fails to create the socket, we can't really
				// recover, so we just blow up
				// this probably indicates a systemic issue, eg lack of
				// SO_REUSEPORT support
				log.WithError(err).Fatal("Error listening for UDP metrics")
			}
			log.WithField("address", addr).Info("Listening for UDP metrics")
			s.ReadMetricSocket(sock, packetPool)
		}()
	}
}

func startStatsdTCP(s *Server, laddr ListeningAddr, packetPool *sync.Pool) {
	var listener net.Listener

	addr, err := laddr.Resolve()
	if err != nil {
		log.WithError(err).WithField("address", addr.String()).
			Fatal("Couldn't resolve TCP listening address")
	}
	listener, err = net.ListenTCP("tcp", addr.(*net.TCPAddr))
	if err != nil {
		logrus.WithError(err).Fatal("Error listening for TCP connections")
	}

	go func() {
		<-s.shutdown
		// TODO: the socket is in use until there are no goroutines blocked in Accept
		// we should wait until the accepting goroutine exits
		err := listener.Close()
		if err != nil {
			log.WithError(err).Warn("Ignoring error closing TCP listener")
		}
	}()

	mode := "unencrypted"
	if s.tlsConfig != nil {
		// wrap the listener with TLS
		listener = tls.NewListener(listener, s.tlsConfig)
		if s.tlsConfig.ClientAuth == tls.RequireAndVerifyClientCert {
			mode = "authenticated"
		} else {
			mode = "encrypted"
		}
	}

	log.WithFields(logrus.Fields{
		"address": addr, "mode": mode,
	}).Info("Listening for TCP connections")

	go func() {
		defer func() {
			ConsumePanic(s.Sentry, s.Statsd, s.Hostname, recover())
		}()
		s.ReadTCPSocket(listener)
	}()
}
