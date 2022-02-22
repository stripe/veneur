package http

import (
	"bytes"
	"compress/zlib"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptrace"
	"net/url"
	"strconv"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stripe/veneur/v14/ssf"
	"github.com/stripe/veneur/v14/trace"
)

var tracer = trace.GlobalTracer

// httpClientTracer is a wrapper around a `net/http/httptrace` that handles
// proper tracing and metric emission.
type httpClientTracer struct {
	prefix      string
	traceClient *trace.Client
	mutex       *sync.Mutex
	ctx         context.Context
	currentSpan *trace.Span
}

// newHTTPClientTracer makes a new HTTPClientTracer with new span created from
// the context.
func newHTTPClientTracer(ctx context.Context, tc *trace.Client, prefix string) *httpClientTracer {
	span, _ := trace.StartSpanFromContext(ctx, "http.start")
	return &httpClientTracer{
		prefix:      prefix,
		traceClient: tc,
		mutex:       &sync.Mutex{},
		ctx:         ctx,
		currentSpan: span,
	}
}

// getClientTrace is a convenience to return a filled-in `ClientTrace`.
func (hct *httpClientTracer) getClientTrace() *httptrace.ClientTrace {
	return &httptrace.ClientTrace{
		GotConn:              hct.gotConn,
		DNSStart:             hct.dnsStart,
		GotFirstResponseByte: hct.gotFirstResponseByte,
		ConnectStart:         hct.connectStart,
		WroteHeaders:         hct.wroteHeaders,
		WroteRequest:         hct.wroteRequest,
	}
}

// startSpan is a convenience that replaces the current span in our
// tracer and flushes the outgoing one.
func (hct *httpClientTracer) startSpan(name string) *trace.Span {
	hct.mutex.Lock()
	defer hct.mutex.Unlock()
	newSpan, _ := trace.StartSpanFromContext(hct.ctx, name)
	hct.currentSpan.ClientFinish(hct.traceClient)

	hct.currentSpan = newSpan
	return newSpan
}

// gotConn signals that the connection — be it new or reused — was fetched
// from the pool.
func (hct *httpClientTracer) gotConn(info httptrace.GotConnInfo) {
	state := "new"
	if info.Reused {
		state = "reused"
	}
	sp := hct.startSpan(fmt.Sprintf("http.gotConnection.%s", state))
	sp.SetTag("was_idle", info.WasIdle)
	sp.Add(ssf.Count(hct.prefix+".connections_used_total", 1, map[string]string{"state": state}))
}

// dnsStart marks the beginning of the DNS lookup
func (hct *httpClientTracer) dnsStart(info httptrace.DNSStartInfo) {
	hct.startSpan("http.resolvingDNS")
}

// gotFirstResponseByte marks us receiving our first byte
func (hct *httpClientTracer) gotFirstResponseByte() {
	hct.startSpan("http.gotFirstByte")
}

// connectStart marks beginning of the connect process
func (hct *httpClientTracer) connectStart(network, addr string) {
	hct.startSpan("http.connecting")
}

// wroteHeaders marks the write being started
func (hct *httpClientTracer) wroteHeaders() {
	hct.startSpan("http.finishedHeaders")
}

// wroteRequest marks the write being completed
func (hct *httpClientTracer) wroteRequest(info httptrace.WroteRequestInfo) {
	hct.startSpan("http.finishedWrite")
}

// finishSpan is called to ensure we're done tracing
func (hct *httpClientTracer) finishSpan() {
	hct.mutex.Lock()
	defer hct.mutex.Unlock()
	hct.currentSpan.ClientFinish(hct.traceClient)
}

type TraceRoundTripper struct {
	inner  http.RoundTripper
	tc     *trace.Client
	prefix string
}

func NewTraceRoundTripper(inner http.RoundTripper, tc *trace.Client, prefix string) http.RoundTripper {
	return &TraceRoundTripper{
		inner:  inner,
		tc:     tc,
		prefix: prefix,
	}
}

func (tripper *TraceRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	span, _ := trace.StartSpanFromContext(req.Context(), "")
	span.SetTag("action", tripper.prefix)
	defer span.ClientFinish(tripper.tc)

	// Add OpenTracing headers to the request, so downstream reqs can be identified:
	trace.GlobalTracer.InjectRequest(span.Trace, req)

	hct := newHTTPClientTracer(span.Attach(req.Context()), tripper.tc, tripper.prefix)
	req = req.WithContext(httptrace.WithClientTrace(req.Context(), hct.getClientTrace()))
	defer hct.finishSpan()

	return tripper.inner.RoundTrip(req)
}

func mergeTags(tags map[string]string, k, v string) map[string]string {
	ret := make(map[string]string, len(tags)+1)
	for k, v := range tags {
		ret[k] = v
	}
	ret[k] = v
	return ret
}

