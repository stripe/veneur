package veneur

import (
	"net"
	"os"
	"syscall"

	"golang.org/x/sys/unix"
)

// see also https://github.com/jbenet/go-reuseport/blob/master/impl_unix.go#L279
func NewSocket(addr *net.UDPAddr, recvBuf int) (net.PacketConn, error) {
	sockFD, err := unix.Socket(unix.AF_INET, unix.SOCK_DGRAM|syscall.SOCK_CLOEXEC|syscall.SOCK_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}

	// unix.SO_REUSEPORT is not defined on linux 386/amd64, see
	// https://github.com/golang/go/issues/16075
	if err := unix.SetsockoptInt(sockFD, unix.SOL_SOCKET, 0xf, 1); err != nil {
		return nil, err
	}
	if err := unix.SetsockoptInt(sockFD, unix.SOL_SOCKET, unix.SO_RCVBUF, recvBuf); err != nil {
		return nil, err
	}

	sockaddr := unix.SockaddrInet4{
		Port: addr.Port,
	}
	if copied := copy(sockaddr.Addr[:], addr.IP); copied != net.IPv4len {
		panic("did not copy enough bytes of ip address")
	}
	if err := unix.Bind(sockFD, &sockaddr); err != nil {
		return nil, err
	}

	osFD := os.NewFile(uintptr(sockFD), "veneursock")
	// this will close the FD we passed to NewFile
	defer osFD.Close()

	// however, FilePacketConn duplicates the FD, so closing the File's FD does
	// not affect this object's FD
	ret, err := net.FilePacketConn(osFD)
	if err != nil {
		return nil, err
	}
	return ret, nil
}
