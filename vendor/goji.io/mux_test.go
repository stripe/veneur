package goji

import (
	"net/http"
	"testing"

	"goji.io/internal"
	"golang.org/x/net/context"
)

func TestMuxExistingPath(t *testing.T) {
	m := NewMux()
	handler := func(ctx context.Context, w http.ResponseWriter, r *http.Request) {
		if path := ctx.Value(internal.Path).(string); path != "/" {
			t.Errorf("expected path=/, got %q", path)
		}
	}
	m.HandleFuncC(boolPattern(true), handler)
	w, r := wr()
	ctx := context.WithValue(context.Background(), internal.Path, "/hello")
	m.ServeHTTPC(ctx, w, r)
}

func TestSubMuxExistingPath(t *testing.T) {
	m := SubMux()
	handler := func(ctx context.Context, w http.ResponseWriter, r *http.Request) {
		if path := ctx.Value(internal.Path).(string); path != "/hello" {
			t.Errorf("expected path=/hello, got %q", path)
		}
	}
	m.HandleFuncC(boolPattern(true), handler)
	w, r := wr()
	ctx := context.WithValue(context.Background(), internal.Path, "/hello")
	m.ServeHTTPC(ctx, w, r)
}
