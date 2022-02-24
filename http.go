package veneur

import (
	"encoding/json"
	"net/http"
	"net/http/pprof"
	"sort"
	"time"

	"github.com/stripe/veneur/v14/samplers"
	"github.com/stripe/veneur/v14/ssf"
	"github.com/stripe/veneur/v14/trace"
	"github.com/stripe/veneur/v14/trace/metrics"
	"gopkg.in/yaml.v2"

	"context"

	"github.com/segmentio/fasthash/fnv1a"
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

	if s.Config.HTTP.Config {
		mux.HandleFunc(pat.Get("/config/json"), s.handleConfigJson)
		mux.HandleFunc(pat.Get("/config/yaml"), s.handleConfigYaml)
	}

	if s.httpQuit {
		mux.HandleFunc(pat.Post(httpQuitEndpoint), func(w http.ResponseWriter, r *http.Request) {
			s.logger.WithField("endpoint", httpQuitEndpoint).
				Info("Received shutdown request on HTTP quit endpoint")
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

func (s *Server) handleConfigJson(w http.ResponseWriter, r *http.Request) {
	config, err := json.MarshalIndent(s.Config, "", "  ")
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}
	w.Header().Add("Content-Type", "application/json")
	w.Write(config)
}

func (s *Server) handleConfigYaml(w http.ResponseWriter, r *http.Request) {
	config, err := yaml.Marshal(s.Config)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}
	w.Header().Add("Content-Type", "application/x-yaml")
	w.Write(config)
}

// ImportMetrics feeds a slice of json metrics to the server's workers
func (s *Server) ImportMetrics(ctx context.Context, jsonMetrics []samplers.JSONMetric) {
	span, _ := trace.StartSpanFromContext(ctx, "veneur.opentracing.import.import_metrics")
	defer span.Finish()

	// we have a slice of json metrics that we need to divide up across the workers
	// we don't want to push one metric at a time (too much channel contention
	// and goroutine switching) and we also don't want to allocate a temp
	// slice for each worker (which we'll have to append to, therefore lots
	// of allocations)
	// instead, we'll compute the fnv hash of every metric in the array,
	// and sort the array by the hashes
	sortedIter := newJSONMetricsByWorker(jsonMetrics, len(s.Workers))
	for sortedIter.Next() {
		nextChunk, workerIndex := sortedIter.Chunk()
		s.Workers[workerIndex].ImportChan <- nextChunk
	}
	metrics.ReportOne(s.TraceClient, ssf.Timing("import.response_duration_ns", time.Since(span.Start), time.Nanosecond, map[string]string{"part": "merge"}))
}

// sorts a set of jsonmetrics by what worker they belong to
type sortableJSONMetrics struct {
	metrics       []samplers.JSONMetric
	workerIndices []uint32
}

func newSortableJSONMetrics(metrics []samplers.JSONMetric, numWorkers int) *sortableJSONMetrics {
	ret := sortableJSONMetrics{
		metrics:       metrics,
		workerIndices: make([]uint32, 0, len(metrics)),
	}
	for _, j := range metrics {
		h := fnv1a.Init32
		h = fnv1a.AddString32(h, j.Name)
		h = fnv1a.AddString32(h, j.Type)
		h = fnv1a.AddString32(h, j.JoinedTags)
		ret.workerIndices = append(ret.workerIndices, h%uint32(numWorkers))
	}
	return &ret
}

var _ sort.Interface = &sortableJSONMetrics{}

func (sjm *sortableJSONMetrics) Len() int {
	return len(sjm.metrics)
}
func (sjm *sortableJSONMetrics) Less(i, j int) bool {
	return sjm.workerIndices[i] < sjm.workerIndices[j]
}
func (sjm *sortableJSONMetrics) Swap(i, j int) {
	sjm.metrics[i], sjm.metrics[j] = sjm.metrics[j], sjm.metrics[i]
	sjm.workerIndices[i], sjm.workerIndices[j] = sjm.workerIndices[j], sjm.workerIndices[i]
}

type jsonMetricsByWorker struct {
	sjm          *sortableJSONMetrics
	currentStart int
	nextStart    int
}

// iterate over a sorted set of jsonmetrics, returning them in contiguous
// nonempty chunks such that each chunk corresponds to a single worker.
func newJSONMetricsByWorker(metrics []samplers.JSONMetric, numWorkers int) *jsonMetricsByWorker {
	ret := &jsonMetricsByWorker{
		sjm: newSortableJSONMetrics(metrics, numWorkers),
	}
	sort.Sort(ret.sjm)
	return ret
}

func (jmbw *jsonMetricsByWorker) Next() bool {
	if jmbw.sjm.Len() == jmbw.nextStart {
		return false
	}

	// look for the first metric whose worker is different from our starting
	// one, or until the end of the list in which case all metrics have the
	// same worker
	for i := jmbw.nextStart; i <= jmbw.sjm.Len(); i++ {
		if i == jmbw.sjm.Len() || jmbw.sjm.workerIndices[i] != jmbw.sjm.workerIndices[jmbw.nextStart] {
			jmbw.currentStart = jmbw.nextStart
			jmbw.nextStart = i
			break
		}
	}
	return true
}
func (jmbw *jsonMetricsByWorker) Chunk() ([]samplers.JSONMetric, int) {
	return jmbw.sjm.metrics[jmbw.currentStart:jmbw.nextStart], int(jmbw.sjm.workerIndices[jmbw.currentStart])
}
