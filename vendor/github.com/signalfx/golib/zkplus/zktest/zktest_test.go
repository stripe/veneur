package zktest

import (
	"fmt"
	"math/rand"
	"testing"
	"time"

	"errors"

	"github.com/samuel/go-zookeeper/zk"
	"github.com/signalfx/golib/log"
	"github.com/stretchr/testify/assert"
)

func TestAfterClose(t *testing.T) {
	s := New()
	z, _, _ := s.Connect()
	z.Close()
	assert.Nil(t, z.patchWatch("/"))
	z.offerEvent(zk.Event{})
}

func ensureCreate(s string, err error) {
	log.IfErr(log.Panic, err)
}

func TestEnsureDelete(t *testing.T) {
	assert.Equal(t, ErrDeleteOnRoot, EnsureDelete(nil, "/"))

	s := New()
	z, _, _ := s.Connect()
	ensureCreate(z.Create("/bob", []byte(""), 0, nil))
	ensureCreate(z.Create("/bob/bob2", []byte(""), 0, nil))
	i := 0
	z.SetErrorCheck(func(s string) error {
		if s == "delete" {
			return errors.New("delete fails")
		}
		i++
		if i >= 2 {
			return errors.New("second ")
		}
		return nil
	})
	assert.Error(t, EnsureDelete(z, "/bob"))
	s.SetErrorCheck(func(s string) error {
		if s == "delete" {
			return errors.New("delete fails")
		}
		return nil
	})
	assert.Equal(t, ErrDeleteFailed, EnsureDelete(z, "/bob"))
}

func TestFixPath(t *testing.T) {
	assert.Equal(t, "/bob", fixPath("bob"))
}

func TestForcedErrorCheck(t *testing.T) {
	s := New()
	s.ChanTimeout = time.Millisecond * 250
	s.SetErrorCheck(func(s string) error {
		if s == "delete" {
			return errors.New("delete fails")
		}
		return nil
	})
	z1, _, _ := s.Connect()
	err := z1.Delete("/", 0)
	assert.Error(t, err)
	s.SetErrorCheck(nil)
	z1.SetErrorCheck(func(s string) error {
		return errors.New("nope")
	})

	f := func(z *ZkConn) {
		_, err := z.Create("/bob", []byte(""), 0, nil)
		assert.Error(t, err)

		_, _, _, err = z.GetW("/bob")
		assert.Error(t, err)

		_, _, _, err = z.ChildrenW("/bob")
		assert.Error(t, err)

		_, _, err = z.Get("/bob")
		assert.Error(t, err)

		_, _, err = z.Exists("/bob")
		assert.Error(t, err)

		_, _, _, err = z.ExistsW("/bob")
		assert.Error(t, err)
	}

	f(z1)
	z2, _, _ := s.Connect()
	s.SetErrorCheck(func(s string) error {
		return errors.New("nope")
	})
	f(z2)

	s.SetErrorCheck(nil)
	_, _, ch, err := z2.ChildrenW("/")
	assert.NoError(t, err)
	b := make(chan struct{})
	go func() {
		<-ch
		b <- struct{}{}
	}()
	z2.Close()
	<-b
}

func EnsureDeleteTesting(t *testing.T, z ZkConnSupported, path string) {
	assert.NoError(t, EnsureDelete(z, path))
}

func TestBasics(t *testing.T) {
	s := New()
	z, _, _ := s.Conn()
	defer z.Close()
	testBasics(t, z)
}

