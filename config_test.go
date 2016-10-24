package veneur

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestReadConfig(t *testing.T) {

	// It's better not to require read access on the filesystem
	// in tests if we can avoid it
	const exampleConfig = `---
api_hostname: https://app.datadoghq.com
metric_max_length: 4096
flush_max_per_body: 25000
debug: true
interval: "10s"
key: "farts"
num_workers: 96
num_readers: 4
percentiles:
  - 0.5
  - 0.75
  - 0.99
read_buffer_size_bytes: 2097152
stats_address: "localhost:8125"
tags:
 - "foo:bar"
#  - "baz:gorch"
udp_address: "localhost:8126"
#http_address: "einhorn@0"
http_address: "localhost:8127"
forward_address: "http://veneur.example.com"
# Defaults to the os.Hostname()!
# hostname: foobar`

	r := strings.NewReader(exampleConfig)
	c, err := readConfig(r)
	if err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, "https://app.datadoghq.com", c.APIHostname)
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
