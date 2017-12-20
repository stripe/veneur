package nettest

import (
	"github.com/signalfx/golib/log"
	"net"
)

// A NetworkListener is a listener that looks for data on a network address.  It is sometimes
// useful in testing to get this address so you can talk to it directly.
type NetworkListener interface {
	Addr() net.Addr
}

// TCPPort of the listener address.  If the listener isn't TCP, this may panic()
func TCPPort(l NetworkListener) uint16 {
	return (uint16)(l.Addr().(*net.TCPAddr).Port)
}

// FreeTCPPort returns a TCP port that is free on "localhost", or panics if it cannot find a port
func FreeTCPPort() uint16 {
	l, _ := net.Listen("tcp", "localhost:0")
	ret := TCPPort(l)
	log.IfErr(log.Panic, l.Close())
	return ret
}
