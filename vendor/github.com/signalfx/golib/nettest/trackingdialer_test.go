package nettest

import (
	"errors"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/signalfx/golib/log"
	"github.com/stretchr/testify/assert"
)

func TestTrackingDialerBadConn(t *testing.T) {
	td := TrackingDialer{}
	defer func() {
		log.IfErr(log.Panic, td.Close())
	}()
	_, err := td.DialTimeout("tcpuseless", "localhost", 0)
	assert.Error(t, err)
}

func TestTrackingDialerKillConn(t *testing.T) {
	td := TrackingDialer{}
	defer func() {
		log.IfErr(log.Panic, td.Close())
	}()
	l, err := net.Listen("tcp", "localhost:0")
	assert.NoError(t, err)
	conn, err := td.DialTimeout("tcp", fmt.Sprintf("localhost:%d", TCPPort(l)), time.Second)
	assert.NoError(t, err)
	assert.NoError(t, td.Close())
	_, err = conn.Write([]byte("hi"))
	assert.Error(t, err)
}

type errConn struct {
	net.Conn
}

func (e *errConn) Close() error {
	return errors.New("nope")
}

func TestTrackingDialerBadClose(t *testing.T) {
	td := TrackingDialer{}
	defer func() {
		log.IfErr(log.Panic, td.Close())
	}()
	td.Conns = append(td.Conns, &errConn{})
	assert.Error(t, td.Close())
}
