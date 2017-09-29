package web

import (
	"net/http"
	"time"

	"golang.org/x/net/context"
)

// AddRequestTime is a web context that adds the current time to the request's context
func AddRequestTime(ctx context.Context, rw http.ResponseWriter, r *http.Request, next ContextHandler) {
	next.ServeHTTPC(AddTime(ctx, time.Now()), rw, r)
}

type metadata int

const (
	requestTime metadata = iota
)

// AddTime will add now to the context's time.  You can get now later with RequestTime
func AddTime(ctx context.Context, now time.Time) context.Context {
	return context.WithValue(ctx, requestTime, now)
}

// RequestTime looks at the context to return the time added with AddTime
func RequestTime(ctx context.Context) time.Time {
	return ctx.Value(requestTime).(time.Time)
}
