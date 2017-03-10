package veneur

import (
	"bytes"
	"compress/gzip"
	"compress/zlib"
	"crypto/tls"
	"crypto/x509"
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"path/filepath"
	"testing"
	"time"

	"github.com/stripe/veneur/plugins/s3/mock"

	"github.com/DataDog/datadog-go/statsd"
	"github.com/Sirupsen/logrus"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/stretchr/testify/assert"
	s3p "github.com/stripe/veneur/plugins/s3"
	"github.com/stripe/veneur/samplers"
	"github.com/stripe/veneur/tdigest"
)

const ε = .00002

const DefaultFlushInterval = 50 * time.Millisecond
const DefaultServerTimeout = 100 * time.Millisecond

var DebugMode bool

func TestMain(m *testing.M) {
	flag.Parse()
	DebugMode = flag.Lookup("test.v").Value.(flag.Getter).Get().(bool)
	os.Exit(m.Run())
}

// On the CI server, we can't be guaranteed that the port will be
// released immediately after the server is shut down. Instead, use
// a unique port for each test. As long as we don't have an insane number
// of integration tests, we should be fine.
var HTTPAddrPort = 8129

// set up a boilerplate local config for later use
func localConfig() Config {
	return generateConfig("http://localhost")
}

// set up a boilerplate global config for later use
func globalConfig() Config {
	return generateConfig("")
}

// generateConfig is not called config to avoid
// accidental variable shadowing
func generateConfig(forwardAddr string) Config {
	// we don't shut down ports so avoid address in use errors
	port := HTTPAddrPort
	HTTPAddrPort++
	metricsPort := HTTPAddrPort
	HTTPAddrPort++
	tracePort := HTTPAddrPort
	HTTPAddrPort++

	return Config{
		APIHostname: "http://localhost",
		Debug:       DebugMode,
		Hostname:    "localhost",

		// Use a shorter interval for tests
		Interval:            DefaultFlushInterval.String(),
		Key:                 "",
		MetricMaxLength:     4096,
		Percentiles:         []float64{.5, .75, .99},
		Aggregates:          []string{"min", "max", "count"},
		ReadBufferSizeBytes: 2097152,
		UdpAddress:          fmt.Sprintf("localhost:%d", metricsPort),
		HTTPAddress:         fmt.Sprintf("localhost:%d", port),
		ForwardAddress:      forwardAddr,
		NumWorkers:          4,

		// Use only one reader, so that we can run tests
		// on platforms which do not support SO_REUSEPORT
		NumReaders: 1,

		// Currently this points nowhere, which is intentional.
		// We don't need internal metrics for the tests, and they make testing
		// more complicated.
		StatsAddress:    "localhost:8125",
		Tags:            []string{},
		SentryDsn:       "",
		FlushMaxPerBody: 1024,

		// Don't use the default port 8128: Veneur sends its own traces there, causing failures
		TraceAddress:    fmt.Sprintf("127.0.0.1:%d", tracePort),
		TraceAPIAddress: forwardAddr,
	}
}

func generateMetrics() (metricValues []float64, expectedMetrics map[string]float64) {
	metricValues = []float64{1.0, 2.0, 7.0, 8.0, 100.0}

	expectedMetrics = map[string]float64{
		"a.b.c.max": 100,
		"a.b.c.min": 1,

		// Count is normalized by second
		// so 5 values/50ms = 100 values/s
		"a.b.c.count": float64(len(metricValues)) * float64(time.Second) / float64(DefaultFlushInterval),

		// tdigest approximation causes this to be off by 1
		"a.b.c.50percentile": 6,
		"a.b.c.75percentile": 42,
		"a.b.c.99percentile": 98,
	}
	return metricValues, expectedMetrics
}

// assertMetrics checks that all expected metrics are present
// and have the correct value
func assertMetrics(t *testing.T, metrics DDMetricsRequest, expectedMetrics map[string]float64) {
	// it doesn't count as accidentally quadratic if it's intentional
	for metricName, expectedValue := range expectedMetrics {
		assertMetric(t, metrics, metricName, expectedValue)
	}
}

