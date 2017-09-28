package veneur

import (
	"fmt"
	"net"
	"testing"

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
