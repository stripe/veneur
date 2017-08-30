package veneur

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/url"
	"sync"

	"github.com/Sirupsen/logrus"
)

// ResolveAddr takes a URL-style listen address specification,
// resolves it and returns a net.Addr that corresponds to the
// string. If any error (in URL decoding, destructuring or resolving)
// occurs, ResolveAddr returns the respective error.
//
// Valid address examples are:
//   udp6://127.0.0.1:8000
//   unix:///tmp/foo.sock
//   tcp://127.0.0.1:9002
func ResolveAddr(str string) (net.Addr, error) {
	u, err := url.Parse(str)
	if err != nil {
		return nil, err
	}
	switch u.Scheme {
	case "unix", "unixgram", "unixpacket":
		addr, err := net.ResolveUnixAddr(u.Scheme, u.Path)
		if err != nil {
			return nil, err
		}
		return addr, nil
	case "tcp6", "tcp4", "tcp":
		addr, err := net.ResolveTCPAddr(u.Scheme, u.Host)
		if err != nil {
			return nil, err
		}
		return addr, nil
	case "udp6", "udp4", "udp":
		addr, err := net.ResolveUDPAddr(u.Scheme, u.Host)
		if err != nil {
			return nil, err
		}
		return addr, nil
	}
	return nil, fmt.Errorf("unknown address family %q on address %q", u.Scheme, u.String())
}

// StartStatsd spawns a goroutine that listens for metrics in statsd
// format on the address a. As this is a setup routine, if any error
// occurs, it panics.
func StartStatsd(s *Server, a net.Addr, packetPool *sync.Pool) {
	switch addr := a.(type) {
	case *net.UDPAddr:
		startStatsdUDP(s, addr, packetPool)
	case *net.TCPAddr:
		startStatsdTCP(s, addr, packetPool)
	default:
		panic(fmt.Sprintf("Can't listen on %v: only TCP and UDP are supported", a))
	}
}

func startStatsdUDP(s *Server, addr *net.UDPAddr, packetPool *sync.Pool) {
	for i := 0; i < s.numReaders; i++ {
		go func() {
			defer func() {
				ConsumePanic(s.Sentry, s.Statsd, s.Hostname, recover())
			}()
			// each goroutine gets its own socket
			// if the sockets support SO_REUSEPORT, then this will cause the
			// kernel to distribute datagrams across them, for better read
			// performance
			sock, err := NewSocket(addr, s.RcvbufBytes, s.numReaders != 1)
			if err != nil {
				// if any goroutine fails to create the socket, we can't really
				// recover, so we just blow up
				// this probably indicates a systemic issue, eg lack of
				// SO_REUSEPORT support
				panic(fmt.Sprintf("couldn't listen on UDP socket %v: %v", addr, err))
			}
			log.WithField("address", addr).Info("Listening for statsd metrics on UDP socket")
			s.ReadMetricSocket(sock, packetPool)
		}()
	}
}

func startStatsdTCP(s *Server, addr *net.TCPAddr, packetPool *sync.Pool) {
	var listener net.Listener
	var err error

	listener, err = net.ListenTCP("tcp", addr)
	if err != nil {
		panic(fmt.Sprintf("couldn't listen on TCP socket %v: %v", addr, err))
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
	}).Info("Listening for statsd metrics on TCP socket")

	go func() {
		defer func() {
			ConsumePanic(s.Sentry, s.Statsd, s.Hostname, recover())
		}()
		s.ReadTCPSocket(listener)
	}()
}

func StartSSF(s *Server, a net.Addr, tracePool *sync.Pool) {
	switch addr := a.(type) {
	case *net.UDPAddr:
		startSSFUDP(s, addr, tracePool)
	default:
		panic(fmt.Sprintf("Can't listen for SSF on %v: only UDP is supported", a))
	}
	log.WithFields(logrus.Fields{
		"address": a.String(),
		"network": a.Network(),
	}).Info("Listening for SSF traces")
}

func startSSFUDP(s *Server, addr *net.UDPAddr, tracePool *sync.Pool) {
	// if we want to use multiple readers, make reuseport a parameter, like ReadMetricSocket.
	listener, err := NewSocket(addr, s.RcvbufBytes, false)
	if err != nil {
		// if any goroutine fails to create the socket, we can't really
		// recover, so we just blow up
		// this probably indicates a systemic issue, eg lack of
		// SO_REUSEPORT support
		panic(fmt.Sprintf("couldn't listen on UDP socket %v: %v", addr, err))
	}
	go func() {
		defer func() {
			ConsumePanic(s.Sentry, s.Statsd, s.Hostname, recover())
		}()
		s.ReadTraceSocket(listener, tracePool)
	}()
}
