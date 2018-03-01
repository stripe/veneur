package veneur

import (
	"fmt"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeReadUDP(listenAddr string, sendHost string) func(*testing.T) {
	return func(t *testing.T) {
		t.Parallel()
		addr, err := net.ResolveUDPAddr("udp", listenAddr)
		require.NoError(t, err, "should have resolved udp address %s correctly", listenAddr)

		sock, err := NewSocket(addr, 2*1024*1024, false)
		require.NoError(t, err, "should have constructed socket correctly")
		defer func() { assert.NoError(t, sock.Close(), "sock.Close should not fail") }()

		sendAddr := fmt.Sprintf("%s:%d", sendHost, sock.LocalAddr().(*net.UDPAddr).Port)
		client, err := net.Dial("udp", sendAddr)
		require.NoError(t, err, "should have connected to socket")
		defer func() { assert.NoError(t, client.Close(), "client.Close should not fail") }()

		_, err = client.Write([]byte("hello world"))
		assert.NoError(t, err, "should have written to socket")

		b := make([]byte, 15)
		n, laddr, err := sock.ReadFrom(b)
		assert.NoError(t, err, "should have read from socket")
		assert.Equal(t, n, 11, "should have read 11 bytes from socket")
		assert.Equal(t, "hello world", string(b[:n]), "should have gotten message from socket")
		assert.Equal(t, client.LocalAddr().String(), laddr.String(), "should have gotten message from client's address")
	}
}

func TestSocket(t *testing.T) {
	// see if the system supports ipv6 by listening to a port
	systemSupportsV6 := true
	conn, err := net.ListenPacket("udp", "[::1]:0")
	if err != nil {
		systemSupportsV6 = false
	} else {
		conn.Close()
	}

	tests := []struct {
		name     string
		addr     string
		sendAddr string
		isIPv6   bool
	}{
		{"IPv4 only", "127.0.0.1:0", "127.0.0.1", false},
		{"IPv6 only", "[::1]:0", "[::1]", true},
		{"IPv4 to any", ":0", "127.0.0.1", false},
		{"IPv6 to any", ":0", "[::1]", true},
	}

	for _, elt := range tests {
		test := elt
		if test.isIPv6 && !systemSupportsV6 {
			continue
		}
		t.Run(test.name, writeReadUDP(test.addr, test.sendAddr))
	}
}
