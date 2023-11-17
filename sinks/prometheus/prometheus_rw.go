package prometheus

import (
	"bufio"
	"bytes"
	"container/list"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/golang/snappy"
	"github.com/pkg/errors"
	"github.com/stripe/veneur/v14"
	"github.com/stripe/veneur/v14/samplers"
	"github.com/stripe/veneur/v14/sinks"
	"github.com/stripe/veneur/v14/sinks/prometheus/mapper"
	"github.com/stripe/veneur/v14/sinks/prometheus/prompb"
	"github.com/stripe/veneur/v14/ssf"
	"github.com/stripe/veneur/v14/trace"
	"github.com/stripe/veneur/v14/util"

	"github.com/google/uuid"
	"github.com/prometheus/common/config"
	"github.com/sirupsen/logrus"
)

// gauge to report current queue size
const metricRwSinkQueueSize = "rw_sink_queue_size"

// gauge to report defined max queue size
const metricRwSinkMaxQueueSize = "rw_sink_max_queue_size"

// count to report failed PRWS requests
const metricRwSinkFailedPrwsRequestsTotal = "rw_sink_prws_failed_requests_total"

// count to report dropped PRWS requests (if queue is full we start dropping)
const metricRwSinkDroppedPrwsRequestsTotal = "rw_sink_prws_dropped_requests_total"

var noTags = map[string]string{}

var queue = NewConcurrentQueue()

type RWRequest struct {
	request      []byte
	ctx          context.Context
	size         int
	totalMetrics int
	timestamp    time.Time
	id           uuid.UUID
}

// queue to avoid discarding metrics when the sink is delayed in sending data by momentary problems happening to Remote Write
type ConcurrentQueue struct {
	list        *list.List
	lock        sync.Mutex
	byteSize    int
	maxByteSize int
	logger      *logrus.Entry
}

func NewConcurrentQueue() *ConcurrentQueue {
	q := &ConcurrentQueue{
		list: list.New(),
	}
	return q
}

// Put the item in the back of the queue
func (q *ConcurrentQueue) Enqueue(span *trace.Span, item RWRequest) {
	q.lock.Lock()
	defer q.lock.Unlock()

	newItemSize := item.size

	if newItemSize > q.maxByteSize {
		q.logger.Error("Enqueue - dropping request because is bigger than queue size! Request size is ", newItemSize, " bytes, max queue size is ", q.maxByteSize, " bytes.")
		tags := map[string]string{"drop_reason": "size_too_big"}
		span.Add(
			ssf.Count(metricRwSinkDroppedPrwsRequestsTotal, float32(1), tags),
		)
		return
	}

	// if there's no space left, make it
	if q.byteSize+newItemSize > q.maxByteSize {
		q.logger.Debug("Need to make space: queue max size is ", q.maxByteSize, ", current size is ", q.byteSize, ", and new element size is ", newItemSize, " bytes.")
		firstDeletableItem := q.list.Front()
		cntDropped := 0
		for q.byteSize+newItemSize > q.maxByteSize && firstDeletableItem != nil {
			q.logger.Debug("Deleting 1 element of ", firstDeletableItem.Value.(RWRequest).size, " bytes.")
			q.byteSize = q.byteSize - firstDeletableItem.Value.(RWRequest).size
			toRemove := firstDeletableItem
			firstDeletableItem = firstDeletableItem.Next()
			q.list.Remove(toRemove)
			cntDropped += 1
		}
		q.logger.Warn("Enqueue - dropped ", cntDropped, " old requests to make room for a new request of size ", newItemSize, " bytes. New queue size is ", q.byteSize, " bytes.")
		tags := map[string]string{"drop_reason": "push_back_make_space"}
		span.Add(
			ssf.Count(metricRwSinkDroppedPrwsRequestsTotal, float32(cntDropped), tags),
		)
	}

	q.list.PushBack(item)
	q.byteSize += item.size
	q.logger.Debug("After insert on back, queue current size is ", q.byteSize, " bytes, queue current length is ", q.list.Len(), " elements (requests).")
}

// Put the item in the front of the queue
func (q *ConcurrentQueue) EnqueueFront(span *trace.Span, item RWRequest) {
	q.lock.Lock()
	defer q.lock.Unlock()

	newItemSize := item.size

	//if there's no space left, don't add the item
	if q.byteSize+newItemSize > q.maxByteSize {
		q.logger.Warn("EnqueueFront - dropping a request of size ", newItemSize, " bytes because queue is full.")
		tags := map[string]string{"drop_reason": "push_front_queue_full"}
		span.Add(
			ssf.Count(metricRwSinkDroppedPrwsRequestsTotal, float32(1), tags),
		)
		return
	}

	q.list.PushFront(item)
	q.byteSize = q.byteSize + item.size
}

