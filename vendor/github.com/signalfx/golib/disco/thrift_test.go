package disco

import (
	"errors"
	"fmt"
	"math/rand"
	"net"
	"testing"
	"time"

	"sync/atomic"

	"git.apache.org/thrift.git/lib/go/thrift"
	"github.com/samuel/go-zookeeper/zk"
	"github.com/signalfx/golib/log"
	"github.com/signalfx/golib/nettest"
	"github.com/signalfx/golib/zkplus"
	"github.com/signalfx/golib/zkplus/zktest"
	"github.com/stretchr/testify/assert"
)

func TestThrift(t *testing.T) {
	l, err := net.Listen("tcp", "localhost:0")
	assert.NoError(t, err)
	defer func() {
		log.IfErr(log.Panic, l.Close())
	}()
	go func() {
		c, err := l.Accept()
		assert.NoError(t, err)
		if err != nil {
			fmt.Printf("Err on accept\n")
			return
		}
		for {
			_, err := c.Write([]byte{0x53})
			if err != nil {
				break
			}
		}
	}()

	s2 := zktest.New()
	z, ch, _ := s2.Connect()
	zkConnFunc := ZkConnCreatorFunc(func() (ZkConn, <-chan zk.Event, error) {
		zkp, err := zkplus.NewBuilder().PathPrefix("/test").Connector(&zkplus.StaticConnector{C: z, Ch: ch}).Build()
		return zkp, zkp.EventChan(), err
	})
	d1, _ := New(zkConnFunc, "localhost", nil)

	s, err := d1.Services("thriftservice")
	updated := make(chan struct{}, 5)
	s.Watch(func() {
		updated <- struct{}{}
	})
	assert.NoError(t, err)
	trans := NewThriftTransport(s, time.Second)
	defer func() {
		log.IfErr(log.Panic, trans.Close())
	}()
	trans.WrappedFactory = thrift.NewTTransportFactory()
	assert.NotNil(t, trans)

	assert.False(t, trans.IsOpen())

	assert.NoError(t, trans.Flush())
	assert.NoError(t, trans.Close())

	_, err = trans.Read([]byte{})
	assert.Error(t, err)

	_, err = trans.Write([]byte{})
	assert.Error(t, err)

	err = trans.Open()
	assert.Error(t, err)

	assert.NoError(t, d1.Advertise("thriftservice", "", nettest.TCPPort(l)))
	<-updated

	assert.NoError(t, trans.NextConnection())
	assert.NoError(t, trans.Open())
	b := []byte{0}
	_, err = trans.Read(b)
	fmt.Printf("%s\n", b)
	assert.NoError(t, err)
	assert.Equal(t, byte(0x53), b[0])

	_, err = trans.Write([]byte{0})
	assert.NoError(t, err)

	assert.NoError(t, trans.Flush())

	assert.NoError(t, trans.NextConnection())
}

var errNope = errors.New("nope")

type errorTransport struct {
}

func (d *errorTransport) Flush() (err error) {
	return errNope
}

func (d *errorTransport) IsOpen() bool {
	return false
}

func (d *errorTransport) Close() error {
	return errNope
}

func (d *errorTransport) Read(b []byte) (n int, err error) {
	return 0, errNope
}

func (d *errorTransport) Write(b []byte) (n int, err error) {
	return 0, errNope
}

func (d *errorTransport) RemainingBytes() uint64 {
	return 1234
}

func (d *errorTransport) Open() error {
	return errNope
}

var _ thrift.TTransport = &errorTransport{}

func TestCurrentTransportErrors(t *testing.T) {
	trans := &ThriftTransport{
		currentTransport: &errorTransport{},
		logger:           log.Discard,
	}
	assert.Error(t, trans.Close())
	trans.currentTransport = &errorTransport{}
	assert.Error(t, trans.Flush())

	trans.currentTransport = &errorTransport{}
	_, err := trans.Read([]byte{})
	assert.Error(t, err)

	trans.currentTransport = &errorTransport{}
	_, err = trans.Write([]byte{})
	assert.Error(t, err)

	trans.currentTransport = &errorTransport{}
	assert.Equal(t, uint64(1234), trans.RemainingBytes())

	trans.currentTransport = nil
	remaining := trans.RemainingBytes()
	assert.True(t, remaining > 0)
}

func TestNoGoodInstances(t *testing.T) {
	instances := []ServiceInstance{
		{
			Address: "bad.address.example.com",
			Port:    0,
		},
	}
	trans := &ThriftTransport{
		randSource: rand.New(rand.NewSource(time.Now().UnixNano())),
		service:    &Service{},
		logger:     log.Discard,
	}
	trans.service.services.Store(instances)

	assert.Equal(t, ErrNoInstanceOpen, trans.NextConnection())
}

