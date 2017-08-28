package veneur

import (
	"compress/zlib"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"time"

	"github.com/DataDog/datadog-go/statsd"
	"github.com/stripe/veneur/samplers"
	"github.com/stripe/veneur/trace"
)

type contextHandler func(c context.Context, w http.ResponseWriter, r *http.Request)

func (ch contextHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()
	ch(ctx, w, r)
}

func handleProxy(p *Proxy) http.Handler {
	return contextHandler(func(ctx context.Context, w http.ResponseWriter, r *http.Request) {
		span, jsonMetrics, err := unmarshalMetricsFromHTTP(ctx, p.Statsd, w, r)
		if err != nil {
			return
		}
		// the server usually waits for this to return before finalizing the
		// response, so this part must be done asynchronously
		go p.ProxyMetrics(span.Attach(ctx), jsonMetrics)
	})
}

func handleSpans(p *Proxy) http.Handler {
	return contextHandler(func(ctx context.Context, w http.ResponseWriter, r *http.Request) {
		span, traces, err := handleTraceRequest(ctx, p.Statsd, w, r)

		if err != nil {
			return
		}
		// the server usually waits for this to return before finalizing the
		// response, so this part must be done asynchronously
		go p.ProxyTraces(span.Attach(ctx), traces)
	})
}

// handleImport generates the handler that responds to POST requests submitting
// metrics to the global veneur instance.
func handleImport(s *Server) http.Handler {
	return contextHandler(func(ctx context.Context, w http.ResponseWriter, r *http.Request) {
		span, jsonMetrics, err := unmarshalMetricsFromHTTP(ctx, s.Statsd, w, r)
		if err != nil {
			return
		}
		// the server usually waits for this to return before finalizing the
		// response, so this part must be done asynchronously
		go s.ImportMetrics(span.Attach(ctx), jsonMetrics)
	})
}

func handleTraceRequest(ctx context.Context, stats *statsd.Client, w http.ResponseWriter, r *http.Request) (*trace.Span, []DatadogTraceSpan, error) {
	var (
		traces []DatadogTraceSpan
		err    error
		span   *trace.Span
	)

	span, err = tracer.ExtractRequestChild("/spans", r, "veneur.opentracing.spans")
	if err != nil {
		log.WithError(err).Info("Could not extract span from request")
		span = tracer.StartSpan("/spans").(*trace.Span)
	} else {
		log.WithField("trace", span.Trace).Info("Extracted span from request")
	}
	defer span.Finish()

	innerLogger := log.WithField("client", r.RemoteAddr)

	if err = json.NewDecoder(r.Body).Decode(&traces); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		span.Error(err)
		innerLogger.WithError(err).Error("Could not decode /spans request")
		stats.Count("import.request_error_total", 1, []string{"cause:json"}, 1.0)
		return nil, nil, err
	}

	if len(traces) == 0 {
		const msg = "Received empty /spans request"
		http.Error(w, msg, http.StatusBadRequest)
		span.Error(errors.New(msg))
		innerLogger.WithError(err).Error(msg)
		return nil, nil, err
	}

	w.WriteHeader(http.StatusAccepted)
	stats.TimeInMilliseconds("spans.response_duration_ns",
		float64(time.Since(span.Start).Nanoseconds()),
		[]string{"part:request"},
		1.0)

	return span, traces, nil
}

// unmarshalMetricsFromHTTP takes care of the common need to unmarshal a slice of metrics from a request body,
// dealing with error handling, decoding, tracing, and the associated metrics.
func unmarshalMetricsFromHTTP(ctx context.Context, stats *statsd.Client, w http.ResponseWriter, r *http.Request) (*trace.Span, []samplers.JSONMetric, error) {
	var (
		jsonMetrics []samplers.JSONMetric
		body        io.ReadCloser
		err         error
		encoding    = r.Header.Get("Content-Encoding")
		span        *trace.Span
	)

	span, err = tracer.ExtractRequestChild("/import", r, "veneur.opentracing.import")
	if err != nil {
		log.WithError(err).Debug("Could not extract span from request")
		span = tracer.StartSpan("veneur.opentracing.import").(*trace.Span)
	} else {
		log.WithField("trace", span.Trace).Debug("Extracted span from request")
	}
	defer span.Finish()

	innerLogger := log.WithField("client", r.RemoteAddr)

	switch encLogger := innerLogger.WithField("encoding", encoding); encoding {
	case "":
		body = r.Body
		encoding = "identity"
	case "deflate":
		body, err = zlib.NewReader(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			span.Error(err)
			encLogger.WithError(err).Error("Could not read compressed request body")
			stats.Count("import.request_error_total", 1, []string{"cause:deflate"}, 1.0)
			return nil, nil, err
		}
		defer body.Close()
	default:
		http.Error(w, encoding, http.StatusUnsupportedMediaType)
		span.Error(errors.New("Could not determine content-encoding of request"))
		encLogger.Error("Could not determine content-encoding of request")
		stats.Count("import.request_error_total", 1, []string{"cause:unknown_content_encoding"}, 1.0)
		return nil, nil, err
	}

	if err = json.NewDecoder(body).Decode(&jsonMetrics); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		span.Error(err)
		innerLogger.WithError(err).Error("Could not decode /import request")
		stats.Count("import.request_error_total", 1, []string{"cause:json"}, 1.0)
		return nil, nil, err
	}

	if len(jsonMetrics) == 0 {
		const msg = "Received empty /import request"
		http.Error(w, msg, http.StatusBadRequest)
		span.Error(errors.New(msg))
		innerLogger.WithError(err).Error(msg)
		return nil, nil, err
	}

	// We want to make sure we have at least one entry
	// that is not empty (ie, all fields are the zero value)
	// because that is usually the sign that we are unmarshalling
	// into the wrong struct type

	if !nonEmpty(span.Attach(ctx), jsonMetrics) {
		const msg = "Received empty or improperly-formed metrics"
		http.Error(w, msg, http.StatusBadRequest)
		span.Error(errors.New(msg))
		innerLogger.Error(msg)
		return nil, nil, err
	}

	w.WriteHeader(http.StatusAccepted)
	stats.TimeInMilliseconds("import.response_duration_ns",
		float64(time.Since(span.Start).Nanoseconds()),
		[]string{"part:request", fmt.Sprintf("encoding:%s", encoding)},
		1.0)

	return span, jsonMetrics, nil
}

// nonEmpty returns true if there is at least one non-empty
// metric
func nonEmpty(ctx context.Context, jsonMetrics []samplers.JSONMetric) bool {

	span, _ := trace.StartSpanFromContext(ctx, "veneur.opentracing.import.nonEmpty")
	defer span.Finish()

	sentinel := samplers.JSONMetric{}
	for _, metric := range jsonMetrics {
		if !reflect.DeepEqual(sentinel, metric) {
			// we have found at least one entry that is properly formed
			return true

		}
	}
	return false
}