func testBasics(t *testing.T, z ZkConnSupported) {
	rand.Seed(time.Now().UnixNano())
	ensureCreate(z.Create("/test", []byte(""), 0, zk.WorldACL(zk.PermAll)))
	ensureCreate(z.Create("/test/testBasics", []byte(""), 0, zk.WorldACL(zk.PermAll)))
	prefix := fmt.Sprintf("/test/testBasics/%d", rand.Intn(10000))
	EnsureDeleteTesting(t, z, prefix)
	time.Sleep(time.Millisecond * 100)
	b, _, err := z.Exists(prefix)
	assert.NoError(t, err)
	assert.False(t, b)

	defer EnsureDeleteTesting(t, z, prefix)
	p, err := z.Create(prefix, []byte("parent"), 0, zk.WorldACL(zk.PermAll))
	assert.NoError(t, err)
	assert.Equal(t, prefix, p)

	_, err = z.Create(prefix, []byte(""), 0, zk.WorldACL(zk.PermAll))
	assert.Equal(t, zk.ErrNodeExists, err)

	EnsureDeleteTesting(t, z, prefix+"/test")
	b, _, _ = z.Exists(prefix + "/test")
	assert.False(t, b)
	p, err = z.Create(prefix+"/test", []byte("hi"), 0, zk.WorldACL(zk.PermAll))
	assert.NoError(t, err)
	assert.Equal(t, prefix+"/test", p)

	defer EnsureDeleteTesting(t, z, prefix+"/test")

	ex, _, _ := z.Exists(prefix + "/test")
	assert.True(t, ex)

	data, _, err := z.Get(prefix + "/test")
	assert.NoError(t, err)

	assert.Equal(t, "hi", string(data))

	assert.Error(t, z.Delete(prefix, 0))

	_, err = z.Set(prefix+"/test", []byte("v2"), 0)
	assert.NoError(t, err)

	data, _, _ = z.Get(prefix + "/test")
	assert.Equal(t, "v2", string(data))

	c, _, err := z.Children(prefix)
	assert.NoError(t, err)
	assert.Equal(t, []string{"test"}, c)

	_, _, err = z.Get(prefix + "/_nothere")
	assert.Equal(t, zk.ErrNoNode, err)

	_, _, err = z.Children(prefix + "/_nothere")
	assert.Equal(t, zk.ErrNoNode, err)

	err = z.Delete(prefix+"/_nothere", 0)
	assert.Equal(t, zk.ErrNoNode, err)

	err = z.Delete(prefix+"/test", 3)
	assert.Equal(t, zk.ErrBadVersion, err)

	EnsureDeleteTesting(t, z, prefix+"/test")
	ex, _, _ = z.Exists(prefix + "/test")
	assert.False(t, ex)

	EnsureDeleteTesting(t, z, prefix)

	_, err = z.Create(prefix+"/_nothere", []byte(""), 0, zk.WorldACL(zk.PermAll))
	assert.Equal(t, zk.ErrNoNode, err)
}

func TestSet(t *testing.T) {
	s := New()
	z, _, _ := s.Connect()
	testSet(t, z)
	assert.Contains(t, s.Pretty(), "testSet")
}

func testSet(t *testing.T, z ZkConnSupported) {
	rand.Seed(time.Now().UnixNano())
	ensureCreate(z.Create("/test", []byte(""), 0, zk.WorldACL(zk.PermAll)))
	ensureCreate(z.Create("/test/testSet", []byte(""), 0, zk.WorldACL(zk.PermAll)))
	prefix := fmt.Sprintf("/test/testSet/%d", rand.Intn(10000))

	_, err := z.Set(prefix, []byte(""), 0)
	assert.Equal(t, zk.ErrNoNode, err)

	EnsureDeleteTesting(t, z, prefix)
	defer EnsureDeleteTesting(t, z, prefix)

	_, err = z.Create(prefix, []byte("v1"), 0, zk.WorldACL(zk.PermAll))
	assert.NoError(t, err)

	_, err = z.Set(prefix, []byte("v2"), 0)
	assert.NoError(t, err)

	_, err = z.Set(prefix, []byte("v3"), 2)
	assert.Equal(t, zk.ErrBadVersion, err)
}

func TestExistsW(t *testing.T) {
	s := New()
	z, e, _ := s.Connect()
	testExistsW(t, z, e)
}

func testExistsW(t *testing.T, z ZkConnSupported, e <-chan zk.Event) {
	rand.Seed(time.Now().UnixNano())
	ensureCreate(z.Create("/test", []byte(""), 0, zk.WorldACL(zk.PermAll)))
	ensureCreate(z.Create("/test/testEvents", []byte(""), 0, zk.WorldACL(zk.PermAll)))
	prefix := fmt.Sprintf("/test/testEvents/%d", rand.Intn(10000))

	// Drain events chan before going forward
out1:
	for {
		select {
		case <-e:
		default:
			break out1
		}
	}

	EnsureDeleteTesting(t, z, prefix)
	defer EnsureDeleteTesting(t, z, prefix)

	exists, _, ch, err := z.ExistsW(prefix)
	assert.NoError(t, err)
	assert.False(t, exists)
	assert.Equal(t, 0, len(ch))
	assert.Equal(t, 0, len(e))

	p, err := z.Create(prefix, []byte("parent"), 0, zk.WorldACL(zk.PermAll))
	assert.NoError(t, err)
	assert.Equal(t, prefix, p)

	select {
	case ev := <-ch:
		assert.Equal(t, prefix, ev.Path)
	case <-time.After(time.Second * 2):
		t.Error("Time out waiting for event")
		return
	}

	select {
	case ev := <-e:
		assert.Equal(t, zk.EventNodeCreated, ev.Type)
		assert.Equal(t, prefix, ev.Path)
	case <-time.After(time.Second * 2):
		t.Error("Time out waiting for event")
	}
}

func TestGetW(t *testing.T) {
	s := New()
	z, e, _ := s.Connect()
	testGetW(t, z, e)
}

