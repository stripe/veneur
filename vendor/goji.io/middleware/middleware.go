/*
Package middleware contains utilities for Goji Middleware authors.

Unless you are writing middleware for your application, you should avoid
importing this package. Instead, use the abstractions provided by your
middleware package.
*/
package middleware

import (
	"net/http"

	"goji.io"
	"goji.io/internal"
	"golang.org/x/net/context"
)

/*
Pattern returns the most recently matched Pattern, or nil if no pattern was
matched.
*/
func Pattern(ctx context.Context) goji.Pattern {
	p := ctx.Value(internal.Pattern)
	if p == nil {
		return nil
	}
	return p.(goji.Pattern)
}

/*
SetPattern returns a new context in which the given Pattern is used as the most
recently matched pattern.
*/
func SetPattern(ctx context.Context, p goji.Pattern) context.Context {
	return context.WithValue(ctx, internal.Pattern, p)
}

/*
Handler returns the handler corresponding to the most recently matched Pattern,
or nil if no pattern was matched.

Internally, Goji converts net/http.Handlers into goji.Handlers using a wrapper
object. Users who wish to retrieve the original http.Handler they passed to Goji
may call UnwrapHandler.

The handler returned by this function is the one that will be dispatched to at
the end of the middleware stack. If the returned Handler is nil, http.NotFound
will be used instead.
*/
func Handler(ctx context.Context) goji.Handler {
	h := ctx.Value(internal.Handler)
	if h == nil {
		return nil
	}
	return h.(goji.Handler)
}

/*
SetHandler returns a new context in which the given Handler was most recently
matched and which consequently will be dispatched to.
*/
func SetHandler(ctx context.Context, h goji.Handler) context.Context {
	return context.WithValue(ctx, internal.Handler, h)
}

/*
UnwrapHandler extracts the original http.Handler from a Goji-wrapped Handler
object, or returns nil if the given Handler has not been wrapped in this way.

This function is necessary because Goji uses goji.Handler as its native data
type internally, and uses a wrapper struct to convert all http.Handlers it is
passed into goji.Handlers.
*/
func UnwrapHandler(h goji.Handler) http.Handler {
	if cw, ok := h.(internal.ContextWrapper); ok {
		return cw.Handler
	}
	return nil
}
