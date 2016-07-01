package internal

import (
	"net/http"

	"golang.org/x/net/context"
)

// ContextWrapper is a standard bridge type from http.Handlers to context-aware
// Handlers. It is included here so that the middleware subpackage can use it to
// unwrap Handlers.
type ContextWrapper struct {
	http.Handler
}

// ServeHTTPC implements goji.Handler.
func (c ContextWrapper) ServeHTTPC(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	c.Handler.ServeHTTP(w, r)
}
