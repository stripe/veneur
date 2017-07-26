package pattern

import (
	"context"
	"net/http"
	"testing"
)

type boolPattern bool

func (b boolPattern) Match(ctx context.Context, r *http.Request) context.Context {
	if b {
		return ctx
	}
	return nil
}

type prefixPattern string

func (p prefixPattern) Match(ctx context.Context, r *http.Request) context.Context {
	return ctx
}
func (p prefixPattern) PathPrefix() string {
	return string(p)
}

func TestPathRoundTrip(t *testing.T) {
	t.Parallel()

	ctx := SetPath(context.Background(), "hi")
	if path := Path(ctx); path != "hi" {
		t.Errorf("expected hi, got %q", path)
	}

	if path := Path(context.Background()); path != "" {
		t.Errorf("expected empty path, got %q", path)
	}
}
