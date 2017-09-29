package web

import (
	"net/http"
	"sync/atomic"

	"golang.org/x/net/context"
)

// CloseHeader is used to control when connections should signal they should be closed
type CloseHeader struct {
	SetCloseHeader int32
}

// OptionallyAddCloseHeader will set Connection: Close on the response if SetCloseHeader is non zero
func (c *CloseHeader) OptionallyAddCloseHeader(ctx context.Context, rw http.ResponseWriter, r *http.Request, next ContextHandler) {
	if atomic.LoadInt32(&c.SetCloseHeader) != 0 {
		rw.Header().Set("Connection", "Close")
	}
	next.ServeHTTPC(ctx, rw, r)
}
