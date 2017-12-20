package log

import (
	"errors"
	. "github.com/smartystreets/goconvey/convey"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"
)

func TestFilter(t *testing.T) {
	Convey("filtered logger", t, func() {
		counter2 := &Counter{}
		filter := &RegexFilter{
			Log:             Discard,
			ErrCallback:     counter2,
			MissingValueKey: "msg",
		}
		Convey("should start disabled", func() {
			So(filter.Disabled(), ShouldBeTrue)
		})
		Convey("should not log at first", func() {
			So(filter.WouldLog("hello world"), ShouldBeFalse)
		})
		Convey("When enabled", func() {
			filter.SetFilters(map[string]*regexp.Regexp{
				"msg": regexp.MustCompile("hello bob"),
			})
			So(filter.Disabled(), ShouldBeFalse)
			Convey("should export stats", func() {
				So(filter.Var().String(), ShouldContainSubstring, "hello bob")
			})
			Convey("Should log", func() {
				So(filter.WouldLog("hello world"), ShouldBeFalse)
				So(filter.WouldLog("hello bob"), ShouldBeTrue)
				So(filter.WouldLog("hello bob two"), ShouldBeTrue)
			})
			Convey("Should not log missing dimensions", func() {
				So(filter.WouldLog("missing", "10"), ShouldBeFalse)
			})
			Convey("Should not log unconvertable dimensions", func() {
				So(counter2.Count, ShouldEqual, 0)
				So(filter.WouldLog(func() {}), ShouldBeFalse)
				So(counter2.Count, ShouldEqual, 1)
			})
		})
	})
}

func TestMatchesWorksOnNilSet(t *testing.T) {
	if matches(nil, nil, nil) {
		t.Error("Expected matches not to match on all nil")
	}
}

func TestMultiFilter(t *testing.T) {
	Convey("A multi filter", t, func() {
		mf := MultiFilter{}
		Convey("starts off disabled", func() {
			So(mf.Disabled(), ShouldBeTrue)
		})
		Convey("Can add a filter", func() {
			counter := &Counter{}
			logger := &RegexFilter{
				Log:             counter,
				ErrCallback:     counter,
				MissingValueKey: "msg",
			}
			mf.Filters = append(mf.Filters, logger)
			mf.PassTo = counter
			So(mf.Disabled(), ShouldBeTrue)
			mf.Log("hello")
			So(counter.Count, ShouldEqual, 0)
			logger.SetFilters(map[string]*regexp.Regexp{
				"msg": regexp.MustCompile("hello bob"),
			})
			So(counter.Count, ShouldEqual, 1)
			mf.Log("hello")
			So(counter.Count, ShouldEqual, 1)
			mf.Log("hello bob")
			So(counter.Count, ShouldEqual, 2)
		})
	})
}

type errResponseWriter struct {
	http.ResponseWriter
}

func (e *errResponseWriter) Write([]byte) (int, error) {
	return 0, errors.New("cannot write")
}

