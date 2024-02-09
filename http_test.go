package veneur

import (
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/stripe/veneur/v14/util"
	"github.com/stripe/veneur/v14/util/build"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

func TestGeneralHealthCheck(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/healthcheck", nil)

	config := localConfig()
	s := setupVeneurServer(t, config, nil, nil, nil, nil)
	defer s.Shutdown()

	w := httptest.NewRecorder()

	handler := s.Handler()
	handler.ServeHTTP(w, r)

	assert.Equal(t, http.StatusOK, w.Code, "Healthcheck did not succeed")
}

func TestOkTraceHealthCheck(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/healthcheck/tracing", nil)

	config := localConfig()
	s := setupVeneurServer(t, config, nil, nil, nil, nil)
	defer s.Shutdown()

	w := httptest.NewRecorder()

	handler := s.Handler()
	handler.ServeHTTP(w, r)

	assert.Equal(t, http.StatusOK, w.Code, "Trace healthcheck did not succeed")
}

func TestNoTracingConfiguredTraceHealthCheck(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/healthcheck/tracing", nil)

	config := localConfig()

	config.SsfListenAddresses = []util.Url{}
	server, _ := NewFromConfig(ServerConfig{
		Logger: logrus.New(),
		Config: config,
	})
	server.Start()
	defer server.Shutdown()

	w := httptest.NewRecorder()

	handler := server.Handler()
	handler.ServeHTTP(w, r)

	assert.Equal(t, http.StatusOK, w.Code, "Trace healthcheck reports tracing is enabled")
}

func TestBuildDate(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/builddate", nil)

	config := localConfig()
	config.SsfListenAddresses = []util.Url{}
	s := setupVeneurServer(t, config, nil, nil, nil, nil)
	defer s.Shutdown()

	w := httptest.NewRecorder()

	handler := s.Handler()
	handler.ServeHTTP(w, r)

	bts, err := ioutil.ReadAll(w.Body)
	assert.NoError(t, err, "error reading /builddate")

	assert.Equal(t, string(bts), build.BUILD_DATE, "received invalid build date")

	// we can't always check this against the current time
	// because that would break local tests when run with `go test`
	if build.BUILD_DATE != "dirty" {
		date, err := strconv.ParseInt(string(bts), 10, 64)
		assert.NoError(t, err, "error parsing date %s", string(bts))

		dt := time.Unix(date, 0)
		duration := time.Since(dt)
		if duration > 60*time.Minute {
			assert.Fail(t, fmt.Sprintf("either date %s is invalid, or our builds are taking more than an hour", dt.Format(time.RFC822)))
		}
	}
}

func TestVersion(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/version", nil)

	config := localConfig()
	config.SsfListenAddresses = []util.Url{}
	s := setupVeneurServer(t, config, nil, nil, nil, nil)
	defer s.Shutdown()

	w := httptest.NewRecorder()

	handler := s.Handler()
	handler.ServeHTTP(w, r)

	bts, err := ioutil.ReadAll(w.Body)
	assert.NoError(t, err, "error reading /version")

	assert.Equal(t, string(bts), build.VERSION, "received invalid version")
}

func TestConfigJson(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/config/json", nil)
	recorder := httptest.NewRecorder()

	filename := filepath.Join("testdata", "http_test_config.json")
	file, err := os.Open(filename)
	assert.NoError(t, err)
	defer file.Close()
	expectedBody, err := io.ReadAll(file)
	assert.NoError(t, err)

	config := Config{
		Aggregates: []string{"min", "max", "count"},
		HTTP: HttpConfig{
			Config: true,
		},
		Interval:            time.Millisecond,
		NumReaders:          1,
		Percentiles:         []float64{.5, .75, .99},
		ReadBufferSizeBytes: 2097152,
		SentryDsn: util.StringSecret{
			Value: "https://public@sentry.example.com/1",
		},
		StatsAddress: "localhost:8125",
	}
	server := setupVeneurServer(t, config, nil, nil, nil, nil)
	handler := server.Handler()
	handler.ServeHTTP(recorder, request)

	assert.Equal(t, "application/json", recorder.Header().Get("Content-Type"))
	body, err := ioutil.ReadAll(recorder.Body)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Equal(t, string(expectedBody), string(body))
}

func TestConfigYaml(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/config/yaml", nil)
	recorder := httptest.NewRecorder()

	filename := filepath.Join("testdata", "http_test_config.yaml")
	file, err := os.Open(filename)
	assert.NoError(t, err)
	defer file.Close()
	expectedBody, err := io.ReadAll(file)
	assert.NoError(t, err)

	config := Config{
		Aggregates: []string{"min", "max", "count"},
		HTTP: HttpConfig{
			Config: true,
		},
		Interval:            time.Millisecond,
		NumReaders:          1,
		Percentiles:         []float64{.5, .75, .99},
		ReadBufferSizeBytes: 2097152,
		SentryDsn: util.StringSecret{
			Value: "https://public@sentry.example.com/1",
		},
		StatsAddress: "localhost:8125",
	}
	server := setupVeneurServer(t, config, nil, nil, nil, nil)
	handler := server.Handler()
	handler.ServeHTTP(recorder, request)

	assert.Equal(t, "application/x-yaml", recorder.Header().Get("Content-Type"))
	body, err := ioutil.ReadAll(recorder.Body)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Equal(t, string(expectedBody), string(body))
}

func TestConfigDisabled(t *testing.T) {
	config := Config{
		Aggregates: []string{"min", "max", "count"},
		HTTP: HttpConfig{
			Config: false,
		},
		Interval:            time.Millisecond,
		NumReaders:          1,
		Percentiles:         []float64{.5, .75, .99},
		ReadBufferSizeBytes: 2097152,
		SentryDsn: util.StringSecret{
			Value: "https://public@sentry.example.com/1",
		},
		StatsAddress: "localhost:8125",
	}
	server := setupVeneurServer(t, config, nil, nil, nil, nil)
	handler := server.Handler()

	jsonRequest := httptest.NewRequest(http.MethodGet, "/config/json", nil)
	jsonRecorder := httptest.NewRecorder()
	handler.ServeHTTP(jsonRecorder, jsonRequest)
	assert.Equal(t, http.StatusNotFound, jsonRecorder.Code)

	yamlRequest := httptest.NewRequest(http.MethodGet, "/config/yaml", nil)
	yamlRecorder := httptest.NewRecorder()
	handler.ServeHTTP(yamlRecorder, yamlRequest)
	assert.Equal(t, http.StatusNotFound, yamlRecorder.Code)
}
