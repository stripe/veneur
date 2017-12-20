package disco

import (
	"bytes"
	"runtime"
	"strings"
	"testing"

	"github.com/signalfx/golib/errors"

	"fmt"
	"github.com/samuel/go-zookeeper/zk"
	"github.com/signalfx/golib/log"
	"github.com/signalfx/golib/zkplus"
	"github.com/signalfx/golib/zkplus/zktest"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUnableToConn(t *testing.T) {
	_, err := New(ZkConnCreatorFunc(func() (ZkConn, <-chan zk.Event, error) {
		return nil, nil, errors.New("bad")
	}), "", nil)
	require.Error(t, err)
}

func TestBadRandomSource(t *testing.T) {
	r := strings.NewReader("")
	_, err := New(nil, "", &Config{RandomSource: r, Logger: log.Discard})
	require.Error(t, err)
}

func TestAdvertiseInZKErrs(t *testing.T) {
	s := zktest.New()
	z, ch, _ := s.Connect()
	b := zkplus.NewBuilder().PathPrefix("/test").Connector(&zkplus.StaticConnector{C: z, Ch: ch})
	_, err := z.Create("/test", []byte(""), 0, zk.WorldACL(zk.PermAll))
	log.IfErr(log.Panic, err)
	d1, _ := New(BuilderConnector(b), "TestDupAdvertise", nil)
	require.Nil(t, d1.Advertise("service1", "", uint16(1234)))
	e1 := errors.New("set error check during delete")

	z.SetErrorCheck(func(s string) error {
		if s == "delete" {
			return e1
		}
		return nil
	})
	require.Contains(t, errors.Details(d1.advertiseInZK(true, "service1", ServiceInstance{})), e1.Error())

	z.SetErrorCheck(func(s string) error {
		if s == "exists" {
			return e1
		}
		return nil
	})
	require.Equal(t, e1, errors.Tail(d1.advertiseInZK(false, "", ServiceInstance{})))
}

func TestDupAdvertise(t *testing.T) {
	s := zktest.New()
	z, ch, _ := s.Connect()
	testDupAdvertise(t, z, ch)

}

func testDupAdvertise(t *testing.T, z zktest.ZkConnSupported, ch <-chan zk.Event) {
	_, err := z.Create("/test", []byte(""), 0, zk.WorldACL(zk.PermAll))
	log.IfErr(log.Panic, err)

	b := zkplus.NewBuilder().PathPrefix("/test").Connector(&zkplus.StaticConnector{C: z, Ch: ch})
	myID := "AAAAAAAAAAAAAAAA"
	guidStr := "41414141414141414141414141414141"
	d1, _ := New(BuilderConnector(b), "TestDupAdvertise", &Config{RandomSource: bytes.NewBufferString(myID)})
	defer d1.Close()
	_, err = z.Create("/test/t1", []byte(""), 0, zk.WorldACL(zk.PermAll))
	log.IfErr(log.Panic, err)
	_, err = z.Create(fmt.Sprintf("/test/t1/%s", guidStr), []byte("nope"), 0, zk.WorldACL(zk.PermAll))
	require.Nil(t, err)
	require.Nil(t, d1.Advertise("t1", "", uint16(1234)))
	data, _, err := z.Get(fmt.Sprintf("/test/t1/%s", guidStr))
	require.Nil(t, err)
	require.NotEqual(t, string(data), "nope")
	close(d1.manualEvents)
	require.Equal(t, ErrDuplicateAdvertise, d1.Advertise("t1", "", uint16(1234)))
}

func TestNinjaMode(t *testing.T) {
	s := zktest.New()
	z, ch, _ := s.Connect()
	b := zkplus.NewBuilder().PathPrefix("/test").Connector(&zkplus.StaticConnector{C: z, Ch: ch})
	d1, err := New(BuilderConnector(b), "TestDupAdvertise", &Config{RandomSource: bytes.NewBufferString("AAAAAAAAAAAAAAAA")})
	require.NoError(t, err)
	d1.NinjaMode(true)
	require.Nil(t, d1.Advertise("test", nil, uint16(1234)))

	d2, _ := New(BuilderConnector(b), "TestDupAdvertise2", &Config{RandomSource: bytes.NewBufferString("AAAAAAAAAAAAAAAA")})
	serv, err := d2.Services("test")
	require.NoError(t, err)
	require.Equal(t, 0, len(serv.ServiceInstances()))
}

func TestJsonMarshalBadAdvertise(t *testing.T) {
	s := zktest.New()
	z, ch, _ := s.Connect()
	b := zkplus.NewBuilder().PathPrefix("/test").Connector(&zkplus.StaticConnector{C: z, Ch: ch})

	d1, _ := New(BuilderConnector(b), "TestAdvertise1", &Config{})
	e := errors.New("nope")
	d1.jsonMarshal = func(v interface{}) ([]byte, error) {
		return nil, e
	}

	require.Equal(t, e, errors.Tail(d1.Advertise("TestAdvertiseService", "", (uint16)(1234))))
}

func TestErrorNoRootCreate(t *testing.T) {
	s := zktest.New()
	z, ch, _ := s.Connect()
	zkConnFunc := ZkConnCreatorFunc(func() (ZkConn, <-chan zk.Event, error) {
		zkp, err := zkplus.NewBuilder().PathPrefix("/test").Connector(&zkplus.StaticConnector{C: z, Ch: ch}).Build()
		return zkp, zkp.EventChan(), err
	})

	c := 0
	z.SetErrorCheck(func(s string) error {
		c++
		if c < 2 {
			return nil
		}
		return zk.ErrNoNode
	})

	d1, _ := New(zkConnFunc, "TestAdvertise1", nil)

	require.Equal(t, zk.ErrNoNode, errors.Tail(d1.Advertise("TestAdvertiseService", "", (uint16)(1234))))
}

func TestBadRefresh(t *testing.T) {
	s2 := zktest.New()
	z, ch, _ := s2.Connect()

	badForce := errors.New("nope")
	c := 0
	z.SetErrorCheck(func(s string) error {
		if s == "childrenw" {
			c++
		}
		if c > 2 && s == "childrenw" {
			return badForce
		}
		return nil
	})

	zkConnFunc := ZkConnCreatorFunc(func() (ZkConn, <-chan zk.Event, error) {
		zkp, err := zkplus.NewBuilder().PathPrefix("/test").Connector(&zkplus.StaticConnector{C: z, Ch: ch}).Build()
		return zkp, zkp.EventChan(), err
	})

	d1, _ := New(zkConnFunc, "TestAdvertise1", nil)

	require.NoError(t, d1.Advertise("TestAdvertiseService", "", (uint16)(1234)))
	s, err := d1.Services("TestAdvertiseService")
	require.NoError(t, err)

	d1.manualEvents <- zk.Event{
		Path: "/TestAdvertiseService",
	}
	for {
		runtime.Gosched()
		err := errors.Tail(s.refresh(z))
		if err == badForce {
			break
		}
	}

	z.SetErrorCheck(nil)
	s2.SetErrorCheck(func(s string) error {
		return zk.ErrNoNode
	})
	d1.manualEvents <- zk.Event{
		Path: "/TestAdvertiseService",
	}
	require.NoError(t, s.refresh(z))
	require.Equal(t, 0, len(s.ServiceInstances()))

	z.SetErrorCheck(nil)
	s2.SetErrorCheck(func(s string) error {
		return badForce
	})

	// Verifying that .Services() doesn't cache a bad result
	delete(d1.watchedServices, "TestAdvertiseService2")
	s, err = d1.Services("TestAdvertiseService2")
	require.Error(t, err)
	_, exists := d1.watchedServices["TestAdvertiseService2"]
	require.False(t, exists)
}

func TestBadRefresh2(t *testing.T) {
	s2 := zktest.New()
	z, ch, _ := s2.Connect()
	badForce := errors.New("nope")
	c := 0
	z.SetErrorCheck(func(s string) error {
		if s == "getw" {
			c++
			if c > 1 {
				return badForce
			}
			return nil
		}
		return nil
	})

	zkp, err := zkplus.NewBuilder().PathPrefix("/test").Connector(&zkplus.StaticConnector{C: z, Ch: ch}).Build()
	zkConnFunc := ZkConnCreatorFunc(func() (ZkConn, <-chan zk.Event, error) {
		return zkp, zkp.EventChan(), err
	})

	d1, _ := New(zkConnFunc, "TestAdvertise1", nil)

	require.NoError(t, d1.Advertise("TestAdvertiseService", "", (uint16)(1234)))
	s, err := d1.Services("TestAdvertiseService")
	require.NoError(t, err)

	d1.manualEvents <- zk.Event{
		Path: "/TestAdvertiseService",
	}
	require.Equal(t, badForce, errors.Tail(s.refresh(zkp)))
}

func TestInvalidServiceJson(t *testing.T) {
	s := zktest.New()
	z, ch, _ := s.Connect()
	var zkp *zkplus.ZkPlus
	zkConnFunc := ZkConnCreatorFunc(func() (ZkConn, <-chan zk.Event, error) {
		var err error
		zkp, err = zkplus.NewBuilder().PathPrefix("/test").Connector(&zkplus.StaticConnector{C: z, Ch: ch}).Build()
		return zkp, zkp.EventChan(), err
	})

	d1, _ := New(zkConnFunc, "TestInvalidServiceJson", nil)
	// Give root paths time to create
	_, _, err := zkp.Exists("/")
	log.IfErr(log.Panic, err)
	exists, _, err := z.Exists("/test")
	require.NoError(t, err)
	require.True(t, exists)
	_, err = z.Create("/test/badservice", []byte(""), 0, zk.WorldACL(zk.PermAll))
	require.NoError(t, err)
	_, err = z.Create("/test/badservice/badjson", []byte("badjson"), 0, zk.WorldACL(zk.PermAll))
	require.NoError(t, err)

	_, err = d1.Services("badservice")
	require.Error(t, err)

}

func TestAdvertise(t *testing.T) {
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
	testAdvertise(t, zkConnFunc, zkConnFunc2)
}

func testAdvertise(t *testing.T, zkConnFunc ZkConnCreatorFunc, zkConnFunc2 ZkConnCreatorFunc) {
	d1, err := New(zkConnFunc, "TestAdvertise1", nil)

	require.NoError(t, err)
	require.NotNil(t, d1)
	defer d1.Close()

	service, err := d1.Services("TestAdvertiseService")
	require.NoError(t, err)
	require.Equal(t, 0, len(service.ServiceInstances()))
	require.Equal(t, "name=TestAdvertiseService|len(watch)=0|instances=[]", service.String())
	seen := make(chan struct{}, 5)
	service.Watch(ChangeWatch(func() {
		seen <- struct{}{}
	}))

	require.NoError(t, d1.Advertise("TestAdvertiseService", "", (uint16)(1234)))
	require.Equal(t, "/TestAdvertiseService/"+d1.GUID(), d1.servicePath("TestAdvertiseService"))
	<-seen

	require.NoError(t, err)
	require.Equal(t, 1, len(service.ServiceInstances()))
	require.Equal(t, "TestAdvertise1:1234", service.ServiceInstances()[0].DialString())

	serviceRepeat, err := d1.Services("TestAdvertiseService")
	require.NoError(t, err)
	require.Exactly(t, serviceRepeat, service)

	d2, err := New(zkConnFunc2, "TestAdvertise2", nil)
	require.NoError(t, err)
	require.NotNil(t, d2)
	defer d2.Close()

	require.NoError(t, d2.Advertise("TestAdvertiseService", "", (uint16)(1234)))

	<-seen

	require.Equal(t, 2, len(service.ServiceInstances()))

	assert.Contains(t, d1.Var().String(), "TestAdvertiseService")
}

func TestServices(t *testing.T) {
	zkServer := zktest.New()
	z, ch, _ := zkServer.Connect()
	z2, _, _ := zkServer.Connect()
	testServices(t, z, ch, z2)
}

func testServices(t *testing.T, z1 zktest.ZkConnSupported, ch <-chan zk.Event, z2 zktest.ZkConnSupported) {
	zkConnFunc := ZkConnCreatorFunc(func() (ZkConn, <-chan zk.Event, error) {
		return z1, ch, nil
	})
	d1, err := New(zkConnFunc, "TestAdvertise1", nil)
	require.NoError(t, err)

	log.IfErr(log.Panic, zktest.EnsureDelete(z2, "/not_here"))
	defer func() {
		go func() {
			log.IfErr(log.Panic, zktest.EnsureDelete(z2, "/not_here"))
		}()
	}()

	s, err := d1.Services("not_here")
	require.NoError(t, err)
	require.Equal(t, 0, len(s.ServiceInstances()))

	onWatchChan := make(chan struct{})
	s.Watch(func() {
		onWatchChan <- struct{}{}
	})

	_, err = z2.Create("/not_here", []byte(""), 0, zk.WorldACL(zk.PermAll))
	require.NoError(t, err)

	_, err = z2.Create("/not_here/s1", []byte("{}"), 0, zk.WorldACL(zk.PermAll))
	require.NoError(t, err)
	<-onWatchChan

	require.Equal(t, 1, len(s.ServiceInstances()))
	doneForce := make(chan struct{})
	// go b/c onWatchChan has zero size
	go func() {
		s.ForceInstances([]ServiceInstance{
			{
				Name: "bob",
			},
		})
		close(doneForce)
	}()
	<-onWatchChan
	require.Nil(t, s.refresh(nil))
	<-doneForce

}

func TestDisco_CreatePersistentEphemeralNode(t *testing.T) {
	Convey("test creation of emphemeral nodes", t, func() {
		s2 := zktest.New()
		z, ch, _ := s2.Connect()
		zkConnFunc := ZkConnCreatorFunc(func() (ZkConn, <-chan zk.Event, error) {
			zkp, err := zkplus.NewBuilder().PathPrefix("/test").Connector(&zkplus.StaticConnector{C: z, Ch: ch}).Build()
			return zkp, zkp.EventChan(), err
		})
		d1, err := New(zkConnFunc, "TestCreatePersistentEphemeralNode", nil)
		So(err, ShouldBeNil)
		defer d1.Close()

		So(d1.CreatePersistentEphemeralNode("config/_meta/sbingest", []byte("payload")), ShouldNotBeNil)
		So(len(d1.myEphemeralNodes), ShouldEqual, 0)
		So(d1.CreatePersistentEphemeralNode("_meta", []byte("payload")), ShouldBeNil)
		So(len(d1.myEphemeralNodes), ShouldEqual, 1)

		d1.NinjaMode(true)
		So(d1.CreatePersistentEphemeralNode("ninja", []byte("payload")), ShouldBeNil)
		So(len(d1.myEphemeralNodes), ShouldEqual, 1)
	})
}

func TestDisco_CreatePersistentEphemeralNodeInZKErr(t *testing.T) {
	s := zktest.New()
	z, ch, _ := s.Connect()
	b := zkplus.NewBuilder().PathPrefix("/test").Connector(&zkplus.StaticConnector{C: z, Ch: ch})
	_, err := z.Create("/test", []byte(""), 0, zk.WorldACL(zk.PermAll))
	log.IfErr(log.Panic, err)
	d1, _ := New(BuilderConnector(b), "TestDupAdvertise", nil)
	require.Nil(t, d1.CreatePersistentEphemeralNode("service1", []byte("blarg")))
	e1 := errors.New("set error check during delete")

	z.SetErrorCheck(func(s string) error {
		if s == "delete" {
			return e1
		}
		return nil
	})
	require.NotNil(t, d1.CreatePersistentEphemeralNode("service1", []byte("blarg")))
}
