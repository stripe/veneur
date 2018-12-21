package dns_connecter

import (
	"context"
	"net"
	"testing"

	"github.com/stretchr/testify/require"
	"gotest.tools/assert"
)

type fake struct {
	vs []string
}

func (f *fake) discover() ([]string, error) {
	return f.vs, nil
}

func TestDNSConnector(t *testing.T) {
	// Set up test with one servers and a fake discovery system
	l, _ := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0})
	defer l.Close()

	f := &fake{vs: []string{l.LocalAddr().String()}}
	d := NewStripeDNSUDP(
		1,
		"veneur-srv",
		OptDiscoverer(f.discover),
		OptTickerChan(nil),
	)
	d.Start()
	defer d.Stop()

	// ensure we return one address

	client, err := d.Conn(context.Background(), "")
	require.NoError(t, err)
	assert.Equal(t, l.LocalAddr().String(), client.RemoteAddr().String())

	// test state change of discoverer returning a different set of servers

	l2, _ := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0})
	defer l2.Close()

	f.vs = []string{l2.LocalAddr().String()}
	d.refresh()
	client, err = d.Conn(context.Background(), "val")
	require.NoError(t, err)
	assert.Equal(t, l2.LocalAddr().String(), client.RemoteAddr().String())

	// ensure that no servers discovered returns an error

	f.vs = []string{}
	d.refresh()
	client, err = d.Conn(context.Background(), "val")
	assert.Check(t, err != nil)
}
