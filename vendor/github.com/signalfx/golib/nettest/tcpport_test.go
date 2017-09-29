package nettest

import (
	"net"
	"testing"

	"fmt"

	"github.com/signalfx/golib/log"
	"github.com/stretchr/testify/assert"
)

func TestListenerPort(t *testing.T) {
	psocket, err := net.Listen("tcp", "127.0.0.1:0")
	assert.NoError(t, err)
	defer func() {
		log.IfErr(log.Panic, psocket.Close())
	}()
	p := TCPPort(psocket)
	assert.True(t, p > 0)
}

func TestFreeTCPPort(t *testing.T) {
	p := FreeTCPPort()
	n, err := net.Listen("tcp", fmt.Sprintf("localhost:%d", p))
	assert.NoError(t, err)
	log.IfErr(log.Panic, n.Close())
}
