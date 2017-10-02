package veneur

import (
	"fmt"
	"net"
	"testing"
	"time"

	"io/ioutil"
	"os"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListenAddr(t *testing.T) {
	tests := []struct {
		input   string
		network string
		laddr   string
	}{
		{"udp://127.0.0.1:8200", "udp", "127.0.0.1:8200"},
		{"tcp://:8200", "tcp", ":8200"},
		{"tcp6://[::1]:8200", "tcp", "[::1]:8200"},
		{"unix:///tmp/foo.sock", "unix", "/tmp/foo.sock"},
		{"unixgram:///tmp/foo.sock", "unixgram", "/tmp/foo.sock"},
		{"unixpacket:///tmp/foo.sock", "unixpacket", "/tmp/foo.sock"},
	}
	for _, test := range tests {
		addr, err := ResolveAddr(test.input)
		if !assert.NoError(t, err) {
			continue
		}
		assert.Equal(t, test.network, addr.Network())
		assert.Equal(t, test.laddr, addr.String(), "Address %#v not correct", addr)
	}
}

func TestMultipleListeners(t *testing.T) {
	srv := &Server{}
	srv.shutdown = make(chan struct{})

	dir, err := ioutil.TempDir("", "unix-listener")
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	addrNet, err := ResolveAddr(fmt.Sprintf("unix://%s/socket", dir))
	require.NoError(t, err)
	addr, ok := addrNet.(*net.UnixAddr)
	require.True(t, ok)

	done := startSSFUnix(srv, addr)
	assert.Panics(t, func() {
		srv2 := &Server{}
		startSSFUnix(srv2, addr)
	})
	close(srv.shutdown)

	// Wait for the server to actually shut down:
	<-done

	srv3 := &Server{}
	srv3.shutdown = make(chan struct{})
	startSSFUnix(srv3, addr)
	close(srv3.shutdown)
}

func TestConnectUNIX(t *testing.T) {
	srv := &Server{}
	srv.shutdown = make(chan struct{})

	dir, err := ioutil.TempDir("", "unix-listener")
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	addrNet, err := ResolveAddr(fmt.Sprintf("unix://%s/socket", dir))
	require.NoError(t, err)
	addr, ok := addrNet.(*net.UnixAddr)
	require.True(t, ok)
	startSSFUnix(srv, addr)

	conns := make(chan struct{})
	for i := 0; i < 5; i++ {
		n := i
		go func() {
			// Dial the server, send it invalid data, wait
			// for it to hang up:
			c, err := net.DialUnix("unix", nil, addr)
			assert.NoError(t, err, "Connecting %d", n)
			wrote, err := c.Write([]byte("foo"))
			if !assert.NoError(t, err, "Writing to %d", n) {
				return
			}
			assert.Equal(t, 3, wrote, "Writing to %d", n)
			assert.NotNil(t, c)

			n, _ = c.Read(make([]byte, 20))
			assert.Equal(t, 0, n)

			err = c.Close()
			assert.NoError(t, err)

			conns <- struct{}{}
		}()
	}
	timeout := time.After(3 * time.Second)
	for i := 0; i < 5; i++ {
		select {
		case <-timeout:
			t.Fatalf("Timed out waiting for connection, %d made it", i)
		case <-conns:
			// pass
		}
	}
	close(srv.shutdown)
}
