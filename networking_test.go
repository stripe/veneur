package veneur

import (
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"io/ioutil"
	"os"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stripe/veneur/protocol"
)

func TestMultipleListeners(t *testing.T) {
	srv := &Server{}
	srv.shutdown = make(chan struct{})

	dir, err := ioutil.TempDir("", "unix-listener")
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	addrNet, err := protocol.ResolveAddr(fmt.Sprintf("unix://%s/socket", dir))
	require.NoError(t, err)
	addr, ok := addrNet.(*net.UnixAddr)
	require.True(t, ok)

	done := startSSFUnix(srv, addr)
	assert.Panics(t, func() {
		srv2 := &Server{}
		startSSFUnix(srv2, addr)
	})
	close(srv.shutdown)

	// Wait for the server to actually shut down:
	<-done

	srv3 := &Server{}
	srv3.shutdown = make(chan struct{})
	startSSFUnix(srv3, addr)
	close(srv3.shutdown)
}

func TestConnectUNIX(t *testing.T) {
	srv := &Server{}
	srv.shutdown = make(chan struct{})

	dir, err := ioutil.TempDir("", "unix-listener")
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	addrNet, err := protocol.ResolveAddr(fmt.Sprintf("unix://%s/socket", dir))
	require.NoError(t, err)
	addr, ok := addrNet.(*net.UnixAddr)
	require.True(t, ok)
	startSSFUnix(srv, addr)

	conns := make(chan struct{})
	for i := 0; i < 5; i++ {
		n := i
		go func() {
			// Dial the server, send it invalid data, wait
			// for it to hang up:
			c, err := net.DialUnix("unix", nil, addr)
			assert.NoError(t, err, "Connecting %d", n)
			wrote, err := c.Write([]byte("foo"))
			if !assert.NoError(t, err, "Writing to %d", n) {
				return
			}
			assert.Equal(t, 3, wrote, "Writing to %d", n)
			assert.NotNil(t, c)

			n, _ = c.Read(make([]byte, 20))
			assert.Equal(t, 0, n)

			err = c.Close()
			assert.NoError(t, err)

			conns <- struct{}{}
		}()
	}
	timeout := time.After(3 * time.Second)
	for i := 0; i < 5; i++ {
		select {
		case <-timeout:
			t.Fatalf("Timed out waiting for connection, %d made it", i)
		case <-conns:
			// pass
		}
	}
	close(srv.shutdown)
}

func TestParseSoftnet(t *testing.T) {
	const input = `
00058bcf 00000001 00000000 00000000 00000000 00000000 00000000 00000000 00000000 00000000 00000000
0004c22d 00000000 00000004 00000000 00000000 00000000 00000000 00000000 00000000 00000000 00000000
00042d39 00000000 00000000 00000000 00000000 00000000 00000000 00000000 00000000 00000000 00000000
00052ffe 00000000 00000000 00000000 00000000 00000000 00000000 00000000 00000000 00000000 00000000
00015a54 00000000 00000000 00000000 00000000 00000000 00000000 00000000 00000000 00000000 00000000
0001897c 00000000 00000000 00000000 00000000 00000000 00000000 00000000 000000c2 00000000 00000000
00233be1 00000000 00000000 00000000 00000000 00000000 00000000 00000000 00000000 000000d0 00000000
0025ec50 00000000 00000000 00000000 00000000 00000000 00000000 00000000 00000000 00000000 00000f00`

	expected := SoftnetData{
		Processors: []SoftnetDataProcessor{
			{
				Processed: 0x00058bcf,
				Dropped:   0x1,
			},
			{
				Processed:   0x0004c22d,
				TimeSqueeze: 0x4,
			},
			{
				Processed: 0x00042d39,
			},
			{
				Processed: 0x00052ffe,
			},
			{
				Processed: 0x00015a54,
			},
			{
				Processed:    0x0001897c,
				CPUCollision: 0xc2,
			},
			{
				Processed:   0x00233be1,
				ReceivedRPS: 0xd0,
			},
			{
				Processed:      0x0025ec50,
				FlowLimitCount: 0xf00,
			},
		},
	}

	r := strings.NewReader(input)
	sd, err := parseSoftnet(r)
	assert.NoError(t, err, "Could not parse valid softnet_stat data")

	assert.Equal(t, sd, expected)
}
