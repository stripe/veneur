package reportsha

import (
	"io/ioutil"
	"os"
	"testing"

	"github.com/signalfx/golib/log"
	. "github.com/smartystreets/goconvey/convey"
)

func TestReportSha(t *testing.T) {
	Convey("when setup", t, func() {
		buildInfo := `{
  "name": "service",
  "version": "1.0-SNAPSHOT",
  "builder": "CI",
  "commit": "b70a843b07741e04ce6845aaefdbcb077787b4a3"
}`
		tmpFile, err := ioutil.TempFile("", "reportsha")
		So(err, ShouldBeNil)
		_, err = tmpFile.WriteString(buildInfo)
		So(err, ShouldBeNil)

		reporter := SHA1Reporter{
			FileName: tmpFile.Name(),
			Logger:   log.Discard,
		}

		Convey("Commit should load ok", func() {
			dps := reporter.Datapoints()
			So(dps[0].Dimensions["commit"], ShouldEqual, "b70a843b07741e04ce6845aaefdbcb077787b4a3")
			So(reporter.Var().String(), ShouldContainSubstring, "b70a843b07741e04ce6845aaefdbcb077787b4a3")
		})

		Convey("Invalid files should not load", func() {
			_, _ = tmpFile.WriteString("asdfsadfsad")
			So(reporter.Var().String(), ShouldNotContainSubstring, "asdf")
		})

		Convey("Invalid file names should not load", func() {
			reporter.FileName = "asdfsadfsad"
			So(reporter.Var().String(), ShouldNotContainSubstring, "asdf")
		})

		Reset(func() {
			So(os.Remove(tmpFile.Name()), ShouldBeNil)
			So(tmpFile.Close(), ShouldBeNil)
		})
	})
}
