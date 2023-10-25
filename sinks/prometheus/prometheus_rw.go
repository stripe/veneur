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

	"github.com/prometheus/common/config"
	"github.com/sirupsen/logrus"
)

var queue = NewConcurrentQueue()

type RWRequest struct {
	metrics   [][]byte
	metadata  []byte
	ctx       context.Context
	size      int
	timestamp time.Time
}

// queue to avoid discarding metrics when the sink is delayed in sending data by momentary problems happening to Remote Write
type ConcurrentQueue struct {
	list        *list.List
	lock        sync.Mutex
	byteSize    int
	maxByteSize int
}

func NewConcurrentQueue() *ConcurrentQueue {
	q := &ConcurrentQueue{
		list:        list.New(),
		maxByteSize: 1000, //TODO settings
	}
	return q
}

// Put the item in the back of the queue
func (q *ConcurrentQueue) Enqueue(item RWRequest) {
	q.lock.Lock()
	defer q.lock.Unlock()

	newItemSize := item.size

	// if there's no space left, make it
	if q.byteSize+newItemSize > q.maxByteSize {
		q.MakeSpace(newItemSize)
	}

	q.list.PushBack(item)
	q.byteSize = q.byteSize + item.size
}

// Put the item in the front of the queue
func (q *ConcurrentQueue) EnqueueFront(item RWRequest) {
	q.lock.Lock()
	defer q.lock.Unlock()

	newItemSize := item.size

	//if there's no space left, don't add the item
	if q.byteSize+newItemSize > q.maxByteSize {
		return
	}

	q.list.PushFront(item)
	q.byteSize = q.byteSize + item.size
}

// Gets the first item from queue, not deleting it
func (q *ConcurrentQueue) GetFirst() *list.Element {
	q.lock.Lock()
	defer q.lock.Unlock()
	item := q.list.Front()
	return item
}

func (q *ConcurrentQueue) IsEmpty() bool {
	return q.list.Len() == 0
}

func (q *ConcurrentQueue) Size() int {
	return q.list.Len()
}

func (q *ConcurrentQueue) MakeSpace(newItemSize int) {
	q.lock.Lock()
	defer q.lock.Unlock()
	item := q.list.Front()
	for q.byteSize+newItemSize > q.maxByteSize {
		q.list.Remove(item)
		q.byteSize = q.byteSize - item.Value.(RWRequest).size
		item = item.Next()
	}
}

func (q *ConcurrentQueue) DeleteItem(item *list.Element) {
	q.lock.Lock()
	defer q.lock.Unlock()
	q.list.Remove(item)
	q.byteSize = q.byteSize - item.Value.(RWRequest).size
}

type PrometheusRemoteWriteSinkConfig struct {
	BearerToken         string `yaml:"bearer_token"`
	FlushMaxConcurrency int    `yaml:"flush_max_concurrency"`
	FlushMaxPerBody     int    `yaml:"flush_max_per_body"`
	FlushRetries        int    `yaml:"flush_retries"`
	FlushInitialBackoff int    `yaml:"flush_retry_initial_backoff"`
	FlushTimeout        int    `yaml:"flush_timeout"`
	AcceptanceWindow    int    `yaml:"acceptance_window"`
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
	flushRetries,
	flushInitialBackoff,
	flushTimeout,
	acceptanceWindow int
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
	if promRWConfig.FlushRetries <= 0 {
		promRWConfig.FlushRetries = 5
	}
	if promRWConfig.FlushInitialBackoff <= 0 {
		promRWConfig.FlushInitialBackoff = 500
	}
	if promRWConfig.FlushTimeout <= 0 {
		promRWConfig.FlushTimeout = 35
	}
	if promRWConfig.AcceptanceWindow <= 0 {
		promRWConfig.AcceptanceWindow = 5
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
		conf.WriteAddress, conf.BearerToken,
		conf.FlushMaxPerBody, conf.FlushMaxConcurrency,
		conf.FlushRetries, conf.FlushInitialBackoff,
		conf.FlushTimeout, conf.AcceptanceWindow,
		config.Hostname, server.Tags, name, logger)
}

// NewPrometheusRemoteWriteSink returns a new RemoteWriteExporter, validating params.
func NewPrometheusRemoteWriteSink(addr string, bearerToken string, flushMaxPerBody int, flushMaxConcurrency int, flushRetries int, flushInitialBackoff int, flushTimeout int, acceptanceWindow int, hostname string, tags []string, name string, logger *logrus.Entry) (*PrometheusRemoteWriteSink, error) {
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
		flushRetries:        flushRetries,
		flushInitialBackoff: flushInitialBackoff,
		flushTimeout:        flushTimeout,
		acceptanceWindow:    acceptanceWindow,
	}, nil
}

// Name returns the name of this sink.
func (prw *PrometheusRemoteWriteSink) Name() string {
	return prw.name
}

// Start begins the sink.
func (prw *PrometheusRemoteWriteSink) Start(cl *trace.Client) error {
	prw.traceClient = cl
	// routine reading from buffer queue
	go func() {
		for {
			prw.logger.Debug("Reading from buffer queue")
			item := queue.GetFirst()
			if item != nil {
				prw.logger.Debug("Found element in buffer queue with timestamp: ", item.Value.(RWRequest).timestamp)
				prw.AsyncFlush(item)
			}
			time.Sleep(1000 * time.Millisecond)
		}
	}()
	return nil
}

