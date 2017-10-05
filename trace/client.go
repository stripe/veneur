package trace

import (
	"context"
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/stripe/veneur/protocol"
	"github.com/stripe/veneur/ssf"
)

func init() {
	cl, err := NewClient(DefaultVeneurAddress)
	if err != nil {
		return
	}
	DefaultClient = cl
}

// op is a function invoked on a backend, such as sending a span
// synchronously, flushing the backend buffer, or closing the backend.
type op func(context.Context, backend)

// Client is a Client that sends traces to Veneur. It represents a
// pump for span packets from user code to the network (whether it be
// UDP or streaming sockets, with or without buffers).
//
// Structure
//
// A Client is composed of two parts (each with its own purpose): A
// serialization part providing backpressure (the front end) and a
// restartable network connection (the backend).
type Client struct {
	backend
	cancel context.CancelFunc
	ops    chan op
}

// Close tears down the entire client. It waits until the backend has
// closed the network connection (if one was established) and returns
// any error from closing the connection.
func (c *Client) Close() error {
	ch := make(chan error)
	c.cancel()
	c.ops <- func(ctx context.Context, s backend) {
		ch <- s.Close()
	}
	close(c.ops)
	return <-ch
}

func (c *Client) run(ctx context.Context) {
	for {
		do, ok := <-c.ops
		if !ok {
			return
		}
		do(ctx, c.backend)
	}
}

// ClientParam is an option for NewClient. Its implementation borrows
// from Dave Cheney's functional options API
// (https://dave.cheney.net/2014/10/17/functional-options-for-friendly-apis).
type ClientParam func(*Client) error

// Buffered sets the client to be buffered with the default buffer
// size (enough to accomodate a single, maximum-sized SSF frame,
// currently about 16MB).
func Buffered(cl *Client) error {
	return BufferedSize(uint(BufferSize))(cl)
}

// BufferedSize indicates that a client should have a buffer size bytes
// large.
func BufferedSize(size uint) ClientParam {
	return func(cl *Client) error {
		cl.backend.params().bufferSize = size
		return nil
	}
}

// Capacity indicates how many spans a client's channel should
// accomodate.
func Capacity(n uint) ClientParam {
	return func(cl *Client) error {
		cl.backend.params().cap = n
		return nil
	}
}

// BackoffTime sets the time increment that backoff time is increased
// (linearly) between every reconnection attempt the backend makes. If
// this option is not used, the backend uses DefaultBackoff.
func BackoffTime(t time.Duration) ClientParam {
	return func(cl *Client) error {
		cl.backend.params().backoff = t
		return nil
	}
}

// MaxBackoffTime sets the maximum time duration waited between
// reconnection attempts. If this option is not used, the backend uses
// DefaultMaxBackoff.
func MaxBackoffTime(t time.Duration) ClientParam {
	return func(cl *Client) error {
		cl.backend.params().maxBackoff = t
		return nil
	}
}

// ConnectTimeout sets the maximum total amount of time a client
// backend spends trying to establish a connection to a veneur. If a
// connection can not be established after this timeout has expired
// (counting from the time the connection is first attempted), the
// span is discarded. If this option is not used, the backend uses
// DefaultConnectTimeout.
func ConnectTimeout(t time.Duration) ClientParam {
	return func(cl *Client) error {
		cl.backend.params().connectTimeout = t
		return nil
	}
}

// NewClient constructs a new client that will attempt to connect
// to addrStr (an address in veneur URL format) using the parameters
// in opts. It returns the constructed client or an error.
func NewClient(addrStr string, opts ...ClientParam) (*Client, error) {
	addr, err := protocol.ResolveAddr(addrStr)
	if err != nil {
		return nil, err
	}
	cl := &Client{}
	switch addr := addr.(type) {
	case *net.UDPAddr:
		cl.backend = &packetBackend{}
	case *net.UnixAddr:
		cl.backend = &streamBackend{}
	default:
		return nil, fmt.Errorf("can not connect to %v addresses", addr.Network())
	}
	params := cl.backend.params()
	params.addr = addr
	params.cap = 1
	for _, opt := range opts {
		if err = opt(cl); err != nil {
			return nil, err
		}
	}
	ch := make(chan op, params.cap)
	cl.ops = ch
	ctx := context.Background()
	ctx, cl.cancel = context.WithCancel(ctx)
	go cl.run(ctx)
	return cl, nil
}

// DefaultClient is the client that trace recording happens on by
// default. If it is nil, no recording happens and ErrNoClient is
// returned from recording functions.
//
// Note that it is not safe to set this variable concurrently with
// other goroutines that use the DefaultClient.
var DefaultClient *Client

// DefaultVeneurAddress is the address that a reasonable veneur should
// listen on. Currently it defaults to UDP port 8128.
const DefaultVeneurAddress string = "udp://127.0.0.1:8128"

// ErrNoClient indicates that no client is yet initialized.
var ErrNoClient = errors.New("client is not initialized")

// ErrWouldBlock indicates that a client is not able to send a span at
// the current time.
var ErrWouldBlock = errors.New("sending span would block")

// Record instructs the client to serialize and send a span. It does
// not wait for a delivery attempt, instead the Client will send the
// result from serializing and submitting the span to the channel
// done, if it is non-nil.
//
// Record returns ErrNoClient if client is nil and ErrWouldBlock if
// the client is not able to accomodate another span.
func Record(cl *Client, span *ssf.SSFSpan, done chan<- error) error {
	if cl == nil {
		return ErrNoClient
	}

	op := func(ctx context.Context, s backend) {
		err := s.sendSync(ctx, span)
		if done != nil {
			done <- err
		}
	}
	select {
	case cl.ops <- op:
		return nil
	default:
	}
	return ErrWouldBlock
}

// Flush instructs a client to flush to the upstream veneur all the
// spans that were serialized up until the moment that the flush was
// received. It will wait until the flush is completed (including all
// reconnection attempts), and return any error caused by flushing the
// buffer.
//
// Flush returns ErrNoClient if client is nil and ErrWouldBlock if the
// client is not able to take more requests.
func Flush(cl *Client) error {
	ch := make(chan error)
	err := FlushAsync(cl, ch)
	if err != nil {
		return err
	}
	return <-ch
}

// FlushAsync instructs a buffered client to flush to the upstream
// veneur all the spans that were serialized up until the moment that
// the flush was received. Once the client has completed the flush,
// any error (or nil) is sent down the error channel.
//
// FlushAsync returns ErrNoClient if client is nil and ErrWouldBlock
// if the client is not able to take more requests.
func FlushAsync(cl *Client, ch chan<- error) error {
	if cl == nil {
		return ErrNoClient
	}
	op := func(ctx context.Context, s backend) {
		err := s.flushSync(ctx)
		if ch != nil {
			ch <- err
		}
	}
	select {
	case cl.ops <- op:
		return nil
	default:
	}
	return ErrWouldBlock
}
