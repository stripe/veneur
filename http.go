package veneur

import (
	"encoding/json"
	"hash/fnv"
	"net/http"
	"sort"
	"time"

	"github.com/Sirupsen/logrus"
	"goji.io"
	"goji.io/pat"
	"golang.org/x/net/context"
)

func (s *Server) Handler() http.Handler {
	mux := goji.NewMux()

	mux.HandleFuncC(pat.Get("/healthcheck"), func(c context.Context, w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok\n"))
	})

	mux.HandleFuncC(pat.Post("/import"), func(c context.Context, w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		var jsonMetrics []JSONMetric
		if err := json.NewDecoder(r.Body).Decode(&jsonMetrics); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			s.logger.WithFields(logrus.Fields{
				logrus.ErrorKey: err,
				"client":        r.RemoteAddr,
			}).Error("Could not decode /import request")
			s.statsd.Count("import.request_error_total", 1, []string{"cause:json"}, 1.0)
			return
		}

		w.WriteHeader(http.StatusAccepted)
		s.statsd.TimeInMilliseconds("import.response_duration_ns", float64(time.Now().Sub(start).Nanoseconds()), []string{"part:request"}, 1.0)

		// the server usually waits for this to return before finalizing the
		// response, so this part must be done asynchronously
		go s.ImportMetrics(jsonMetrics)
	})

	return mux
}

// feed a slice of json metrics to the server's workers
func (s *Server) ImportMetrics(jsonMetrics []JSONMetric) {
	start := time.Now()

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

	s.statsd.TimeInMilliseconds("import.response_duration_ns", float64(time.Now().Sub(start).Nanoseconds()), []string{"part:merge"}, 1.0)
}

// sorts a set of jsonmetrics by what worker they belong to
type sortableJSONMetrics struct {
	metrics       []JSONMetric
	workerIndices []uint32
}

func newSortableJSONMetrics(metrics []JSONMetric, numWorkers int) *sortableJSONMetrics {
	ret := sortableJSONMetrics{
		metrics:       metrics,
		workerIndices: make([]uint32, 0, len(metrics)),
	}
	for _, j := range metrics {
		h := fnv.New32a()
		h.Write([]byte(j.Name))
		h.Write([]byte(j.Type))
		h.Write([]byte(j.JoinedTags))
		ret.workerIndices = append(ret.workerIndices, h.Sum32()%uint32(numWorkers))
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
// nonempty chunks such that each chunk correpsonds to a single worker
func newJSONMetricsByWorker(metrics []JSONMetric, numWorkers int) *jsonMetricsByWorker {
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
func (jmbw *jsonMetricsByWorker) Chunk() ([]JSONMetric, int) {
	return jmbw.sjm.metrics[jmbw.currentStart:jmbw.nextStart], int(jmbw.sjm.workerIndices[jmbw.currentStart])
}
