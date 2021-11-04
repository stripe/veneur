package veneur

import (
	"net/http"
	"net/http/pprof"
	"sort"
	"time"

	"github.com/segmentio/fasthash/fnv1a"
	"github.com/stripe/veneur/v14/samplers"
	"github.com/stripe/veneur/v14/ssf"
	"github.com/stripe/veneur/v14/trace"
	"github.com/stripe/veneur/v14/trace/metrics"

	"context"

	"goji.io"
	"goji.io/pat"
)

// Handler returns the Handler responsible for routing request processing.
func (s *Server) Handler() http.Handler {
	mux := goji.NewMux()

	mux.HandleFunc(pat.Get("/healthcheck"), func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok\n"))
	})

	mux.HandleFunc(pat.Get("/builddate"), func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(BUILD_DATE))
	})

	mux.HandleFunc(pat.Get("/version"), func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(VERSION))
	})

	if s.httpQuit {
		mux.HandleFunc(pat.Post(httpQuitEndpoint), func(w http.ResponseWriter, r *http.Request) {
			log.WithField("endpoint", httpQuitEndpoint).Info("Received shutdown request on HTTP quit endpoint")
			w.Write([]byte("Beginning graceful shutdown....\n"))
			s.Shutdown()
		})
	}

	// TODO3.0: Maybe remove this endpoint as it is kinda useless now that tracing is always on.
	mux.HandleFunc(pat.Get("/healthcheck/tracing"), func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok\n"))
	})

	mux.Handle(pat.Post("/import"), handleImport(s))

	mux.Handle(pat.Get("/debug/pprof/cmdline"), http.HandlerFunc(pprof.Cmdline))
	mux.Handle(pat.Get("/debug/pprof/profile"), http.HandlerFunc(pprof.Profile))
	mux.Handle(pat.Get("/debug/pprof/symbol"), http.HandlerFunc(pprof.Symbol))
	mux.Handle(pat.Get("/debug/pprof/trace"), http.HandlerFunc(pprof.Trace))
	// TODO match without trailing slash as well
	mux.Handle(pat.Get("/debug/pprof/*"), http.HandlerFunc(pprof.Index))

	return mux
}

// ImportMetrics feeds a slice of json metrics to the server's workers
func (s *Server) ImportMetrics(ctx context.Context, jsonMetrics []samplers.JSONMetric) {
	span, _ := trace.StartSpanFromContext(ctx, "veneur.opentracing.import.import_metrics")
	defer span.Finish()

	// We have a slice of JSONMetrics that we need to divide up across the
	// server WorkerSets. Per WorkerSet, we don't want to push one metric at a
	// time (too much channel contention and goroutine switching) and we also
	// don't want to allocate a temp slice for each worker (which we'll have to
	// append to, therefore lots of allocations). Instead, we'll compute the
	// fnv hash of every metric in the array, and sort the array by the hashes.
	sortedJSONMetrics := newSortableJSONMetrics(jsonMetrics)
	sort.Sort(sortedJSONMetrics)

	for _, workerSet := range s.WorkerSets {
		iterableSortedJSONMetrics := newJSONMetricsByWorkerSet(sortedJSONMetrics, workerSet)
		for iterableSortedJSONMetrics.Next() {
			nextChunk, workerIndex := iterableSortedJSONMetrics.Chunk()
			workerSet.Workers[workerIndex].ImportChan <- nextChunk
		}
	}

	metrics.ReportOne(s.TraceClient, ssf.Timing("import.response_duration_ns", time.Since(span.Start), time.Nanosecond, map[string]string{"part": "merge"}))
}

// sorts a set of jsonmetrics by fnv1a of the jsonmetrics
type sortableJSONMetrics struct {
	metrics []samplers.JSONMetric
	digests []uint32
}

func newSortableJSONMetrics(metrics []samplers.JSONMetric) *sortableJSONMetrics {
	ret := sortableJSONMetrics{
		metrics: metrics,
		digests: make([]uint32, 0, len(metrics)),
	}
	for _, j := range metrics {
		h := fnv1a.Init32
		h = fnv1a.AddString32(h, j.Name)
		h = fnv1a.AddString32(h, j.Type)
		h = fnv1a.AddString32(h, j.JoinedTags)
		ret.digests = append(ret.digests, h)
	}
	return &ret
}

var _ sort.Interface = &sortableJSONMetrics{}

func (sjm *sortableJSONMetrics) Len() int {
	return len(sjm.metrics)
}
func (sjm *sortableJSONMetrics) Less(i, j int) bool {
	return sjm.digests[i] < sjm.digests[j]
}
func (sjm *sortableJSONMetrics) Swap(i, j int) {
	sjm.metrics[i], sjm.metrics[j] = sjm.metrics[j], sjm.metrics[i]
	sjm.digests[i], sjm.digests[j] = sjm.digests[j], sjm.digests[i]
}

type jsonMetricsByWorkerSet struct {
	sortedJSONMetrics *sortableJSONMetrics
	workerSet         WorkerSet
	currentStart      int
	nextStart         int
	calledNextCount   int
}

// Iterable chunks of JSONMetrics for consumption by a WorkerSet. Each chunk
// should correspond to a single Worker within a WorkerSet
func newJSONMetricsByWorkerSet(metrics *sortableJSONMetrics, workerSet WorkerSet) *jsonMetricsByWorkerSet {
	filteredMetrics := make([]samplers.JSONMetric, 0, metrics.Len())
	filteredDigests := make([]uint32, 0, metrics.Len())
	for i := 0; i < metrics.Len(); i++ {
		metric := metrics.metrics[i]
		digest := metrics.digests[i]
		if workerSet.MatcherConfigs.Match(metric.MetricKey.Name, metric.Tags) {
			filteredMetrics = append(filteredMetrics, metric)
			filteredDigests = append(filteredDigests, digest)
		}
	}
	ret := &jsonMetricsByWorkerSet{
		sortedJSONMetrics: &sortableJSONMetrics{
			metrics: filteredMetrics,
			digests: filteredDigests,
		},
		workerSet: workerSet,
	}
	return ret
}

func (jmbws *jsonMetricsByWorkerSet) Next() bool {
	if jmbws.sortedJSONMetrics.Len() == jmbws.nextStart {
		return false
	}

	// look for the first metric whose worker is different from our starting
	// one, or until the end of the list in which case all metrics have the
	// same worker
	for i := jmbws.nextStart; i <= jmbws.sortedJSONMetrics.Len(); i++ {
		if i == jmbws.sortedJSONMetrics.Len() || jmbws.sortedJSONMetrics.digests[i] != jmbws.sortedJSONMetrics.digests[jmbws.nextStart] {
			jmbws.currentStart = jmbws.nextStart
			jmbws.nextStart = i
			break
		}
	}
	jmbws.calledNextCount += 1
	return true
}

func (jmbws *jsonMetricsByWorkerSet) Chunk() ([]samplers.JSONMetric, int) {
	return jmbws.sortedJSONMetrics.metrics[jmbws.currentStart:jmbws.nextStart], (jmbws.calledNextCount - 1) % jmbws.workerSet.WorkerCount
}
