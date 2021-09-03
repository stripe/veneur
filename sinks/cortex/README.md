# Cortex Sink

This sink sends Veneur metrics to cortex via prometheus [remote-write protocol](https://prometheus.io/docs/practices/remote_write/)

# Configuration

```yaml
metric_sinks:
  - kind: cortex
    name: cortex1
    config:
# The URL of the endpoint to send samples to.
      url: http://localhost:9090/api/v1/receive
# Timeout for requests to the remote write endpoint.
      remote_timeout: 30s
# Optional proxy URL.
      proxy_url: http://localhost:1080
```
