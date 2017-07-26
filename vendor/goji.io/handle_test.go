package goji

import (
	"net/http"
	"testing"
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
	m.ServeHTTP(w, r)
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
	m.ServeHTTP(w, r)
	if !called {
		t.Error("expected handler to be called")
	}
}