func assertMetric(t *testing.T, metrics DDMetricsRequest, metricName string, value float64) {
	defer func() {
		if r := recover(); r != nil {
			assert.Fail(t, "error extracting metrics", r)
		}
	}()
	for _, metric := range metrics.Series {
		if metric.Name == metricName {
			assert.Equal(t, int(value+.5), int(metric.Value[0][1]+.5), "Incorrect value for metric %s", metricName)
			return
		}
	}
	assert.Fail(t, "did not find expected metric", metricName)
}

// setupVeneurServer creates a local server from the specified config
// and starts listening for requests. It returns the server for inspection.
func setupVeneurServer(t *testing.T, config Config) Server {
	server, err := NewFromConfig(config)
	if err != nil {
		t.Fatal(err)
	}

	server.Start()

	go server.HTTPServe()
	return server
}

// DDMetricsRequest represents the body of the POST request
// for sending metrics data to Datadog
// Eventually we'll want to define this symmetrically.
type DDMetricsRequest struct {
	Series []samplers.DDMetric
}

// fixture sets up a mock Datadog API server and Veneur
type fixture struct {
	api             *httptest.Server
	server          Server
	ddmetrics       chan DDMetricsRequest
	interval        time.Duration
	flushMaxPerBody int
}

func newFixture(t *testing.T, config Config) *fixture {
	interval, err := config.ParseInterval()
	assert.NoError(t, err)

	// Set up a remote server (the API that we're sending the data to)
	// (e.g. Datadog)
	f := &fixture{nil, Server{}, make(chan DDMetricsRequest, 10), interval, config.FlushMaxPerBody}
	f.api = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		zr, err := zlib.NewReader(r.Body)
		if err != nil {
			t.Fatal(err)
		}

		var ddmetrics DDMetricsRequest

		err = json.NewDecoder(zr).Decode(&ddmetrics)
		if err != nil {
			t.Fatal(err)
		}

		f.ddmetrics <- ddmetrics
		w.WriteHeader(http.StatusAccepted)
	}))

	config.APIHostname = f.api.URL
	config.NumWorkers = 1
	f.server = setupVeneurServer(t, config)

	return f
}

func (f *fixture) Close() {
	// make Close safe to call multiple times
	if f.ddmetrics == nil {
		return
	}

	f.api.Close()
	f.server.Shutdown()
	close(f.ddmetrics)
	f.ddmetrics = nil
}

// TestLocalServerUnaggregatedMetrics tests the behavior of
// the veneur client when operating without a global veneur
// instance (ie, when sending data directly to the remote server)
func TestLocalServerUnaggregatedMetrics(t *testing.T) {
	metricValues, expectedMetrics := generateMetrics()
	config := localConfig()
	f := newFixture(t, config)
	defer f.Close()

	for _, value := range metricValues {
		f.server.Workers[0].ProcessMetric(&samplers.UDPMetric{
			MetricKey: samplers.MetricKey{
				Name: "a.b.c",
				Type: "histogram",
			},
			Value:      value,
			Digest:     12345,
			SampleRate: 1.0,
			Scope:      samplers.LocalOnly,
		})
	}

	f.server.Flush()

	ddmetrics := <-f.ddmetrics
	assert.Equal(t, 6, len(ddmetrics.Series), "incorrect number of elements in the flushed series on the remote server")
	assertMetrics(t, ddmetrics, expectedMetrics)
}

func TestGlobalServerFlush(t *testing.T) {
	metricValues, expectedMetrics := generateMetrics()
	config := globalConfig()
	f := newFixture(t, config)
	defer f.Close()

	for _, value := range metricValues {
		f.server.Workers[0].ProcessMetric(&samplers.UDPMetric{
			MetricKey: samplers.MetricKey{
				Name: "a.b.c",
				Type: "histogram",
			},
			Value:      value,
			Digest:     12345,
			SampleRate: 1.0,
			Scope:      samplers.LocalOnly,
		})
	}

	f.server.Flush()

	ddmetrics := <-f.ddmetrics
	assert.Equal(t, len(expectedMetrics), len(ddmetrics.Series), "incorrect number of elements in the flushed series on the remote server")
	assertMetrics(t, ddmetrics, expectedMetrics)
}

