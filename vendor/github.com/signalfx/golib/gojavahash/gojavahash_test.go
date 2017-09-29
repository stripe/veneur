package gojavahash

import (
	"errors"
	. "github.com/smartystreets/goconvey/convey"
	"math/rand"
	"net"
	"testing"
	"time"
)

type fakeaddr struct {
	network string
	name    string
}

func (f fakeaddr) Network() string {
	return f.network
}

func (f fakeaddr) String() string {
	return f.name
}

var _ net.Addr = fakeaddr{}

func faker(s string) (net.Addr, error) {
	return fakeaddr{network: "blarg", name: "192.168.10." + s[len(s)-1:] + ":11211"}, nil
}

func TestGoJavaHash(t *testing.T) {
	Convey("With a new JavaServerSelector", t, func() {
		js := &JavaServerSelector{lookuper: faker}
		a, err := js.PickServer("test")
		So(a, ShouldBeNil)
		So(err, ShouldNotBeNil)
		Convey("Add Single Server should work", func() {
			err := js.AddServer("testdata1:11211")
			So(err, ShouldBeNil)
			a, err := js.PickServer("test")
			So(err, ShouldBeNil)
			So(a.String(), ShouldEqual, "192.168.10.1:11211")
			Convey("Add More Servers should work", func() {
				err = js.AddServer("testdata2:11211")
				So(err, ShouldBeNil)
				err = js.AddServer("testdata3:11211")
				So(err, ShouldBeNil)
				Convey("Pick server should pick the correct server", func() {
					a, err = js.PickServer("FdskjlLkdsflJsd")
					So(err, ShouldBeNil)
					So(a.String(), ShouldEqual, "192.168.10.1:11211")
				})
				Convey("For each should address all servers and return errors returned", func() {
					addrs := []net.Addr{}
					err = js.Each(func(a net.Addr) error {
						addrs = append(addrs, a)
						return nil
					})
					So(len(addrs), ShouldEqual, 3)
					So(err, ShouldBeNil)
					err = js.Each(func(a net.Addr) error {
						return errors.New("nope")
					})
					So(err, ShouldNotBeNil)
				})
			})
		})
	})
	Convey("An invalid server name should fail", t, func() {
		_, err := New("blarg:11211")
		So(err, ShouldNotBeNil)
	})
	Convey("An invalid port should fail", t, func() {
		_, err := New("blarg11211")
		So(err, ShouldNotBeNil)
	})
	Convey("A unix name should work", t, func() {
		_, err := New("localhost/127.0.0.1:11211")
		So(err, ShouldBeNil)
	})
	Convey("With a new JavaServerSelector with small reps", t, func() {
		js := &JavaServerSelector{numReps: 1, lookuper: faker}
		err := js.AddServer("testdata1:11211")
		So(err, ShouldBeNil)
		err = js.AddServer("testdata2:11211")
		So(err, ShouldBeNil)
		So(len(js.ring), ShouldEqual, 2)
		a, err := js.PickServer("UndWAeWGmN")
		So(err, ShouldBeNil)
		So(a.String(), ShouldEqual, "192.168.10.1:11211")
	})
}

func init() {
	rand.Seed(time.Now().UnixNano())
}

var letterRunes = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")

func RandStringRunes(n int) string {
	b := make([]rune, n)
	for i := range b {
		b[i] = letterRunes[rand.Intn(len(letterRunes))]
	}
	return string(b)
}
