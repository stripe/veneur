package goji

import (
	"net/http"

	"goji.io/internal"
	"golang.org/x/net/context"
)

/*
Mux is a HTTP multiplexer / router similar to net/http.ServeMux.

Muxes multiplex traffic between many Handlers by selecting the first applicable
Pattern. They then call a common middleware stack, finally passing control to
the selected Handler. See the documentation on the Handle function for more
information about how routing is performed, the documentation on the Pattern
type for more information about request matching, and the documentation for the
Use method for more about middleware.

Muxes cannot be configured concurrently from multiple goroutines, nor can they
be configured concurrently with requests.
*/
type Mux struct {
	handler    Handler
	middleware []func(Handler) Handler
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
when using one Mux as a Handler registered to another "parent" Mux.

For example, a common pattern is to organize applications in a way that mirrors
the structure of its URLs: a photo-sharing site might have URLs that start with
"/users/" and URLs that start with "/albums/", and might be organized using
three Muxes:

	root := NewMux()
	users := SubMux()
	root.HandleC(pat.New("/users/*"), users)
	albums := SubMux()
	root.HandleC(pat.New("/albums/*"), albums)

	// e.g., GET /users/carl
	users.HandleC(pat.Get("/:name"), renderProfile)
	// e.g., POST /albums/
	albums.HandleC(pat.Post("/"), newAlbum)
*/
func SubMux() *Mux {
	m := &Mux{}
	m.buildChain()
	return m
}

/*
ServeHTTP implements net/http.Handler. It uses context.TODO as the root context
in order to ease the conversion of non-context-aware Handlers to context-aware
ones using static analysis.

Users who know that their mux sits at the top of the request hierarchy should
consider creating a small helper http.Handler that calls this Mux's ServeHTTPC
function with context.Background.
*/
func (m *Mux) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	m.ServeHTTPC(context.TODO(), w, r)
}

/*
ServeHTTPC implements Handler.
*/
func (m *Mux) ServeHTTPC(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	if m.root {
		ctx = context.WithValue(ctx, internal.Path, r.URL.EscapedPath())
	}
	ctx = m.router.route(ctx, r)
	m.handler.ServeHTTPC(ctx, w, r)
}

var _ http.Handler = &Mux{}
var _ Handler = &Mux{}
