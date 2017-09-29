package web

import (
	"net/http"
	"sync"

	"golang.org/x/net/context"
)

// Note: Borrowed greatly from goji, but I would rather use context as the middleware context than
//       something else

// ContextHandler is just like http.Handler but also takes a context
type ContextHandler interface {
	ServeHTTPC(ctx context.Context, rw http.ResponseWriter, r *http.Request)
}

// HandlerFunc can turn a func() into a ContextHandler
type HandlerFunc func(ctx context.Context, rw http.ResponseWriter, r *http.Request)

// ServeHTTPC calls the underline func
func (h HandlerFunc) ServeHTTPC(ctx context.Context, rw http.ResponseWriter, r *http.Request) {
	h(ctx, rw, r)
}

// FromHTTP creates a ContextHandler from a http.Handler by throwing away the context
func FromHTTP(f http.Handler) ContextHandler {
	return HandlerFunc(func(_ context.Context, rw http.ResponseWriter, r *http.Request) {
		f.ServeHTTP(rw, r)
	})
}

// ToHTTP converts a ContextHandler into a http.Handler by calling c() with the added ctx
func ToHTTP(ctx context.Context, c ContextHandler) http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		c.ServeHTTPC(ctx, rw, r)
	})
}

// Constructor defines how we creates context handling middleware
type Constructor interface {
	CreateMiddleware(next ContextHandler) ContextHandler
}

// ConstructorFunc allows a func to become a Constructor
type ConstructorFunc func(next ContextHandler) ContextHandler

// CreateMiddleware calls the underline function
func (c ConstructorFunc) CreateMiddleware(next ContextHandler) ContextHandler {
	return c(next)
}

// NextConstructor creates a Constructor that calls the given function to forward requests
type NextConstructor func(ctx context.Context, rw http.ResponseWriter, r *http.Request, next ContextHandler)

// CreateMiddleware returns a middleware layer that passes next to n()
func (n NextConstructor) CreateMiddleware(next ContextHandler) ContextHandler {
	return HandlerFunc(func(ctx context.Context, rw http.ResponseWriter, r *http.Request) {
		n(ctx, rw, r, next)
	})
}

// NextHTTP is like NextConstructor but when the next praameter is a http.Handler
type NextHTTP func(rw http.ResponseWriter, r *http.Request, next http.Handler)

// CreateMiddleware creates a middleware layer that saves the context for the next layer
func (n NextHTTP) CreateMiddleware(next ContextHandler) ContextHandler {
	return HandlerFunc(func(ctx context.Context, rw http.ResponseWriter, r *http.Request) {
		n(rw, r, http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			next.ServeHTTPC(ctx, rw, r)
		}))
	})
}

// HTTPConstructor is generally discouraged but allows us to turn a normal http.Handler constructor
// into a context constructor
type HTTPConstructor func(next http.Handler) http.Handler

type outerWrap struct {
	next ContextHandler
	ctx  context.Context
}

func (o *outerWrap) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	o.next.ServeHTTPC(o.ctx, rw, r)
}

// CreateMiddleware creates a middleware layer for a HTTPConstructor.  It is unsafe to call this
// ContextHandler from multiple threads.  Our Handler layer will make sure this doesn't happen
func (h HTTPConstructor) CreateMiddleware(next ContextHandler) ContextHandler {
	o := outerWrap{
		next: next,
	}
	outer := h(&o)
	return HandlerFunc(func(ctx context.Context, rw http.ResponseWriter, r *http.Request) {
		//		// TODO: A million dollars to who can figure out how to do this cleaner with the called
		//		//       function really owning this context while also not calling h() each time
		if o.ctx != nil {
			panic("I was never reset.  Logic error!")
		}
		o.ctx = ctx
		outer.ServeHTTP(rw, r)
		o.ctx = nil
	})
}

// Handler turns a stack of HTTP handlers into a single handler that threads a context between each
type Handler struct {
	Ending          ContextHandler
	Chain           []Constructor
	StartingContext context.Context
	Pool            sync.Pool
}

func (h *Handler) newStack() ContextHandler {
	result := h.Ending
	for i := len(h.Chain) - 1; i >= 0; i-- {
		result = h.Chain[i].CreateMiddleware(result)
	}
	return result
}

// Add a middleware layer to this handler
func (h *Handler) Add(parts ...Constructor) *Handler {
	h.Chain = append(h.Chain, parts...)
	return h
}

// ServeHTTP passes the default starting context
func (h *Handler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	h.ServeHTTPC(h.StartingContext, rw, r)
}

// ServeHTTPC will pass the request between each middleware layer
func (h *Handler) ServeHTTPC(ctx context.Context, rw http.ResponseWriter, r *http.Request) {
	stack := h.Pool.Get().(ContextHandler)
	stack.ServeHTTPC(ctx, rw, r)
	h.Pool.Put(stack)
}

//NewHandler creates a new handler with no middleware layers
func NewHandler(StartingContext context.Context, Ending ContextHandler) *Handler {
	h := &Handler{
		Ending:          Ending,
		Chain:           []Constructor{},
		StartingContext: StartingContext,
	}
	h.Pool.New = func() interface{} {
		return h.newStack()
	}
	return h
}

// VarAdder is a middleware layer that adds to the context Key/Value
type VarAdder struct {
	Key   interface{}
	Value interface{}
}

// Generate creates the next middleware layer for VarAdder
func (i *VarAdder) Generate(next ContextHandler) ContextHandler {
	return HandlerFunc(func(ctx context.Context, rw http.ResponseWriter, r *http.Request) {
		next.ServeHTTPC(context.WithValue(ctx, i.Key, i.Value), rw, r)
	})
}

// InvalidContentType is a HTTP handler helper to signal that the content type header is wrong
func InvalidContentType(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "Invalid content type:"+r.Header.Get("Content-Type"), http.StatusBadRequest)
}