// Gets the first batch of items with the same id from queue
func (q *ConcurrentQueue) GetFirstBatch() []list.Element {
	q.lock.Lock()
	defer q.lock.Unlock()
	item := q.list.Front()
	if item == nil {
		return nil
	}
	id := item.Value.(RWRequest).id
	itemList := []list.Element{}

	for item != nil && item.Value.(RWRequest).id == id {
		itemList = append(itemList, *item)
		toDelete := item
		item = item.Next()
		q.list.Remove(toDelete)
		q.byteSize = q.byteSize - toDelete.Value.(RWRequest).size
	}
	return itemList
}

func (q *ConcurrentQueue) IsEmpty() bool {
	return q.list.Len() == 0
}

func (q *ConcurrentQueue) Size() int {
	return q.list.Len()
}

func (q *ConcurrentQueue) CurrentQueueByteSize() int {
	q.lock.Lock()
	defer q.lock.Unlock()
	return q.byteSize
}

type PrometheusRemoteWriteSinkConfig struct {
	BearerToken         string `yaml:"bearer_token"`
	FlushMaxConcurrency int    `yaml:"flush_max_concurrency"`
	FlushMaxPerBody     int    `yaml:"flush_max_per_body"`
	FlushInterval       int    `yaml:"flush_interval"`
	FlushTimeout        int    `yaml:"flush_timeout"`
	BufferQueueSize     int    `yaml:"buffer_queue_size"`
	WriteAddress        string `yaml:"write_address"`
}

// PrometheusRemoteWriteSink is a metric sink for Prometheus via remote write.
type PrometheusRemoteWriteSink struct {
	name        string
	addr        string
	headers     []string
	tags        []string
	logger      *logrus.Entry
	traceClient *trace.Client
	promClient  *http.Client
	flushMaxPerBody,
	flushMaxConcurrency,
	flushInterval,
	flushTimeout,
	bufferQueueSize int
}

func ParseRWMetricConfig(name string, config interface{}) (veneur.MetricSinkConfig, error) {
	promRWConfig := PrometheusRemoteWriteSinkConfig{}
	err := util.DecodeConfig(name, config, &promRWConfig)
	if err != nil {
		return nil, err
	}
	if promRWConfig.FlushMaxPerBody <= 0 {
		promRWConfig.FlushMaxPerBody = 5000
	}
	if promRWConfig.FlushMaxConcurrency <= 0 {
		promRWConfig.FlushMaxConcurrency = 10
	}
	if promRWConfig.FlushInterval <= 0 {
		promRWConfig.FlushInterval = 1000
	}
	if promRWConfig.FlushTimeout <= 0 {
		promRWConfig.FlushTimeout = 35
	}
	if promRWConfig.BufferQueueSize <= 0 {
		// if not (or wrongly) configured, fallback to default
		promRWConfig.BufferQueueSize = 512
	} else if promRWConfig.BufferQueueSize < 64 {
		// we need at least 64 MB
		promRWConfig.BufferQueueSize = 64
	}
	return promRWConfig, nil
}

func CreateRWMetricSink(
	server *veneur.Server, name string, logger *logrus.Entry,
	config veneur.Config, sinkConfig veneur.MetricSinkConfig,
) (sinks.MetricSink, error) {
	conf, ok := sinkConfig.(PrometheusRemoteWriteSinkConfig)
	if !ok {
		return nil, errors.New("invalid sink config type")
	}

	return NewPrometheusRemoteWriteSink(
		conf.WriteAddress, conf.BearerToken, conf.BufferQueueSize,
		conf.FlushMaxPerBody, conf.FlushMaxConcurrency,
		conf.FlushInterval, conf.FlushTimeout,
		config.Hostname, server.Tags, name, logger)
}

// NewPrometheusRemoteWriteSink returns a new RemoteWriteExporter, validating params.
func NewPrometheusRemoteWriteSink(addr string, bearerToken string, bufferQueueSize int, flushMaxPerBody int, flushMaxConcurrency int, flushInterval int, flushTimeout int, hostname string, tags []string, name string, logger *logrus.Entry) (*PrometheusRemoteWriteSink, error) {
	if _, err := url.ParseRequestURI(addr); err != nil {
		return nil, err
	}

	httpClientConfig := config.HTTPClientConfig{BearerToken: config.Secret(bearerToken)}
	httpClient, err := config.NewClientFromConfig(httpClientConfig, "venuerSink", false)
	if err != nil {
		return nil, err
	}

	return &PrometheusRemoteWriteSink{
		name:                name,
		addr:                addr,
		logger:              logger.WithFields(logrus.Fields{"sink_type": "prometheus_rw"}),
		tags:                append(tags, "host:"+hostname),
		promClient:          httpClient,
		flushMaxPerBody:     flushMaxPerBody,
		flushMaxConcurrency: flushMaxConcurrency,
		flushInterval:       flushInterval,
		flushTimeout:        flushTimeout,
		bufferQueueSize:     bufferQueueSize,
	}, nil
}

