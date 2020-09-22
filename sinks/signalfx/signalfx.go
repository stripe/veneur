package signalfx

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/pkg/errors"
	"github.com/signalfx/golib/datapoint"
	"github.com/signalfx/golib/datapoint/dpsink"
	"github.com/signalfx/golib/event"
	"github.com/signalfx/golib/sfxclient"
	"github.com/sirupsen/logrus"
	"github.com/stripe/veneur/protocol/dogstatsd"
	"github.com/stripe/veneur/samplers"
	"github.com/stripe/veneur/sinks"
	"github.com/stripe/veneur/ssf"
	"github.com/stripe/veneur/trace"
)

const EventNameMaxLength = 256
const EventDescriptionMaxLength = 256

var datapointURL *url.URL
var eventURL *url.URL

const (
	datapointAddr string = "/v2/datapoint"
	eventAddr     string = "/v2/event"
)

func init() {
	var err error
	datapointURL, err = url.Parse(datapointAddr)
	if err != nil {
		panic(fmt.Sprintf("Could not parse %q: %v", datapointAddr, err))
	}
	eventURL, err = url.Parse(eventAddr)
	if err != nil {
		panic(fmt.Sprintf("Could not parse %q: %v", eventAddr, err))
	}
}

// collection is a structure that aggregates signalfx data points
// per-endpoint. It takes care of collecting the metrics by the tag
// values that identify where to send them, and
type collection struct {
	sink        *SignalFxSink
	points      []*datapoint.Datapoint
	pointsByKey map[string][]*datapoint.Datapoint
}

func (c *collection) addPoint(ctx context.Context, key string, point *datapoint.Datapoint) {
	span, _ := trace.StartSpanFromContext(ctx, "")
	defer span.ClientFinish(c.sink.traceClient)

	c.sink.clientsByTagValueMu.RLock()
	defer c.sink.clientsByTagValueMu.RUnlock()

	if c.sink.clientsByTagValue != nil {
		if _, ok := c.sink.clientsByTagValue[key]; ok {
			c.pointsByKey[key] = append(c.pointsByKey[key], point)
			return
		}
		span.Add(ssf.Count("flush.fallback_client_points_flushed", 1, map[string]string{"vary_by": c.sink.varyBy, "key": key, "sink": "signalfx", "veneurglobalonly": "true"}))
	}
	c.points = append(c.points, point)
}

func submitDatapoints(ctx context.Context, wg *sync.WaitGroup, cl *trace.Client, client dpsink.Sink, points []*datapoint.Datapoint, errs chan<- error) {
	defer wg.Done()

	span, ctx := trace.StartSpanFromContext(ctx, "")
	span.SetTag("datapoint_count", len(points))
	defer span.ClientFinish(cl)

	err := client.AddDatapoints(ctx, points)
	if err != nil {
		span.Error(err)
		span.Add(ssf.Count("flush.error_total", 1, map[string]string{"cause": "io", "sink": "signalfx"}))
	}
	errs <- err
}

func (c *collection) submit(ctx context.Context, cl *trace.Client, maxPerFlush int) error {
	span, ctx := trace.StartSpanFromContext(ctx, "")
	defer span.ClientFinish(cl)

	wg := &sync.WaitGroup{}
	resultCh := make(chan error)
	errorCh := make(chan error) // the consolidated error we'll return from submit

	go func() {
		errors := []error{}
		for err := range resultCh {
			if err != nil {
				span.Add(ssf.Count("flush.error_total", 1, map[string]string{"cause": "io", "sink": "signalfx"}))
				errors = append(errors, err)
			}
		}
		if len(errors) > 0 {
			errorCh <- fmt.Errorf("unable to submit to all endpoints: %v", errors)
		}
		errorCh <- nil
	}()

	submitBatch := func(client dpsink.Sink, points []*datapoint.Datapoint) {
		perFlush := maxPerFlush
		if perFlush == 0 {
			perFlush = len(points)
		}
		for i := 0; i < len(points); i += perFlush {
			end := i + perFlush
			if end > len(points) {
				end = len(points)
			}
			wg.Add(1)
			go submitDatapoints(ctx, wg, cl, client, points[i:end], resultCh)
		}
	}

	submitBatch(c.sink.defaultClient, c.points)
	for key, points := range c.pointsByKey {
		submitBatch(c.sink.client(key), points)
	}
	wg.Wait()

	close(resultCh)
	err := <-errorCh
	if err != nil {
		span.Error(err)
	}
	return err
}

