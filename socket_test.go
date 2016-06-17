package veneur

import (
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSocket(t *testing.T) {
	addr, err := net.ResolveUDPAddr("udp", "127.0.0.1:8200")
	assert.NoError(t, err, "should have constructed udp address correctly")

	sock, err := NewSocket(addr, 2*1024*1024)
	assert.NoError(t, err, "should have constructed socket correctly")

	client, err := net.DialUDP("udp", nil, addr)
	assert.NoError(t, err, "should have connected to socket")

	_, err = client.Write([]byte("hello world"))
	assert.NoError(t, err, "should have written to socket")

	b := make([]byte, 15)
	n, laddr, err := sock.ReadFrom(b)
	assert.NoError(t, err, "should have read from socket")
	assert.Equal(t, n, 11, "should have read 11 bytes from socket")
	assert.Equal(t, "hello world", string(b[:n]), "should have gotten message from socket")
	assert.Equal(t, client.LocalAddr().String(), laddr.String(), "should have gotten message from client's address")
}