// Flush only stores metrics in the buffer queue, AsyncFlush does the actual flushing
func (prw *PrometheusRemoteWriteSink) Flush(ctx context.Context, interMetrics []samplers.InterMetric) error {

	promMetrics, promMetadata := prw.finalizeMetrics(interMetrics)

	// break the metrics into chunks of approximately equal size, such that
	// each chunk is less than the limit
	// we compute the chunks using rounding-up integer division
	workers := ((len(promMetrics) - 1) / prw.flushMaxPerBody) + 1
	chunkSize := ((len(promMetrics) - 1) / workers) + 1
	prw.logger.WithField("workers", workers).Debug("Worker count chosen")
	prw.logger.WithField("chunkSize", chunkSize).Debug("Chunk size chosen")

	// serializing data
	dataRequest := [][]byte{}
	totalByteSize := 0
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
		totalByteSize = totalByteSize + binary.Size(byteRequest)
		dataRequest = append(dataRequest, byteRequest)
	}
	// serializing metadata
	metadataRequest := prompb.WriteRequest{Metadata: promMetadata}
	metaRequest, err := prw.buildRequest(metadataRequest)
	if err != nil {
		return nil // already logged failure
	}
	totalByteSize = totalByteSize + binary.Size(metaRequest)

	rwReq := RWRequest{
		metrics:   dataRequest,
		metadata:  metaRequest,
		size:      totalByteSize,
		ctx:       ctx,
		timestamp: time.Now(),
	}
	queue.Enqueue(rwReq)
	prw.logger.Debug("RW Sink buffer queue size: ", queue.Size())

	return nil
}

// Sends metrics to Remote Write
func (prw *PrometheusRemoteWriteSink) AsyncFlush(item *list.Element) error {
	span, _ := trace.StartSpanFromContext(item.Value.(RWRequest).ctx, "")
	defer span.ClientFinish(prw.traceClient)

	var wg sync.WaitGroup
	flushStart := time.Now()

	// a blocking channel to keep concurrency under control
	semaphoreChan := make(chan struct{}, prw.flushMaxConcurrency)
	defer close(semaphoreChan)

	queuedRequest := func(request []byte) {
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
			prw.flushRequest(span.Attach(item.Value.(RWRequest).ctx), request, &wg, item.Value.(RWRequest).timestamp)
		}()

	}

	// deleting original request from queue
	prw.logger.Debug("Removing item from queue before trying to send it")
	queue.DeleteItem(item)
	prw.logger.Debug("Buffer queue length after deletion = ", queue.Size())
	prw.logger.Debug("Buffer queue byte size after deletion = ", queue.byteSize)

	// first flush metadata (TODO: not every time, check if metadata enabled..)
	if item.Value.(RWRequest).metadata != nil {
		prw.logger.Debug("Sending metadata")
		queuedRequest(item.Value.(RWRequest).metadata)
	}
	// then all data chunks
	for i, el := range item.Value.(RWRequest).metrics {
		prw.logger.Debug("Sending data chunk number ", i)
		queuedRequest(el)
	}

	wg.Wait()
	tags := map[string]string{"sink": prw.Name()}
	span.Add(
		ssf.Timing(sinks.MetricKeyMetricFlushDuration, time.Since(flushStart), time.Nanosecond, tags),
		ssf.Count(sinks.MetricKeyTotalMetricsFlushed, float32(len(item.Value.(RWRequest).metrics)), tags),
	)
	prw.logger.WithField("metrics", len(item.Value.(RWRequest).metrics)).Info("Completed flush to Prometheus Remote Write")
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

func (prw *PrometheusRemoteWriteSink) flushRequest(ctx context.Context, request []byte, wg *sync.WaitGroup, timestamp time.Time) {
	defer wg.Done()

	_, _, err := prw.store(ctx, request)
	if err != nil {
		_, recoverable := err.(recoverableError)
		if recoverable {
			// put it back at the top of the queue, as a single request (ignoring difference between data and metadata)
			dataRequest := [][]byte{}
			dataRequest = append(dataRequest, request)

			singleReq := RWRequest{
				metrics:   dataRequest,
				size:      binary.Size(request),
				ctx:       ctx,
				timestamp: timestamp,
			}
			queue.EnqueueFront(singleReq)
			prw.logger.Debug("Enqueued failed request, queue length = ", queue.Size())
			prw.logger.Debug("Buffer queue byte size = ", queue.byteSize)
			return
		}
		// not recoverable
		prw.logger.Errorf("Failed: %v, not retryable", err.Error())
	}
	return
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
			FlushRetries:        conf.PrometheusRemoteFlushRetries,
			FlushInitialBackoff: conf.PrometheusRemoteFlushRetryInitialBackoff,
			FlushTimeout:        conf.PrometheusRemoteFlushTimeout,
			AcceptanceWindow:    conf.PrometheusRemoteAcceptanceWindow,
		},
	})
}
