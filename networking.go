package veneur

import (
	"crypto/tls"
	"fmt"
	"net"
	"os"
	"sync"

	"github.com/sirupsen/logrus"
	flock "github.com/theckman/go-flock"
)

// StartStatsd spawns a goroutine that listens for metrics in statsd
// format on the address a, and returns the concrete listening
// address. As this is a setup routine, if any error occurs, it
// panics.
func StartStatsd(s *Server, a net.Addr, packetPool *sync.Pool) net.Addr {
	switch addr := a.(type) {
	case *net.UDPAddr:
		return startStatsdUDP(s, addr, packetPool)
	case *net.TCPAddr:
		return startStatsdTCP(s, addr, packetPool)
	default:
		panic(fmt.Sprintf("Can't listen on %v: only TCP and UDP are supported", a))
	}
}

func startStatsdUDP(s *Server, addr *net.UDPAddr, packetPool *sync.Pool) net.Addr {
	reusePort := s.numReaders != 1
	// If we're reusing the port, make sure we're listening on the
	// exact same address always; this is mostly relevant for
	// tests, where port is typically 0 and the initial ListenUDP
	// call results in a contrete port.
	if reusePort {
		sock, err := NewSocket(addr, s.RcvbufBytes, reusePort)
		defer sock.Close()
		if err != nil {
			panic(fmt.Sprintf("couldn't listen on UDP socket %v: %v", addr, err))
		}
		addr = sock.LocalAddr().(*net.UDPAddr)
	}
	addrChan := make(chan net.Addr, 1)
	once := sync.Once{}
	for i := 0; i < s.numReaders; i++ {
		go func() {
			defer func() {
				ConsumePanic(s.Sentry, s.TraceClient, s.Hostname, recover())
			}()
			// each goroutine gets its own socket
			// if the sockets support SO_REUSEPORT, then this will cause the
			// kernel to distribute datagrams across them, for better read
			// performance
			sock, err := NewSocket(addr, s.RcvbufBytes, reusePort)
			if err != nil {
				// if any goroutine fails to create the socket, we can't really
				// recover, so we just blow up
				// this probably indicates a systemic issue, eg lack of
				// SO_REUSEPORT support
				panic(fmt.Sprintf("couldn't listen on UDP socket %v: %v", addr, err))
			}
			// Pass the address that we are listening on
			// back to whoever spawned this goroutine so
			// it can return that address.
			once.Do(func() {
				addrChan <- sock.LocalAddr()
				log.WithField("address", sock.LocalAddr()).
					Info("Listening for statsd metrics on UDP socket")
				close(addrChan)
			})

			s.ReadMetricSocket(sock, packetPool)
		}()
	}
	return <-addrChan
}

func startStatsdTCP(s *Server, addr *net.TCPAddr, packetPool *sync.Pool) net.Addr {
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
			ConsumePanic(s.Sentry, s.TraceClient, s.Hostname, recover())
		}()
		s.ReadTCPSocket(listener)
	}()
	return listener.Addr()
}

// StartSSF starts listening for SSF on an address a, and returns the
// concrete address that the server is listening on.
func StartSSF(s *Server, a net.Addr, tracePool *sync.Pool) net.Addr {
	switch addr := a.(type) {
	case *net.UDPAddr:
		a = startSSFUDP(s, addr, tracePool)
	case *net.UnixAddr:
		_, a = startSSFUnix(s, addr)
	default:
		panic(fmt.Sprintf("Can't listen for SSF on %v: only udp:// & unix:// are supported", a))
	}
	log.WithFields(logrus.Fields{
		"address": a.String(),
		"network": a.Network(),
	}).Info("Listening for SSF traces")
	return a
}

func startSSFUDP(s *Server, addr *net.UDPAddr, tracePool *sync.Pool) net.Addr {
	// TODO: Make this actually use readers / add a predicate
	// function for testing if we should SO_REUSEPORT.
	listener, err := NewSocket(addr, s.RcvbufBytes, s.numReaders > 1)
	if err != nil {
		// if any goroutine fails to create the socket, we can't really
		// recover, so we just blow up
		// this probably indicates a systemic issue, eg lack of
		// SO_REUSEPORT support
		panic(fmt.Sprintf("couldn't listen on UDP socket %v: %v", addr, err))
	}
	go func() {
		defer func() {
			ConsumePanic(s.Sentry, s.TraceClient, s.Hostname, recover())
		}()
		s.ReadSSFPacketSocket(listener, tracePool)
	}()
	return listener.LocalAddr()
}

// startSSFUnix starts listening for connections that send framed SSF
// spans on a UNIX domain socket address. It does so until the
// server's shutdown socket is closed. startSSFUnix returns a channel
// that is closed once the listener has terminated.
func startSSFUnix(s *Server, addr *net.UnixAddr) (<-chan struct{}, net.Addr) {
	done := make(chan struct{})
	if addr.Network() != "unix" {
		panic(fmt.Sprintf("Can't listen for SSF on %v: only udp:// and unix:// addresses are supported", addr))
	}
	// ensure we are the only ones locking this socket:
	lockname := fmt.Sprintf("%s.lock", addr.String())
	lock := flock.NewFlock(lockname)
	locked, err := lock.TryLock()
	if err != nil {
		panic(fmt.Sprintf("Could not acquire the lock %q to listen on %v: %v", lockname, addr, err))
	}
	if !locked {
		panic(fmt.Sprintf("Lock file %q for %v is in use by another process already", lockname, addr))
	}
	// We have the exclusive use of the socket, clear away any old sockets and listen:
	_ = os.Remove(addr.String())
	listener, err := net.ListenUnix(addr.Network(), addr)
	if err != nil {
		panic(fmt.Sprintf("Couldn't listen on UNIX socket %v: %v", addr, err))
	}

	// Make the socket connectable by everyone with access to the socket pathname:
	err = os.Chmod(addr.String(), 0666)
	if err != nil {
		panic(fmt.Sprintf("Couldn't set permissions on %v: %v", addr, err))
	}

	go func() {
		conns := make(chan net.Conn)
		go func() {
			defer func() {
				lock.Unlock()
				close(done)
			}()
			for {
				conn, err := listener.AcceptUnix()
				if err != nil {
					select {
					case <-s.shutdown:
						// occurs when cleanly shutting down the server e.g. in tests; ignore errors
						log.WithError(err).Info("Ignoring Accept error while shutting down")
						return
					default:
						log.WithError(err).Fatal("Unix accept failed")
					}
				}
				conns <- conn
			}
		}()
		for {
			select {
			case conn := <-conns:
				go s.ReadSSFStreamSocket(conn)
			case <-s.shutdown:
				listener.Close()
				return
			}
		}
	}()
	return done, listener.Addr()
}
