package goji

import (
	"context"
	"net/http"
	"testing"

	"goji.io/internal"
)

func TestMuxExistingPath(t *testing.T) {
	m := NewMux()
	handler := func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		if path := ctx.Value(internal.Path).(string); path != "/" {
			t.Errorf("expected path=/, got %q", path)
		}
	}
	m.HandleFunc(boolPattern(true), handler)
	w, r := wr()
	ctx := context.WithValue(context.Background(), internal.Path, "/hello")
	r = r.WithContext(ctx)
	m.ServeHTTP(w, r)
}

func TestSubMuxExistingPath(t *testing.T) {
	m := SubMux()
	handler := func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		if path := ctx.Value(internal.Path).(string); path != "/hello" {
			t.Errorf("expected path=/hello, got %q", path)
		}
	}
	m.HandleFunc(boolPattern(true), handler)
	w, r := wr()
	ctx := context.WithValue(context.Background(), internal.Path, "/hello")
	r = r.WithContext(ctx)
	m.ServeHTTP(w, r)
}
