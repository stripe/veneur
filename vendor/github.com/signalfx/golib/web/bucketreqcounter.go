package web

import (
	"net/http"
	"sync/atomic"

	"github.com/signalfx/golib/datapoint"
	"github.com/signalfx/golib/sfxclient"
)

// BucketRequestCounter is a negroni handler that tracks connection stats including p99/etc.  It's, ideally, an
// improvment over reqcounter.go
type BucketRequestCounter struct {
	ActiveConnections int64
	Bucket            *sfxclient.RollingBucket
}

var _ HTTPConstructor = (&BucketRequestCounter{}).Wrap
var _ NextHTTP = (&BucketRequestCounter{}).ServeHTTP

// Wrap returns a handler that forwards calls to next and counts the calls forwarded
func (m *BucketRequestCounter) Wrap(next http.Handler) http.Handler {
	f := func(w http.ResponseWriter, r *http.Request) {
		m.ServeHTTP(w, r, next)
	}
	return http.HandlerFunc(f)
}

func (m *BucketRequestCounter) ServeHTTP(rw http.ResponseWriter, r *http.Request, next http.Handler) {
	atomic.AddInt64(&m.ActiveConnections, 1)
	start := m.Bucket.Timer.Now()
	defer func() {
		atomic.AddInt64(&m.ActiveConnections, -1)
		end := m.Bucket.Timer.Now()
		reqDuration := end.Sub(start)
		m.Bucket.AddAt(reqDuration.Seconds(), end)
	}()
	next.ServeHTTP(rw, r)
}

// Datapoints returns active connections plus wrapped bucket stats
func (m *BucketRequestCounter) Datapoints() []*datapoint.Datapoint {
	dps := m.Bucket.Datapoints()
	return append(dps,
		sfxclient.Gauge(m.Bucket.MetricName+".active_connections", m.Bucket.Dimensions, atomic.LoadInt64(&m.ActiveConnections)),
	)
}
