package web

import (
	"github.com/signalfx/golib/log"
	"golang.org/x/net/context"
	"math/rand"
	"net/http"
	"strconv"
	"sync"
)

// HeaderCtxFlag sets a debug value in the context if HeaderName is not empty, a flag string has
// been set to non empty, and the header HeaderName or query string HeaderName is equal to the set
// flag string
type HeaderCtxFlag struct {
	HeaderName string

	mu          sync.RWMutex
	expectedVal string
}

// CreateMiddleware creates a handler that calls next as the next in the chain
func (m *HeaderCtxFlag) CreateMiddleware(next ContextHandler) ContextHandler {
	return HandlerFunc(func(ctx context.Context, rw http.ResponseWriter, r *http.Request) {
		m.ServeHTTPC(ctx, rw, r, next)
	})
}

// SetFlagStr enabled flag setting for HeaderName if it's equal to headerVal
func (m *HeaderCtxFlag) SetFlagStr(headerVal string) {
	m.mu.Lock()
	m.expectedVal = headerVal
	m.mu.Unlock()
}

// WithFlag returns a new Context that has the flag for this context set
func (m *HeaderCtxFlag) WithFlag(ctx context.Context) context.Context {
	return context.WithValue(ctx, m, struct{}{})
}

// HasFlag returns true if WithFlag has been set for this context
func (m *HeaderCtxFlag) HasFlag(ctx context.Context) bool {
	return ctx.Value(m) != nil
}

// FlagStr returns the currently set flag header
func (m *HeaderCtxFlag) FlagStr() string {
	m.mu.RLock()
	ret := m.expectedVal
	m.mu.RUnlock()
	return ret
}

// ServeHTTPC calls next with a context flagged if the headers match.  Note it checks both headers and query parameters.
func (m *HeaderCtxFlag) ServeHTTPC(ctx context.Context, rw http.ResponseWriter, r *http.Request, next ContextHandler) {
	debugStr := m.FlagStr()
	if debugStr != "" && m.HeaderName != "" {
		if r.Header.Get(m.HeaderName) == debugStr {
			ctx = m.WithFlag(ctx)
		} else if r.URL.Query().Get(m.HeaderName) == debugStr {
			ctx = m.WithFlag(ctx)
		}
	}
	next.ServeHTTPC(ctx, rw, r)
}

// HeadersInRequest adds headers to any context with a flag set
type HeadersInRequest struct {
	Headers map[string]string
}

// ServeHTTPC will add headers to rw if ctx has the flag set
func (m *HeadersInRequest) ServeHTTPC(ctx context.Context, rw http.ResponseWriter, r *http.Request, next ContextHandler) {
	for k, v := range m.Headers {
		rw.Header().Add(k, v)
	}
	next.ServeHTTPC(ctx, rw, r)
}

// CreateMiddleware creates a handler that calls next as the next in the chain
func (m *HeadersInRequest) CreateMiddleware(next ContextHandler) ContextHandler {
	return HandlerFunc(func(ctx context.Context, rw http.ResponseWriter, r *http.Request) {
		m.ServeHTTPC(ctx, rw, r, next)
	})
}

// CtxWithFlag adds useful request parameters to the logging context, as well as a random request_id
// to the request
type CtxWithFlag struct {
	CtxFlagger *log.CtxDimensions
	HeaderName string
}

// ServeHTTPC adds useful request dims to the next context
func (m *CtxWithFlag) ServeHTTPC(ctx context.Context, rw http.ResponseWriter, r *http.Request, next ContextHandler) {
	headerid := rand.Int63()
	rw.Header().Add(m.HeaderName, strconv.FormatInt(headerid, 10))
	ctx = m.CtxFlagger.Append(ctx, "header_id", headerid, "http_remote_addr", r.RemoteAddr, "http_method", r.Method, "http_url", r.URL.String())
	next.ServeHTTPC(ctx, rw, r)
}

// CreateMiddleware creates a handler that calls next as the next in the chain
func (m *CtxWithFlag) CreateMiddleware(next ContextHandler) ContextHandler {
	return HandlerFunc(func(ctx context.Context, rw http.ResponseWriter, r *http.Request) {
		m.ServeHTTPC(ctx, rw, r, next)
	})
}
