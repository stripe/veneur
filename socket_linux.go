package veneur

import (
	"net"
	"os"
	"syscall"

	"golang.org/x/sys/unix"
)

// see also https://github.com/jbenet/go-reuseport/blob/master/impl_unix.go#L279
func NewSocket(addr *net.UDPAddr, recvBuf int, reuseport bool) (net.PacketConn, error) {
	// default to AF_INET6 to be equivalent to net.ListenUDP()
	domain := unix.AF_INET6
	if addr.IP.To4() != nil {
		domain = unix.AF_INET
	}
	sockFD, err := unix.Socket(domain, unix.SOCK_DGRAM|syscall.SOCK_CLOEXEC|syscall.SOCK_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}

	// unix.SO_REUSEPORT is not defined on linux 386/amd64, see
	// https://github.com/golang/go/issues/16075
	if reuseport {
		if err := unix.SetsockoptInt(sockFD, unix.SOL_SOCKET, 0xf, 1); err != nil {
			unix.Close(sockFD)
			return nil, err
		}
	}
	if err = unix.SetsockoptInt(sockFD, unix.SOL_SOCKET, unix.SO_RCVBUF, recvBuf); err != nil {
		unix.Close(sockFD)
		return nil, err
	}

	var sa unix.Sockaddr
	if domain == unix.AF_INET {
		sockaddr := &unix.SockaddrInet4{
			Port: addr.Port,
		}
		if copied := copy(sockaddr.Addr[:], addr.IP.To4()); copied != net.IPv4len {
			panic("did not copy enough bytes of ip address")
		}
		sa = sockaddr
	} else {
		sockaddr := &unix.SockaddrInet6{
			Port: addr.Port,
		}
		// addr.IP will be length 0 for "bind all interfaces"
		if copied := copy(sockaddr.Addr[:], addr.IP.To16()); !(copied == net.IPv6len || copied == 0) {
			panic("did not copy enough bytes of ip address")
		}
		sa = sockaddr
	}
	if err = unix.Bind(sockFD, sa); err != nil {
		unix.Close(sockFD)
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
