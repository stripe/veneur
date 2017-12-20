package distconf

import (
	"errors"
	"testing"
	"time"

	"github.com/samuel/go-zookeeper/zk"
	"github.com/signalfx/golib/log"
	"github.com/signalfx/golib/zkplus/zktest"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/assert"
)

func TestZkNewlyCreated(t *testing.T) {
	zkServer := zktest.New()
	z, err := Zk(ZkConnectorFunc(func() (ZkConn, <-chan zk.Event, error) {
		return zkServer.Connect()
	}), &ZkConfig{})
	defer z.Close()
	assert.NoError(t, err)
	seenCallbacks := make(chan string, 2)
	callback := func(s string) {
		seenCallbacks <- s
	}
	log.IfErr(log.Panic, z.(Dynamic).Watch("hello", callback))
	assert.Equal(t, 0, len(seenCallbacks))
	assert.Nil(t, z.Write("hello", []byte("test")))
	seenString := <-seenCallbacks
	assert.Equal(t, "hello", seenString)
}

func TestZkConf(t *testing.T) {
	DefaultLogger.Log("TestZkConf")
	zkServer := zktest.New()
	z, err := Zk(ZkConnectorFunc(func() (ZkConn, <-chan zk.Event, error) {
		return zkServer.Connect()
	}), &ZkConfig{})
	defer z.Close()
	assert.NoError(t, err)

	b, err := z.Get("TestZkConf")
	assert.NoError(t, err)
	assert.Nil(t, b)

	assert.NoError(t, z.Write("TestZkConf", nil))

	signalChan := make(chan string, 4)
	DefaultLogger.Log("Setting watches")
	log.IfErr(log.Panic, z.(Dynamic).Watch("TestZkConf", backingCallbackFunction(func(S string) {
		DefaultLogger.Log("Watch fired!")
		assert.Equal(t, "TestZkConf", S)
		signalChan <- S
	})))

	// The write should work and I should get a single signal on the chan
	DefaultLogger.Log("Doing write 1")
	assert.NoError(t, z.Write("TestZkConf", []byte("newval")))
	DefaultLogger.Log("Write done")
	b, err = z.Get("TestZkConf")
	DefaultLogger.Log("Get done")
	assert.NoError(t, err)
	assert.Equal(t, []byte("newval"), b)
	DefaultLogger.Log("Blocking for values")
	res := <-signalChan
	assert.Equal(t, "TestZkConf", res)

	// Should send another signal
	DefaultLogger.Log("Doing write 2")
	assert.NoError(t, z.Write("TestZkConf", []byte("newval_v2")))
	res = <-signalChan
	assert.Equal(t, "TestZkConf", res)

	DefaultLogger.Log("Doing write 3")
	assert.NoError(t, z.Write("TestZkConf", nil))
	select {
	case res = <-signalChan:
	case <-time.After(time.Second * 3):
	}
	assert.Equal(t, "TestZkConf", res)
}

func TestCloseNormal(t *testing.T) {
	zkServer := zktest.New()
	zkServer.SetErrorCheck(func(s string) error {
		return errors.New("nope")
	})

	z, err := Zk(ZkConnectorFunc(func() (ZkConn, <-chan zk.Event, error) {
		return zkServer.Connect()
	}), nil)
	assert.NoError(t, err)

	z.Close()

	// Should not deadlock
	<-z.(*zkConfig).shouldQuit
}

func TestCallbackMap(t *testing.T) {
	Convey("when callback map is made", t, func() {
		m := callbackMap{
			callbacks: map[string][]backingCallbackFunction{},
		}
		Convey("Unknown key gets should return empty list", func() {
			l := m.get("unknown")
			So(l, ShouldNotBeNil)
			So(len(l), ShouldEqual, 0)
		})
	})
}

func TestErrorReregister(t *testing.T) {
	zkServer := zktest.New()
	zkServer.ChanTimeout = time.Millisecond

	z, err := Zk(ZkConnectorFunc(func() (ZkConn, <-chan zk.Event, error) {
		return zkServer.Connect()
	}), nil)
	assert.NoError(t, err)
	defer z.Close()
	log.IfErr(log.Panic, z.(Dynamic).Watch("hello", func(string) {

	}))
	zkServer.SetErrorCheck(func(s string) error {
		return errors.New("nope")
	})
	z.(*zkConfig).setRefreshDelay(time.Millisecond)
	go func() {
		time.Sleep(time.Millisecond * 10)
		zkServer.SetErrorCheck(nil)
	}()
	z.(*zkConfig).refreshWatches(DefaultLogger)
}

func TestCloseQuitChan(t *testing.T) {
	zkServer := zktest.New()
	zkServer.SetErrorCheck(func(s string) error {
		return errors.New("nope")
	})

	z, err := Zk(ZkConnectorFunc(func() (ZkConn, <-chan zk.Event, error) {
		return zkServer.Connect()
	}), nil)
	assert.NoError(t, err)

	// Should not deadlock
	close(z.(*zkConfig).shouldQuit)

	// Give drain() loop time to exit, for code coverage
	time.Sleep(time.Millisecond * 100)
}

func TestZkConfErrors(t *testing.T) {
	zkServer := zktest.New()
	zkServer.SetErrorCheck(func(s string) error {
		return errors.New("nope")
	})
	zkServer.ChanTimeout = time.Millisecond * 10

	z, err := Zk(ZkConnectorFunc(func() (ZkConn, <-chan zk.Event, error) {
		return zkServer.Connect()
	}), nil)
	defer z.Close()
	assert.NoError(t, err)

	_, err = z.Get("TestZkConfErrors")
	assert.Error(t, err)

	assert.Error(t, z.(Dynamic).Watch("TestZkConfErrors", nil))
	assert.Error(t, z.Write("TestZkConfErrors", nil))
	assert.Error(t, z.(*zkConfig).reregisterWatch("TestZkConfErrors", DefaultLogger))

	z.(*zkConfig).conn.Close()

	//	zkp.GlobalChan <- zk.Event{
	//		State: zk.StateDisconnected,
	//	}
	//	// Let the thread switch back to get code coverage
	time.Sleep(10 * time.Millisecond)
	//	zkp.Close()
}

func TestErrorLoader(t *testing.T) {
	_, err := Zk(ZkConnectorFunc(func() (ZkConn, <-chan zk.Event, error) {
		return nil, nil, errors.New("nope")
	}), nil)
	assert.Error(t, err)
}
