package goji

import (
	"context"
	"net/http"
	"testing"

	"goji.io/internal"
)

func TestDispatch(t *testing.T) {
	t.Parallel()

	var d dispatch

	w, r := wr()
	d.ServeHTTP(w, r)
	if w.Code != 404 {
		t.Errorf("status: expected %d, got %d", 404, w.Code)
	}

	w, r = wr()
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(123)
	})
	ctx := context.WithValue(context.Background(), internal.Handler, h)
	r = r.WithContext(ctx)
	d.ServeHTTP(w, r)
	if w.Code != 123 {
		t.Errorf("status: expected %d, got %d", 123, w.Code)
	}
}
