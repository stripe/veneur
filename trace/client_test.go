package trace

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/DataDog/datadog-go/statsd"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stripe/veneur/protocol"
	"github.com/stripe/veneur/ssf"
)

func mustRecord(t *testing.T, client *Client, tr *Trace) (retries int) {
	for {
		err := tr.ClientRecord(client, "", map[string]string{})
		if err != ErrWouldBlock {
			assert.NoError(t, err)
			return
		}
		t.Log("retrying record")
		retries++
	}
}

func mustFlush(t *testing.T, client *Client) (retries int) {
	anyBlockage := func(err *FlushError) bool {
		for _, subErr := range err.Errors {
			if subErr != ErrWouldBlock {
				return false
			}
		}
		return true
	}
	for {
		err := Flush(client)
		if err == nil || !anyBlockage(err.(*FlushError)) {
			require.NoError(t, err)
			return
		}
		t.Log("retrying flush")
		retries++
	}
}

func TestNoClient(t *testing.T) {
	err := Record(nil, nil, nil)
	assert.Equal(t, ErrNoClient, err)
}

func TestUDP(t *testing.T) {
	// arbitrary
	const BufferSize = 1087152

	traceAddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	assert.NoError(t, err)
	serverConn, err := net.ListenUDP("udp", traceAddr)
	assert.NoError(t, err)
	defer serverConn.Close()
	err = serverConn.SetReadBuffer(BufferSize)
	assert.NoError(t, err)

	client, err := NewClient(fmt.Sprintf("udp://%s", serverConn.LocalAddr().String()), Capacity(4))
	require.NoError(t, err)
	defer client.Close()

	sentCh := make(chan error)

	for i := 0; i < 4; i++ {
		name := fmt.Sprintf("Testing-%d", i)
		tr := StartTrace(name)
		tr.Sent = sentCh
		err = tr.ClientRecord(client, name, map[string]string{})
		assert.NoError(t, err)
	}
	for i := 0; i < 4; i++ {
		assert.NoError(t, <-sentCh)
	}
}

func TestUNIX(t *testing.T) {
	dir, err := ioutil.TempDir("", "test_unix")
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	sockName := filepath.Join(dir, "sock")
	laddr, err := net.ResolveUnixAddr("unix", sockName)
	require.NoError(t, err)

	outPkg := make(chan *ssf.SSFSpan, 4)
	cleanup := serveUNIX(t, laddr, func(in net.Conn) {
		for {
			pkg, err := protocol.ReadSSF(in)
			if err == io.EOF {
				return
			}
			assert.NoError(t, err)
			outPkg <- pkg
		}
	})
	defer cleanup()

	client, err := NewClient((&url.URL{Scheme: "unix", Path: sockName}).String(), Capacity(4))
	require.NoError(t, err)
	defer client.Close()

	sentCh := make(chan error)
	for i := 0; i < 4; i++ {
		name := fmt.Sprintf("Testing-%d", i)
		tr := StartTrace(name)
		tr.Sent = sentCh
		mustRecord(t, client, tr)
	}
	for i := 0; i < 4; i++ {
		assert.NoError(t, <-sentCh)
		<-outPkg
	}
}

func TestUNIXBuffered(t *testing.T) {
	dir, err := ioutil.TempDir("", "test_unix")
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	sockName := filepath.Join(dir, "sock")
	laddr, err := net.ResolveUnixAddr("unix", sockName)
	require.NoError(t, err)

	outPkg := make(chan *ssf.SSFSpan, 4)
	cleanup := serveUNIX(t, laddr, func(in net.Conn) {
		for {
			pkg, err := protocol.ReadSSF(in)
			if err == io.EOF {
				return
			}
			assert.NoError(t, err)
			outPkg <- pkg
		}
	})
	defer cleanup()

	client, err := NewClient((&url.URL{Scheme: "unix", Path: sockName}).String(),
		Capacity(4),
		ParallelBackends(1),
		Buffered)
	require.NoError(t, err)
	defer client.Close()

	sentCh := make(chan error)
	for i := 0; i < 4; i++ {
		name := fmt.Sprintf("Testing-%d", i)
		tr := StartTrace(name)
		tr.Sent = sentCh
		mustRecord(t, client, tr)
	}
	for i := 0; i < 4; i++ {
		assert.NoError(t, <-sentCh)
	}
	assert.Equal(t, 0, len(outPkg), "Should not have sent any packets yet")

	mustFlush(t, client)
	for i := 0; i < 4; i++ {
		<-outPkg
	}
}

