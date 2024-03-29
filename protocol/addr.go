package protocol

import (
	"fmt"
	"net"
	"net/url"
)

// ResolveAddr takes a URL-style listen address specification,
// resolves it and returns a net.Addr that corresponds to the
// string. If any error (in URL decoding, destructuring or resolving)
// occurs, ResolveAddr returns the respective error.
//
// Valid address examples are:
//   udp6://127.0.0.1:8000
//   unix:///tmp/foo.sock
//   tcp://127.0.0.1:9002
func ResolveAddr(u *url.URL) (net.Addr, error) {
	switch u.Scheme {
	case "unix", "unixgram", "unixpacket":
		var path string
		if u.Opaque != "" {
			path = u.Opaque
		} else {
			path = u.Path
		}

		addr, err := net.ResolveUnixAddr(u.Scheme, path)
		if err != nil {
			return nil, err
		}
		return addr, nil
	case "tcp6", "tcp4", "tcp":
		addr, err := net.ResolveTCPAddr(u.Scheme, u.Host)
		if err != nil {
			return nil, err
		}
		return addr, nil
	case "udp6", "udp4", "udp":
		addr, err := net.ResolveUDPAddr(u.Scheme, u.Host)
		if err != nil {
			return nil, err
		}
		return addr, nil
	}
	return nil, fmt.Errorf("unknown address family %q on address %q", u.Scheme, u.String())
}
