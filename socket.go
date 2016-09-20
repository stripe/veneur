// +build !linux

package veneur

import (
	"net"
)

// NewSocket creates a socket which is intended for use by a single goroutine.
func NewSocket(addr *net.UDPAddr, recvBuf int, reuseport bool) (net.PacketConn, error) {
	if reuseport {
		panic("SO_REUSEPORT not supported on this platform")
	}
	serverConn, err := net.ListenUDP("udp", addr)
	if err != nil {
		return nil, err
	}
	if err := serverConn.SetReadBuffer(recvBuf); err != nil {
		return nil, err
	}
	return serverConn, nil
}
