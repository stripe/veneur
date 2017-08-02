package goji

import (
	"net/http"
	"testing"

	"golang.org/x/net/context"
)

func TestHandle(t *testing.T) {
	t.Parallel()

	m := NewMux()
	called := false
	fn := func(w http.ResponseWriter, r *http.Request) {
		called = true
	}
	m.Handle(boolPattern(true), http.HandlerFunc(fn))

	w, r := wr()
	m.ServeHTTPC(context.Background(), w, r)
	if !called {
		t.Error("expected handler to be called")
	}
}

func TestHandleFunc(t *testing.T) {
	t.Parallel()

	m := NewMux()
	called := false
	fn := func(w http.ResponseWriter, r *http.Request) {
		called = true
	}
	m.HandleFunc(boolPattern(true), fn)

	w, r := wr()
	m.ServeHTTPC(context.Background(), w, r)
	if !called {
		t.Error("expected handler to be called")
	}
}

func TestHandleC(t *testing.T) {
	t.Parallel()

	m := NewMux()
	called := false
	fn := func(ctx context.Context, w http.ResponseWriter, r *http.Request) {
		called = true
	}
	m.HandleC(boolPattern(true), HandlerFunc(fn))

	w, r := wr()
	m.ServeHTTPC(context.Background(), w, r)
	if !called {
		t.Error("expected handler to be called")
	}
}

func TestHandleFuncC(t *testing.T) {
	t.Parallel()

	m := NewMux()
	called := false
	fn := func(ctx context.Context, w http.ResponseWriter, r *http.Request) {
		called = true
	}
	m.HandleFuncC(boolPattern(true), fn)

	w, r := wr()
	m.ServeHTTPC(context.Background(), w, r)
	if !called {
		t.Error("expected handler to be called")
	}
}

type cHTTP chan string

func (c cHTTP) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	c <- "http"
}

func (c cHTTP) ServeHTTPC(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	c <- "context"
}

func TestHandleUpgrade(t *testing.T) {
	t.Parallel()

	ch := make(cHTTP, 1)
	m := NewMux()
	m.Handle(boolPattern(true), ch)
	w, r := wr()
	m.ServeHTTPC(context.Background(), w, r)
	if v := <-ch; v != "context" {
		t.Errorf("got %v, expected %v", v, "context")
	}
}
