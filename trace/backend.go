package trace

import (
	"bufio"
	"context"
	"io"
	"net"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/stripe/veneur/protocol"
	"github.com/stripe/veneur/ssf"
)

// DefaultBackoff defaults to 10 milliseconds of initial wait
// time. Subsequent wait times will add this backoff to the time they
// wait.
const DefaultBackoff = 20 * time.Millisecond

// DefaultMaxBackoff defaults to 1 second. No reconnection attempt
// wait interval will be longer than this.
const DefaultMaxBackoff = 1 * time.Second

// DefaultConnectTimeout is to 10 seconds. Any attempt to (re)connect
// to a veneur will take longer than this. If it would take longer,
// the span is discarded.
const DefaultConnectTimeout = 10 * time.Second

// BufferSize is the default size of the SSF buffer per connection. It
// defaults to enough bytes to accomodate the largest SSF span.
const BufferSize int = int(protocol.MaxSSFPacketLength + protocol.SSFFrameLength)

type backendParams struct {
	addr           net.Addr
	backoff        time.Duration
	maxBackoff     time.Duration
	connectTimeout time.Duration
	bufferSize     uint
}

func (p *backendParams) params() *backendParams {
	return p
}

// ClientBackend represents the ability of a client to transport SSF
// spans to a veneur server.
type ClientBackend interface {
	io.Closer

	// SendSync synchronously sends a span to an upstream
	// veneur.
	//
	// On a networked connection, if it encounters a protocol
	// error in sending, it must loop forever, backing off by
	// n*the backoff interval (until it reaches the maximal
	// backoff interval) and tries to reconnect. If SendSync
	// encounters any non-protocol errors (e.g. in serializing the
	// SSF span), it must return them without reconnecting.
	SendSync(ctx context.Context, span *ssf.SSFSpan) error
}

// FlushableClientBackend represents the ability of a client to flush
// any buffered SSF spans over to a veneur server.
type FlushableClientBackend interface {
	ClientBackend

	// FlushSync causes all (potentially) buffered data to be sent to
	// the upstream veneur.
	FlushSync(ctx context.Context) error
}

// networkBackend is a structure that can send an SSF span to a
// destination over a persistent connection, handling
// reconnections. When encountering connection errors, a
// networkBackend will automatically attempt to reconnect and blocks
// until reconnecting succeeds.
//
// Data loss / resiliency to failure
//
// If a networkBackend encounters an error sending a span, it should
// discard the span and attempt to reconnect. This is intended to make
// the networkBackend resilient against "poison pill" spans, at the
// cost of losing that span if there are connection problems, such as
// veneurs getting restarted.
type networkBackend interface {
	ClientBackend

	params() *backendParams
	connection(net.Conn)
}

// packetBackend represents a UDP connection to a veneur server. It
// does no buffering.
type packetBackend struct {
	backendParams
	conn net.Conn
}

func (s *packetBackend) connection(conn net.Conn) {
	s.conn = conn
}

func (s *packetBackend) Close() error {
	if s.conn == nil {
		return nil
	}
	return s.conn.Close()
}

func (s *packetBackend) SendSync(ctx context.Context, span *ssf.SSFSpan) error {
	if s.conn == nil {
		if err := connect(ctx, s); err != nil {
			return err
		}
	}

	data, err := proto.Marshal(span)
	if err != nil {
		return err
	}
	_, err = s.conn.Write(data)
	return err
}

var _ networkBackend = &packetBackend{}

// streamBackend is a backend for streaming connections.
type streamBackend struct {
	backendParams
	conn   net.Conn
	output io.Writer
	buffer *bufio.Writer
}

func connect(ctx context.Context, s networkBackend) error {
	dialer := net.Dialer{}

	params := s.params()
	backoff := params.backoff
	if backoff == 0 {
		backoff = DefaultBackoff
	}

	maxBackoff := params.maxBackoff
	if maxBackoff == 0 {
		maxBackoff = DefaultMaxBackoff
	}

	connectTimeout := params.connectTimeout
	if connectTimeout == 0 {
		connectTimeout = DefaultConnectTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, connectTimeout)
	defer cancel()

	var wait time.Duration
	var conn net.Conn
	var err error
	for {
		conn, err = dialer.DialContext(ctx, params.addr.Network(), params.addr.String())
		if err == nil {
			break
		}

		if ctx.Err() != nil {
			return ctx.Err()
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(wait):
			wait += backoff
			if backoff > maxBackoff {
				wait = maxBackoff
			}
		}
	}
	s.connection(conn)
	return nil
}

func (ds *streamBackend) connection(conn net.Conn) {
	ds.conn = conn
	ds.output = conn
	if ds.bufferSize > 0 {
		ds.buffer = bufio.NewWriterSize(conn, int(ds.bufferSize))
		ds.output = ds.buffer
	}
}

// SendSync on a streamBackend attempts to write the packet on the
// connection to the upstream veneur directly. If it encounters a
// protocol error, SendSync will return the original protocol error once
// the connection is re-established.
func (ds *streamBackend) SendSync(ctx context.Context, span *ssf.SSFSpan) error {
	if ds.conn == nil {
		if err := connect(ctx, ds); err != nil {
			return err
		}
	}
	_, err := protocol.WriteSSF(ds.output, span)
	if err != nil {
		if protocol.IsFramingError(err) {
			_ = ds.conn.Close()
			ds.conn = nil
		}
	}
	return err
}

func (ds *streamBackend) Close() error {
	if ds.conn == nil {
		return nil
	}
	return ds.conn.Close()
}

// FlushSync on a streamBackend flushes the buffer if one exists. If the
// connection was disconnected prior to flushing, FlushSync re-establishes
// it and discards the buffer.
func (ds *streamBackend) FlushSync(ctx context.Context) error {
	if ds.buffer == nil {
		return nil
	}
	if ds.conn == nil {
		if err := connect(ctx, ds); err != nil {
			return err
		}
	}
	err := ds.buffer.Flush()
	if err != nil {
		// buffer is poisoned, and we have no idea if the
		// connection is still valid. We better reconnect.
		_ = ds.conn.Close()
		ds.conn = nil
	}
	return err
}

var _ networkBackend = &streamBackend{}
