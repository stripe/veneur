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

	interval, err := c.ParseInterval()
	assert.NoError(t, err)
	assert.Equal(t, interval, 10*time.Second)
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

func TestVeneurExamples(t *testing.T) {
	tests := []string{
		"example.yaml",
		"example_host.yaml",
	}
	for _, elt := range tests {
		test := elt
		t.Run(test, func(t *testing.T) {
			t.Parallel()
			_, err := ReadConfig(test)
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
			_, err := ReadProxyConfig(test)
			assert.NoError(t, err)
		})
	}
}