func TestLocalServerMixedMetrics(t *testing.T) {
	// The exact gob stream that we will receive might differ, so we can't
	// test against the bytestream directly. But the two streams should unmarshal
	// to t-digests that have the same key properties, so we can test
	// those.
	const ExpectedGobStream = "\r\xff\x87\x02\x01\x02\xff\x88\x00\x01\xff\x84\x00\x007\xff\x83\x03\x01\x01\bCentroid\x01\xff\x84\x00\x01\x03\x01\x04Mean\x01\b\x00\x01\x06Weight\x01\b\x00\x01\aSamples\x01\xff\x86\x00\x00\x00\x17\xff\x85\x02\x01\x01\t[]float64\x01\xff\x86\x00\x01\b\x00\x00/\xff\x88\x00\x05\x01\xfe\xf0?\x01\xfe\xf0?\x00\x01@\x01\xfe\xf0?\x00\x01\xfe\x1c@\x01\xfe\xf0?\x00\x01\xfe @\x01\xfe\xf0?\x00\x01\xfeY@\x01\xfe\xf0?\x00\x05\b\x00\xfeY@\x05\b\x00\xfe\xf0?\x05\b\x00\xfeY@"
	tdExpected := tdigest.NewMerging(100, false)
	err := tdExpected.GobDecode([]byte(ExpectedGobStream))
	assert.NoError(t, err, "Should not have encountered error in decoding expected gob stream")

	var HistogramValues = []float64{1.0, 2.0, 7.0, 8.0, 100.0}

	// Number of events observed (in 50ms interval)
	var HistogramCountRaw = len(HistogramValues)

	// Normalize to events/second
	// Explicitly convert to int to avoid confusing Stringer behavior
	var HistogramCountNormalized = float64(HistogramCountRaw) * float64(time.Second) / float64(DefaultFlushInterval)

	// Number of events observed
	const CounterNumEvents = 40

	expectedMetrics := map[string]float64{
		// 40 events/50ms = 800 events/s
		"x.y.z":     CounterNumEvents * float64(time.Second) / float64(DefaultFlushInterval),
		"a.b.c.max": 100,
		"a.b.c.min": 1,

		// Count is normalized by second
		// so 5 values/50ms = 100 values/s
		"a.b.c.count": float64(HistogramCountNormalized),
	}

	// This represents the global veneur instance, which receives request from
	// the local veneur instances, aggregates the data, and sends it to the remote API
	// (e.g. Datadog)
	globalTD := make(chan *tdigest.MergingDigest)
	globalVeneur := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, r.URL.Path, "/import", "Global veneur should receive request on /import path")

		zr, err := zlib.NewReader(r.Body)
		if err != nil {
			t.Fatal(err)
		}

		type requestItem struct {
			Name      string      `json:"name"`
			Tags      interface{} `json:"tags"`
			Tagstring string      `json:"tagstring"`
			Type      string      `json:"type"`
			Value     []byte      `json:"value"`
		}

		var metrics []requestItem

		err = json.NewDecoder(zr).Decode(&metrics)
		if err != nil {
			t.Fatal(err)
		}

		assert.Equal(t, 1, len(metrics), "incorrect number of elements in the flushed series")

		td := tdigest.NewMerging(100, false)
		err = td.GobDecode(metrics[0].Value)
		assert.NoError(t, err, "Should not have encountered error in decoding gob stream")
		globalTD <- td
		w.WriteHeader(http.StatusAccepted)
	}))
	defer globalVeneur.Close()

	config := localConfig()
	config.ForwardAddress = globalVeneur.URL
	f := newFixture(t, config)
	defer f.Close()

	// Create non-local metrics that should be passed to the global veneur instance
	for _, value := range HistogramValues {
		f.server.Workers[0].ProcessMetric(&samplers.UDPMetric{
			MetricKey: samplers.MetricKey{
				Name: "a.b.c",
				Type: "histogram",
			},
			Value:      value,
			Digest:     12345,
			SampleRate: 1.0,
			Scope:      samplers.MixedScope,
		})
	}

	// Create local-only metrics that should be passed directly to the remote API
	for i := 0; i < CounterNumEvents; i++ {
		f.server.Workers[0].ProcessMetric(&samplers.UDPMetric{
			MetricKey: samplers.MetricKey{
				Name: "x.y.z",
				Type: "counter",
			},
			Value:      1.0,
			Digest:     12345,
			SampleRate: 1.0,
			Scope:      samplers.LocalOnly,
		})
	}

	f.server.Flush()

	// the global veneur instance should get valid data
	td := <-globalTD
	assert.Equal(t, expectedMetrics["a.b.c.min"], td.Min(), "Minimum value is incorrect")
	assert.Equal(t, expectedMetrics["a.b.c.max"], td.Max(), "Maximum value is incorrect")

	// The remote server receives the raw count, *not* the normalized count
	assert.InEpsilon(t, HistogramCountRaw, td.Count(), ε)
	assert.Equal(t, tdExpected, td, "Underlying tdigest structure is incorrect")
}

