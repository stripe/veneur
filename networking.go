package veneur

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"os"
	"strings"
	"sync"

	"github.com/sirupsen/logrus"
	"github.com/stripe/veneur/v14/protocol/dogstatsd"
	"github.com/stripe/veneur/v14/ssf"
	flock "github.com/theckman/go-flock"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
)

type UdpMetricsSource struct {
	logger *logrus.Entry
}

// StartStatsd spawns a goroutine that listens for metrics in statsd
// format on the address a, and returns the concrete listening
// address. As this is a setup routine, if any error occurs, it
// panics.
func (source *UdpMetricsSource) StartStatsd(
	s *Server, a net.Addr, packetPool *sync.Pool,
) net.Addr {
	switch addr := a.(type) {
	case *net.UDPAddr:
		return source.startStatsdUDP(s, addr, packetPool)
	case *net.TCPAddr:
		return source.startStatsdTCP(s, addr, packetPool)
	case *net.UnixAddr:
		_, b := source.startStatsdUnix(s, addr, packetPool)
		return b
	default:
		panic(fmt.Sprintf("Can't listen on %v: only TCP, UDP and unixgram:// are supported", a))
	}
}

// udpProcessor is a function that reads packets from a socket, using
// the pool provided.
type udpProcessor func(net.PacketConn, *sync.Pool)

// startProcessingOnUDP starts network num_readers listeners on the
// given address in one goroutine each, using the passed pool. When
// the listener is established, it starts the udpProcessor with the
// listener.
func startProcessingOnUDP(
	s *Server, protocol string, addr *net.UDPAddr, pool *sync.Pool,
	proc udpProcessor,
) net.Addr {
	reusePort := s.numReaders != 1
	// If we're reusing the port, make sure we're listening on the
	// exact same address always; this is mostly relevant for
	// tests, where port is typically 0 and the initial ListenUDP
	// call results in a contrete port.
	if reusePort {
		sock, err := NewSocket(addr, s.RcvbufBytes, reusePort)
		if err != nil {
			panic(fmt.Sprintf("couldn't listen on UDP socket %v: %v", addr, err))
		}
		defer sock.Close()
		addr = sock.LocalAddr().(*net.UDPAddr)
	}
	addrChan := make(chan net.Addr, 1)
	once := sync.Once{}
	for i := 0; i < s.numReaders; i++ {
		go func() {
			defer func() {
				ConsumePanic(s.TraceClient, s.Hostname, recover())
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
				s.logger.WithFields(logrus.Fields{
					"address":   sock.LocalAddr(),
					"protocol":  protocol,
					"listeners": s.numReaders,
				}).Info("Listening on UDP address")
				close(addrChan)
			})

			proc(sock, pool)
		}()
	}
	return <-addrChan
}

func (source *UdpMetricsSource) startStatsdUDP(
	s *Server, addr *net.UDPAddr, packetPool *sync.Pool,
) net.Addr {
	return startProcessingOnUDP(
		s, "statsd", addr, packetPool, s.ReadMetricSocket)
}

func (source *UdpMetricsSource) startStatsdTCP(
	s *Server, addr *net.TCPAddr, packetPool *sync.Pool,
) net.Addr {
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
			source.logger.WithError(err).Warn("Ignoring error closing TCP listener")
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
func (source *UdpMetricsSource) startStatsdUnix(s *Server, addr *net.UnixAddr, packetPool *sync.Pool) (<-chan struct{}, net.Addr) {
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
		go s.ReadStatsdDatagramSocket(conn, packetPool)
	}
	return done, addr
}

type SsfMetricsSource struct {
	logger *logrus.Entry
}

// StartSSF starts listening for SSF on an address a, and returns the
// concrete address that the server is listening on.
func (source *SsfMetricsSource) StartSSF(
	s *Server, a net.Addr, tracePool *sync.Pool,
) net.Addr {
	switch addr := a.(type) {
	case *net.UDPAddr:
		a = source.startSSFUDP(s, addr, tracePool)
	case *net.UnixAddr:
		_, a = source.startSSFUnix(s, addr)
	default:
		panic(fmt.Sprintf("Can't listen for SSF on %v: only udp:// & unix:// are supported", a))
	}
	source.logger.WithFields(logrus.Fields{
		"address": a.String(),
		"network": a.Network(),
	}).Info("Listening for SSF traces")
	return a
}

func (source *SsfMetricsSource) startSSFUDP(
	s *Server, addr *net.UDPAddr, tracePool *sync.Pool,
) net.Addr {
	return startProcessingOnUDP(
		s, "ssf", addr, tracePool, s.ReadSSFPacketSocket)
}

