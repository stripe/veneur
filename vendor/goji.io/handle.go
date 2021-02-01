package goji

import "net/http"

/*
Handle adds a new route to the Mux. Requests that match the given Pattern will
be dispatched to the given http.Handler.

Routing is performed in the order in which routes are added: the first route
with a matching Pattern will be used. In particular, Goji guarantees that
routing is performed in a manner that is indistinguishable from the following
algorithm:

	// Assume routes is a slice that every call to Handle appends to
	for _, route := range routes {
		// For performance, Patterns can opt out of this call to Match.
		// See the documentation for Pattern for more.
		if r2 := route.pattern.Match(r); r2 != nil {
			route.handler.ServeHTTP(w, r2)
			break
		}
	}

It is not safe to concurrently register routes from multiple goroutines, or to
register routes concurrently with requests.
*/
func (m *Mux) Handle(p Pattern, h http.Handler) {
	m.router.add(p, h)
}

/*
HandleFunc adds a new route to the Mux. It is equivalent to calling Handle on a
handler wrapped with http.HandlerFunc, and is provided only for convenience.
*/
func (m *Mux) HandleFunc(p Pattern, h func(http.ResponseWriter, *http.Request)) {
	m.Handle(p, http.HandlerFunc(h))
}
