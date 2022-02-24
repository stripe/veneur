package veneur

import (
	"compress/zlib"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"reflect"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stripe/veneur/v14/samplers"
	"github.com/stripe/veneur/v14/ssf"
	"github.com/stripe/veneur/v14/trace"
)

type contextHandler func(c context.Context, w http.ResponseWriter, r *http.Request)

func (ch contextHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()
	ch(ctx, w, r)
}

func handleProxy(p *Proxy) http.Handler {
	return contextHandler(func(ctx context.Context, w http.ResponseWriter, r *http.Request) {
		p.logger.WithFields(logrus.Fields{
			"path": r.URL.Path,
			"host": r.URL.Host,
		}).Debug("Importing metrics on proxy")
		span, jsonMetrics, err := unmarshalMetricsFromHTTP(
			ctx, p.TraceClient, w, r, p.logger)
		if err != nil {
			p.logger.WithError(err).
				Error("Error unmarshalling metrics in proxy import")
			return
		}
		// the server usually waits for this to return before finalizing the
		// response, so this part must be done asynchronously
		go p.ProxyMetrics(span.Attach(ctx), jsonMetrics, strings.SplitN(r.RemoteAddr, ":", 2)[0])
	})
}

// handleImport generates the handler that responds to POST requests submitting
// metrics to the global veneur instance.
func handleImport(s *Server) http.Handler {
	return contextHandler(func(ctx context.Context, w http.ResponseWriter, r *http.Request) {
		span, jsonMetrics, err := unmarshalMetricsFromHTTP(
			ctx, s.TraceClient, w, r, s.logger)
		if err != nil {
			s.logger.WithError(err).Error("Error unmarshalling metrics in global import")
			span.Add(ssf.Count("import.unmarshal.errors_total", 1, nil))
			return
		}
		// the server usually waits for this to return before finalizing the
		// response, so this part must be done asynchronously
		go s.ImportMetrics(span.Attach(ctx), jsonMetrics)
	})
}

// unmarshalMetricsFromHTTP takes care of the common need to unmarshal a slice of metrics from a request body,
// dealing with error handling, decoding, tracing, and the associated metrics.
func unmarshalMetricsFromHTTP(
	ctx context.Context, client *trace.Client, w http.ResponseWriter,
	r *http.Request, logger *logrus.Entry,
) (*trace.Span, []samplers.JSONMetric, error) {
	var (
		jsonMetrics []samplers.JSONMetric
		body        io.ReadCloser
		err         error
		encoding    = r.Header.Get("Content-Encoding")
		span        *trace.Span
	)
	span, err = tracer.ExtractRequestChild("/import", r, "veneur.opentracing.import")
	if err != nil {
		logger.WithError(err).Debug("Could not extract span from request")
		span = tracer.StartSpan("veneur.opentracing.import").(*trace.Span)
	} else {
		logger.WithField("trace", span.Trace).Debug("Extracted span from request")
	}
	defer span.ClientFinish(client)

	innerLogger := logger.WithField("client", r.RemoteAddr)

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
			span.Add(ssf.Count("import.request_error_total", 1, map[string]string{"cause": "deflate"}))
			return span, nil, err
		}
		defer body.Close()
	default:
		http.Error(w, encoding, http.StatusUnsupportedMediaType)
		span.Error(errors.New("could not determine content-encoding of request"))
		encLogger.Error("Could not determine content-encoding of request")
		span.Add(ssf.Count("import.request_error_total", 1, map[string]string{"cause": "unknown_content_encoding"}))
		return span, nil, err
	}
	span.Add(ssf.Count("import.bytes", float32(r.ContentLength), nil))

	if err = json.NewDecoder(body).Decode(&jsonMetrics); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		span.Error(err)
		innerLogger.WithError(err).Error("Could not decode /import request")
		span.Add(ssf.Count("import.request_error_total", 1, map[string]string{"cause": "json"}))
		return span, nil, err
	}

	if len(jsonMetrics) == 0 {
		const msg = "received empty /import request"
		http.Error(w, msg, http.StatusBadRequest)
		span.Error(errors.New(msg))
		innerLogger.WithError(err).Error(msg)
		return span, nil, err
	}

	// We want to make sure we have at least one entry
	// that is not empty (ie, all fields are the zero value)
	// because that is usually the sign that we are unmarshalling
	// into the wrong struct type

	if !nonEmpty(span.Attach(ctx), client, jsonMetrics) {
		const msg = "received empty or improperly-formed metrics"
		http.Error(w, msg, http.StatusBadRequest)
		span.Error(errors.New(msg))
		innerLogger.Error(msg)
		return span, nil, err
	}

	w.WriteHeader(http.StatusAccepted)
	span.Add(ssf.RandomlySample(0.1,
		ssf.Timing("import.response_duration_ns", time.Since(span.Start),
			time.Nanosecond, map[string]string{
				"part":     "request",
				"encoding": encoding,
			}))...)
	return span, jsonMetrics, nil
}

// nonEmpty returns true if there is at least one non-empty
// metric
func nonEmpty(ctx context.Context, client *trace.Client, jsonMetrics []samplers.JSONMetric) bool {

	span, _ := trace.StartSpanFromContext(ctx, "veneur.opentracing.import.nonEmpty")
	defer span.ClientFinish(client)

	sentinel := samplers.JSONMetric{}
	for _, metric := range jsonMetrics {
		if !reflect.DeepEqual(sentinel, metric) {
			// we have found at least one entry that is properly formed
			return true

		}
	}
	return false
}
