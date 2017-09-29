package web

import (
	"golang.org/x/net/context"
	"net/http"
)

// Request is a request put on the TestHandler queue that records eacn request sent through
type Request struct {
	Ctx context.Context
	Rw  http.ResponseWriter
	Req *http.Request
}

// Recorder stores into Queue each request that comes across the recorder
type Recorder struct {
	Queue chan Request
}

// ServeHTTPC stores the req into Queue and calls next
func (t *Recorder) ServeHTTPC(ctx context.Context, rw http.ResponseWriter, r *http.Request, next ContextHandler) {
	t.Queue <- Request{
		Ctx: ctx,
		Rw:  rw,
		Req: r,
	}
	if next != nil {
		next.ServeHTTPC(ctx, rw, r)
	}
}

// AsHandler returns a Recoder that can be a destination handler
func (t *Recorder) AsHandler() ContextHandler {
	return HandlerFunc(func(ctx context.Context, rw http.ResponseWriter, r *http.Request) {
		t.ServeHTTPC(ctx, rw, r, nil)
	})
}

// ServeHTTP stores the request into the Queue
func (t *Recorder) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	t.Queue <- Request{
		Rw:  rw,
		Req: r,
	}
}