// Name returns the name of this sink.
func (prw *PrometheusRemoteWriteSink) Name() string {
	return prw.name
}

// Start begins the sink.
func (prw *PrometheusRemoteWriteSink) Start(cl *trace.Client) error {
	prw.traceClient = cl
	// initializing the queue with the correct size, in bytes
	queue.maxByteSize = 1024 * 1024 * prw.bufferQueueSize
	queue.logger = prw.logger
	prw.logger.Debug("Initializing buffer queue with max size of ", queue.maxByteSize, " bytes.")
	// routine reading from buffer queue
	go func() {
		for {
			prw.logger.Debug("Reading from buffer queue, currently there are ", queue.list.Len(), " elements.")
			itemList := queue.GetFirstBatch()
			if itemList != nil {
				prw.AsyncFlush(itemList)
			}
			time.Sleep(time.Duration(prw.flushInterval) * time.Millisecond)
		}
	}()
	return nil
}

// Flush only stores metrics in the buffer queue, AsyncFlush does the actual flushing
func (prw *PrometheusRemoteWriteSink) Flush(ctx context.Context, interMetrics []samplers.InterMetric) error {

	span, _ := trace.StartSpanFromContext(ctx, "")
	defer span.ClientFinish(prw.traceClient)

	promMetrics, promMetadata := prw.finalizeMetrics(interMetrics)

	// break the metrics into chunks of approximately equal size, such that
	// each chunk is less than the limit
	// we compute the chunks using rounding-up integer division
	workers := ((len(promMetrics) - 1) / prw.flushMaxPerBody) + 1
	chunkSize := ((len(promMetrics) - 1) / workers) + 1
	prw.logger.WithField("workers", workers).Debug("Worker count chosen")
	prw.logger.WithField("chunkSize", chunkSize).Debug("Chunk size chosen")

	timestamp := time.Now()
	requestUuid := uuid.New()

	// serializing metadata
	metadataRequest := prompb.WriteRequest{Metadata: promMetadata}
	metaRequest, err := prw.buildRequest(metadataRequest)
	if err != nil {
		return nil // already logged failure
	}
	metaReq := RWRequest{
		request:   metaRequest,
		size:      binary.Size(metaRequest),
		ctx:       ctx,
		timestamp: timestamp,
		id:        requestUuid,
	}
	queue.Enqueue(span, metaReq)

	// serializing data
	for i := 0; i < workers; i++ {
		chunk := promMetrics[i*chunkSize:]
		if i < workers-1 {
			// trim to chunk size unless this is the last one
			chunk = chunk[:chunkSize]
		}
		writeRequest := prompb.WriteRequest{Timeseries: chunk}
		byteRequest, err := prw.buildRequest(writeRequest)
		if err != nil {
			return nil // already logged failure
		}
		rwReq := RWRequest{
			request:      byteRequest,
			size:         binary.Size(byteRequest),
			totalMetrics: len(chunk),
			ctx:          ctx,
			timestamp:    timestamp,
			id:           requestUuid,
		}
		queue.Enqueue(span, rwReq)
	}

	return nil
}