// SignalFxSink is a MetricsSink implementation.
type SignalFxSink struct {
	defaultClient             DPClient
	clientsByTagValue         map[string]DPClient
	clientsByTagValueMu       *sync.RWMutex
	enableDynamicPerTagTokens bool
	defaultToken              string
	dynamicKeyRefreshPeriod   time.Duration
	keyClients                map[string]dpsink.Sink
	varyBy                    string
	hostnameTag               string
	hostname                  string
	commonDimensions          map[string]string
	log                       *logrus.Logger
	traceClient               *trace.Client
	excludedTags              map[string]struct{}
	metricNamePrefixDrops     []string
	metricTagPrefixDrops      []string
	derivedMetrics            samplers.DerivedMetricsProcessor
	maxPointsInBatch          int
	metricsEndpoint           string
	apiEndpoint               string
	httpClient                *http.Client
}

// A DPClient is a client that can be used to submit signalfx data
// points to an upstream consumer. It wraps the dpsink.Sink interface.
type DPClient dpsink.Sink

// NewClient constructs a new signalfx HTTP client for the given
// endpoint and API token.
func NewClient(endpoint, apiKey string, client *http.Client) DPClient {
	baseURL, err := url.Parse(endpoint)
	if err != nil {
		panic(fmt.Sprintf("Could not parse endpoint base URL %q: %v", endpoint, err))
	}
	httpSink := sfxclient.NewHTTPSink()
	httpSink.AuthToken = apiKey
	httpSink.DatapointEndpoint = baseURL.ResolveReference(datapointURL).String()
	httpSink.EventEndpoint = baseURL.ResolveReference(eventURL).String()
	httpSink.Client = client
	return httpSink
}

// NewSignalFxSink creates a new SignalFx sink for metrics.
func NewSignalFxSink(hostnameTag string, hostname string, commonDimensions map[string]string, log *logrus.Logger, client DPClient, varyBy string, perTagClients map[string]DPClient, metricNamePrefixDrops []string, metricTagPrefixDrops []string, derivedMetrics samplers.DerivedMetricsProcessor, maxPointsInBatch int, defaultToken string, enableDynamicPerTagTokens bool, dynamicKeyRefreshPeriod time.Duration, metricsEndpoint string, apiEndpoint string, httpClient *http.Client) (*SignalFxSink, error) {
	var endpointStr string
	if apiEndpoint != "" {
		endpoint, err := url.Parse(apiEndpoint)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to parse signalfx api endpoint")
		}

		endpoint, err = endpoint.Parse("/v2/token")
		if err != nil {
			return nil, errors.Wrapf(err, "failed to generate signalfx token endpoint")
		}
		endpointStr = endpoint.String()
	}

	return &SignalFxSink{
		defaultClient:             client,
		clientsByTagValueMu:       &sync.RWMutex{},
		clientsByTagValue:         perTagClients,
		enableDynamicPerTagTokens: enableDynamicPerTagTokens,
		defaultToken:              defaultToken,
		dynamicKeyRefreshPeriod:   dynamicKeyRefreshPeriod,
		hostnameTag:               hostnameTag,
		hostname:                  hostname,
		commonDimensions:          commonDimensions,
		log:                       log,
		varyBy:                    varyBy,
		metricNamePrefixDrops:     metricNamePrefixDrops,
		metricTagPrefixDrops:      metricTagPrefixDrops,
		derivedMetrics:            derivedMetrics,
		maxPointsInBatch:          maxPointsInBatch,
		metricsEndpoint:           metricsEndpoint,
		apiEndpoint:               endpointStr,
		httpClient:                httpClient,
	}, nil
}

