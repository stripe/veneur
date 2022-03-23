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
# Optional batch write size
# note: this doesn't mean if a given flush contains 13 metrics that only 10 will be written, rather that
# for a veneur flush interval the sink will do two writes: one for 10 metrics and another for 3 
      batch_write_size: 10
# Optional proxy URL.
      proxy_url: http://localhost:1080
# Optional added headers
      headers:
        My-Header: custom header content
# Optional basic auth
      basic_auth:
        username: user1
        password: the-password
# Optional authorization (Bearer or other), if no basic auth
      authorization:
        # Defaults to Bearer if not specified
        type: Bearer
        credentials: my-token
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