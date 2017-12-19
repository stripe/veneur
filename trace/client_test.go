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
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stripe/veneur/protocol"
	"github.com/stripe/veneur/ssf"
)

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
		err = tr.ClientRecord(client, name, map[string]string{})
		assert.NoError(t, err)
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
		Buffered)
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
	assert.Equal(t, 0, len(outPkg), "Should not have sent any packets yet")
	err = Flush(client)
	assert.NoError(t, err)
	for i := 0; i < 4; i++ {
		<-outPkg
	}
}

func TestUNIXBufferedFlushing(t *testing.T) {
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
	flushChan := make(chan time.Time)

	client, err := NewClient((&url.URL{Scheme: "unix", Path: sockName}).String(),
		Capacity(4),
		Buffered,
		FlushChannel(flushChan, func() {}))
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
	assert.Equal(t, 0, len(outPkg), "Should not have sent any packets yet")
	flushChan <- time.Now()
	assert.NoError(t, err)
	for i := 0; i < 4; i++ {
		<-outPkg
	}
}

func serveUNIX(t *testing.T, laddr *net.UnixAddr, onconnect func(conn net.Conn)) (cleanup func() error) {
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
		err = tr.ClientRecord(client, name, map[string]string{})
		assert.NoError(t, err)
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

	client, err := NewClient((&url.URL{Scheme: "unix", Path: sockName}).String(), Capacity(4))
	require.NoError(t, err)
	defer client.Close()

	sentCh := make(chan error)
	{
		name := "Testing-success"
		tr := StartTrace(name)
		tr.Sent = sentCh
		t.Logf("submitting span")
		err = tr.ClientRecord(client, name, map[string]string{})
		assert.NoError(t, err)

		// A span was successfully received by the server:
		assert.NoError(t, <-sentCh)
		<-outPkg
	}
	{
		name := "Testing-failure"
		tr := StartTrace(name)
		tr.Sent = sentCh
		t.Logf("submitting span")
		err = tr.ClientRecord(client, name, map[string]string{})
		assert.NoError(t, err)

		// Since reconnections throw away the span, nothing
		// was received:
		assert.Error(t, <-sentCh)
	}
	{
		name := "Testing-success2"
		tr := StartTrace(name)
		tr.Sent = sentCh
		t.Logf("submitting span")
		err = tr.ClientRecord(client, name, map[string]string{})
		assert.NoError(t, err)

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
		Buffered)
	require.NoError(t, err)
	defer client.Close()

	sentCh := make(chan error, 1)
	{
		name := "Testing-success"
		tr := StartTrace(name)
		tr.Sent = sentCh
		t.Logf("submitting span")
		err = tr.ClientRecord(client, name, map[string]string{})
		assert.NoError(t, err)

		assert.NoError(t, <-sentCh, "at %q", name)
		t.Logf("Flushing")
		go assert.NoError(t, Flush(client))
		// A span was successfully received by the server:
		<-outPkg
	}
	{
		name := "Testing-failure"
		tr := StartTrace(name)
		tr.Sent = sentCh
		t.Logf("submitting span")
		err = tr.ClientRecord(client, name, map[string]string{})
		assert.NoError(t, err)

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
		err = tr.ClientRecord(client, name, map[string]string{})
		assert.NoError(t, err)

		assert.NoError(t, <-sentCh, "at %q", name)

		t.Logf("Flushing")
		go assert.NoError(t, Flush(client))
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
