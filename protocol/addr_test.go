package protocol

import (
	"testing"

	"github.com/stretchr/testify/assert"
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
