/*
Package goji is a minimalistic and flexible HTTP request multiplexer.

Goji itself has very few features: it is first and foremost a standard set of
interfaces for writing web applications. Several subpackages are distributed
with Goji to provide standard production-ready implementations of several of the
interfaces, however users are also encouraged to implement the interfaces on
their own, especially if their needs are unusual.
*/
package goji

import (
	"net/http"

	"golang.org/x/net/context"
)

/*
Pattern determines whether a given request matches some criteria. Goji users
looking for a concrete type that implements this interface should consider
Goji's "pat" sub-package, which implements a small domain specific language for
HTTP routing.

Patterns typically only examine a small portion of incoming requests, most
commonly the HTTP method and the URL's RawPath. As an optimization, Goji can
elide calls to your Pattern for requests it knows cannot match. Pattern authors
who wish to take advantage of this functionality (and in some cases an
asymptotic performance improvement) can augment their Pattern implementations
with any of the following methods:

	// HTTPMethods returns a set of HTTP methods that this Pattern matches,
	// or nil if it's not possible to determine which HTTP methods might be
	// matched. Put another way, requests with HTTP methods not in the
	// returned set are guaranteed to never match this Pattern.
	HTTPMethods() map[string]struct{}

	// PathPrefix returns a string which all RawPaths that match this
	// Pattern must have as a prefix. Put another way, requests with
	// RawPaths that do not contain the returned string as a prefix are
	// guaranteed to never match this Pattern.
	PathPrefix() string

The presence or lack of these performance improvements should be viewed as an
implementation detail and are not part of Goji's API compatibility guarantee. It
is the responsibility of Pattern authors to ensure that their Match function
always returns correct results, even if these optimizations are not performed.

All operations on Patterns must be safe for concurrent use by multiple
goroutines.
*/
type Pattern interface {
	// Match examines the request and request context to determine if the
	// request is a match. If so, it returns a non-nil context.Context
	// (likely one derived from the input Context, and perhaps simply the
	// input Context unchanged). The returned context may be used to store
	// request-scoped data, such as variables extracted from the Request.
	//
	// Match must not mutate the passed request.
	Match(context.Context, *http.Request) context.Context
}

/*
Handler is a context-aware variant of net/http.Handler. It has the same
semantics as http.Handler, except that a request-scoped context is additionally
passed to the handler function.
*/
type Handler interface {
	ServeHTTPC(context.Context, http.ResponseWriter, *http.Request)
}

/*
HandlerFunc is a context-aware variant of net/http.HandlerFunc. It has the same
semantics as http.HandlerFunc, except that a request-scoped context is
additionally passed to the function.

HandlerFunc implements both the Handler and http.Handler interfaces.
*/
type HandlerFunc func(context.Context, http.ResponseWriter, *http.Request)

/*
ServeHTTP implements net/http.Handler. It calls the underlying function with a
context of context.TODO in order to ease the conversion of non-context-aware
Handlers to context-aware ones using static analysis.
*/
func (h HandlerFunc) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h(context.TODO(), w, r)
}

/*
ServeHTTPC implements Handler.
*/
func (h HandlerFunc) ServeHTTPC(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	h(ctx, w, r)
}