func testGetW(t *testing.T, z ZkConnSupported, e <-chan zk.Event) {
	rand.Seed(time.Now().UnixNano())
	ensureCreate(z.Create("/test", []byte(""), 0, zk.WorldACL(zk.PermAll)))
	ensureCreate(z.Create("/test/testGetW", []byte(""), 0, zk.WorldACL(zk.PermAll)))
	prefix := fmt.Sprintf("/test/testGetW/%d", rand.Intn(10000))

	// Drain events chan before going forward
out1:
	for {
		select {
		case <-e:
		default:
			break out1
		}
	}

	_, _, _, err := z.GetW(prefix)
	assert.Error(t, err)
	p, err := z.Create(prefix, []byte(""), 0, zk.WorldACL(zk.PermAll))
	assert.NoError(t, err)
	assert.Equal(t, prefix, p)

	_, st, ch, err := z.GetW(prefix)
	assert.NoError(t, err)

	st, err = z.Set(prefix, []byte("new"), st.Version)
	assert.NoError(t, err)

	select {
	case ev := <-e:
		assert.Equal(t, prefix, ev.Path)
	case <-time.After(time.Second * 2):
		t.Error("Time out waiting for event")
	}

	select {
	case ev := <-ch:
		assert.Equal(t, prefix, ev.Path)
	case <-time.After(time.Second * 2):
		t.Error("Time out waiting for event")
	}
}

func TestChildrenW(t *testing.T) {
	s := New()
	z1, e, _ := s.Connect()
	z2, _, _ := s.Connect()
	testChildrenW(t, z1, z2, e)
}

func testChildrenW(t *testing.T, z ZkConnSupported, z2 ZkConnSupported, e <-chan zk.Event) {
	rand.Seed(time.Now().UnixNano())
	ensureCreate(z.Create("/test", []byte(""), 0, zk.WorldACL(zk.PermAll)))
	ensureCreate(z.Create("/test/testChildrenW", []byte(""), 0, zk.WorldACL(zk.PermAll)))
	prefix := fmt.Sprintf("/test/testChildrenW/%d", rand.Intn(10000))

	EnsureDeleteTesting(t, z, prefix)
	defer func() {
		go EnsureDeleteTesting(t, z, prefix)
	}()
	_, err := z.Create(prefix, []byte(""), 0, zk.WorldACL(zk.PermAll))
	assert.NoError(t, err)

	_, _, _, err = z.ChildrenW(prefix + "/bob/bob")
	assert.Error(t, err)

	c, _, ch, err := z.ChildrenW(prefix)
	assert.NoError(t, err)
	assert.Equal(t, 0, len(c))

	// Drain events chan before going forward
out1:
	for {
		select {
		case <-e:
		default:
			break out1
		}
	}

	p, err := z2.Create(prefix+"/testChildrenW", []byte(""), 0, zk.WorldACL(zk.PermAll))
	assert.NoError(t, err)
	assert.Equal(t, prefix+"/testChildrenW", p)

	select {
	case ev := <-ch:
		assert.Equal(t, prefix, ev.Path)
	case <-time.After(time.Second * 2):
		t.Error("Time out waiting for event")
	}

	select {
	case ev := <-e:
		assert.Equal(t, zk.EventNodeChildrenChanged, ev.Type)
		assert.Equal(t, prefix, ev.Path)
	case <-time.After(time.Second * 2):
		t.Error("Time out waiting for event")
	}
}

func TestChildrenWNotHere(t *testing.T) {
	s := New()
	z1, e, _ := s.Connect()
	z2, _, _ := s.Connect()
	testChildrenWNotHere(t, z1, z2, e)
}

func testChildrenWNotHere(t *testing.T, z ZkConnSupported, z2 ZkConnSupported, e <-chan zk.Event) {
	rand.Seed(time.Now().UnixNano())
	ensureCreate(z.Create("/test", []byte(""), 0, zk.WorldACL(zk.PermAll)))
	ensureCreate(z.Create("/test/testChildrenWNotHere", []byte(""), 0, zk.WorldACL(zk.PermAll)))
	prefix := fmt.Sprintf("/test/testChildrenWNotHere/%d", rand.Intn(10000))
	EnsureDeleteTesting(t, z, prefix)
	defer func() {
		go EnsureDeleteTesting(t, z, prefix)
	}()

	_, _, e, err := z.ChildrenW(prefix)
	assert.Equal(t, zk.ErrNoNode, err)
	ensureCreate(z2.Create(prefix, []byte(""), 0, zk.WorldACL(zk.PermAll)))
	ensureCreate(z2.Create(prefix+"/test", []byte(""), 0, zk.WorldACL(zk.PermAll)))
	select {
	case <-e:
		panic("Should never see event!")
	case <-time.After(time.Second):
	}
}
