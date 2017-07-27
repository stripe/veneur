package goji

import (
	"net/http"
	"testing"

	"golang.org/x/net/context"
)

func expectSequence(t *testing.T, ch chan string, seq ...string) {
	for i, str := range seq {
		if msg := <-ch; msg != str {
			t.Errorf("[%d] expected %s, got %s", i, str, msg)
		}
	}
}

func TestMiddlewareNetHTTP(t *testing.T) {
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

func makeMiddleware(ch chan string, name string) func(Handler) Handler {
	return func(h Handler) Handler {
		fn := func(ctx context.Context, w http.ResponseWriter, r *http.Request) {
			ch <- "before " + name
			h.ServeHTTPC(ctx, w, r)
			ch <- "after " + name
		}
		return HandlerFunc(fn)
	}
}

func TestMiddlewareC(t *testing.T) {
	t.Parallel()

	m := NewMux()
	ch := make(chan string, 10)
	m.UseC(makeMiddleware(ch, "one"))
	m.UseC(makeMiddleware(ch, "two"))
	m.Handle(boolPattern(true), HandlerFunc(func(ctx context.Context, w http.ResponseWriter, r *http.Request) {
		ch <- "handler"
	}))

	w, r := wr()
	m.ServeHTTPC(context.Background(), w, r)

	expectSequence(t, ch, "before one", "before two", "handler", "after two", "after one")
}

func TestMiddlewareReconfigure(t *testing.T) {
	t.Parallel()

	m := NewMux()
	ch := make(chan string, 10)
	m.UseC(makeMiddleware(ch, "one"))
	m.UseC(makeMiddleware(ch, "two"))
	m.Handle(boolPattern(true), HandlerFunc(func(ctx context.Context, w http.ResponseWriter, r *http.Request) {
		ch <- "handler"
	}))

	w, r := wr()
	m.ServeHTTPC(context.Background(), w, r)

	expectSequence(t, ch, "before one", "before two", "handler", "after two", "after one")

	m.UseC(makeMiddleware(ch, "three"))

	w, r = wr()
	m.ServeHTTPC(context.Background(), w, r)

	expectSequence(t, ch, "before one", "before two", "before three",
		"handler", "after three", "after two", "after one")
}
