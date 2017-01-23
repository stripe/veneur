package veneur

import (
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
)

func writeReadUDP(t *testing.T, sock net.PacketConn, addr string) {
	client, err := net.Dial("udp", addr)
	assert.NoError(t, err, "should have connected to socket")
	_, err = client.Write([]byte("hello world"))
	assert.NoError(t, err, "should have written to socket")

	b := make([]byte, 15)
	n, laddr, err := sock.ReadFrom(b)
	assert.NoError(t, err, "should have read from socket")
	assert.Equal(t, n, 11, "should have read 11 bytes from socket")
	assert.Equal(t, "hello world", string(b[:n]), "should have gotten message from socket")
	assert.Equal(t, client.LocalAddr().String(), laddr.String(), "should have gotten message from client's address")
	err = client.Close()
	assert.NoError(t, err, "client.Close should not fail")
}

func TestSocket(t *testing.T) {
	const portString = "8200"
	const v4Localhost = "127.0.0.1:" + portString
	const v6Localhost = "[::1]:" + portString

	tests := []struct {
		addr         string
		supportsIPv4 bool
		supportsIPv6 bool
	}{
		{v4Localhost, true, false},
		{v6Localhost, false, true},
		{":" + portString, true, true},
	}

	for _, test := range tests {
		addr, err := net.ResolveUDPAddr("udp", test.addr)
		assert.NoError(t, err, "should have resolved udp address %s correctly", test.addr)

		sock, err := NewSocket(addr, 2*1024*1024, false)
		assert.NoError(t, err, "should have constructed socket correctly")

		if test.supportsIPv4 {
			writeReadUDP(t, sock, v4Localhost)
		}
		if test.supportsIPv6 {
			writeReadUDP(t, sock, v6Localhost)
		}

		err = sock.Close()
		assert.NoError(t, err, "close should not fail")
	}
}