func serveUNIX(t testing.TB, laddr *net.UnixAddr, onconnect func(conn net.Conn)) (cleanup func() error) {
	srv, err := net.ListenUnix(laddr.Network(), laddr)
	require.NoError(t, err)
	cleanup = srv.Close

	go func() {
		for {
			in, err := srv.Accept()
			if err != nil {
				return
			}
			go onconnect(in)
		}
	}()

	return
}

func TestFailingUDP(t *testing.T) {
	// arbitrary
	const BufferSize = 1087152

	traceAddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	assert.NoError(t, err)
	serverConn, err := net.ListenUDP("udp", traceAddr)
	assert.NoError(t, err)
	defer serverConn.Close()
	err = serverConn.SetReadBuffer(BufferSize)
	assert.NoError(t, err)

	client, err := NewClient(fmt.Sprintf("udp://%s", serverConn.LocalAddr().String()), Capacity(4))
	require.NoError(t, err)
	defer client.Close()

	serverConn.Close() // Flake before anything gets sent

	// We'll send packets regardless:

	sentCh := make(chan error)

	for i := 0; i < 4; i++ {
		name := fmt.Sprintf("Testing-%d", i)
		tr := StartTrace(name)
		tr.Sent = sentCh
		mustRecord(t, client, tr)
	}
	for i := 0; i < 4; i++ {
		// Linux reports an error when sending to a
		// non-listened-to address, and macos doesn't,
		// so we can't usefully assert absence or
		// presence of an error here. This function
		// will get called, though.
		<-sentCh
	}
}

func TestReconnectUNIX(t *testing.T) {
	dir, err := ioutil.TempDir("", "test_unix")
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	sockName := filepath.Join(dir, "sock")
	laddr, err := net.ResolveUnixAddr("unix", sockName)
	require.NoError(t, err)

	outPkg := make(chan *ssf.SSFSpan, 4)
	// A server that can read one span and then immediately closes
	// the connection:
	cleanup := serveUNIX(t, laddr, func(in net.Conn) {
		pkg, err := protocol.ReadSSF(in)
		if err == io.EOF {
			return
		}
		t.Logf("received span")
		assert.NoError(t, err)
		t.Logf("Closing connection")
		in.Close()
		outPkg <- pkg
	})
	defer cleanup()

	client, err := NewClient((&url.URL{Scheme: "unix", Path: sockName}).String(),
		Capacity(4),
		ParallelBackends(1),
	)
	require.NoError(t, err)
	defer client.Close()

	sentCh := make(chan error)
	{
		name := "Testing-success"
		tr := StartTrace(name)
		tr.Sent = sentCh
		t.Logf("submitting span")
		mustRecord(t, client, tr)

		// A span was successfully received by the server:
		assert.NoError(t, <-sentCh)
		<-outPkg
	}
	{
		name := "Testing-failure"
		tr := StartTrace(name)
		tr.Sent = sentCh
		t.Logf("submitting span")
		mustRecord(t, client, tr)

		// Since reconnections throw away the span, nothing
		// was received:
		assert.Error(t, <-sentCh)
	}
	{
		name := "Testing-success2"
		tr := StartTrace(name)
		tr.Sent = sentCh
		t.Logf("submitting span")
		mustRecord(t, client, tr)

		// A span was successfully received by the server:
		assert.NoError(t, <-sentCh)
		<-outPkg
	}
}

func TestReconnectBufferedUNIX(t *testing.T) {
	dir, err := ioutil.TempDir("", "test_unix")
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	sockName := filepath.Join(dir, "sock")
	laddr, err := net.ResolveUnixAddr("unix", sockName)
	require.NoError(t, err)

	outPkg := make(chan *ssf.SSFSpan, 4)
	// A server that can read one span and then immediately closes
	// the connection:
	cleanup := serveUNIX(t, laddr, func(in net.Conn) {
		pkg, err := protocol.ReadSSF(in)
		if err == io.EOF {
			return
		}
		t.Logf("received span")
		assert.NoError(t, err)
		t.Logf("Closing connection")
		in.Close()
		outPkg <- pkg
	})
	defer cleanup()

	client, err := NewClient((&url.URL{Scheme: "unix", Path: sockName}).String(),
		Capacity(4),
		ParallelBackends(1),
		Buffered)
	require.NoError(t, err)
	defer client.Close()

	sentCh := make(chan error, 1)
	{
		name := "Testing-success"
		tr := StartTrace(name)
		tr.Sent = sentCh
		t.Logf("submitting span")
		mustRecord(t, client, tr)

		assert.NoError(t, <-sentCh, "at %q", name)
		t.Logf("Flushing")
		go mustFlush(t, client)
		// A span was successfully received by the server:
		<-outPkg
	}
	{
		name := "Testing-failure"
		tr := StartTrace(name)
		tr.Sent = sentCh
		t.Logf("submitting span")
		mustRecord(t, client, tr)

		assert.NoError(t, <-sentCh)
		t.Logf("Flushing")
		go assert.Error(t, Flush(client))
		// Since reconnections throw away the span, nothing
		// was received.
	}
	{
		name := "Testing-success2"
		tr := StartTrace(name)
		tr.Sent = sentCh
		t.Logf("submitting span")
		mustRecord(t, client, tr)

		assert.NoError(t, <-sentCh, "at %q", name)

		t.Logf("Flushing")
		go mustFlush(t, client)
		// A span was successfully received by the server:
		<-outPkg
	}
}