// Name returns the name of this sink.
func (sfx *SignalFxSink) Name() string {
	return "signalfx"
}

// Start begins the sink. For SignalFx this starts the clientByTagUpdater
func (sfx *SignalFxSink) Start(traceClient *trace.Client) error {
	sfx.traceClient = traceClient
	go sfx.clientByTagUpdater()

	return nil
}

// client returns a client that can be used to submit to vary-by tag's
// value. If no client is specified for that tag value, the default
// client is returned.
func (sfx *SignalFxSink) client(key string) DPClient {
	sfx.clientsByTagValueMu.RLock()
	defer sfx.clientsByTagValueMu.RUnlock()

	if cl, ok := sfx.clientsByTagValue[key]; ok {
		return cl
	}
	return sfx.defaultClient
}

func (sfx *SignalFxSink) clientByTagUpdater() {
	if !sfx.enableDynamicPerTagTokens {
		return
	}

	ticker := time.NewTicker(sfx.dynamicKeyRefreshPeriod)
	for range ticker.C {
		tokens, err := fetchAPIKeys(sfx.httpClient, sfx.apiEndpoint, sfx.defaultToken)
		if err != nil {
			sfx.log.WithError(err).Warn("Failed to fetch new tokens from SignalFX")
			continue
		}

		for name, token := range tokens {
			sfx.clientsByTagValueMu.Lock()
			sfx.clientsByTagValue[name] = NewClient(sfx.metricsEndpoint, token, sfx.httpClient)
			sfx.clientsByTagValueMu.Unlock()
		}
		sfx.log.Debugf("Fetched %d tokens in total", len(tokens))
	}
}

const (
	offsetQueryParam = "offset"
	limitQueryParam  = "limit"
	nameQueryParam   = "name"

	limitQueryValue = 200
)

func getTokensApiResponseFromOffset(client *http.Client, endpoint, apiToken string, offset int) (*bytes.Buffer, error) {
	b := &bytes.Buffer{}

	req, err := http.NewRequest(http.MethodGet, endpoint, bytes.NewReader(b.Bytes()))
	if err != nil {
		return nil, err
	}

	req.Header.Set(sfxclient.TokenHeaderName, apiToken)
	req.Header.Set("Content-Type", "application/json")

	q := req.URL.Query()
	q.Add(limitQueryParam, fmt.Sprint(limitQueryValue))
	q.Add(nameQueryParam, "")

	q.Del(offsetQueryParam)
	q.Add(offsetQueryParam, fmt.Sprint(offset))

	req.URL.RawQuery = q.Encode()

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()

	// The API always returns OK, even if it doesn't have any tokens to return
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("signalfx api returned unknown response code: %s", resp.Status)
	}

	body := &bytes.Buffer{}
	_, err = body.ReadFrom(resp.Body)

	if err != nil {
		return nil, err
	}
	return body, nil
}

func fetchAPIKeys(client *http.Client, endpoint, apiToken string) (map[string]string, error) {
	allFetched := false
	offset := 0

	apiTokensByName := make(map[string]string)

	for !allFetched {
		body, err := getTokensApiResponseFromOffset(client, endpoint, apiToken, offset)
		if err != nil {
			return nil, err
		}
		count, err := extractTokensFromResponse(apiTokensByName, body)
		if err != nil {
			return nil, err
		}

		allFetched = count == 0
		offset += limitQueryValue
	}

	return apiTokensByName, nil
}

