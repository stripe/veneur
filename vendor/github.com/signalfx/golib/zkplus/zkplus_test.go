package zkplus

import (
	"testing"

	"errors"
	"time"

	"github.com/samuel/go-zookeeper/zk"
	"github.com/signalfx/golib/log"
	"github.com/signalfx/golib/zkplus/zktest"
	"github.com/stretchr/testify/assert"
)

func TestPrefix(t *testing.T) {
	z, ch, _ := zktest.New().Connect()
	zkp, err := NewBuilder().PathPrefix("/test").Connector(&StaticConnector{C: z, Ch: ch}).Build()
	defer zkp.Close()
	assert.NoError(t, err)
	testPrefix(t, zkp)
}

func testPrefix(t *testing.T, zkp *ZkPlus) {
	defer func() {
		log.IfErr(log.Panic, zktest.EnsureDelete(zkp, "modifyNode"))
	}()
	s, err := zkp.Create("modifyNode", []byte("v1"), 0, zk.WorldACL(zk.PermAll))

	assert.NoError(t, err)
	assert.Equal(t, s, "modifyNode")

	b, _, err := zkp.Exists("modifyNode_NOTHERE")
	assert.NoError(t, err)
	assert.False(t, b)

	b, _, err = zkp.Exists("modifyNode")
	assert.NoError(t, err)
	assert.True(t, b)

	bt, _, err := zkp.Get("modifyNode")
	assert.NoError(t, err)
	assert.Equal(t, "v1", string(bt))

	chil, st, err := zkp.Children("/")
	assert.NoError(t, err)
	assert.Equal(t, []string{"modifyNode"}, chil)

	err = zkp.Delete("modifyNode_NOTHERE", st.Version)
	assert.Error(t, err)

	st, err = zkp.Set("modifyNode", []byte("v2"), st.Version)
	assert.NoError(t, err)

	bt, _, err = zkp.Get("modifyNode")
	assert.NoError(t, err)
	assert.Equal(t, "v2", string(bt))

	err = zkp.Delete("modifyNode", st.Version)
	assert.NoError(t, err)
}

func TestErrorEnsureRoot(t *testing.T) {
	zkp := &ZkPlus{
		pathPrefix: "/a/b/c",
	}
	z, ch, _ := zktest.New().Connect()
	createError := make(chan struct{}, 3)
	z.SetErrorCheck(func(s string) error {
		if s == "create" {
			createError <- struct{}{}
			return errors.New("i don't allow create")
		}
		return nil
	})
	assert.Error(t, zkp.ensureRootPath(z))
	<-createError

	zkp, err := NewBuilder().PathPrefix("/test").Connector(&StaticConnector{C: z, Ch: ch}).Build()

	assert.NoError(t, err)
	<-createError
	zkp.Close()
}

func TestWatches(t *testing.T) {
	z, ch, _ := zktest.New().Connect()
	zkp, err := NewBuilder().PathPrefix("/test").Connector(&StaticConnector{C: z, Ch: ch}).Build()
	defer zkp.Close()
	assert.NoError(t, err)
	testWatches(t, zkp)
}

func testWatches(t *testing.T, zkp *ZkPlus) {
	ch := zkp.EventChan()
	defer func() {
		log.IfErr(log.Panic, zktest.EnsureDelete(zkp, "testWatches"))
	}()

	create(t, zkp)

	bytes, st, ch2, err := zkp.GetW("/testWatches")
	assert.NoError(t, err)
	assert.Equal(t, "v1", string(bytes))

	_, err = zkp.Set("/testWatches", []byte("v2"), st.Version)
	assert.NoError(t, err)

	select {
	case ev := <-ch:
		assert.Equal(t, "/testWatches", ev.Path)
		assert.Equal(t, zk.EventNodeDataChanged, ev.Type)
	case <-time.After(time.Second * 2):
		t.Error("Time out waiting for event")
		return
	}

	select {
	case ev := <-ch2:
		// Note: the sub channel currently doesn't change the path
		assert.Equal(t, zk.EventNodeDataChanged, ev.Type)
		assert.Equal(t, ev.Path, "/test/testWatches")
	case <-time.After(time.Second * 2):
		t.Error("Time out waiting for event")
		return
	}

	_, _, ch2, err = zkp.ChildrenW("")
	assert.NoError(t, err)

	e, _, _ := zkp.Exists("/testWatches")
	assert.True(t, e)

	assert.NoError(t, zktest.EnsureDelete(zkp, "/testWatches"))

	select {
	case ev := <-ch:
		assert.Equal(t, "/", ev.Path)
	case <-time.After(time.Second * 2):
		t.Error("Time out waiting for event")
		return
	}

	select {
	case ev := <-ch2:
		// Note: the sub channel currently doesn't change the path
		assert.Equal(t, ev.Path, "/test")
	case <-time.After(time.Second * 2):
		t.Error("Time out waiting for event")
		return
	}
}

func create(t *testing.T, zkp *ZkPlus) {
	ch := zkp.EventChan()
	exists, _, ch2, err := zkp.ExistsW("testWatches")
	assert.NoError(t, err)
	assert.False(t, exists)

	s, err := zkp.Create("testWatches", []byte("v1"), 0, zk.WorldACL(zk.PermAll))

	assert.NoError(t, err)
	assert.Equal(t, "testWatches", s)

outer:
	for {
		select {
		case ev := <-ch:
			if ev.State != zk.StateConnected {
				continue
			}
			assert.Equal(t, "/testWatches", ev.Path)
			break outer
		case <-time.After(time.Second * 2):
			t.Error("Time out waiting for event")
			return
		}
	}

	select {
	case ev := <-ch2:
		// Note: the sub channel currently doesn't change the path
		assert.Contains(t, ev.Path, "testWatches")
	case <-time.After(time.Second * 2):
		t.Error("Time out waiting for event")
		return
	}
}

func TestBadConnection(t *testing.T) {
	z, err := NewBuilder().PathPrefix("/test").Connector(ZkConnectorFunc(func() (zktest.ZkConnSupported, <-chan zk.Event, error) {
		return nil, nil, errors.New("unable to connect")
	})).Build()
	assert.NoError(t, err)
	var conn zktest.ZkConnSupported
	go func() {
		conn = z.blockOnConn()
	}()
	time.Sleep(200 * time.Millisecond)
	assert.Nil(t, conn)
	z.Close()
}
