package internal

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"golang.org/x/net/context"
)

func TestContextWrapper(t *testing.T) {
	t.Parallel()

	// This one is kind of silly.
	called := false
	rw := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/", nil)
	h := func(w http.ResponseWriter, r *http.Request) {
		if w != rw {
			t.Errorf("rw: expected %v, got %v", rw, w)
		}
		if r != req {
			t.Errorf("req: expected %v, got %v", req, r)
		}
		called = true
	}
	cw := ContextWrapper{http.HandlerFunc(h)}
	cw.ServeHTTPC(context.Background(), rw, req)
	if !called {
		t.Error("expected handler to be called")
	}
}