// Sends metrics to Remote Write
func (prw *PrometheusRemoteWriteSink) AsyncFlush(items []list.Element) error {
	span, _ := trace.StartSpanFromContext(items[0].Value.(RWRequest).ctx, "")
	defer span.ClientFinish(prw.traceClient)

	var wg sync.WaitGroup
	flushStart := time.Now()

	// a blocking channel to keep concurrency under control
	semaphoreChan := make(chan struct{}, prw.flushMaxConcurrency)
	defer close(semaphoreChan)

	requestsOutcome := make([]bool, len(items))
	queuedRequest := func(request list.Element, idx int) {
		wg.Add(1)
		if prw.flushMaxConcurrency > 0 {
			// block until the semaphore channel has room
			semaphoreChan <- struct{}{}
		}

		go func() {
			defer func() {
				if prw.flushMaxConcurrency > 0 {
					// clear a spot in the semaphore channel
					<-semaphoreChan
				}
			}()
			requestsOutcome[idx] = prw.flushRequest(span, span.Attach(items[0].Value.(RWRequest).ctx), request, &wg)
		}()

	}

	for i, v := range items {
		queuedRequest(v, i)
	}

	wg.Wait()

	// flushing all values (the first one of the slice should be metadata) (TODO: check if metadata enabled..)
	// also calculating total number of metrics for stats
	totalSentMetrics := 0
	totalFailedMetrics := 0
	for i, v := range items {
		if requestsOutcome[i] {
			totalSentMetrics += v.Value.(RWRequest).totalMetrics
		} else {
			totalFailedMetrics += v.Value.(RWRequest).totalMetrics
		}
	}

	tags := map[string]string{"sink": prw.Name()}
	span.Add(
		ssf.Timing(sinks.MetricKeyMetricFlushDuration, time.Since(flushStart), time.Nanosecond, tags),
		ssf.Count(sinks.MetricKeyTotalMetricsFlushed, float32(totalSentMetrics), tags),
		ssf.Gauge(metricRwSinkQueueSize, float32(queue.CurrentQueueByteSize()), noTags),
		ssf.Gauge(metricRwSinkMaxQueueSize, float32(queue.maxByteSize), noTags),
	)
	prw.logger.WithField("sentMetrics", totalSentMetrics).WithField("notSentMetric", totalFailedMetrics).Info("Completed flush operation to Prometheus Remote Write")
	return nil
}

// FlushOtherSamples sends events to SignalFx. This is a no-op for Prometheus
// sinks as Prometheus does not support other samples.
func (prw *PrometheusRemoteWriteSink) FlushOtherSamples(ctx context.Context, samples []ssf.SSFSample) {
}

func (prw *PrometheusRemoteWriteSink) finalizeMetrics(metrics []samplers.InterMetric) ([]prompb.TimeSeries, []prompb.MetricMetadata) {
	promMetrics := make([]prompb.TimeSeries, 0, len(metrics))
	metadataStore := make(map[string]samplers.MetricType, 100)

	for _, m := range metrics {
		if !sinks.IsAcceptableMetric(m, prw) {
			continue
		}

		mappedName := mapper.EscapeMetricName(m.Name)
		mtype, ok := metadataStore[mappedName]
		if !ok || mtype != m.Type {
			metadataStore[mappedName] = m.Type
		}
		if ok && mtype != m.Type {
			prw.logger.Warnf("Found inconsistent type for metric %s; %s vs %s", mappedName, mtype, m.Type)
		}

		seenKeys := make(map[string]struct{}, len(m.Tags)+1)
		SEEN := struct{}{} // sentinel value for set

		promLabels := make([]prompb.Label, 0, len(m.Tags)+1)
		promLabels = append(promLabels, prompb.Label{Name: "__name__", Value: mappedName})
		seenKeys["__name__"] = SEEN

		allTags := make([]string, 0, len(m.Tags)+len(prw.tags))
		allTags = append(allTags, m.Tags...)
		allTags = append(allTags, prw.tags...)
		for _, tag := range allTags {
			var key, value string
			if strings.Contains(tag, ":") {
				keyvalpair := strings.SplitN(tag, ":", 2)
				key, value = mapper.EscapeMetricName(keyvalpair[0]), keyvalpair[1]
			} else {
				key, value = mapper.EscapeMetricName(tag), "true"
			}

			if _, ok := seenKeys[key]; ok {
				prw.logger.Warnf("Dropping label %s: %s for metric %s; duplicate key", key, value, m.Name)
				continue
			}
			seenKeys[key] = SEEN
			promLabels = append(promLabels, prompb.Label{Name: key, Value: value})
		}

		promMetrics = append(promMetrics, prompb.TimeSeries{
			Labels:  promLabels,
			Samples: []prompb.Sample{{Timestamp: m.Timestamp * time.Second.Nanoseconds() / 1e6, Value: m.Value}},
		})
	}

	promMetadata := make([]prompb.MetricMetadata, 0, len(metadataStore))
	unknownTypeMetrics := 0
	for metricName, metricType := range metadataStore {
		pm := prompb.MetricMetadata{MetricFamilyName: metricName}
		switch metricType {
		case samplers.CounterMetric:
			pm.Type = prompb.MetricMetadata_DELTA_COUNTER
		case samplers.GaugeMetric:
			pm.Type = prompb.MetricMetadata_GAUGE
		default:
			unknownTypeMetrics++
			continue // skip unknown types
		}
		promMetadata = append(promMetadata, pm)
	}
	if unknownTypeMetrics > 0 {
		prw.logger.Warnf("Ignored metadata for %d metrics with unsupported type", unknownTypeMetrics)
	}
	return promMetrics, promMetadata
}

