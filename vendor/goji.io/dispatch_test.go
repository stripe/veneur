package goji

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"goji.io/internal"
	"golang.org/x/net/context"
)

func TestDispatch(t *testing.T) {
	t.Parallel()

	var d dispatch

	w := httptest.NewRecorder()
	d.ServeHTTPC(context.Background(), w, nil)
	if w.Code != 404 {
		t.Errorf("status: expected %d, got %d", 404, w.Code)
	}

	w = httptest.NewRecorder()
	h := HandlerFunc(func(ctx context.Context, w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(123)
	})
	ctx := context.WithValue(context.Background(), internal.Handler, h)
	d.ServeHTTPC(ctx, w, nil)
	if w.Code != 123 {
		t.Errorf("status: expected %d, got %d", 123, w.Code)
	}
}