func TestSplitBytes(t *testing.T) {
	rand.Seed(time.Now().Unix())
	buf := make([]byte, 1000)

	for i := 0; i < 1000; i++ {
		// we construct a string of random length which is approximately 1/3rd A
		// and the other 2/3rds B
		buf = buf[:rand.Intn(cap(buf))]
		for i := range buf {
			if rand.Intn(3) == 0 {
				buf[i] = 'A'
			} else {
				buf[i] = 'B'
			}
		}
		checkBufferSplit(t, buf)
		buf = buf[:cap(buf)]
	}

	// also test pathological cases that the fuzz is unlikely to find
	checkBufferSplit(t, nil)
	checkBufferSplit(t, []byte{})
}

func checkBufferSplit(t *testing.T, buf []byte) {
	var testSplit [][]byte
	sb := samplers.NewSplitBytes(buf, 'A')
	for sb.Next() {
		testSplit = append(testSplit, sb.Chunk())
	}

	// now compare our split to the "real" implementation of split
	assert.EqualValues(t, bytes.Split(buf, []byte{'A'}), testSplit, "should have split %s correctly", buf)
}

type dummyPlugin struct {
	logger *logrus.Logger
	statsd *statsd.Client
	flush  func([]samplers.DDMetric, string) error
}

func (dp *dummyPlugin) Flush(metrics []samplers.DDMetric, hostname string) error {
	return dp.flush(metrics, hostname)
}

func (dp *dummyPlugin) Name() string {
	return "dummy_plugin"
}

// TestGlobalServerPluginFlush tests that we are able to
// register a dummy plugin on the server, and that when we do,
// flushing on the server causes the plugin to flush
func TestGlobalServerPluginFlush(t *testing.T) {

	RemoteResponseChan := make(chan struct{}, 1)
	defer func() {
		select {
		case <-RemoteResponseChan:
			// all is safe
			return
		case <-time.After(DefaultServerTimeout):
			assert.Fail(t, "Global server did not complete all responses before test terminated!")
		}
	}()

	metricValues, expectedMetrics := generateMetrics()
	config := globalConfig()
	f := newFixture(t, config)
	defer f.Close()

	dp := &dummyPlugin{logger: log, statsd: f.server.statsd}

	dp.flush = func(metrics []samplers.DDMetric, hostname string) error {
		assert.Equal(t, len(expectedMetrics), len(metrics))

		firstName := metrics[0].Name
		assert.Equal(t, expectedMetrics[firstName], metrics[0].Value[0][1])

		assert.Equal(t, hostname, f.server.Hostname)

		RemoteResponseChan <- struct{}{}
		return nil
	}

	f.server.registerPlugin(dp)

	for _, value := range metricValues {
		f.server.Workers[0].ProcessMetric(&samplers.UDPMetric{
			MetricKey: samplers.MetricKey{
				Name: "a.b.c",
				Type: "histogram",
			},
			Value:      value,
			Digest:     12345,
			SampleRate: 1.0,
			Scope:      samplers.LocalOnly,
		})
	}

	f.server.Flush()
}

