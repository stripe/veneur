package middleware

import (
	"context"
	"net/http"
	"testing"
)

type testPattern bool

func (t testPattern) Match(r *http.Request) *http.Request {
	if t {
		return r
	}
	return nil
}

type testHandler struct{}

func (t testHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {}

func TestPattern(t *testing.T) {
	t.Parallel()

	pat := testPattern(true)
	ctx := SetPattern(context.Background(), pat)
	if pat2 := Pattern(ctx); pat2 != pat {
		t.Errorf("got ctx=%v, expected %v", pat2, pat)
	}

	if pat2 := Pattern(context.Background()); pat2 != nil {
		t.Errorf("got ctx=%v, expecte nil", pat2)
	}
}

func TestHandler(t *testing.T) {
	t.Parallel()

	h := testHandler{}
	ctx := SetHandler(context.Background(), h)
	if h2 := Handler(ctx); h2 != h {
		t.Errorf("got handler=%v, expected %v", h2, h)
	}

	if h2 := Handler(context.Background()); h2 != nil {
		t.Errorf("got handler=%v, expected nil", h2)
	}
}
