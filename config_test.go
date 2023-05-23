package veneur

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

func TestReadConfig(t *testing.T) {
	exampleConfig, err := os.Open("example.yaml")
	assert.NoError(t, err)
	defer exampleConfig.Close()

	c, err := readConfig(exampleConfig)
	if err != nil {
		t.Fatal(err)
	}
	logger := logrus.NewEntry(logrus.New())
	c.applyDefaults(logger)

	//assert.Equal(t, "https://app.datadoghq.com", c.DatadogAPIHostname)
	assert.Equal(t, 96, c.NumWorkers)

	assert.Equal(t, c.Interval, 10*time.Second)
}

func TestReadBadConfig(t *testing.T) {
	const exampleConfig = `--- api_hostname: :bad`
	r := strings.NewReader(exampleConfig)
	c, err := readConfig(r)

	assert.NotNil(t, err, "Should have encountered parsing error when reading invalid config file")
	assert.Equal(t, c, Config{}, "Parsing invalid config file should return zero struct")
}

func TestReadUnknownKeysConfig(t *testing.T) {
	const config = `---
no_such_key: 1
hostname: foobar
`
	r := strings.NewReader(config)
	c, err := readConfig(r)
	assert.Error(t, err)
	_, ok := err.(*UnknownConfigKeys)
	t.Log(err)
	assert.True(t, ok, "Returned error should indicate a strictness error")
	assert.Equal(t, "foobar", c.Hostname)
}

func TestReadUnknownKeysProxyConfig(t *testing.T) {
	const config = `---
no_such_key: 1
debug: true
`
	r := strings.NewReader(config)
	c, err := readProxyConfig(r)
	assert.Error(t, err)
	_, ok := err.(*UnknownConfigKeys)
	t.Log(err)
	assert.True(t, ok, "Returned error should indicate a strictness error")
	assert.Equal(t, true, c.Debug)
}

func TestHostname(t *testing.T) {
	const hostnameConfig = "hostname: foo"
	r := strings.NewReader(hostnameConfig)
	c, err := readConfig(r)
	assert.Nil(t, err, "Should parsed valid config file: %s", hostnameConfig)
	assert.Equal(t, c.Hostname, "foo", "Should have parsed hostname into Config")

	const noHostname = "hostname: ''"
	r = strings.NewReader(noHostname)
	c, err = readConfig(r)
	assert.Nil(t, err, "Should parsed valid config file: %s", noHostname)
	currentHost, err := os.Hostname()
	assert.Nil(t, err, "Could not get current hostname")
	logger := logrus.NewEntry(logrus.New())
	c.applyDefaults(logger)
	assert.Equal(t, c.Hostname, currentHost, "Should have used current hostname in Config")

	const omitHostname = "omit_empty_hostname: true"
	r = strings.NewReader(omitHostname)
	c, err = readConfig(r)
	assert.Nil(t, err, "Should parsed valid config file: %s", omitHostname)
	c.applyDefaults(logger)
	assert.Equal(t, c.Hostname, "", "Should have respected omit_empty_hostname")
}

func TestConfigDefaults(t *testing.T) {
	const emptyConfig = "---"
	r := strings.NewReader(emptyConfig)
	c, err := readConfig(r)
	assert.Nil(t, err, "Should parsed empty config file: %s", emptyConfig)

	expectedConfig := defaultConfig
	currentHost, err := os.Hostname()
	assert.Nil(t, err, "Could not get current hostname")
	expectedConfig.Hostname = currentHost

	logger := logrus.NewEntry(logrus.New())
	c.applyDefaults(logger)
	assert.Equal(t, c, expectedConfig, "Should have applied all config defaults")
}

func TestDefaultConfigEquivalence(t *testing.T) {
	assert.Equal(t,
		defaultConfig.Interval,
		defaultConfig.SplunkHecMaxConnectionLifetime,
		"splunk_hec_max_connection_lifetime should default to interval")

}

func TestProxyConfigDefaults(t *testing.T) {
	const emptyConfig = "---"
	r := strings.NewReader(emptyConfig)
	c, err := readProxyConfig(r)
	assert.Nil(t, err, "Should parsed empty config file: %s", emptyConfig)

	expectedConfig := defaultProxyConfig
	logger := logrus.NewEntry(logrus.New())
	c.applyDefaults(logger)
	assert.Equal(t, c, expectedConfig, "Should have applied all config defaults")
}

func TestVeneurExamples(t *testing.T) {
	tests := []string{
		"example.yaml",
		"example_host.yaml",
	}
	for _, elt := range tests {
		test := elt
		t.Run(test, func(t *testing.T) {
			t.Parallel()
			logger := logrus.NewEntry(logrus.New())
			_, err := ReadConfig(logger, test)
			assert.NoError(t, err)
		})
	}
}

func TestProxyExamples(t *testing.T) {
	tests := []string{"example_proxy.yaml"}
	for _, elt := range tests {
		test := elt
		t.Run(test, func(t *testing.T) {
			t.Parallel()
			logger := logrus.NewEntry(logrus.New())
			_, err := ReadProxyConfig(logger, test)
			assert.NoError(t, err)
		})
	}
}

func TestReadConfigBackwardsCompatible(t *testing.T) {
	// set the deprecated config options
	const config = `
flush_max_per_body: 1234
ssf_buffer_size: 3456
trace_lightstep_access_token: "123"
trace_lightstep_collector_host: "http://example.com"
trace_lightstep_reconnect_period: "10s"
trace_lightstep_maximum_spans: 1
trace_lightstep_num_clients: 2
`
	c, err := readConfig(strings.NewReader(config))
	logger := logrus.NewEntry(logrus.New())
	c.applyDefaults(logger)
	if err != nil {
		t.Fatal(err)
	}

	// they should get copied to the new config options
	assert.Equal(t, 1234, c.DatadogFlushMaxPerBody)
	assert.Equal(t, 3456, c.DatadogSpanBufferSize)
	assert.Equal(t, "123", c.LightstepAccessToken.Value)
	assert.Equal(t, "http://example.com", c.LightstepCollectorHost.Value.String())
	assert.Equal(t, 10*time.Second, c.LightstepReconnectPeriod)
	assert.Equal(t, 1, c.LightstepMaximumSpans)
	assert.Equal(t, 2, c.LightstepNumClients)
}

func TestReadConfigEnvironmentVariables(t *testing.T) {
	const fakeApiKey = "fake_api_key"
	defer os.Unsetenv("VENEUR_DATADOGAPIKEY")
	os.Setenv("VENEUR_DATADOGAPIKEY", fakeApiKey)
	// read in empty config
	c, err := readConfig(strings.NewReader(""))
	if err != nil {
		t.Fatal(err)
	}
	// make sure value was pulled from environment variable
	assert.Equal(t, fakeApiKey, c.DatadogAPIKey.Value)
}
