package goji

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"golang.org/x/net/context"
)

func TestHandlerFunc(t *testing.T) {
	t.Parallel()

	called := false

	rw := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/", nil)
	h := func(ctx context.Context, w http.ResponseWriter, r *http.Request) {
		if ctx != context.TODO() {
			t.Errorf("ctx: expected %v, got %v", context.TODO(), ctx)
		}
		if w != rw {
			t.Errorf("rw: expected %v, got %v", rw, w)
		}
		if r != req {
			t.Errorf("req: expected %v, got %v", req, r)
		}
		called = true
	}

	HandlerFunc(h).ServeHTTP(rw, req)
	if !called {
		t.Error("expected handler to be called")
	}
}