type testBackend struct {
	t  *testing.T
	ch chan *ssf.SSFSpan
}

func (tb *testBackend) Close() error {
	tb.t.Logf("Closing backend")
	close(tb.ch)
	return nil
}

func (tb *testBackend) SendSync(ctx context.Context, span *ssf.SSFSpan) error {
	tb.t.Logf("Sending span")
	tb.ch <- span
	return nil
}

func (tb *testBackend) FlushSync(ctx context.Context) error {
	return nil
}

func TestInternalBackend(t *testing.T) {
	received := make(chan *ssf.SSFSpan)

	tb := testBackend{t, received}
	cl, err := NewBackendClient(&tb, Capacity(5))
	require.NoError(t, err)

	sent := make(chan error, 10)
	somespan := func() error {
		tr := StartTrace("hi there")
		tr.Sent = sent
		return tr.ClientRecord(cl, "hi there", map[string]string{})
	}

	inflight := 0
	for {
		t.Logf("submitting span %d", inflight)
		err := somespan()
		if err == ErrWouldBlock {
			t.Logf("got note that we'd block, breaking")
			break
		}
		inflight++
		require.NoError(t, err)
	}

	// Receive the sent/queued spans:
	for i := 0; i < inflight; i++ {
		t.Logf("Receiving %d", i)
		<-received
		assert.NoError(t, <-sent)
	}
	assert.NoError(t, cl.Close())
}

type successTestBackend struct {
	t         *testing.T
	block     chan chan struct{}
	flushChan chan chan<- error
}

func (tb *successTestBackend) Close() error {
	// no-op
	return nil
}

func (tb *successTestBackend) SendSync(ctx context.Context, span *ssf.SSFSpan) error {
	tb.t.Logf("Sending span")
	select {
	case block := <-tb.block:
		<-block
	default:
	}
	return nil
}

func (tb *successTestBackend) FlushSync(ctx context.Context) error {
	tb.t.Logf("flushing")
	select {
	case block := <-tb.block:
		<-block
	default:
	}
	return nil
}

func (tb *successTestBackend) FlushChan() chan chan<- error {
	return tb.flushChan
}

func TestDropStatistics(t *testing.T) {
	blockNext := make(chan chan struct{}, 1)
	flushChan := make(chan chan<- error)
	defer close(flushChan)
	tb := successTestBackend{t: t, block: blockNext, flushChan: flushChan}

	// Make a client that blocks if nothing listens:
	cl, err := NewBackendClient(&tb, Capacity(0))
	require.NoError(t, err)

	// reset client stats:
	stats, err := statsd.NewBuffered("127.0.0.1:8200", 4096)
	require.NoError(t, err)
	SendClientStatistics(cl, stats, nil)

	// Actually test the client:
	flushRetries := mustFlush(t, cl)
	assert.NoError(t, err, "Flushing an empty client should succeed")
	assert.Equal(t, int64(1), cl.successfulFlushes, "successful flushes")

	done := make(chan struct{})
	blockNext <- done
	retries := mustRecord(t, cl, StartTrace("hi there"))
	assert.Equal(t, int64(1), cl.successfulRecords, "successful records")

	err = StartTrace("hi there").ClientRecord(cl, "", map[string]string{})
	assert.Error(t, err)
	assert.Equal(t, ErrWouldBlock, err, "Expected to report a blocked channel")
	assert.Equal(t, int64(1+retries), cl.failedRecords, "failed records")

	err = Flush(cl)
	assert.Equal(t, int64(1+flushRetries), cl.failedFlushes, "failed flushes")
	close(done)
	close(blockNext)
}