// PostHelper is shared code for POSTing to an endpoint, that consumes JSON, is zlib-
// compressed, that returns 202 on success, that has a small response
// action as a string used for statsd metric names and log messages emitted from
// this function - probably a static string for each callsite
// you can disable compression with compress=false for endpoints that don't
// support it
func PostHelper(
	ctx context.Context, httpClient *http.Client, tc *trace.Client, method string,
	endpoint string, bodyObject interface{}, action string, compress bool,
	extraTags map[string]string, log *logrus.Entry,
) error {
	span, _ := trace.StartSpanFromContext(ctx, "")
	span.SetTag("action", action)
	for k, v := range extraTags {
		span.SetTag(k, v)
	}
	defer span.ClientFinish(tc)

	// attach this field to all the logs we generate
	innerLogger := log.WithField("action", action)

	marshalStart := time.Now()
	var (
		bodyBuffer bytes.Buffer
		encoder    *json.Encoder
		compressor *zlib.Writer
	)
	if compress {
		compressor = zlib.NewWriter(&bodyBuffer)
		encoder = json.NewEncoder(compressor)
	} else {
		encoder = json.NewEncoder(&bodyBuffer)
	}
	if err := encoder.Encode(bodyObject); err != nil {
		span.Error(err)
		span.Add(ssf.Count(action+".error_total", 1, mergeTags(extraTags, "cause", "json")))
		innerLogger.WithError(err).Error("Could not render JSON")
		return err
	}
	if compress {
		// don't forget to flush leftover compressed bytes to the buffer
		if err := compressor.Close(); err != nil {
			span.Error(err)
			span.Add(ssf.Count(action+".error_total", 1, mergeTags(extraTags, "cause", "compress")))
			innerLogger.WithError(err).Error("Could not finalize compression")
			return err
		}
	}
	span.Add(ssf.Timing(action+".duration_ns", time.Since(marshalStart), time.Nanosecond, mergeTags(extraTags, "part", "json")))

	// Len reports the unread length, so we have to record this before the
	// http client consumes it
	bodyLength := bodyBuffer.Len()
	span.Add(ssf.Count(action+".content_length_bytes", float32(bodyLength), nil))

	req, err := http.NewRequest(method, endpoint, &bodyBuffer)
	if err != nil {
		span.Error(err)
		span.Add(ssf.Count(action+".error_total", 1, mergeTags(extraTags, "cause", "construct")))
		innerLogger.WithError(err).Error("Could not construct request")
		return err
	}

	req = req.WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	if compress {
		req.Header.Set("Content-Encoding", "deflate")
	}

	err = tracer.InjectRequest(span.Trace, req)
	if err != nil {
		span.Add(ssf.Count("opentracing.flush.inject.errors", 1, nil))
		innerLogger.WithError(err).Error("Error injecting header")
	}

	// Attach a ClientTrace that emits span for all our HTTP bits.
	hct := newHTTPClientTracer(span.Attach(ctx), tc, action)
	req = req.WithContext(httptrace.WithClientTrace(req.Context(), hct.getClientTrace()))
	defer hct.finishSpan()

	resp, err := httpClient.Do(req)
	if err != nil {
		if urlErr, ok := err.(*url.Error); ok {
			// if the error has the url in it, then retrieve the inner error
			// and ditch the url (which might contain secrets)
			err = urlErr.Err
		}
		span.Error(err)
		span.Add(ssf.Count(action+".error_total", 1, mergeTags(extraTags, "cause", "io")))
		// Log at Warn level instead of Error, because we don't want to create
		// Sentry events for these (they're only important in large numbers, and
		// we already have Datadog metrics for them)
		innerLogger.WithError(err).WithFields(logrus.Fields{
			"host": req.URL.Host,
			"path": req.URL.Path,
		}).Warn("Could not execute request")
		return err
	}
	defer resp.Body.Close()

	responseBody, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		// this error is not fatal, since we only need the body for reporting
		// purposes
		span.Error(err)
		span.Add(ssf.Count(action+".error_total", 1, mergeTags(extraTags, "cause", "readresponse")))
		innerLogger.WithError(err).Error("Could not read response body")
	}
	resultLogger := innerLogger.WithFields(logrus.Fields{
		"endpoint":         endpoint,
		"request_length":   bodyLength,
		"request_headers":  req.Header,
		"status":           resp.Status,
		"response_headers": resp.Header,
		"response":         string(responseBody),
	})

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		err := fmt.Errorf("%v", resp.StatusCode)
		span.Error(err)
		span.Add(ssf.Count(action+".error_total", 1, mergeTags(extraTags, "cause", strconv.Itoa(resp.StatusCode))))
		resultLogger.WithError(err).Warn("Could not POST")
		return err
	}

	// make sure the error metric isn't sparse
	span.Add(ssf.Count(action+".error_total", 0, nil))
	resultLogger.Debug("POSTed successfully")
	return nil
}
