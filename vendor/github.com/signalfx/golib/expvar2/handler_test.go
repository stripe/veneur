package expvar2

import (
	"expvar"
	"net/http/httptest"
	"testing"

	"os"

	"net/http"

	"github.com/signalfx/golib/log"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/assert"
)

var toFind = "asdfasdfdsa"

func init() {
	expvar.NewString("expvar2.TestExpvarHandler.a").Set("abc")
	expvar.NewString("expvar2.TestExpvarHandler.b").Set(toFind)
}

func TestExpvarHandler(t *testing.T) {
	Convey("When setup", t, func() {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/bob", nil)
		e := New()
		Convey("And hello world registered", func() {
			e.Exported["hello"] = expvar.Func(func() interface{} {
				return "world"
			})
			Convey("Should see world in output", func() {
				e.ServeHTTP(w, req)
				So(w.Body.String(), ShouldContainSubstring, "world")
			})
			Convey("Should not see world in output if filtered", func() {
				reqWithFilter, _ := http.NewRequest("GET", "/bob?filter=NOT", nil)
				e.ServeHTTP(w, reqWithFilter)
				So(w.Body.String(), ShouldNotContainSubstring, "world")
			})
			Convey("Should still see world in output if filter allows", func() {
				reqWithFilter, _ := http.NewRequest("GET", "/bob?filter=hello", nil)
				e.ServeHTTP(w, reqWithFilter)
				So(w.Body.String(), ShouldContainSubstring, "world")
			})
		})
		Convey("Pretty printing should be larger", func() {
			reqWithPretty, _ := http.NewRequest("GET", "/bob?pretty=true", nil)
			e.ServeHTTP(w, req)
			w2 := httptest.NewRecorder()
			e.ServeHTTP(w2, reqWithPretty)
			So(w2.Body.Len(), ShouldBeGreaterThan, w.Body.Len())
		})
		Convey("And using name of something already exported", func() {
			e.Exported["expvar2.TestExpvarHandler.b"] = expvar.Func(func() interface{} {
				return "replaced"
			})
			Convey("Should not see global exported string in output", func() {
				e.ServeHTTP(w, req)
				So(w.Body.String(), ShouldNotContainSubstring, toFind)
			})
		})
	})
}

func TestEnviromentalVariables(t *testing.T) {
	log.IfErr(log.Panic, os.Setenv("TestEnviromentalVariables", "abcdefg"))
	s := EnviromentalVariables().String()
	assert.Contains(t, s, "TestEnviromentalVariables")
	assert.Contains(t, s, "abcdefg")

	s = enviromentalVariables(func() []string {
		return []string{
			"abc=b",
			"abcd",
			"z=",
			"a=b=c",
		}
	}).String()
	assert.NotContains(t, s, "abcd")
	assert.Contains(t, s, "z")
	assert.Contains(t, s, "b=c")
}
