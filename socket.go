// +build !linux

package veneur

import (
	"net"
)

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
