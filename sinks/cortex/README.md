# Cortex Sink

This sink sends Veneur metrics to cortex via prometheus [remote-write protocol](https://prometheus.io/docs/practices/remote_write/)

## Configuration

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

## Duplicate Labels

Cortex does that handle duplicate labels, it will reject the write. So we've added logic to the cortex sink to dedupe 
labels with a last-one-wins approach. 

For example, for the given metric:
```text
my_metric{foo="bar", host="computer1", foo="baz"} 48.9
```

We'll dedupe it to:
```text
my_metric{host="computer1", foo="baz"} 48.9
```

### What about `common tags`?

Veneur has a mechanism for specifying tags that should be attached to _all_ metrics. In the case these tags duplicate
already existing one on the metric, the common tag will take precedence. 