// startSSFUnix starts listening for connections that send framed SSF
// spans on a UNIX domain socket address. It does so until the
// server's shutdown socket is closed. startSSFUnix returns a channel
// that is closed once the listener has terminated.
func (source *SsfMetricsSource) startSSFUnix(
	s *Server, addr *net.UnixAddr,
) (<-chan struct{}, net.Addr) {
	done := make(chan struct{})
	if addr.Network() != "unix" {
		panic(fmt.Sprintf("Can't listen for SSF on %v: only udp:// and unix:// addresses are supported", addr))
	}

	isAbstractSocket := isAbstractSocket(addr)

	// ensure we are the only ones locking this socket if it's a file:
	var lock *flock.Flock
	if !isAbstractSocket {
		lock = acquireLockForSocket(addr)
	}

	listener, err := net.ListenUnix(addr.Network(), addr)
	if err != nil {
		panic(fmt.Sprintf("Couldn't listen on UNIX socket %v: %v", addr, err))
	}

	// Make the socket connectable by everyone with access to the socket pathname:
	if !isAbstractSocket {
		err = os.Chmod(addr.String(), 0666)
		if err != nil {
			panic(fmt.Sprintf("Couldn't set permissions on %v: %v", addr, err))
		}
	}

	go func() {
		conns := make(chan net.Conn)
		go func() {
			defer func() {
				if !isAbstractSocket {
					lock.Unlock()
				}
				close(done)
			}()
			for {
				conn, err := listener.AcceptUnix()
				if err != nil {
					select {
					case <-s.shutdown:
						// occurs when cleanly shutting down the server e.g. in tests;
						// ignore errors
						source.logger.WithError(err).
							Info("Ignoring Accept error while shutting down")
						return
					default:
						source.logger.WithError(err).Fatal("Unix accept failed")
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

type GrpcMetricsSource struct {
	logger *logrus.Entry
}

// StartGRPC starts listening for spans over HTTP
func (source *GrpcMetricsSource) StartGRPC(s *Server, a net.Addr) net.Addr {
	switch addr := a.(type) {
	case *net.TCPAddr:
		_, a = source.startGRPCTCP(s, addr)
	default:
		panic(fmt.Sprintf("Can't listen for GRPC on %s because it's not tcp://", a))
	}
	source.logger.WithFields(logrus.Fields{
		"address": a.String(),
		"network": a.Network(),
	}).Info("Listening for GRPC stats data")
	return a
}

type grpcStatsServer struct {
	server *Server
}

//This is the function that fulfils the ssf server proto
func (grpcsrv *grpcStatsServer) SendPacket(ctx context.Context, packet *dogstatsd.DogstatsdPacket) (*dogstatsd.Empty, error) {
	//We use processMetricPacket instead of handleMetricPacket because process can split the byte array into multiple packets if needed
	grpcsrv.server.processMetricPacket(len(packet.GetPacketBytes()), packet.GetPacketBytes(), nil, DOGSTATSD_GRPC)
	return &dogstatsd.Empty{}, nil
}

// This is the function that fulfils the dogstatsd server proto
func (grpcsrv *grpcStatsServer) SendSpan(ctx context.Context, span *ssf.SSFSpan) (*ssf.Empty, error) {
	grpcsrv.server.handleSSF(span, "packet", SSF_GRPC)
	return &ssf.Empty{}, nil
}

func (source *GrpcMetricsSource) startGRPCTCP(
	s *Server, addr *net.TCPAddr,
) (*grpc.Server, net.Addr) {
	listener, err := net.Listen("tcp", addr.String())
	if err != nil {
		source.logger.Fatalf("failed to listen: %v", err)
	}
	var grpcServer *grpc.Server
	mode := "unencrypted"
	if s.tlsConfig != nil {
		tlsCreds := credentials.NewTLS(s.tlsConfig)
		if s.tlsConfig.ClientAuth == tls.RequireAndVerifyClientCert {
			mode = "authenticated"
		} else {
			mode = "encrypted"
		}
		grpcServer = grpc.NewServer(grpc.Creds(tlsCreds))
	} else {
		grpcServer = grpc.NewServer()
	}
	healthServer := health.NewServer()
	healthServer.SetServingStatus("veneur", grpc_health_v1.HealthCheckResponse_SERVING)

	statsServer := &grpcStatsServer{server: s}

	grpc_health_v1.RegisterHealthServer(grpcServer, healthServer)
	ssf.RegisterSSFGRPCServer(grpcServer, statsServer)
	dogstatsd.RegisterDogstatsdGRPCServer(grpcServer, statsServer)

	source.logger.WithFields(logrus.Fields{
		"address": addr, "mode": mode,
	}).Info("Listening for metrics on GRPC socket")
	go grpcServer.Serve(listener)
	return grpcServer, listener.Addr()
}

// Acquires exclusive use lock for a given socket file and returns the lock
// Panic's if unable to acquire lock
func acquireLockForSocket(addr *net.UnixAddr) *flock.Flock {
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
	return lock
}

func isAbstractSocket(addr *net.UnixAddr) bool {
	return strings.HasPrefix(addr.String(), "@")
}
