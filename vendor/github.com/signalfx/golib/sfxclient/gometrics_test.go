package sfxclient

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestGoMetricsSource(t *testing.T) {
	Convey("go stats should fetch", t, func() {
		So(len(GoMetricsSource.Datapoints()), ShouldEqual, 30)
	})
}