func TestFilterChangeHandler(t *testing.T) {
	Convey("A filter and handler", t, func() {
		counter := &Counter{}
		logger := &RegexFilter{
			Log:             counter,
			ErrCallback:     Panic,
			MissingValueKey: Msg,
		}

		handler := FilterChangeHandler{
			Filter: logger,
			Log:    Discard,
		}
		Convey("Should get as Empty", func() {
			rw := httptest.NewRecorder()
			req, err := http.NewRequest("GET", "", nil)
			So(err, ShouldBeNil)
			handler.ServeHTTP(rw, req)
			So(rw.Code, ShouldEqual, http.StatusOK)
			So(rw.Body.String(), ShouldContainSubstring, "There are currently 0 filters")
			So(len(logger.GetFilters()), ShouldEqual, 0)
		})
		Convey("Should 404 on non GET/POST", func() {
			rw := httptest.NewRecorder()
			req, err := http.NewRequest("PUT", "", nil)
			So(err, ShouldBeNil)
			handler.ServeHTTP(rw, req)
			So(rw.Code, ShouldEqual, http.StatusNotFound)
		})
		Convey("Should error if cannot write template", func() {
			rw := httptest.NewRecorder()
			req, err := http.NewRequest("GET", "", nil)
			So(err, ShouldBeNil)
			handler.ServeHTTP(&errResponseWriter{rw}, req)
			So(rw.Code, ShouldEqual, http.StatusInternalServerError)
		})

		Convey("Should be updatable", func() {
			rw := httptest.NewRecorder()
			req, err := http.NewRequest("POST", "", strings.NewReader(""))
			So(err, ShouldBeNil)
			So(req.ParseForm(), ShouldBeNil)
			req.Form.Add("newregex", "id:1")
			handler.ServeHTTP(rw, req)
			So(rw.Code, ShouldEqual, http.StatusOK)
			So(rw.Body.String(), ShouldContainSubstring, "There are currently 1 filters")
			So(len(logger.GetFilters()), ShouldEqual, 1)
			Convey("Does nothing on invalid updates", func() {
				rw := httptest.NewRecorder()
				req, err := http.NewRequest("POST", "", strings.NewReader(""))
				So(err, ShouldBeNil)
				So(req.ParseForm(), ShouldBeNil)
				req.Form.Add("newregex", "id")
				handler.ServeHTTP(rw, req)
				So(rw.Code, ShouldEqual, http.StatusOK)
				So(rw.Body.String(), ShouldContainSubstring, "There are currently 1 filters")
				So(len(logger.GetFilters()), ShouldEqual, 1)
			})
			Convey("Does nothing on invalid regex", func() {
				rw := httptest.NewRecorder()
				req, err := http.NewRequest("POST", "", strings.NewReader(""))
				So(err, ShouldBeNil)
				So(req.ParseForm(), ShouldBeNil)
				req.Form.Add("newregex", "id:[123")
				handler.ServeHTTP(rw, req)
				So(rw.Code, ShouldEqual, http.StatusOK)
				So(rw.Body.String(), ShouldContainSubstring, "There are currently 1 filters")
				So(len(logger.GetFilters()), ShouldEqual, 1)
			})

			Convey("Does nothing on invalid POST", func() {
				rw := httptest.NewRecorder()
				req, err := http.NewRequest("POST", "", nil)
				So(err, ShouldBeNil)
				handler.ServeHTTP(rw, req)
				So(rw.Code, ShouldEqual, http.StatusOK)
				So(rw.Body.String(), ShouldContainSubstring, "There are currently 1 filters")
				So(len(logger.GetFilters()), ShouldEqual, 1)
			})

			Convey("and can change back", func() {
				rw := httptest.NewRecorder()
				req, err := http.NewRequest("POST", "", strings.NewReader(""))
				So(err, ShouldBeNil)
				So(req.ParseForm(), ShouldBeNil)
				req.Form.Add("newregex", "")
				handler.ServeHTTP(rw, req)
				So(rw.Code, ShouldEqual, http.StatusOK)
				So(rw.Body.String(), ShouldContainSubstring, "There are currently 0 filters")
				So(len(logger.GetFilters()), ShouldEqual, 0)
			})
		})
	})
}

func BenchmarkFilterDisabled(b *testing.B) {
	counter := &Counter{}
	filter := &RegexFilter{
		Log:             Discard,
		ErrCallback:     Panic,
		MissingValueKey: Msg,
	}
	for i := 0; i < b.N; i++ {
		if filter.WouldLog("hello world") {
			b.Error("Expected log to not happen")
		}
	}
	if counter.Count != 0 {
		b.Errorf("Expected a zero count, got %d\n", counter.Count)
	}
}

func BenchmarkFilterEnabled(b *testing.B) {
	filter := &RegexFilter{
		Log:             Discard,
		ErrCallback:     Panic,
		MissingValueKey: Msg,
	}
	filter.SetFilters(map[string]*regexp.Regexp{
		"id": regexp.MustCompile(`^1234$`),
	})
	for i := 0; i < b.N; i++ {
		var idToLog int64
		if i%2 == 0 {
			idToLog = 1235
		} else {
			idToLog = 1234
		}
		if filter.WouldLog("id", idToLog, "hello world") && idToLog != 1234 {
			b.Error("Logging when I didn't expect to")
		}
	}
}

func BenchmarkFilterEnabled30(b *testing.B) {
	filter := &RegexFilter{
		Log:             Discard,
		ErrCallback:     Panic,
		MissingValueKey: "msg",
	}
	filter.SetFilters(map[string]*regexp.Regexp{
		"id": regexp.MustCompile(`^1234$`),
	})
	wg := sync.WaitGroup{}
	wg.Add(30)
	for n := 0; n < 30; n++ {
		go func() {
			for i := 0; i < b.N; i++ {
				var idToLog int64
				if i%2 == 0 {
					idToLog = 1235
				} else {
					idToLog = 1234
				}
				if filter.WouldLog("id", idToLog, "hello world") && idToLog != 1234 {
					b.Error("Unexpected wouldlog")
				}
			}
			wg.Done()
		}()
	}
	wg.Wait()
}
