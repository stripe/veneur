package statsd

import (
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"os"
	"sync"

	"github.com/sirupsen/logrus"
	"github.com/stripe/veneur/v14"
	"github.com/stripe/veneur/v14/protocol"
	"github.com/stripe/veneur/v14/sources"
	"github.com/stripe/veneur/v14/util"
	"github.com/theckman/go-flock"
)

type StatsdSourceConfig struct {
	ListenAddresses []util.Url `yaml:"listen_addresses"`
}

type StatsdSource struct {
	listenAddresses []util.Url
	logger          *logrus.Entry
	name            string
	packetPool      *sync.Pool
	server          *veneur.Server
}

func ParseConfig(
	name string, config interface{},
) (veneur.ParsedSourceConfig, error) {
	sourceConfig := StatsdSourceConfig{}
	err := util.DecodeConfig(name, config, &sourceConfig)
	if err != nil {
		return nil, err
	}

	return sourceConfig, nil
}

func Create(
	server *veneur.Server, name string, logger *logrus.Entry,
	sourceConfig veneur.ParsedSourceConfig,
) (sources.Source, error) {
	statsdSourceConfig, ok := sourceConfig.(StatsdSourceConfig)
	if !ok {
		return nil, errors.New("invalid sink config type")
	}

	return StatsdSource{
		listenAddresses: statsdSourceConfig.ListenAddresses,
		logger:          logger,
		name:            name,
	}, nil
}

func (source StatsdSource) Name() string {
	return source.name
}

func (source StatsdSource) Start(ingest sources.Ingest) error {
	resolvedAddresses := make([]net.Addr, len(source.listenAddresses))
	for index, address := range source.listenAddresses {
		var err error
		resolvedAddress, err := protocol.ResolveAddr(address.Value)
		if err != nil {
			return err
		}

		switch addr := resolvedAddress.(type) {
		case *net.UDPAddr:
			source.startStatsdUDP(addr)
		case *net.TCPAddr:
			source.startStatsdTCP(addr)
		case *net.UnixAddr:
			source.startStatsdUnix(addr)
		default:
			return fmt.Errorf(
				"can't listen on %v: only TCP, UDP and unixgram:// are supported", addr)
		}
	}

	return nil
}

func (source *StatsdSource) startStatsdUDP(addr *net.UDPAddr) {
	startProcessingOnUDP(
		source.server, "statsd", addr, source.packetPool,
		source.server.ReadMetricSocket)
}

func (source *StatsdSource) startStatsdTCP(
	s *veneur.Server, addr *net.TCPAddr,
) net.Addr {
	listener, err := net.ListenTCP("tcp", addr)
	if err != nil {
		panic(fmt.Sprintf("couldn't listen on TCP socket %v: %v", addr, err))
	}

	go func() {
		<-source.server.shutdown
		// TODO: the socket is in use until there are no goroutines blocked in Accept
		// we should wait until the accepting goroutine exits
		err := listener.Close()
		if err != nil {
			source.logger.WithError(err).Warn("Ignoring error closing TCP listener")
		}
	}()

	mode := "unencrypted"
	if source.server.tlsConfig != nil {
		// wrap the listener with TLS
		listener = tls.NewListener(listener, source.server.tlsConfig)
		if source.server.tlsConfig.ClientAuth == tls.RequireAndVerifyClientCert {
			mode = "authenticated"
		} else {
			mode = "encrypted"
		}
	}

	source.logger.WithFields(logrus.Fields{
		"address": addr, "mode": mode,
	}).Info("Listening for statsd metrics on TCP socket")

	go func() {
		defer func() {
			ConsumePanic(s.TraceClient, s.Hostname, recover())
		}()
		s.ReadTCPSocket(listener)
	}()
	return listener.Addr()
}

// startStatsdUnix starts listening for datagram statsd metric packets
// on a UNIX domain socket address. It does so until the
// server's shutdown socket is closed. startStatsdUnix returns a channel
// that is closed once the listening connection has terminated.
func (source *StatsdSource) startStatsdUnix(
	s *Server, addr *net.UnixAddr,
) (<-chan struct{}, net.Addr) {
	done := make(chan struct{})

	isAbstractSocket := isAbstractSocket(addr)

	// ensure we are the only ones locking this socket if it's a file:
	var lock *flock.Flock
	if !isAbstractSocket {
		lock = acquireLockForSocket(addr)
	}
	fmt.Println(addr.String())
	conn, err := net.ListenUnixgram(addr.Network(), addr)
	if err != nil {
		panic(fmt.Sprintf("Couldn't listen on UNIX socket %v: %v", addr, err))
	}

	if rcvbufsize := s.RcvbufBytes; rcvbufsize != 0 {
		if err := conn.SetReadBuffer(rcvbufsize); err != nil {
			panic(fmt.Sprintf("Couldn't set buffer size for UNIX socket %v: %v", addr, err))
		}
	}

	// Make the socket connectable by everyone with access to the socket pathname:
	if !isAbstractSocket {
		err = os.Chmod(addr.String(), 0666)
		if err != nil {
			panic(fmt.Sprintf("Couldn't set permissions on %v: %v", addr, err))
		}
	}

	go func() {
		defer func() {
			if !isAbstractSocket {
				lock.Unlock()
			}
			close(done)
		}()
		for {
			_, open := <-s.shutdown
			// occurs when cleanly shutting down the server e.g. in tests; ignore errors
			if !open {
				conn.Close()
				return
			}
		}
	}()
	for i := 0; i < s.numReaders; i++ {
		go s.ReadStatsdDatagramSocket(conn, source.packetPool)
	}
	return done, addr
}

func (source StatsdSource) Stop() {
	// Do nothing.
}