// TestGlobalServerS3PluginFlush tests that we are able to
// register the S3 plugin on the server, and that when we do,
// flushing on the server causes the S3 plugin to flush to S3.
// This is the function that actually tests the S3Plugin.Flush()
// method
func TestGlobalServerS3PluginFlush(t *testing.T) {

	RemoteResponseChan := make(chan struct{}, 1)
	defer func() {
		select {
		case <-RemoteResponseChan:
			// all is safe
			return
		case <-time.After(DefaultServerTimeout):
			assert.Fail(t, "Global server did not complete all responses before test terminated!")
		}
	}()

	metricValues, _ := generateMetrics()

	config := globalConfig()
	f := newFixture(t, config)
	defer f.Close()

	client := &s3Mock.MockS3Client{}
	client.SetPutObject(func(input *s3.PutObjectInput) (*s3.PutObjectOutput, error) {
		f, err := os.Open(path.Join("fixtures", "aws", "PutObject", "2016", "10", "14", "1476481302.tsv.gz"))
		assert.NoError(t, err)
		defer f.Close()

		records, err := parseGzipTSV(input.Body)
		assert.NoError(t, err)

		expectedRecords, err := parseGzipTSV(f)
		assert.NoError(t, err)

		assert.Equal(t, len(expectedRecords), len(records))

		assertCSVFieldsMatch(t, expectedRecords, records, []int{0, 1, 2, 3, 4, 5, 6})
		//assert.Equal(t, expectedRecords, records)

		RemoteResponseChan <- struct{}{}
		return &s3.PutObjectOutput{ETag: aws.String("912ec803b2ce49e4a541068d495ab570")}, nil
	})

	s3p := &s3p.S3Plugin{Logger: log, Svc: client}

	f.server.registerPlugin(s3p)

	plugins := f.server.getPlugins()
	assert.Equal(t, 1, len(plugins))

	for _, value := range metricValues {
		f.server.Workers[0].ProcessMetric(&samplers.UDPMetric{
			MetricKey: samplers.MetricKey{
				Name: "a.b.c",
				Type: "histogram",
			},
			Value:      value,
			Digest:     12345,
			SampleRate: 1.0,
			Scope:      samplers.LocalOnly,
		})
	}

	f.server.Flush()
}

func parseGzipTSV(r io.Reader) ([][]string, error) {
	gzr, err := gzip.NewReader(r)
	if err != nil {
		return nil, err
	}
	cr := csv.NewReader(gzr)
	cr.Comma = '\t'
	return cr.ReadAll()
}

// assertCSVFieldsMatch asserts that all fields of all rows match, but it allows us
// to skip some columns entirely, if we know that they won't match (e.g. timestamps)
func assertCSVFieldsMatch(t *testing.T, expected, actual [][]string, columns []int) {
	if columns == nil {
		columns = make([]int, len(expected[0]))
		for i := 0; i < len(columns); i++ {
			columns[i] = i
		}
	}

	for i, row := range expected {
		assert.Equal(t, len(row), len(actual[i]))
		for _, column := range columns {
			assert.Equal(t, row[column], actual[i][column], "mismatch at column %d", column)
		}
	}
}

