package goji

import (
	"context"
	"net/http"

	"goji.io/internal"
)

/*
Mux is a HTTP multiplexer / router similar to net/http.ServeMux.

Muxes multiplex traffic between many http.Handlers by selecting the first
applicable Pattern. They then call a common middleware stack, finally passing
control to the selected http.Handler. See the documentation on the Handle
function for more information about how routing is performed, the documentation
on the Pattern type for more information about request matching, and the
documentation for the Use method for more about middleware.

Muxes cannot be configured concurrently from multiple goroutines, nor can they
be configured concurrently with requests.
*/
type Mux struct {
	handler    http.Handler
	middleware []func(http.Handler) http.Handler
	router     router
	root       bool
}

/*
NewMux returns a new Mux with no configured middleware or routes.
*/
func NewMux() *Mux {
	m := SubMux()
	m.root = true
	return m
}

/*
SubMux returns a new Mux with no configured middleware or routes, and which
inherits routing information from the passed context. This is especially useful
when using one Mux as a http.Handler registered to another "parent" Mux.

For example, a common pattern is to organize applications in a way that mirrors
the structure of its URLs: a photo-sharing site might have URLs that start with
"/users/" and URLs that start with "/albums/", and might be organized using
three Muxes:

	root := NewMux()
	users := SubMux()
	root.Handle(pat.New("/users/*"), users)
	albums := SubMux()
	root.Handle(pat.New("/albums/*"), albums)

	// e.g., GET /users/carl
	users.Handle(pat.Get("/:name"), renderProfile)
	// e.g., POST /albums/
	albums.Handle(pat.Post("/"), newAlbum)
*/
func SubMux() *Mux {
	m := &Mux{}
	m.buildChain()
	return m
}

// ServeHTTP implements net/http.Handler.
func (m *Mux) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if m.root {
		ctx := r.Context()
		ctx = context.WithValue(ctx, internal.Path, r.URL.EscapedPath())
		r = r.WithContext(ctx)
	}
	r = m.router.route(r)
	m.handler.ServeHTTP(w, r)
}

var _ http.Handler = &Mux{}
