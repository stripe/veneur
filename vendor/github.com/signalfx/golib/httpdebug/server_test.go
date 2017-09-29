package httpdebug

import (
	"fmt"
	"github.com/signalfx/golib/nettest"
	"github.com/signalfx/golib/pointer"
	. "github.com/smartystreets/goconvey/convey"
	"io/ioutil"
	"net"
	"net/http"
	"testing"
	"time"
)

type toExplore struct {
	name string
	age  int
}

func TestDebugServer(t *testing.T) {
	Convey("debug server should be setupable", t, func() {
		explorable := toExplore{
			name: "bob123",
			age:  10,
		}
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		So(err, ShouldBeNil)
		ser := New(&Config{
			ReadTimeout:   pointer.Duration(time.Millisecond * 100),
			WriteTimeout:  pointer.Duration(time.Millisecond * 100),
			ExplorableObj: explorable,
		})
		listenPort := nettest.TCPPort(listener)
		done := make(chan error)
		go func() {
			done <- ser.Serve(listener)
			close(done)
		}()
		serverURL := fmt.Sprintf("http://127.0.0.1:%d", listenPort)
		Convey("and find commandline", func() {
			resp, err := http.Get(serverURL + "/debug/pprof/cmdline")
			So(err, ShouldBeNil)
			So(resp.StatusCode, ShouldEqual, http.StatusOK)
		})
		Convey("and find explorable", func() {
			resp, err := http.Get(serverURL + "/debug/explorer/name")
			So(err, ShouldBeNil)
			So(resp.StatusCode, ShouldEqual, http.StatusOK)
			s, err := ioutil.ReadAll(resp.Body)
			So(err, ShouldBeNil)
			So(string(s), ShouldContainSubstring, "bob123")
		})

		Reset(func() {
			So(listener.Close(), ShouldBeNil)
			<-done
		})
	})
}