func readTestKeysCerts() (map[string]string, error) {
	// reads the insecure test keys and certificates in fixtures generated with:
	// # Generate the authority key and certificate (512-bit RSA signed using SHA-256)
	// openssl genrsa -out cakey.pem 512
	// openssl req -new -x509 -key cakey.pem -out cacert.pem -days 1095 -subj "/O=Example Inc/CN=Example Certificate Authority"

	// # Generate the server key and certificate, signed by the authority
	// openssl genrsa -out serverkey.pem 512
	// openssl req -new -key serverkey.pem -out serverkey.csr -days 1095 -subj "/O=Example Inc/CN=localhost"
	// openssl x509 -req -in serverkey.csr -CA cacert.pem -CAkey cakey.pem -CAcreateserial -out servercert.pem -days 1095

	// # Generate a client key and certificate, signed by the authority
	// openssl genrsa -out clientkey.pem 512
	// openssl req -new -key clientkey.pem -out clientkey.csr -days 1095 -subj "/O=Example Inc/CN=Veneur client key"
	// openssl x509 -req -in clientkey.csr -CA cacert.pem -CAkey cakey.pem -CAcreateserial -out clientcert.pem -days 1095

	// # Generate another ca and sign the client key
	// openssl genrsa -out wrongcakey.pem 512
	// openssl req -new -x509 -key wrongcakey.pem -out wrongcacert.pem -days 1095 -subj "/O=Wrong Inc/CN=Wrong Certificate Authority"
	// openssl x509 -req -in clientkey.csr -CA wrongcacert.pem -CAkey wrongcakey.pem -CAcreateserial -out wrongclientcert.pem -days 1095

	pems := map[string]string{}
	pemFileNames := []string{
		"cacert.pem",
		"clientcert_correct.pem",
		"clientcert_wrong.pem",
		"clientkey.pem",
		"servercert.pem",
		"serverkey.pem",
	}
	for _, fileName := range pemFileNames {
		b, err := ioutil.ReadFile(filepath.Join("fixtures", fileName))
		if err != nil {
			return nil, err
		}
		pems[fileName] = string(b)
	}
	return pems, nil
}

// TestTCPConfig checks that invalid configurations are errors
func TestTCPConfig(t *testing.T) {
	config := localConfig()

	config.TcpAddress = " invalid:invalid"
	_, err := NewFromConfig(config)
	if err == nil {
		t.Error("invalid TCP address is a config error")
	}

	config.TcpAddress = "localhost:8129"
	config.TLSKey = "somekey"
	config.TLSCertificate = ""
	_, err = NewFromConfig(config)
	if err == nil {
		t.Error("key without certificate is a config error")
	}

	pems, err := readTestKeysCerts()
	if err != nil {
		t.Fatal("could not read test keys/certs:", err)
	}
	config.TLSKey = pems["serverkey.pem"]
	config.TLSCertificate = "somecert"
	_, err = NewFromConfig(config)
	if err == nil {
		t.Error("invalid key and certificate is a config error")
	}

	config.TLSKey = pems["serverkey.pem"]
	config.TLSCertificate = pems["servercert.pem"]
	s, err := NewFromConfig(config)
	if err != nil {
		t.Error("expected valid config")
	}
	if s.TCPAddr == nil || s.TCPAddr.Port != 8129 {
		t.Error("TCPAddr not set correctly:", s.TCPAddr)
	}
}

func sendTCPMetrics(addr string, tlsConfig *tls.Config, f *fixture) error {
	// TODO: attempt to ensure the accept goroutine opens the port before we attempt to connect
	// connect and send stats in two parts
	var conn net.Conn
	var err error
	if tlsConfig != nil {
		conn, err = tls.Dial("tcp", addr, tlsConfig)
	} else {
		conn, err = net.Dial("tcp", addr)
	}
	if err != nil {
		return err
	}
	defer conn.Close()

	_, err = conn.Write([]byte("page.views:1|c\npage.views:1|c\n"))
	if err != nil {
		return err
	}
	err = conn.Close()
	if err != nil {
		return err
	}

	// check that the server received the stats; HACK: sleep to ensure workers process before flush
	time.Sleep(20 * time.Millisecond)
	f.server.Flush()
	select {
	case ddmetrics := <-f.ddmetrics:
		if len(ddmetrics.Series) != 1 {
			return fmt.Errorf("unexpected Series: %v", ddmetrics.Series)
		}
		if !(ddmetrics.Series[0].Name == "page.views" && ddmetrics.Series[0].Value[0][1] == 40) {
			return fmt.Errorf("unexpected metric: %v", ddmetrics.Series[0])
		}

	case <-time.After(100 * time.Millisecond):
		return fmt.Errorf("timed out waiting for metrics")
	}

	return nil
}

