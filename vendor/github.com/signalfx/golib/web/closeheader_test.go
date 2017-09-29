package web

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"golang.org/x/net/context"
)

func TestCloseHeader(t *testing.T) {
	h := CloseHeader{}
	rw := httptest.NewRecorder()
	r, _ := http.NewRequest("", "", nil)
	ctx := context.Background()
	next := HandlerFunc(func(ctx context.Context, rw http.ResponseWriter, r *http.Request) {
	})
	h.OptionallyAddCloseHeader(ctx, rw, r, next)
	assert.Equal(t, "", rw.Header().Get("Connection"))

	h.SetCloseHeader = 1
	h.OptionallyAddCloseHeader(ctx, rw, r, next)
	assert.Equal(t, "Close", rw.Header().Get("Connection"))
}
