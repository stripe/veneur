// +build integration

package sfxclient

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/signalfx/golib/datapoint"
	. "github.com/smartystreets/goconvey/convey"
	"golang.org/x/net/context"
)

type tokenInfo struct {
	AuthToken string
	Endpoint  string
}

func createClient() (*HTTPSink, error) {
	f, err := os.Open("authinfo.json")
	if err != nil {
		return nil, err
	}
	var t tokenInfo
	err = json.NewDecoder(f).Decode(&t)
	if err != nil {
		return nil, err
	}
	h := NewHTTPSink()
	h.AuthToken = t.AuthToken
	if t.Endpoint != "" {
		h.Endpoint = t.Endpoint
	}
	return h, nil
}

var numPoints int

func init() {
	flag.IntVar(&numPoints, "numpoints", 1, "Number of points to test sending")
}

func TestDatapointSending(t *testing.T) {
	Convey("with a client", t, func() {
		c, err := createClient()
		So(err, ShouldBeNil)
		ctx := context.Background()
		Convey("Should be able to send a datapoint", func() {
			now := time.Now()
			p := make([]*datapoint.Datapoint, 0, numPoints)
			for i := 0; i < numPoints; i++ {
				p = append(p, Gauge("TestDatapointSending", map[string]string{"index": fmt.Sprintf("%d", i)}, 1))
			}
			So(c.AddDatapoints(ctx, p), ShouldBeNil)
			t.Logf("It took %s to send %d point(s)", time.Now().Sub(now).String(), numPoints)
		})
	})
}
