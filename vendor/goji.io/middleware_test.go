package goji

import (
	"net/http"
	"testing"
)

func expectSequence(t *testing.T, ch chan string, seq ...string) {
	for i, str := range seq {
		if msg := <-ch; msg != str {
			t.Errorf("[%d] expected %s, got %s", i, str, msg)
		}
	}
}

func TestMiddleware(t *testing.T) {
	t.Parallel()

	m := NewMux()
	ch := make(chan string, 10)
	m.Use(func(h http.Handler) http.Handler {
		fn := func(w http.ResponseWriter, r *http.Request) {
			ch <- "before one"
			h.ServeHTTP(w, r)
			ch <- "after one"
		}
		return http.HandlerFunc(fn)
	})
	m.Use(func(h http.Handler) http.Handler {
		fn := func(w http.ResponseWriter, r *http.Request) {
			ch <- "before two"
			h.ServeHTTP(w, r)
			ch <- "after two"
		}
		return http.HandlerFunc(fn)
	})
	m.Handle(boolPattern(true), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ch <- "handler"
	}))

	m.ServeHTTP(wr())

	expectSequence(t, ch, "before one", "before two", "handler", "after two", "after one")
}

func makeMiddleware(ch chan string, name string) func(http.Handler) http.Handler {
	return func(h http.Handler) http.Handler {
		fn := func(w http.ResponseWriter, r *http.Request) {
			ch <- "before " + name
			h.ServeHTTP(w, r)
			ch <- "after " + name
		}
		return http.HandlerFunc(fn)
	}
}

func TestMiddlewareReconfigure(t *testing.T) {
	t.Parallel()

	m := NewMux()
	ch := make(chan string, 10)
	m.Use(makeMiddleware(ch, "one"))
	m.Use(makeMiddleware(ch, "two"))
	m.Handle(boolPattern(true), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ch <- "handler"
	}))

	w, r := wr()
	m.ServeHTTP(w, r)

	expectSequence(t, ch, "before one", "before two", "handler", "after two", "after one")

	m.Use(makeMiddleware(ch, "three"))

	w, r = wr()
	m.ServeHTTP(w, r)

	expectSequence(t, ch, "before one", "before two", "before three",
		"handler", "after three", "after two", "after one")
}
