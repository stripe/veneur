`veneur-prometheus` is a command line utility for periodically polling
[a Prometheus metrics endpoint](https://prometheus.io/docs/instrumenting/exposition_formats/)
and passing those metrics along to an instance of Veneur.

## SSF

At present these metrics are emitting as DogStatsD style metrics. We will add SSF at a later date.

# Usage

```
Usage of ./veneur-prometheus:
  -d	Enable debug mode
  -h string
    	The full URL — like 'http://localhost:9090/metrics' to query for Prometheus metrics. (default "http://localhost:9090/metrics")
  -i string
    	The interval at which to query. Value must be parseable by time.ParseDuration (https://golang.org/pkg/time/#ParseDuration). (default "10s")
  -s string
    	The host and port — like '127.0.0.1:8126' — to send our metrics to. (default "127.0.0.1:8126")
```
