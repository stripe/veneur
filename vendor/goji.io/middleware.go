package goji

import "net/http"

/*
Use appends a middleware to the Mux's middleware stack.

Middleware are composable pieces of functionality that augment http.Handlers.
Common examples of middleware include request loggers, authentication checkers,
and metrics gatherers.

Middleware are evaluated in the reverse order in which they were added, but the
resulting http.Handlers execute in "normal" order (i.e., the http.Handler
returned by the first Middleware to be added gets called first).

For instance, given middleware A, B, and C, added in that order, Goji will
behave similarly to this snippet:

	augmentedHandler := A(B(C(yourHandler)))
	augmentedHandler.ServeHTTP(w, r)

Assuming each of A, B, and C look something like this:

	func A(inner http.Handler) http.Handler {
		log.Print("A: called")
		mw := func(w http.ResponseWriter, r *http.Request) {
			log.Print("A: before")
			inner.ServeHTTP(w, r)
			log.Print("A: after")
		}
		return http.HandlerFunc(mw)
	}

we'd expect to see the following in the log:

	C: called
	B: called
	A: called
	---
	A: before
	B: before
	C: before
	yourHandler: called
	C: after
	B: after
	A: after

Note that augmentedHandler will called many times, producing the log output
below the divider, while the outer middleware functions (the log output above
the divider) will only be called a handful of times at application boot.

Middleware in Goji is called after routing has been performed. Therefore it is
possible to examine any routing information placed into the Request context by
Patterns, or to view or modify the http.Handler that will be routed to.
Middleware authors should read the documentation for the "middleware" subpackage
for more information about how this is done.

The http.Handler returned by the given middleware must be safe for concurrent
use by multiple goroutines. It is not safe to concurrently register middleware
from multiple goroutines, or to register middleware concurrently with requests.
*/
func (m *Mux) Use(middleware func(http.Handler) http.Handler) {
	m.middleware = append(m.middleware, middleware)
	m.buildChain()
}

// Pre-compile a http.Handler for us to use during dispatch. Yes, this means
// that adding middleware is quadratic, but it (a) happens during configuration
// time, not at "runtime", and (b) n should ~always be small.
func (m *Mux) buildChain() {
	m.handler = dispatch{}
	for i := len(m.middleware) - 1; i >= 0; i-- {
		m.handler = m.middleware[i](m.handler)
	}
}