// TestTCPMetrics checks that a server can accept metrics over a TCP socket.
func TestTCPMetrics(t *testing.T) {
	pems, err := readTestKeysCerts()
	if err != nil {
		t.Fatal("could not read test keys/certs:", err)
	}

	// all supported TCP connection modes
	serverConfigs := []struct {
		name                   string
		serverKey              string
		serverCertificate      string
		authorityCertificate   string
		expectedConnectResults [4]bool
	}{
		{"TCP", "", "", "", [4]bool{true, false, false, false}},
		{"encrypted", pems["serverkey.pem"], pems["servercert.pem"], "",
			[4]bool{false, true, true, true}},
		{"authenticated", pems["serverkey.pem"], pems["servercert.pem"], pems["cacert.pem"],
			[4]bool{false, false, false, true}},
	}

	// load all the various keys and certificates for the client
	trustServerCA := x509.NewCertPool()
	ok := trustServerCA.AppendCertsFromPEM([]byte(pems["cacert.pem"]))
	if !ok {
		t.Fatal("could not load server certificate")
	}
	wrongCert, err := tls.X509KeyPair(
		[]byte(pems["clientcert_wrong.pem"]), []byte(pems["clientkey.pem"]))
	if err != nil {
		t.Fatal("could not load wrong client cert/key:", err)
	}
	wrongConfig := &tls.Config{
		RootCAs:      trustServerCA,
		Certificates: []tls.Certificate{wrongCert},
	}
	correctCert, err := tls.X509KeyPair(
		[]byte(pems["clientcert_correct.pem"]), []byte(pems["clientkey.pem"]))
	if err != nil {
		t.Fatal("could not load correct client cert/key:", err)
	}
	correctConfig := &tls.Config{
		RootCAs:      trustServerCA,
		Certificates: []tls.Certificate{correctCert},
	}

	// all supported client configurations
	clientConfigs := []struct {
		name      string
		tlsConfig *tls.Config
	}{
		{"TCP", nil},
		{"TLS no cert", &tls.Config{RootCAs: trustServerCA}},
		{"TLS wrong cert", wrongConfig},
		{"TLS correct cert", correctConfig},
	}

	for _, serverConfig := range serverConfigs {
		config := localConfig()
		config.NumWorkers = 1
		// Use a unique port to avoid race with shutting down accept goroutine on Linux
		config.TcpAddress = fmt.Sprintf("localhost:%d", HTTPAddrPort)
		HTTPAddrPort++
		config.TLSKey = serverConfig.serverKey
		config.TLSCertificate = serverConfig.serverCertificate
		config.TLSAuthorityCertificate = serverConfig.authorityCertificate
		f := newFixture(t, config)
		defer f.Close() // ensure shutdown if the test aborts

		// attempt to connect and send stats with each of the client configurations
		for i, clientConfig := range clientConfigs {
			expectedSuccess := serverConfig.expectedConnectResults[i]
			err := sendTCPMetrics(config.TcpAddress, clientConfig.tlsConfig, f)
			if err != nil {
				if expectedSuccess {
					t.Errorf("server config: '%s' client config: '%s' failed: %s",
						serverConfig.name, clientConfig.name, err.Error())
				} else {
					fmt.Printf("SUCCESS server config: '%s' client config: '%s' got expected error: %s\n",
						serverConfig.name, clientConfig.name, err.Error())
				}
			} else if !expectedSuccess {
				t.Errorf("server config: '%s' client config: '%s' worked; should fail!",
					serverConfig.name, clientConfig.name)
			} else {
				fmt.Printf("SUCCESS server config: '%s' client config: '%s'\n",
					serverConfig.name, clientConfig.name)
			}
		}

		f.Close()
	}
}
