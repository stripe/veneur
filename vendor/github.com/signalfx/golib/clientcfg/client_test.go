package clientcfg

import (
	"errors"
	"net/url"
	"testing"

	"context"
	"github.com/signalfx/golib/datapoint/dptest"
	"github.com/signalfx/golib/distconf"
	"github.com/signalfx/golib/log"
	"github.com/signalfx/golib/sfxclient"
	. "github.com/smartystreets/goconvey/convey"
)

func TestClient(t *testing.T) {
	Convey("With a testing zk", t, func() {
		mem := distconf.Mem()
		dconf := distconf.New([]distconf.Reader{mem})
		conf := &ClientConfig{}
		logger := log.Discard
		conf.Load(dconf)
		ctx := context.Background()
		Convey("normal sinks should just forward", func() {
			tsink := dptest.NewBasicSink()
			tsink.Resize(1)
			sink := WatchSinkChanges(tsink, conf, logger)
			So(sink, ShouldEqual, tsink)
			So(sink.AddDatapoints(ctx, nil), ShouldBeNil)
		})
		Convey("default dimensions", func() {
			Convey("should use sourceName", func() {
				mem.Write("signalfuse.sourceName", []byte("h1"))
				dims, err := DefaultDimensions(conf)
				So(err, ShouldBeNil)
				So(dims, ShouldResemble, map[string]string{"sf_source": "h1"})
			})
			Convey("should use hostname", func() {
				conf.OsHostname = func() (name string, err error) {
					return "ahost", nil
				}
				dims, err := DefaultDimensions(conf)
				So(err, ShouldBeNil)
				So(dims, ShouldResemble, map[string]string{"sf_source": "ahost"})
			})
			Convey("check hostname error", func() {
				conf.OsHostname = func() (name string, err error) {
					return "", errors.New("nope")
				}
				dims, err := DefaultDimensions(conf)
				So(err, ShouldNotBeNil)
				So(dims, ShouldBeNil)
			})
		})
		Convey("HTTPSink should wrap", func() {
			hsink := sfxclient.NewHTTPSink()
			sink := WatchSinkChanges(hsink, conf, logger)
			So(sink, ShouldNotEqual, hsink)
			hsink.DatapointEndpoint = ""
			So(sink.AddDatapoints(ctx, nil), ShouldBeNil)
			Convey("Only hostname should append v2/datapoint", func() {
				mem.Write("sf.metrics.statsendpoint", []byte("https://ingest-2.signalfx.com"))
				So(hsink.DatapointEndpoint, ShouldEqual, "https://ingest-2.signalfx.com/v2/datapoint")
				Convey("Failing URL parses should be OK", func() {
					sink.(*ClientConfigChangerSink).urlParse = func(string) (*url.URL, error) {
						return nil, errors.New("nope")
					}
					mem.Write("sf.metrics.statsendpoint", []byte("_will_not_parse"))
					So(hsink.DatapointEndpoint, ShouldEqual, "https://ingest-2.signalfx.com/v2/datapoint")
				})
			})
			Convey("http should add by default", func() {
				mem.Write("sf.metrics.statsendpoint", []byte("ingest-3.signalfx.com:28080"))
				So(hsink.DatapointEndpoint, ShouldEqual, "http://ingest-3.signalfx.com:28080/v2/datapoint")
			})
			Convey("direct URL should be used", func() {
				mem.Write("sf.metrics.statsendpoint", []byte("https://ingest-4.signalfx.com/v2"))
				So(hsink.DatapointEndpoint, ShouldEqual, "https://ingest-4.signalfx.com/v2")
			})
		})
	})
}