func extractTokensFromResponse(tokensByName map[string]string, body *bytes.Buffer) (count int, err error) {
	var response = make(map[string]interface{})
	err = json.Unmarshal(body.Bytes(), &response)
	if err != nil {
		return count, err
	}

	results, ok := response["results"].([]interface{})
	if !ok {
		return count, fmt.Errorf("unknown results structure returned from signalfx api")
	}

	for _, object := range results {
		result, ok := object.(map[string]interface{})
		if !ok {
			return count, fmt.Errorf("unknown result structure returned from signalfx api")
		}

		name, ok := result["name"].(string)
		if !ok {
			return count, fmt.Errorf("failed to extract name from result")
		}

		apiKey, ok := result["secret"].(string)
		if !ok {
			return count, fmt.Errorf("failed to extract api key from result")
		}

		tokensByName[name] = apiKey

		count++
	}

	return count, nil
}

// newPointCollection creates an empty collection object and returns it
func (sfx *SignalFxSink) newPointCollection() *collection {
	return &collection{
		sink:        sfx,
		points:      []*datapoint.Datapoint{},
		pointsByKey: map[string][]*datapoint.Datapoint{},
	}
}

// Flush sends metrics to SignalFx
func (sfx *SignalFxSink) Flush(ctx context.Context, interMetrics []samplers.InterMetric) error {
	span, subCtx := trace.StartSpanFromContext(ctx, "")
	defer span.ClientFinish(sfx.traceClient)

	flushStart := time.Now()
	coll := sfx.newPointCollection()
	numPoints := 0
	countSkipped := 0
	countStatusMetrics := 0

METRICLOOP: // Convenience label so that inner nested loops and `continue` easily
	for _, metric := range interMetrics {
		if !sinks.IsAcceptableMetric(metric, sfx) {
			countSkipped++
			continue
		}
		if len(sfx.metricNamePrefixDrops) > 0 {
			for _, pre := range sfx.metricNamePrefixDrops {
				if strings.HasPrefix(metric.Name, pre) {
					countSkipped++
					continue METRICLOOP
				}
			}
		}
		if len(sfx.metricTagPrefixDrops) > 0 {
			for _, dropTag := range sfx.metricTagPrefixDrops {
				for _, tag := range metric.Tags {
					if strings.HasPrefix(tag, dropTag) {
						countSkipped++
						continue METRICLOOP
					}
				}
			}
		}
		dims := map[string]string{}
		// Set the hostname as a tag, since SFx doesn't have a first-class hostname field
		dims[sfx.hostnameTag] = sfx.hostname
		for _, tag := range metric.Tags {
			kv := strings.SplitN(tag, ":", 2)
			key := kv[0]

			if len(kv) == 1 {
				dims[key] = ""
			} else {
				dims[key] = kv[1]
			}
		}

		// metric-specified API key, if present, should override the common dimension
		metricKey := ""
		metricVaryByOverride := false

		if sfx.varyBy != "" {
			if val, ok := dims[sfx.varyBy]; ok {
				metricKey = val
				metricVaryByOverride = true
			}
		}

		// Copy common dimensions
		for k, v := range sfx.commonDimensions {
			dims[k] = v
		}

		// re-copy metric-specified API key, if present
		if metricVaryByOverride {
			dims[sfx.varyBy] = metricKey
		}

		for k := range sfx.excludedTags {
			delete(dims, k)
		}
		delete(dims, "veneursinkonly")

		var point *datapoint.Datapoint
		switch metric.Type {
		case samplers.GaugeMetric:
			point = sfxclient.GaugeF(metric.Name, dims, metric.Value)
		case samplers.CounterMetric:
			// TODO I am not certain if this should be a Counter or a Cumulative
			point = sfxclient.Counter(metric.Name, dims, int64(metric.Value))
		case samplers.StatusMetric:
			countStatusMetrics++
			point = sfxclient.GaugeF(metric.Name, dims, metric.Value)
		}
		coll.addPoint(subCtx, metricKey, point)
		numPoints++
	}
	tags := map[string]string{"sink": "signalfx"}
	span.Add(ssf.Count(sinks.MetricKeyTotalMetricsSkipped, float32(countSkipped), tags))
	err := coll.submit(subCtx, sfx.traceClient, sfx.maxPointsInBatch)
	if err != nil {
		span.Error(err)
	}
	span.Add(ssf.Timing(sinks.MetricKeyMetricFlushDuration, time.Since(flushStart), time.Nanosecond, tags))
	span.Add(ssf.Count(sinks.MetricKeyTotalMetricsFlushed, float32(numPoints), tags))
	sfx.log.WithFields(logrus.Fields{
		"metrics": len(interMetrics),
		"success": err == nil,
	}).Info("Completed flush to SignalFx")

	return err
}

