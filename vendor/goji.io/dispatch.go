package goji

import (
	"net/http"

	"goji.io/internal"
	"golang.org/x/net/context"
)

type dispatch struct{}

func (d dispatch) ServeHTTPC(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	h := ctx.Value(internal.Handler)
	if h == nil {
		http.NotFound(w, r)
	} else {
		h.(Handler).ServeHTTPC(ctx, w, r)
	}
}
