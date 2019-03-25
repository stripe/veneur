// Package envoyhttp provides an http.RoundTripper implementation that
// inspects outgoing HTTP requests' Contexts and sets HTTP headers on
// them that instruct the Envoy proxy to handle the request such that
// they match the parameters in the Context.
//
// Notes and References
//
// HTTP headers Envoy understands: https://www.envoyproxy.io/docs/envoy/latest/configuration/http_filters/router_filter#http-headers-consumed
package envoyhttp

import (
	"context"
	"net/http"
	"strconv"
	"time"
)

// Transport is a http.RoundTripper implementation that adds envoy
// request headers to HTTP requests made with it.
//
// This Transport will inspect each http.Request, and apply each
// request's Deadline and other contextual information as envoy
// headers. If any of the corresponding HTTP headers are set on the
// outgoing Request, the Transport will not touch them.
//
// It uses the Innex http.Transport to actually round-trip the
// requests.
type Transport struct {
	// The http round-tripper that will actually execute this
	// request. If nil, http.DefaultTransport is used.
	Inner http.RoundTripper

	// ExtraAllowance is an additional amount of time that the
	// transport should account for when setting the overall
	// request timeout, reducing envoy's timeout by this amount.
	//
	// If zero, the Transport sets the envoy timeout header to be
	// equal to the deadline from the time the request is made,
	// which will make it very likely that the request's Context
	// expires before envoy has a chance to return a timeout
	// expiry status.
	//
	// ExtraAllowance is given in millisecond granularity -
	// durations smaller than a millisecond will be truncated
	// down.
	ExtraAllowance time.Duration
}

const (
	envoyHeaderRqTimeout     = "x-envoy-upstream-rq-timeout-ms"
	envoyHeaderPerTryTimeout = "x-envoy-upstream-rq-per-try-timeout-ms"
	envoyHeaderMaxRetries    = "x-envoy-max-retries"
)

func (t *Transport) RoundTrip(r *http.Request) (*http.Response, error) {
	ctx := r.Context()
	headers := r.Header

	// Set the deadline header:
	if dl, ok := ctx.Deadline(); ok {
		allowance := t.ExtraAllowance
		if allowance < 0 {
			allowance = 0
		}
		if r.Header.Get(envoyHeaderRqTimeout) == "" {
			timeoutMs := (dl.Sub(time.Now()) - allowance).
				Truncate(time.Millisecond)
			if timeoutMs > 0 {
				headers.Add(envoyHeaderRqTimeout, strconv.Itoa(int(timeoutMs/time.Millisecond)))
			}
		}
	}

	// Set number of attempts:
	if tries := maxRetries(ctx); tries > 0 && r.Header.Get(envoyHeaderMaxRetries) == "" {
		headers.Add(envoyHeaderMaxRetries, strconv.Itoa(int(tries)))
	}

	// Set amount of timeout per attempt:
	if perTry := perRetryTimeout(ctx); perTry > 0 && r.Header.Get(envoyHeaderPerTryTimeout) == "" {
		headers.Add(envoyHeaderPerTryTimeout, strconv.Itoa(int(perTry/time.Millisecond)))
	}

	r.Header = headers

	inner := t.Inner
	if inner == nil {
		inner = http.DefaultTransport
	}
	return inner.RoundTrip(r)
}

type ctxKey int

const (
	maxTryKey     ctxKey = iota
	perTryTimeout ctxKey = iota
)

// WithMaxRetries sets the "x-envoy-max-retries" header, the maximum
// number of times Envoy should retry an HTTP request. If set to 0, no
// header will get set.
func WithMaxRetries(ctx context.Context, attempts uint) context.Context {
	return context.WithValue(ctx, maxTryKey, attempts)
}

// WithPerRetryTimeout sets "x-envoy-upstream-rq-per-try-timeout-ms",
// the amount of time each retry will be allocated by Envoy. Setting a
// per-retry timeout of 0 disables setting the header.
//
// The per-retry timeout is given in millisecond granularity. Any
// finer intervals are truncated down.
func WithPerRetryTimeout(ctx context.Context, timeout time.Duration) context.Context {
	return context.WithValue(ctx, perTryTimeout, timeout)
}

func maxRetries(ctx context.Context) uint {
	r, _ := ctx.Value(maxTryKey).(uint)
	return r
}

func perRetryTimeout(ctx context.Context) time.Duration {
	r, _ := ctx.Value(perTryTimeout).(time.Duration)
	return r
}
