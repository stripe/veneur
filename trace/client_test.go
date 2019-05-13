package trace_test

import (
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
	"github.com/stripe/veneur/trace"
	"github.com/stripe/veneur/trace/testbackend"
)

func mustRecord(t *testing.T, client *trace.Client, tr *trace.Trace) (retries int) {
	for {
		err := tr.ClientRecord(client, "", map[string]string{})
		if err != trace.ErrWouldBlock {
			assert.NoError(t, err)
			return
		}
		t.Log("retrying record")
		retries++
	}
}

func mustFlush(t *testing.T, client *trace.Client) (retries int) {
	anyBlockage := func(err *trace.FlushError) bool {
		for _, subErr := range err.Errors {
			if subErr != trace.ErrWouldBlock {
				return false
			}
		}
		return true
	}
	for {
		err := trace.Flush(client)
		if err == nil || !anyBlockage(err.(*trace.FlushError)) {
			require.NoError(t, err)
			return
		}
		t.Log("retrying flush")
		retries++
	}
}

func TestNoClient(t *testing.T) {
	err := trace.Record(nil, nil, nil)
	assert.Equal(t, trace.ErrNoClient, err)
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

	client, err := trace.NewClient(fmt.Sprintf("udp://%s", serverConn.LocalAddr().String()), trace.Capacity(4))
	require.NoError(t, err)
	defer client.Close()

	sentCh := make(chan error)

	for i := 0; i < 4; i++ {
		name := fmt.Sprintf("Testing-%d", i)
		tr := trace.StartTrace(name)
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

	client, err := trace.NewClient((&url.URL{Scheme: "unix", Path: sockName}).String(), trace.Capacity(4))
	require.NoError(t, err)
	defer client.Close()

	sentCh := make(chan error)
	for i := 0; i < 4; i++ {
		name := fmt.Sprintf("Testing-%d", i)
		tr := trace.StartTrace(name)
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

	client, err := trace.NewClient((&url.URL{Scheme: "unix", Path: sockName}).String(),
		trace.Capacity(4),
		trace.ParallelBackends(1),
		trace.Buffered)
	require.NoError(t, err)
	defer client.Close()

	sentCh := make(chan error)
	for i := 0; i < 4; i++ {
		name := fmt.Sprintf("Testing-%d", i)
		tr := trace.StartTrace(name)
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

func TestUDPError(t *testing.T) {
	// arbitrary
	const BufferSize = 1087152

	traceAddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	assert.NoError(t, err)
	serverConn, err := net.ListenUDP("udp", traceAddr)
	assert.NoError(t, err)
	defer serverConn.Close()
	err = serverConn.SetReadBuffer(BufferSize)
	assert.NoError(t, err)

	client, err := trace.NewClient(fmt.Sprintf("udp://%s", serverConn.LocalAddr().String()), trace.Capacity(4))
	require.NoError(t, err)
	defer client.Close()

	serverConn.Close() // Flake before anything gets sent

	// We'll send packets regardless:

	sentCh := make(chan error)

	for i := 0; i < 4; i++ {
		name := fmt.Sprintf("Testing-%d", i)
		tr := trace.StartTrace(name)
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

	client, err := trace.NewClient((&url.URL{Scheme: "unix", Path: sockName}).String(),
		trace.Capacity(4),
		trace.ParallelBackends(1),
	)
	require.NoError(t, err)
	defer client.Close()

	sentCh := make(chan error)
	{
		name := "Testing-success"
		tr := trace.StartTrace(name)
		tr.Sent = sentCh
		t.Logf("submitting span")
		mustRecord(t, client, tr)

		// A span was successfully received by the server:
		assert.NoError(t, <-sentCh)
		<-outPkg
	}
	{
		name := "Testing-failure"
		tr := trace.StartTrace(name)
		tr.Sent = sentCh
		t.Logf("submitting span")
		mustRecord(t, client, tr)

		// Since reconnections throw away the span, nothing
		// was received:
		assert.Error(t, <-sentCh)
	}
	{
		name := "Testing-success2"
		tr := trace.StartTrace(name)
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

	client, err := trace.NewClient((&url.URL{Scheme: "unix", Path: sockName}).String(),
		trace.Capacity(4),
		trace.ParallelBackends(1),
		trace.Buffered)
	require.NoError(t, err)
	defer client.Close()

	sentCh := make(chan error, 1)
	{
		name := "Testing-success"
		tr := trace.StartTrace(name)
		tr.Sent = sentCh
		t.Logf("submitting span")
		mustRecord(t, client, tr)

		require.NoError(t, <-sentCh, "at %q", name)
		t.Logf("Flushing")
		go mustFlush(t, client)
		// A span was successfully received by the server:
		<-outPkg
	}
	{
		name := "Testing-failure"
		tr := trace.StartTrace(name)
		tr.Sent = sentCh
		t.Logf("submitting span")
		mustRecord(t, client, tr)

		require.NoError(t, <-sentCh)
		t.Logf("Flushing to the closed socket")
		flushErr := make(chan error)
		// Just keep trying until we get a flush kicked off
	FlushRetries:
		for {
			require.NoError(t, trace.FlushAsync(client, flushErr))
			err := (<-flushErr).(*trace.FlushError)
			// same as the number of parallel backends
			require.Len(t, err.Errors, 1)
			for _, err := range err.Errors {
				if err != trace.ErrWouldBlock {
					require.Error(t, err, "Expected an error flushing on the broken socket")
					break FlushRetries
				}
			}
			t.Log("Retrying flush on closed socket")
			time.Sleep(1 * time.Millisecond)
		}
	}
	{
		name := "Testing-success2"
		tr := trace.StartTrace(name)
		tr.Sent = sentCh
		t.Logf("submitting span")
		mustRecord(t, client, tr)

		require.NoError(t, <-sentCh, "at %q", name)

		t.Logf("Flushing")
		go mustFlush(t, client)
		// A span was successfully received by the server again:
		select {
		case <-outPkg:
			return
		case <-time.After(4 * time.Second):
			t.Fatal("Timed out waiting for the succeeded packet after 4s")
		}
	}
}

func TestInternalBackend(t *testing.T) {
	received := make(chan *ssf.SSFSpan)

	cl, err := trace.NewBackendClient(testbackend.NewBackend(received), trace.Capacity(5))
	require.NoError(t, err)

	sent := make(chan error, 10)
	somespan := func() error {
		tr := trace.StartTrace("hi there")
		tr.Sent = sent
		return tr.ClientRecord(cl, "hi there", map[string]string{})
	}

	inflight := 0
	for {
		t.Logf("submitting span %d", inflight)
		err := somespan()
		if err == trace.ErrWouldBlock {
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

type testStatsCollector map[string]int64

func (ts testStatsCollector) Count(metric string, count int64, _ []string, _ float64) error {
	ts[metric] = count
	return nil
}

func TestDropStatistics(t *testing.T) {
	blockNext := make(chan chan struct{}, 1)
	shouldBlock := func() error {
		select {
		case block := <-blockNext:
			<-block
		default:
		}
		return nil
	}
	flushChan := make(chan []*ssf.SSFSpan, 1)
	tb := testbackend.NewFlushingBackend(flushChan,
		testbackend.FlushErrors(
			func(*ssf.SSFSpan) error { return shouldBlock() },
			func([]*ssf.SSFSpan) error { return shouldBlock() }))

	// Make a client that blocks if nothing listens:
	cl, err := trace.NewBackendClient(tb, trace.Capacity(0))
	require.NoError(t, err)

	// reset client stats:
	stats := testStatsCollector{}
	trace.SendClientStatistics(cl, stats, nil)

	// Actually test the client:
	mustFlush(t, cl)
	assert.NoError(t, err, "Flushing an empty client should succeed")
	trace.SendClientStatistics(cl, stats, nil)
	assert.Equal(t, int64(1), stats["trace_client.flushes_succeeded_total"], "successful flushes")

	done := make(chan struct{})
	blockNext <- done
	mustRecord(t, cl, trace.StartTrace("hi there"))
	trace.SendClientStatistics(cl, stats, nil)
	assert.Equal(t, int64(1), stats["trace_client.records_succeeded_total"], "successful records")

	err = trace.StartTrace("hi there").ClientRecord(cl, "", map[string]string{})
	assert.Error(t, err)
	assert.Equal(t, trace.ErrWouldBlock, err, "Expected to report a blocked channel")
	trace.SendClientStatistics(cl, stats, nil)
	assert.Equal(t, int64(1), stats["trace_client.records_failed_total"], "failed records")

	err = trace.Flush(cl)
	trace.SendClientStatistics(cl, stats, nil)
	assert.Equal(t, int64(1), stats["trace_client.flushes_failed_total"], "failed flushes")
	close(done)
	close(blockNext)
}

func TestNormalize(t *testing.T) {
	received := make(chan *ssf.SSFSpan, 1)

	normalizer := func(sample *ssf.SSFSample) {
		sample.Scope = ssf.SSFSample_Scope(ssf.Global)
		sample.Tags["woo_yay"] = "blort"
	}
	cl, err := trace.NewBackendClient(testbackend.NewBackend(received),
		trace.Capacity(5),
		trace.NormalizeSamples(normalizer))
	require.NoError(t, err)

	span := trace.StartTrace("hi there")
	span.Add(ssf.Gauge("whee.gauge", 20, map[string]string{}))
	span.Add(ssf.Count("whee.counter", 20, map[string]string{}, ssf.Scope(ssf.Local)))
	go mustRecord(t, cl, span)

	out := <-received
	for _, sample := range out.Metrics {
		assert.Equal(t, ssf.SSFSample_Scope(ssf.Global), sample.Scope,
			"sample: %v", sample)
		assert.Equal(t, "blort", sample.Tags["woo_yay"],
			"sample: %v", sample)
	}
}