func (prw *PrometheusRemoteWriteSink) flushRequest(span *trace.Span, ctx context.Context, item list.Element, wg *sync.WaitGroup) bool {
	defer wg.Done()

	httpStatusCode, _, err := prw.store(ctx, item.Value.(RWRequest).request)
	if err != nil {
		if httpStatusCode != -1 {
			prw.logger.Warn("Failed sending request with timestamp ", item.Value.(RWRequest).timestamp, " to PRWS, status code ", httpStatusCode)
			tags := map[string]string{"status_code": strconv.Itoa(httpStatusCode)}
			span.Add(ssf.Count(metricRwSinkFailedPrwsRequestsTotal, 1, tags))
		}
		_, recoverable := err.(recoverableError)
		if recoverable {
			// put it back at the top of the queue
			queue.EnqueueFront(span, item.Value.(RWRequest))
		}
		return false
	}
	return true
}

func (prw *PrometheusRemoteWriteSink) buildRequest(request prompb.WriteRequest) (req []byte, err error) {
	var reqBuf []byte
	if reqBuf, err = proto.Marshal(&request); err != nil {
		prw.logger.Errorf("failed to marshal the WriteRequest %v", err)
		return nil, err
	}

	compressed := snappy.Encode(nil, reqBuf)
	if err != nil {
		prw.logger.Errorf("failed to compress the WriteRequest %v", err)
		return nil, err
	}
	return compressed, nil
}

// used to signify that the error from store is worth retry-ing
type recoverableError struct {
	error
}

// storeRequest sends a marshalled batch of samples to the HTTP endpoint
// returns statuscode or -1 if the request didn't get to the server
// response body is returned as []byte
func (prw *PrometheusRemoteWriteSink) store(ctx context.Context, req []byte) (int, []byte, error) {
	httpReq, err := http.NewRequest("POST", prw.addr, bytes.NewReader(req))
	if err != nil {
		return -1, nil, err
	}
	httpReq.Header.Add("Content-Encoding", "snappy")
	httpReq.Header.Set("Content-Type", "application/x-protobuf")
	httpReq.Header.Set("User-Agent", fmt.Sprintf("Venuer Prometheus RW sink"))
	httpReq.Header.Set("Sysdig-Custom-Metric-Category", "PROMETHEUS_NON_COMPLIANT")

	newCtx, cancel := context.WithTimeout(context.Background(), time.Duration(prw.flushTimeout)*time.Second)

	defer cancel()

	httpResp, err := prw.promClient.Do(httpReq.WithContext(newCtx))
	if err != nil {
		return -1, nil, recoverableError{err}
	}
	defer func() {
		io.Copy(ioutil.Discard, httpResp.Body)
		httpResp.Body.Close()
	}()

	scanner := bufio.NewScanner(io.LimitReader(httpResp.Body, 2048 /*maxErrMsgLen*/))
	var responseBody []byte
	if scanner.Scan() {
		responseBody = scanner.Bytes()
	}

	if httpResp.StatusCode/100 != 2 {
		err = errors.Errorf("server returned HTTP status %s: %s", httpResp.Status, string(responseBody))
	}
	if httpResp.StatusCode/100 == 5 || httpResp.StatusCode == 429 {
		return httpResp.StatusCode, responseBody, recoverableError{err}
	}
	if httpResp.StatusCode/100 == 2 {
		prw.logger.Debug("Successfully sent metrics to Remote Write")
	}
	return httpResp.StatusCode, responseBody, err
}

func MigrateRWConfig(conf *veneur.Config) {
	if conf.PrometheusRemoteWriteAddress == "" {
		return
	}

	conf.MetricSinks = append(conf.MetricSinks, veneur.SinkConfig{
		Kind: "prometheus_rw",
		Name: "prometheus_rw",
		Config: PrometheusRemoteWriteSinkConfig{
			WriteAddress:        conf.PrometheusRemoteWriteAddress,
			FlushMaxConcurrency: conf.PrometheusRemoteFlushMaxConcurrency,
			FlushMaxPerBody:     conf.PrometheusRemoteFlushMaxPerBody,
			BearerToken:         conf.PrometheusRemoteBearerToken,
			FlushInterval:       conf.PrometheusRemoteFlushInterval,
			FlushTimeout:        conf.PrometheusRemoteFlushTimeout,
			BufferQueueSize:     conf.PrometheusRemoteBufferQueueSize,
		},
	})
}