type calcHandler struct {
	forcedSleep int64
	server      *thrift.TSimpleServer
	transport   *thrift.TServerSocket
}

var _ Calculator = &calcHandler{}

func (c *calcHandler) Start(listenPort uint16, timeout time.Duration) {
	processorFactory := thrift.NewTProcessorFactory(NewCalculatorProcessor(c))
	var err error
	c.transport, err = thrift.NewTServerSocketTimeout(fmt.Sprintf("localhost:%d", listenPort), timeout)
	if err != nil {
		panic("failure")
	}
	framedFactory := thrift.NewTFramedTransportFactory(thrift.NewTTransportFactory())
	binaryFactory := thrift.NewTBinaryProtocolFactoryDefault()
	c.server = thrift.NewTSimpleServerFactory4(processorFactory, c.transport, framedFactory, binaryFactory)
	err = c.server.Listen()
	if err != nil {
		panic("Unable to listen")
	}
	go func() {
		_ = c.server.AcceptLoop()
	}()
}

func (c *calcHandler) Add(num1 int32, num2 int32) (r int32, err error) {
	sleepAmnt := atomic.LoadInt64(&c.forcedSleep)
	if sleepAmnt > 0 {
		time.Sleep(time.Millisecond * time.Duration(sleepAmnt))
	}
	return num1 + num2, nil
}

func TestThriftConnect(t *testing.T) {
	h1 := calcHandler{}
	h2 := calcHandler{}
	port1 := nettest.FreeTCPPort()
	h1.Start(port1, time.Second)

	port2 := nettest.FreeTCPPort()
	h2.Start(port2, time.Second)

	s2 := zktest.New()
	z, ch, _ := s2.Connect()
	z2, ch2, _ := s2.Connect()
	zkConnFunc := ZkConnCreatorFunc(func() (ZkConn, <-chan zk.Event, error) {
		zkp, err := zkplus.NewBuilder().PathPrefix("/test").Connector(&zkplus.StaticConnector{C: z, Ch: ch}).Build()
		return zkp, zkp.EventChan(), err
	})
	zkConnFunc2 := ZkConnCreatorFunc(func() (ZkConn, <-chan zk.Event, error) {
		zkp, err := zkplus.NewBuilder().PathPrefix("/test").Connector(&zkplus.StaticConnector{C: z2, Ch: ch2}).Build()
		return zkp, zkp.EventChan(), err
	})

	d1, err := New(zkConnFunc, "localhost", nil)
	assert.NoError(t, err)

	d2, err := New(zkConnFunc2, "localhost", nil)
	assert.NoError(t, err)

	assert.NoError(t, d1.Advertise("testing", struct{}{}, port1))
	assert.NoError(t, d2.Advertise("testing", struct{}{}, port2))

	service, err := d1.Services("testing")
	assert.NoError(t, err)

	transport := NewThriftTransport(service, time.Millisecond*250)
	log.IfErr(log.Panic, transport.Open())

	time.Sleep(time.Millisecond * 10)

	thriftClient := NewCalculatorClientFactory(transport, thrift.NewTBinaryProtocolFactoryDefault())
	num, err := thriftClient.Add(1, 2)
	assert.NoError(t, err)
	assert.Equal(t, int32(3), num)

	atomic.StoreInt64(&h1.forcedSleep, int64(time.Second))
	atomic.StoreInt64(&h2.forcedSleep, int64(time.Second))

	num, err = thriftClient.Add(3, 2)
	assert.Equal(t, int32(0), num)
	assert.Error(t, err)

	atomic.StoreInt64(&h1.forcedSleep, int64(0))
	atomic.StoreInt64(&h2.forcedSleep, int64(0))

	for i := 0; i < 2; i++ {
		assert.NoError(t, transport.NextConnection())

		num, err = thriftClient.Add(3, 2)
		assert.NoError(t, err)
		assert.Equal(t, int32(5), num)
	}

	assert.NoError(t, h1.server.Stop())
	assert.NoError(t, h1.transport.Close())

	for i := 0; i < 3; i++ {
		assert.NoError(t, transport.NextConnection())

		num, err = thriftClient.Add(3, 2)
		assert.NoError(t, err)
		assert.Equal(t, int32(5), num)
	}

	listenSocket, err := net.Listen("tcp", fmt.Sprintf("localhost:%d", port1))
	assert.NoError(t, err)
	defer func() {
		log.IfErr(log.Panic, listenSocket.Close())
	}()

	found := false
	for j := 0; j < 32; j++ {
		assert.NoError(t, transport.NextConnection())
		num, err = thriftClient.Add(3, 2)
		if err != nil {
			continue
		}
		found = true
		assert.Equal(t, int32(5), num)
		break
	}
	assert.True(t, found)
}
