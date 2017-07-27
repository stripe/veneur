package goji

import (
	"net/http"

	"goji.io/internal"
	"golang.org/x/net/context"
)

/*
Handle adds a new route to the Mux. Requests that match the given Pattern will
be dispatched to the given http.Handler. If the http.Handler also supports
Handler, that interface will be used instead.

Routing is performed in the order in which routes are added: the first route
with a matching Pattern will be used. In particular, Goji guarantees that
routing is performed in a manner that is indistinguishable from the following
algorithm:

	// Assume routes is a slice that every call to Handle appends to
	for route := range routes {
		// For performance, Patterns can opt out of this call to Match.
		// See the documentation for Pattern for more.
		if ctx2 := route.pattern.Match(ctx, r); ctx2 != nil {
			route.handler.ServeHTTPC(ctx2, w, r)
			break
		}
	}

It is not safe to concurrently register routes from multiple goroutines, or to
register routes concurrently with requests.
*/
func (m *Mux) Handle(p Pattern, h http.Handler) {
	gh, ok := h.(Handler)
	if !ok {
		gh = internal.ContextWrapper{Handler: h}
	}
	m.router.add(p, gh)
}

/*
HandleFunc adds a new route to the Mux. It is equivalent to calling Handle on a
handler wrapped with http.HandlerFunc, and is provided only for convenience.
*/
func (m *Mux) HandleFunc(p Pattern, h func(http.ResponseWriter, *http.Request)) {
	m.Handle(p, http.HandlerFunc(h))
}

/*
HandleC adds a new context-aware route to the Mux. See the documentation for
Handle for more information about the semantics of routing.

It is not safe to concurrently register routes from multiple goroutines, or to
register routes concurrently with requests.
*/
func (m *Mux) HandleC(p Pattern, h Handler) {
	m.router.add(p, h)
}

/*
HandleFuncC adds a new context-aware route to the Mux. It is equivalent to
calling HandleC on a handler wrapped with HandlerFunc, and is provided for
convenience.
*/
func (m *Mux) HandleFuncC(p Pattern, h func(context.Context, http.ResponseWriter, *http.Request)) {
	m.HandleC(p, HandlerFunc(h))
}
