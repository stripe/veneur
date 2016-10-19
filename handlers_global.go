package veneur

import (
	"compress/zlib"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"time"

	"github.com/stripe/veneur/samplers"

	"golang.org/x/net/context"
)

type contextHandler func(c context.Context, w http.ResponseWriter, r *http.Request)

func (ch contextHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// nil context is bad form, but since we don't use it, it's a quick hack
	// to allow us to write tests before we upgrade to Go 1.7 Context type
	// and the newer goji
	// TODO(aditya) actually update this
	ch(nil, w, r)
}

// handleImport generates the handler that responds to POST requests submitting
// metrics to the global veneur instance.
func handleImport(s *Server) http.Handler {
	return contextHandler(func(c context.Context, w http.ResponseWriter, r *http.Request) {
		innerLogger := log.WithField("client", r.RemoteAddr)
		start := time.Now()

		var (
			jsonMetrics []samplers.JSONMetric
			body        io.ReadCloser
			err         error
			encoding    = r.Header.Get("Content-Encoding")
		)
		switch encLogger := innerLogger.WithField("encoding", encoding); encoding {
		case "":
			body = r.Body
			encoding = "identity"
		case "deflate":
			body, err = zlib.NewReader(r.Body)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				encLogger.WithError(err).Error("Could not read compressed request body")
				s.statsd.Count("import.request_error_total", 1, []string{"cause:deflate"}, 1.0)
				return
			}
			defer body.Close()
		default:
			http.Error(w, encoding, http.StatusUnsupportedMediaType)
			encLogger.Error("Could not determine content-encoding of request")
			s.statsd.Count("import.request_error_total", 1, []string{"cause:unknown_content_encoding"}, 1.0)
			return
		}

		if err := json.NewDecoder(body).Decode(&jsonMetrics); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			innerLogger.WithError(err).Error("Could not decode /import request")
			s.statsd.Count("import.request_error_total", 1, []string{"cause:json"}, 1.0)
			return
		}

		if len(jsonMetrics) == 0 {
			const msg = "Received empty /import request"
			http.Error(w, msg, http.StatusBadRequest)
			innerLogger.WithError(err).Error(msg)
			return
		}

		// We want to make sure we have at least one entry
		// that is not empty (ie, all fields are the zero value)
		// because that is usually the sign that we are unmarshalling
		// into the wrong struct type
		var nonEmpty bool
		sentinel := samplers.JSONMetric{}
		for _, metric := range jsonMetrics {
			if !reflect.DeepEqual(sentinel, metric) {
				// we have found at least one entry that is properly formed
				nonEmpty = true
				break
			}
		}

		if !nonEmpty {
			const msg = "Received empty or improperly-formed metrics"
			http.Error(w, msg, http.StatusBadRequest)
			innerLogger.Error(msg)
			return
		}

		w.WriteHeader(http.StatusAccepted)
		s.statsd.TimeInMilliseconds("import.response_duration_ns",
			float64(time.Now().Sub(start).Nanoseconds()),
			[]string{"part:request", fmt.Sprintf("encoding:%s", encoding)},
			1.0)

		// the server usually waits for this to return before finalizing the
		// response, so this part must be done asynchronously
		go s.ImportMetrics(jsonMetrics)
	})
}
