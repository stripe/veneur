package web

import (
	"net/http"
	"sync/atomic"
	"time"

	"github.com/signalfx/golib/datapoint"
	"github.com/signalfx/golib/sfxclient"
)

// RequestCounter is a negroni handler that tracks connection stats
type RequestCounter struct {
	TotalConnections      int64
	ActiveConnections     int64
	TotalProcessingTimeNs int64
}

var _ HTTPConstructor = (&RequestCounter{}).Wrap
var _ NextHTTP = (&RequestCounter{}).ServeHTTP

// Wrap returns a handler that forwards calls to next and counts the calls forwarded
func (m *RequestCounter) Wrap(next http.Handler) http.Handler {
	f := func(w http.ResponseWriter, r *http.Request) {
		m.ServeHTTP(w, r, next)
	}
	return http.HandlerFunc(f)
}

func (m *RequestCounter) ServeHTTP(rw http.ResponseWriter, r *http.Request, next http.Handler) {
	atomic.AddInt64(&m.TotalConnections, 1)
	atomic.AddInt64(&m.ActiveConnections, 1)
	defer atomic.AddInt64(&m.ActiveConnections, -1)
	start := time.Now()
	next.ServeHTTP(rw, r)
	reqDuration := time.Since(start)
	atomic.AddInt64(&m.TotalProcessingTimeNs, reqDuration.Nanoseconds())
}

// Datapoints returns stats on total connections, active connections, and total processing time
func (m *RequestCounter) Datapoints() []*datapoint.Datapoint {
	return []*datapoint.Datapoint{
		sfxclient.Cumulative("total_connections", nil, atomic.LoadInt64(&m.TotalConnections)),
		sfxclient.Cumulative("total_time_ns", nil, atomic.LoadInt64(&m.TotalProcessingTimeNs)),
		sfxclient.Gauge("active_connections", nil, atomic.LoadInt64(&m.ActiveConnections)),
	}
}
