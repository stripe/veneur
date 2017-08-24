package veneur

import (
	"fmt"
	"net"
	"net/url"

	yaml "gopkg.in/yaml.v2"
)

// ListeningAddr implements the net.Addr interface and gets
// deserialized from the YAML config file by interpreting it as a URL,
// where the Scheme corresponds to the "net" argument to net.Listen,
// and the host&port or path are the "laddr" arg.
//
// Valid address examples are:
//   - udp6://127.0.0.1:8000
//   - unix:///tmp/foo.sock
//   - tcp://127.0.0.1:9002
type ListeningAddr struct {
	address string
	network string
}

var _ net.Addr = &ListeningAddr{}

func (a *ListeningAddr) Network() string {
	return a.network
}

func (a *ListeningAddr) String() string {
	return a.address
}

func (a *ListeningAddr) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var addrStr string
	if err := unmarshal(&addrStr); err != nil {
		return err
	}

	u, err := url.Parse(addrStr)
	if err != nil {
		return fmt.Errorf("couldn't parse listening address %q: %v", addrStr, err)
	}
	switch u.Scheme {
	case "unix", "unixpacket", "unixgram":
		a.address = u.Path
	case "tcp6", "tcp4", "tcp", "udp6", "udp4", "udp":
		a.address = u.Host
	default:
		return fmt.Errorf("unknown address family %q on address %q", u.Scheme, addrStr)
	}
	a.network = u.Scheme
	return nil
}
