// +build !linux

package veneur

import (
	"net"
)

func NewSocket(addr *net.UDPAddr, recvBuf int) (net.PacketConn, error) {
	serverConn, err := net.ListenUDP("udp", addr)
	if err != nil {
		return nil, err
	}
	if err := serverConn.SetReadBuffer(recvBuf); err != nil {
		return nil, err
	}
	return serverConn, nil
}
