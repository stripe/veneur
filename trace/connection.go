package trace

import (
	"errors"
	"fmt"
	"net"
	"sync"

	"github.com/Sirupsen/logrus"
	"github.com/gogo/protobuf/proto"
	"github.com/stripe/veneur/ssf"
)

// used to initialize udpConn inside sendSample
var udpInitOnce sync.Once
var udpInitFunc = func() { initUDPConn(localVeneurAddress) }
var udpConnUninitializedErr = errors.New("UDP connection is not yet initialized")
var udpConn *net.UDPConn

const localVeneurAddress = "127.0.0.1:8128"

// sendSample marshals the sample using protobuf and sends it
// over UDP to the local veneur instance
func sendSample(sample *ssf.SSFSpan) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err, ok := r.(error)
			if !ok {
				err = fmt.Errorf("encountered panic while sending sample %#v", err)
			}
			logrus.WithError(err).Error("Panicked while dialing UDP connection - traces will not be sent")
		}
	}()

	if Disabled() {
		return nil
	}

	udpInitOnce.Do(udpInitFunc)

	// at this point, the connection should already be initialized

	if udpConn == nil {
		return udpConnUninitializedErr
	}

	data, err := proto.Marshal(sample)
	if err != nil {
		return
	}

	_, err = udpConn.Write(data)
	if err != nil {
		return
	}

	return nil
}

// initUDPConn will initialize the global UDP connection.
// It will panic if it encounters an error.
// It should only be called via the corresponding sync.Once
func initUDPConn(address string) {
	serverAddr, err := net.ResolveUDPAddr("udp", localVeneurAddress)
	if err != nil {
		panic(err)
	}

	conn, err := net.DialUDP("udp", nil, serverAddr)
	if err != nil {
		panic(err)
	}
	udpConn = conn
}
