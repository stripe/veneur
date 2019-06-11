`veneur-prometheus` is a command line utility for periodically polling
[a Prometheus metrics endpoint](https://prometheus.io/docs/instrumenting/exposition_formats/)
and passing those metrics along to an instance of Veneur.

## SSF

At present these metrics are emitting as DogStatsD style metrics. We will add SSF at a later date.

# Usage

```
Usage of ./veneur-prometheus:
  -cacert string
    	The path to a CA cert used to validate the server certificate. Only used if using mTLS.
  -cert string
    	The path to a client cert to present to the server. Only used if using mTLS.
  -d    Enable debug mode
  -h string
    	The full URL — like 'http://localhost:9090/metrics' to query for Prometheus metrics. (default "http://localhost:9090/metrics")
  -i string
    	The interval at which to query. Value must be parseable by time.ParseDuration (https://golang.org/pkg/time/#ParseDuration). (default "10s")
  -ignored-labels string
    	A comma-seperated list of label name regexes to not export
  -ignored-metrics string
    	A comma-seperated list of metric name regexes to not export
  -key string
    	The path to a private key to use for mTLS. Only used if using mTLS.
  -p string
    	A prefix to append to any metrics emitted. Do not include a trailing period.
  -s string
    	The host and port — like '127.0.0.1:8126' — to send our metrics to. (default "127.0.0.1:8126")
```
