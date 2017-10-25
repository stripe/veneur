package veneur

import (
	"os"
	"strings"
	"testing"
	"time"

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

	assert.Equal(t, "https://app.datadoghq.com", c.DatadogAPIHostname)
	assert.Equal(t, 96, c.NumWorkers)

	interval, err := time.ParseDuration(c.Interval)
	assert.NoError(t, err)
	assert.Equal(t, interval, 10*time.Second)

	assert.Equal(t, "http://localhost:7777", c.DatadogTraceAPIAddress)

}

func TestReadBadConfig(t *testing.T) {
	const exampleConfig = `--- api_hostname: :bad`
	r := strings.NewReader(exampleConfig)
	c, err := readConfig(r)

	assert.NotNil(t, err, "Should have encountered parsing error when reading invalid config file")
	assert.Equal(t, c, Config{}, "Parsing invalid config file should return zero struct")
}

func TestReadConfigBackwardsCompatible(t *testing.T) {
	// set the deprecated config options
	const config = `
api_hostname: "http://api"
key: apikey
trace_api_address: http://trace_api
trace_address: trace_address:12345
udp_address: 127.0.0.1:8002
tcp_address: 127.0.0.1:8003
`
	c, err := readConfig(strings.NewReader(config))
	if err != nil {
		t.Fatal(err)
	}

	// they should get copied to the new config options
	assert.Equal(t, "http://api", c.DatadogAPIHostname)
	assert.Equal(t, "apikey", c.DatadogAPIKey)
	assert.Equal(t, "http://trace_api", c.DatadogTraceAPIAddress)
	assert.Equal(t, "trace_address:12345", c.SsfAddress)
	assert.Contains(t, c.StatsdListenAddresses, "udp://127.0.0.1:8002")
	assert.Contains(t, c.StatsdListenAddresses, "tcp://127.0.0.1:8003")
	assert.Contains(t, c.SsfListenAddresses, "udp://trace_address:12345")
}

func TestReadSSFConfigBackwardsCompatible(t *testing.T) {
	// set the deprecated config options
	const config = `
trace_api_address: http://trace_api
ssf_address: trace_address:12345
`
	c, err := readConfig(strings.NewReader(config))
	if err != nil {
		t.Fatal(err)
	}

	// they should get copied to the new config options
	assert.Equal(t, c.SsfListenAddresses, []string{"udp://trace_address:12345"})
}

func TestHostname(t *testing.T) {
	const hostnameConfig = "hostname: foo"
	r := strings.NewReader(hostnameConfig)
	c, err := readConfig(r)
	assert.Nil(t, err, "Should parsed valid config file: %s", hostnameConfig)
	assert.Equal(t, c, Config{Hostname: "foo", ReadBufferSizeBytes: defaultBufferSizeBytes},
		"Should have parsed hostname into Config")

	const noHostname = "hostname: ''"
	r = strings.NewReader(noHostname)
	c, err = readConfig(r)
	assert.Nil(t, err, "Should parsed valid config file: %s", noHostname)
	currentHost, err := os.Hostname()
	assert.Nil(t, err, "Could not get current hostname")
	assert.Equal(t, c, Config{Hostname: currentHost, ReadBufferSizeBytes: defaultBufferSizeBytes},
		"Should have used current hostname in Config")

	const omitHostname = "omit_empty_hostname: true"
	r = strings.NewReader(omitHostname)
	c, err = readConfig(r)
	assert.Nil(t, err, "Should parsed valid config file: %s", omitHostname)
	assert.Equal(t, c, Config{
		Hostname:            "",
		ReadBufferSizeBytes: defaultBufferSizeBytes,
		OmitEmptyHostname:   true})
}