var successSpanTags = map[string]string{"sink": "signalfx", "results": "success"}
var failureSpanTags = map[string]string{"sink": "signalfx", "results": "failure"}

// FlushOtherSamples sends events to SignalFx. Event type samples will be serialized as SFX
// Events directly. All other metric types are ignored
func (sfx *SignalFxSink) FlushOtherSamples(ctx context.Context, samples []ssf.SSFSample) {
	span, _ := trace.StartSpanFromContext(ctx, "")
	defer span.ClientFinish(sfx.traceClient)
	var countFailed = 0
	var countSuccess = 0
	for _, sample := range samples {
		if _, ok := sample.Tags[dogstatsd.EventIdentifierKey]; ok {
			err := sfx.reportEvent(ctx, &sample)
			if err != nil {
				countFailed++
			} else {
				countSuccess++
			}
		}
	}
	if countSuccess > 0 {
		span.Add(ssf.Count(sinks.EventReportedCount, float32(countSuccess), successSpanTags))
	}
	if countFailed > 0 {
		span.Add(ssf.Count(sinks.EventReportedCount, float32(countFailed), failureSpanTags))
	}
}

// SetExcludedTags sets the excluded tag names. Any tags with the
// provided key (name) will be excluded.
func (sfx *SignalFxSink) SetExcludedTags(excludes []string) {

	tagsSet := map[string]struct{}{}
	for _, tag := range excludes {
		tagsSet[tag] = struct{}{}
	}
	sfx.excludedTags = tagsSet
}

type ddSampleKind int

const (
	ddSampleUnknown ddSampleKind = iota
	ddSampleEvent
	ddSampleServiceCheck
)

func (sfx *SignalFxSink) reportEvent(ctx context.Context, sample *ssf.SSFSample) error {
	// Copy common dimensions in
	dims := map[string]string{}
	for k, v := range sfx.commonDimensions {
		dims[k] = v
	}
	// And hostname
	dims[sfx.hostnameTag] = sfx.hostname

	for k, v := range sample.Tags {
		if k == dogstatsd.EventIdentifierKey {
			// Don't copy this tag
			continue
		}
		dims[k] = v
	}

	for k := range sfx.excludedTags {
		delete(dims, k)
	}
	name := sample.Name
	if len(name) > EventNameMaxLength {
		name = name[0:EventNameMaxLength]
	}
	message := sample.Message
	if len(message) > EventDescriptionMaxLength {
		message = message[0:EventDescriptionMaxLength]
	}
	// Datadog requires some weird chars to do markdown… SignalFx does not so
	// we'll chop those out
	// https://docs.datadoghq.com/graphing/event_stream/#markdown-events
	message = strings.Replace(message, "%%% \n", "", 1)
	message = strings.Replace(message, "\n %%%", "", 1)
	// Sometimes there are leading and trailing spaces
	message = strings.TrimSpace(message)

	ev := event.Event{
		EventType:  name,
		Category:   event.USERDEFINED,
		Dimensions: dims,
		Timestamp:  time.Unix(sample.Timestamp, 0),
		Properties: map[string]interface{}{
			"description": message,
		},
	}

	// TODO: Split events out the same way as points
	// report evt
	return sfx.defaultClient.AddEvents(ctx, []*event.Event{&ev})
}
