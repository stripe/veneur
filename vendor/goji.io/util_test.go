package goji

import (
	"net/http"
	"net/http/httptest"
	"strings"

	"goji.io/internal"
	"golang.org/x/net/context"
)

type boolPattern bool

func (b boolPattern) Match(ctx context.Context, _ *http.Request) context.Context {
	if b {
		return ctx
	}
	return nil
}

type testPattern struct {
	index   int
	mark    *int
	methods []string
	prefix  string
}

func (t testPattern) Match(ctx context.Context, r *http.Request) context.Context {
	if t.index < *t.mark {
		return nil
	}
	path := ctx.Value(internal.Path).(string)
	if !strings.HasPrefix(path, t.prefix) {
		return nil
	}
	if t.methods != nil {
		if _, ok := t.HTTPMethods()[r.Method]; !ok {
			return nil
		}
	}
	return ctx
}

func (t testPattern) PathPrefix() string {
	return t.prefix
}

func (t testPattern) HTTPMethods() map[string]struct{} {
	if t.methods == nil {
		return nil
	}
	m := make(map[string]struct{})
	for _, method := range t.methods {
		m[method] = struct{}{}
	}
	return m
}

type intHandler int

func (i intHandler) ServeHTTPC(ctx context.Context, w http.ResponseWriter, r *http.Request) {
}

func wr() (*httptest.ResponseRecorder, *http.Request) {
	w := httptest.NewRecorder()
	r, err := http.NewRequest("GET", "/", nil)
	if err != nil {
		panic(err)
	}
	return w, r
}
