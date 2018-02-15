package trace

import (
	"context"
	"errors"
	"fmt"
	"net"
	"time"

	"sync/atomic"

	"github.com/DataDog/datadog-go/statsd"
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
type op func(context.Context, ClientBackend)

// Client is a Client that sends traces to Veneur over the network. It
// represents a pump for span packets from user code to the network
// (whether it be UDP or streaming sockets, with or without buffers).
//
// Structure
//
// A Client is composed of two parts (each with its own purpose): A
// serialization part providing backpressure (the front end) and a
// backend (which is called on a single goroutine).
type Client struct {
	backend ClientBackend
	cap     uint
	cancel  context.CancelFunc
	flush   func(context.Context)
	ops     chan op

	failedFlushes     int64
	successfulFlushes int64
	failedRecords     int64
	successfulRecords int64
}

// Close tears down the entire client. It waits until the backend has
// closed the network connection (if one was established) and returns
// any error from closing the connection.
func (c *Client) Close() error {
	ch := make(chan error)
	c.cancel()
	c.ops <- func(ctx context.Context, s ClientBackend) {
		ch <- s.Close()
	}
	close(c.ops)
	return <-ch
}

func (c *Client) run(ctx context.Context) {
	if c.flush != nil {
		go c.flush(ctx)
	}
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
//
// Unless otherwise noted, ClientParams only apply to networked
// backends (i.e., those used by NewClient). Using them on
// non-network-backed clients will return ErrClientNotNetworked on
// client creation.
type ClientParam func(*Client) error

// ErrClientNotNetworked indicates that the client being constructed
// does not support options relevant only to networked clients.
var ErrClientNotNetworked = fmt.Errorf("client is not using a network backend")

// Capacity indicates how many spans a client's channel should
// accommodate. This parameter can be used on both generic and
// networked backends.
func Capacity(n uint) ClientParam {
	return func(cl *Client) error {
		cl.cap = n
		return nil
	}
}

// Buffered sets the client to be buffered with the default buffer
// size (enough to accomodate a single, maximum-sized SSF frame,
// currently about 16MB).
//
// When using buffered clients, since buffers tend to be large and SSF
// packets are fairly small, it might appear as if buffered clients
// are not sending any spans at all.
//
// Code using a buffered client should ensure that the client gets
// flushed in a reasonable interval, either by calling Flush manually
// in an appropriate goroutine, or by also using the FlushInterval
// functional option.
func Buffered(cl *Client) error {
	return BufferedSize(uint(BufferSize))(cl)
}

// BufferedSize indicates that a client should have a buffer size
// bytes large. See the note on the Buffered option about flushing the
// buffer.
func BufferedSize(size uint) ClientParam {
	return func(cl *Client) error {
		if nb, ok := cl.backend.(networkBackend); ok {
			nb.params().bufferSize = size
			return nil
		}
		return ErrClientNotNetworked
	}
}

// FlushInterval sets up a buffered client to perform one synchronous
// flush per time interval in a new goroutine. The goroutine closes
// down when the Client's Close method is called.
//
// This uses a time.Ticker to trigger the flush, so will not trigger
// multiple times if flushing should be slower than the trigger
// interval.
func FlushInterval(interval time.Duration) ClientParam {
	t := time.NewTicker(interval)
	return FlushChannel(t.C, t.Stop)
}

// FlushChannel sets up a buffered client to perform one synchronous
// flush any time the given channel has a Time element ready. When the
// Client is closed, FlushWith invokes the passed stop function.
//
// This functional option is mostly useful for tests; code intended to
// be used in production should rely on FlushInterval instead, as
// time.Ticker is set up to deal with slow flushes.
func FlushChannel(ch <-chan time.Time, stop func()) ClientParam {
	return func(cl *Client) error {
		if _, ok := cl.backend.(networkBackend); !ok {
			return ErrClientNotNetworked
		}
		cl.flush = func(ctx context.Context) {
			defer stop()
			for {
				select {
				case <-ch:
					_ = Flush(cl)
				case <-ctx.Done():
					return
				}
			}
		}
		return nil
	}
}

// BackoffTime sets the time increment that backoff time is increased
// (linearly) between every reconnection attempt the backend makes. If
// this option is not used, the backend uses DefaultBackoff.
func BackoffTime(t time.Duration) ClientParam {
	return func(cl *Client) error {
		if nb, ok := cl.backend.(networkBackend); ok {
			nb.params().backoff = t
			return nil
		}
		return ErrClientNotNetworked
	}
}

// MaxBackoffTime sets the maximum time duration waited between
// reconnection attempts. If this option is not used, the backend uses
// DefaultMaxBackoff.
func MaxBackoffTime(t time.Duration) ClientParam {
	return func(cl *Client) error {
		if nb, ok := cl.backend.(networkBackend); ok {
			nb.params().maxBackoff = t
			return nil
		}
		return ErrClientNotNetworked
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
		if nb, ok := cl.backend.(networkBackend); ok {
			nb.params().connectTimeout = t
			return nil
		}
		return ErrClientNotNetworked
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
	var nb networkBackend
	switch addr := addr.(type) {
	case *net.UDPAddr:
		nb = &packetBackend{}
	case *net.UnixAddr:
		nb = &streamBackend{}
	default:
		return nil, fmt.Errorf("can not connect to %v addresses", addr.Network())
	}
	cl.backend = nb
	params := nb.params()
	params.addr = addr
	cl.cap = DefaultCapacity
	for _, opt := range opts {
		if err = opt(cl); err != nil {
			return nil, err
		}
	}
	ch := make(chan op, cl.cap)
	cl.ops = ch
	ctx := context.Background()
	ctx, cl.cancel = context.WithCancel(ctx)
	go cl.run(ctx)
	return cl, nil
}

// NewBackendClient constructs and returns a Client sending to the
// ClientBackend passed. Most user code should use NewClient, as
// NewBackendClient is primarily useful for processing spans
// internally (e.g. in veneur itself or in test code), without making
// trips over the network.
func NewBackendClient(b ClientBackend, opts ...ClientParam) (*Client, error) {
	cl := &Client{}
	cl.backend = b
	cl.cap = 1

	for _, opt := range opts {
		if err := opt(cl); err != nil {
			return nil, err
		}
	}
	cl.ops = make(chan op, cl.cap)
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

// DefaultCapacity is the capacity of the span submission queue in a
// veneur client.
const DefaultCapacity = 64

// DefaultVeneurAddress is the address that a reasonable veneur should
// listen on. Currently it defaults to UDP port 8128.
const DefaultVeneurAddress string = "udp://127.0.0.1:8128"

// ErrNoClient indicates that no client is yet initialized.
var ErrNoClient = errors.New("client is not initialized")

// ErrWouldBlock indicates that a client is not able to send a span at
// the current time.
var ErrWouldBlock = errors.New("sending span would block")

// SendClientStatistics uses the client's recorded backpressure
// statistics (failed/successful flushes, failed/successful records)
// and reports them with the given statsd client, and resets the
// statistics to zero again.
func SendClientStatistics(cl *Client, stats *statsd.Client, tags []string) {
	stats.Count("trace_client.flushes_failed_total", atomic.SwapInt64(&cl.failedFlushes, 0), tags, 1.0)
	stats.Count("trace_client.flushes_succeeded_total", atomic.SwapInt64(&cl.successfulFlushes, 0), tags, 1.0)
	stats.Count("trace_client.records_failed_total", atomic.SwapInt64(&cl.failedRecords, 0), tags, 1.0)
	stats.Count("trace_client.records_succeeded_total", atomic.SwapInt64(&cl.successfulRecords, 0), tags, 1.0)
}

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

	op := func(ctx context.Context, s ClientBackend) {
		err := s.SendSync(ctx, span)
		if done != nil {
			done <- err
		}
	}
	select {
	case cl.ops <- op:
		atomic.AddInt64(&cl.successfulRecords, 1)
		return nil
	default:
	}
	atomic.AddInt64(&cl.failedRecords, 1)
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
	op := func(ctx context.Context, s ClientBackend) {
		err := s.FlushSync(ctx)
		if ch != nil {
			ch <- err
		}
	}
	select {
	case cl.ops <- op:
		atomic.AddInt64(&cl.successfulFlushes, 1)
		return nil
	default:
	}
	atomic.AddInt64(&cl.failedFlushes, 1)
	return ErrWouldBlock
}
