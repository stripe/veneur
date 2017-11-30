<div align="center">
  <img src="https://raw.githubusercontent.com/stripe/veneur/gh-pages/veneur_logo.svg?sanitize=true">
</div>


[![Build Status](https://travis-ci.org/stripe/veneur.svg?branch=master)](https://travis-ci.org/stripe/veneur)
[![GoDoc](https://godoc.org/github.com/stripe/veneur?status.svg)](http://godoc.org/github.com/stripe/veneur)

Veneur (venn-urr) is a distributed, fault-tolerant pipeline for runtime data. It provides a server implementation of the [DogStatsD protocol](http://docs.datadoghq.com/guides/dogstatsd/#datagram-format) or [SSF](https://github.com/stripe/veneur/tree/master/ssf) for aggregating metrics and sending them to downstream storage to one or more supported sinks. It can also act as a [global aggregator](#global-aggregation) for histograms, sets and counters.

More generically, Veneur is a convenient sink for various observability primitives.

See also:

* A unified, standard format for observability primitives, the [SSF](https://github.com/stripe/veneur/tree/master/ssf/#readme)
* A proxy for resilient distributed aggregation, [veneur-proxy](https://github.com/stripe/veneur/tree/master/cmd/veneur-proxy/#readme)
* A command line tool for emitting metrics, [veneur-emit](https://github.com/stripe/veneur/tree/master/cmd/veneur-emit/#readme)

# Status

Veneur is currently handling all metrics for Stripe and is considered production ready. It is under active development and maintenance! Starting with v1.6, Veneur operates on a six-week release cycle, and all releases are tagged in git.

Building Veneur requires Go 1.8 or later.

# Motivation

We wanted percentiles, histograms and sets to be global. Veneur helps us do that!

Veneur is a DogStatsD implementation that acts as a local collector and — optionally — as an aggregator for
some metric types, such that the metrics are *global* rather than host-local. This is particularly useful for histograms,
timers and sets, as in their normal, per-host configuration the percentiles for histograms can be less effective or even
meaningless. Per-host unique sets are also often not what's desired.

Global \*StatsD installations can be problematic, as they either require client-side or proxy sharding behavior to prevent an
instance being a Single Point of Failure (SPoF) for all metrics. Veneur aims to solve this problem. Non-global
metrics like counters and gauges are collected by a local Veneur instance and sent to storage at flush time. Global metrics (histograms and sets)
are forwarded to a central Veneur instance for aggregation before being sent to storage.

# How Veneur Is Different Than Official DogStatsD

Veneur is different for a few reasons. They are enumerated here.

## Protocol

Veneur adheres to [the official DogStatsD datagram format](http://docs.datadoghq.com/guides/dogstatsd/#datagram-format) with the exceptions below:

* The tag `veneurlocalonly` is stripped and influences forwarding behavior, as discussed below.
* The tag `veneurglobalonly` is stripped and influences forwarding behavior, as discussed below.

## Global Aggregation

If configured to do so, Veneur can selectively aggregate global metrics to be cumulative across all instances that report to a central Veneur, allowing global percentile calculation and global set counts.

For example, say you emit a timer `foo.bar.call_duration_ms` from 20 hosts that are configured to forward to a central veneur. In Datadog you'll see the following:

* Metrics that have been "globalized"
  * `foo.bar.call_duration_ms.50percentile`: the p50 across all hosts, by tag
  * `foo.bar.call_duration_ms.90percentile`: the p90 across all hosts, by tag
  * `foo.bar.call_duration_ms.95percentile`: the p95 across all hosts, by tag
  * `foo.bar.call_duration_ms.99percentile`: the p99 across all hosts, by tag
* Metrics that remain host-local
  * `foo.bar.call_duration_ms.avg`: by-host tagged average
  * `foo.bar.call_duration_ms.count`: by-host tagged count which (when summed) shows the total count of times this metric was emitted
  * `foo.bar.call_duration_ms.max`: by-host tagged maximum value
  * `foo.bar.call_duration_ms.median`: by-host tagged median value
  * `foo.bar.call_duration_ms.min`: by-host tagged minimum value
  * `foo.bar.call_duration_ms.sum`: by-host tagged sum value representing the total time

Clients can choose to override this behavior by [including the tag `veneurlocalonly`](#magic-tag).

## Approximate Histograms

Because Veneur is built to handle lots and lots of data, it uses approximate histograms. We have our own implementation of [Dunning's t-digest](tdigest/merging_digest.go), which has bounded memory consumption and reduced error at extreme quantiles. Metrics are consistently routed to the same worker to distribute load and to be added to the same histogram.

Datadog's DogStatsD — and StatsD — uses an exact histogram which retains all samples and is reset every flush period. This means that there is a loss of precision when using Veneur, but
the resulting percentile values are meant to be more representative of a global view.

## Approximate Sets

Veneur uses [HyperLogLogs](https://github.com/clarkduvall/hyperloglog) for approximate unique sets. These are a very efficient unique counter with fixed memory consumption.

## Global Counters

Via an optional [magic tag](#magic-tag) Veneur will forward counters to a global host for accumulation. This feature was primarily developed to control tag cardinality. Some counters are valuable but do not require per-host tagging.

## Lack of Host Tags for Aggregated Metrics

By definition the hostname is not applicable to *global* metrics that Veneur processes. Note that if you
do include a hostname tag, Veneur will **not** strip it for you. Veneur will add its own hostname as configured to metrics sent to Datadog.

## Expiration

Veneur expires all metrics on each flush. If a metric is no longer being sent (or is sent sparsely) Veneur will not send it as zeros! This was chosen because the combination of the approximation's features and the additional hysteresis imposed by *retaining* these approximations over time was deemed more complex than desirable.

# Concepts

* Global metrics are those that benefit from being aggregated for chunks — or all — of your infrastructure. These are histograms (including the percentiles generated by timers) and sets.
* Metrics that are sent to another Veneur instance for aggregation are said to be "forwarded". This terminology helps to decipher configuration and metric options below.
* Flushed, in Veneur, means metrics sent to Datadog.

## By Metric Type Behavior

To clarify how each metric type behaves in Veneur, please use the following:
* Counters: Locally accrued, flushed to Datadog (see [magic tags](#magic-tag) for global version)
* Gauges: Locally accrued, flushed to Datadog  (see [magic tags](#magic-tag) for global version)
* Histograms: Locally accrued, count, max and min flushed to Datadog, percentiles forwarded to `forward_address` for global aggregation when set.
* Timers: Locally accrued, count, max and min flushed to Datadog, percentiles forwarded to `forward_address` for global aggregation when set.
* Sets: Locally accrued, forwarded to `forward_address` for global aggregation when set.

# Other Notes

* Veneur aligns its flush timing with the local clock. For the default interval of `10s` Veneur will generally emit metrics at 00, 10, 20, 30, … seconds after the minute.
* Veneur will delay it's first metric emission to align the clock as stated above. This may result in a brief quiet period on a restart at worst < `interval` seconds long.

# Usage

```
veneur -f example.yaml
```

See example.yaml for a sample config. Be sure to set your Datadog API `key`!

# Plugins

Veneur [includes optional plugins](tree/master/plugins) to extend its capabilities. These plugins are enabled via configuration options. Please consult each plugin's README for more information:

* [S3 Plugin](plugins/s3) - Emit flushed metrics as a TSV file to Amazon S3

# Setup

Here we'll document some explanations of setup choices you may make when using Veneur.

## With Datadog

Veneur can act as a replacement for the stock DogStatsD. In our environment we disable the Datadog DogStatsd port by setting `use_dogstatsd` to false. We run Veneur on an alternate port (8200) so as to not accidentally ingest metrics from legacy StatsD clients and we configure our Datadog agent to use this port by `dogstatsd_port` to 8200 to match.

## Clients

Veneur is capable of ingesting:

* [DogStatsD](https://docs.datadoghq.com/guides/dogstatsd/) including events and service checks
* [SSF](https://github.com/stripe/veneur/tree/master/ssf) (experimental)
* StatsD as a subset of DogStatsD, but this may cause trouble depending on where you store your metrics.

To use clients with Veneur you need only configure your client of choice to the proper host and port combination. This port should match one of:

* `udp_address` for UDP-based clients
* `tcp_address` for TCP-based clients
* `ssf_address` for SSF-based clients

## Einhorn Usage

When you upgrade Veneur (deploy, stop, start with new binary) there will be a
brief period where Veneur will not be able to handle HTTP requests. At Stripe
we use [Einhorn](https://github.com/stripe/einhorn) as a shared socket manager to
bridge the gap until Veneur is ready to handle HTTP requests again.

You'll need to consult Einhorn's documentation for installation, setup and usage.
But once you've done that you can tell Veneur to use Einhorn by setting `http_address`
to `einhorn@0`. This informs [goji/bind](https://github.com/zenazn/goji/tree/master/bind) to use its
Einhorn handling code to bind to the file descriptor for HTTP.

## Forwarding

Veneur instances can be configured to forward their global metrics to another Veneur instance. You can use this feature to get the best of both worlds: metrics that benefit from global aggregation can be passed up to a single global Veneur, but other metrics can be published locally with host-scoped information. Note: **Forwarding adds an additional delay to metric availability corresponding to the value of the `interval` configuration option**, as the local veneur will flush it to its configured upstream, which will then flush any recieved metrics when its interval expires.

If a local instance receives a histogram or set, it will publish the local parts of that metric (the count, min and max) directly to DataDog, but instead of publishing percentiles, it will package the entire histogram and send it to the global instance. The global instance will aggregate all the histograms together and publish their percentiles to DataDog.

Note that the global instance can also receive metrics over UDP. It will publish a count, min and max for the samples that were sent directly to it, but not counting any samples from other Veneur instances (this ensures that things don't get double-counted). You can even chain multiple levels of forwarding together if you want. This might be useful if, for example, your global Veneur is under too much load. The root of the tree will be the Veneur instance that has an empty `forward_address`. (Do not tell a Veneur instance to forward metrics to itself. We don't support that and it doesn't really make sense in the first place.)

With respect to the `tags` configuration option, the tags that will be added are those of the Veneur that actually publishes to DataDog. If a local instance forwards its histograms and sets to a global instance, the local instance's tags will not be attached to the forwarded structures. It will still use its own tags for the other metrics it publishes, but the percentiles will get extra tags only from the global instance.

### Proxy

To improve availability, you can [leverage veneur-proxy](https://github.com/stripe/veneur/tree/master/cmd/veneur-proxy/#readme) in conjunction with [Consul](https://www.consul.io) service discovery.

The proxy can be configured to query the Consul API for instances of a service using `consul_forward_service_name`. Each **healthy** instance is then entered in to a hash ring. When choosing which host to forward to, Veneur will use a combination of metric name and tags to _consistently_ choose the same host for forwarding.

See [more documentation for Proxy Veneur](https://github.com/stripe/veneur/tree/master/cmd/veneur-proxy/#readme).

### Static Configuration

For static configuration you need one Veneur, which we'll call the _global_ instance, and one or more other Veneurs, which we'll call _local_ instances. The local instances should have their `forward_address` configured to the global instance's `http_address`. The global instance should have an empty `forward_address` (ie just don't set it). You can then report metrics to any Veneur's `udp_address` as usual.

### Magic Tag

If you want a metric to be strictly host-local, you can tell Veneur not to forward it by including a `veneurlocalonly` tag in the metric packet, eg `foo:1|h|#veneurlocalonly`. This tag will not actually appear in DataDog; Veneur removes it.

#### Global Counters And Gauges

Relatedly, if you want to forward a counter or gauge to the global Veneur instance to reduce tag cardinality, you can tell Veneur to flush it to the global instance by including a `veneurglobalonly` tag in the metric's packet. This `veneurglobalonly` tag is stripped and will not be passed on to sinks.

**Note**: For global counters to report correctly, the local and global Veneur instances should be configured to have the same flush interval.

**Note**: Global gauges are "random write wins" since they are merged in a non-deterministic order at the global Veneur.

#### Hostname and Device

Veneur also honors the same "magic" tags that the dogstatsd daemon includes in the datadog agent. The tag `host` will override `Hostname` in the metric and `device` will override `DeviceName`.

# Configuration

Veneur expects to have a config file supplied via `-f PATH`. The included `example.yaml` outlines the options:

### Collection
* `statsd_listen_addresses` - The address(es) on which to listen for metrics, in URI form. Examples: `udp://127.0.0.1:8126`, `tcp://127.0.0.1:8126`. DogStatsD listens on 8125, so you might want to choose a different port.

### Behavior
* `forward_address` - The address of an upstream Veneur to forward metrics to. See below.
* `interval` - How often to flush. Something like 10s seems good. **Note: If you change this, it breaks all kinds of things on Datadog's side. You'll have to change all your metric's metadata.**
* `stats_address` - The address to send internally generated metrics. Probably `127.0.0.1:8126`. In practice this means you'll be sending metrics to yourself. This is expected!
* `http_address` - The address to serve HTTP healthchecks and other endpoints. This can be a simple ip:port combination like `127.0.0.1:8127`. If you're under einhorn, you probably want `einhorn@0`.

### Metrics configuration
* `hostname` - The hostname to be tagged on each metric sent. Defaults to `os.Hostname()`
* `omit_empty_hostname` - If true and `hostname` is empty (`""`) Veneur will *not* add a host tag to its own metrics.
* `tags` - Tags to add to every metric that is sent to Veneur. Expects an array of strings!
* `percentiles` - The percentiles to generate from our timers and histograms. Specified as array of float64s
* `aggregates` - The aggregates to generate from our timers and histograms. Specified as array of strings, choices: min, max, median, avg, count, sum. Default: min, max, count

### Performance
* `num_workers` - The number of worker goroutines to start.
* `num_readers` - The number of reader goroutines to start. Veneur supports SO_REUSEPORT on Linux to scale to multiple readers. On other platforms, this should always be 1; other values will probably cause errors at startup. See below.

### Limits
* `metric_max_length` - How big a buffer to allocate for incoming metric lengths. Metrics longer than this will get truncated!
* `trace_max_length_bytes` - How big a buffer to allocate for incoming traces
* `ssf_buffer_size` - The number of SSF packets that can be processed per flush interval
* `read_buffer_size_bytes` - The size of the buffer we'll use to buffer socket reads. Tune this if you think Veneur needs more room to keep up with all packets.
* `flush_max_per_body` - how many metrics to include in each JSON body POSTed to Datadog. Veneur will POST multiple bodies in parallel if it goes over this limit. A value around 5k-10k is recommended; in practice we've seen Datadog reject bodies over about 195k.

### Diagnostics
* `debug` - Should we output lots of debug info? :)
* `sentry_dsn` A [DSN](https://docs.sentry.io/hosted/quickstart/#configure-the-dsn) for [Sentry](https://sentry.io/), where errors will be sent when they happen.
* `enable_profiling` - Enables Go profiling

### Sinks
#### Datadog
* `datadog_api_key` - Your Datadog API key
* `datadog_api_hostname` - The Datadog API URL to post to. Probably `https://app.datadoghq.com`.
* `datadog_trace_api_address` - The hostname to send Datadog traces to

#### LightStep
* `trace_lightstep_access_token` - The access token for sending to LightStep
* `trace_lightstep_collector_host` - The hostname to send trace data to
* `trace_lightstep_reconnect_period` - How often to reconnect to LightStep collectors

### Plugins
#### S3
* `aws_access_key_id` - The AWS access key ID, used in conjunction with `aws_secret_access_key` to authenticate to AWS
* `aws_secret_access_key` - The AWS secret access key, used in conjunction with `aws_access_key_id` to authenticate to AWS
* `aws_region` - The region to write to
* `aws_s3_bucket` - The bucket to write to

#### LocalFile
* `flush_file` - The local file path to write metrics to

## Configuration via Environment Variables

Veneur and veneur-proxy each allow configuration via environment variables using [envconfig](https://github.com/kelseyhightower/envconfig). Options provided via environment variables take precedent over those in config. This allows stuff like:

```
VENEUR_DEBUG=true veneur -f someconfig.yml
```

**Note**: The environment variables used for configuration map to the field names in [config.go](https://github.com/stripe/veneur/blob/master/config.go), capitalized, with the prefix `VENEUR_`. For example, the environment variable equivalent of `api_hostname` is `VENEUR_APIHOSTNAME`.

You may specify configurations that are arrays by separating them with a comma, for example `VENEUR_AGGREGATES="min,max"`

# Monitoring

Here are the important things to monitor with Veneur:

## At Local Node

When running as a local instance, you will be primarily concerned with the following metrics:
* `veneur.flush*.error_total` as a count of errors when flushing metrics to Datadog. This should rarely happen. Occasional errors are fine, but sustained is bad.
* `veneur.flush.total_duration_ns` and `veneur.flush.total_duration_ns.count`. These metrics track the per-host time spent performing a flush to Datadog. The time should be minimal!

### Forwarding

If you are forwarding metrics to central Veneur, you'll want to monitor these:
* `veneur.forward.error_total` and the `cause` tag. This should pretty much never happen and definitely not be sustained.
* `veneur.forward.duration_ns` and `veneur.forward.duration_ns.count`. These metrics track the per-host time spent performing a forward. The time should be minimal!

## At Global Node

When forwarding you'll want to also monitor the global nodes you're using for aggregation:
* `veneur.import.request_error_total` and the `cause` tag. This should pretty much never happen and definitely not be sustained.
* `veneur.import.response_duration_ns` and `veneur.import.response_duration_ns.count` to monitor duration and number of received forwards. This should not fail and not take very long. How long it takes will depend on how many metrics you're forwarding.
* And the same `veneur.flush.*` metrics from the "At Local Node" section.

## Metrics

Veneur will emit metrics to the `stats_address` configured above in DogStatsD form. Those metrics are:

* `veneur.packet.error_total` - Number of packets that Veneur could not parse due to some sort of formatting error by the client. Tagged by `packet_type` and `reason`.
* `veneur.flush.post_metrics_total` - The total number of time-series points that will be submitted to Datadog via POST. Datadog's rate limiting is roughly proportional to this number.
* `veneur.forward.post_metrics_total` - Indicates how many metrics are being forwarded in a given POST request. A "metric", in this context, refers to a unique combination of name, tags and metric type.
* `veneur.*.content_length_bytes.*` - The number of bytes in a single POST body. Remember that Veneur POSTs large sets of metrics in multiple separate bodies in parallel. Uses a histogram, so there are multiple metrics generated depending on your local DogStatsD config.
* `veneur.flush.duration_ns` - Time taken for a single POST transaction to the Datadog API. Tagged by `part` for each sub-part `marshal` (assembling the request body) and `post` (blocking on an HTTP response).
* `veneur.forward.duration_ns` - Same as `flush.duration_ns`, but for forwarding requests.
* `veneur.flush.total_duration_ns` - Total time spent POSTing to Datadog, across all parallel requests. Under most circumstances, this should be roughly equal to the total `veneur.flush.duration_ns`. If it's not, then some of the POSTs are happening in sequence, which suggests some kind of goroutine scheduling issue.
* `veneur.flush.error_total` - Number of errors received POSTing to Datadog.
* `veneur.forward.error_total` - Number of errors received POSTing to an upstream Veneur. See also `import.request_error_total` below.
* `veneur.flush.worker_duration_ns` - Per-worker timing — tagged by `worker` - for flush. This is important as it is the time in which the worker holds a lock and is unavailable for other work.
* `veneur.gc.number` - Number of completed GC cycles.
* `veneur.gc.pause_total_ns` - Total seconds of STW GC since the program started.
* `veneur.mem.heap_alloc_bytes` - Total number of reachable and unreachable but uncollected heap objects in bytes.
* `veneur.worker.metrics_processed_total` - Total number of metric packets processed between flushes by workers, tagged by `worker`. This helps you find hot spots where a single worker is handling a lot of metrics. The sum across all workers should be approximately proportional to the number of packets received.
* `veneur.worker.metrics_flushed_total` - Total number of metrics flushed at each flush time, tagged by `metric_type`. A "metric", in this context, refers to a unique combination of name, tags and metric type. You can use this metric to detect when your clients are introducing new instrumentation, or when you acquire new clients.
* `veneur.worker.metrics_imported_total` - Total number of metrics received via the importing endpoint. A "metric", in this context, refers to a unique combination of name, tags, type _and originating host_. This metric indicates how much of a Veneur instance's load is coming from imports.
* `veneur.import.response_duration_ns` - Time spent responding to import HTTP requests. This metric is broken into `part` tags for `request` (time spent blocking the client) and `merge` (time spent sending metrics to workers).
* `veneur.import.request_error_total` - A counter for the number of import requests that have errored out. You can use this for monitoring and alerting when imports fail.

## Error Handling

In addition to logging, Veneur will dutifully send any errors it generates to a [Sentry](https://sentry.io/) instance. This will occur if you set the `sentry_dsn` configuration option. Not setting the option will disable Sentry reporting.

# Performance

Processing packets quickly is the name of the game.

## Benchmarks

The common use case for Veneur is as an aggregator and host-local replacement for DogStatsD, therefore processing UDP fast is no longer the priority. That said,
we were processing > 60k packets/second in production before shifting to the current local aggregation method. This outperformed both the Datadog-provided DogStatsD
and StatsD in our infrastructure.

## Compressed, Chunked POST

Datadog's API is tuned for small POST bodies from lots of hosts since they work on a per-host basis. Also there are limits on the size of the body that
can be posted. As a result Veneur chunks metrics in to smaller bits — governed by `flush_max_per_body` — and sends them (compressed) concurrently to
Datadog. This is essential for reasonable performance as Datadog's API seems to be somewhat `O(n)` with the size of the body (which is proportional
to the number of metrics).

We've found that our hosts generate around 5k metrics and have reasonable performance, so in our case 5k is used as the `flush_max_per_body`.

## Sysctl

The following `sysctl` settings are used in testing, and are the same one would use for StatsD:

```
sysctl -w net.ipv4.udp_rmem_min=67108864
sysctl -w net.ipv4.udp_wmem_min=67108864
sysctl -w net.core.netdev_max_backlog=200000
sysctl -w net.core.rmem_max=16777216
sysctl -w net.core.rmem_default=16777216
sysctl -w net.ipv4.udp_mem="4648512 6198016 9297024"
```

## SO_REUSEPORT

As [other implementations](http://githubengineering.com/brubeck/) have observed, there's a limit to how many UDP packets a single kernel thread can consume before it starts to fall over. Veneur supports the `SO_REUSEPORT` socket option on Linux, allowing multiple threads to share the UDP socket with kernel-space balancing between them. If you've tried throwing more cores at Veneur and it's just not going fast enough, this feature can probably help by allowing more of those cores to work on the socket (which is Veneur's hottest code path by far). Note that this is only supported on Linux (right now). We have not added support for other platforms, like darwin and BSDs.

## TCP connections

Veneur supports reading the statds protocol from TCP connections. This is mostly to support TLS encryption and authentication, but might be useful on its own. Since TCP is a continuous stream of bytes, this requires each stat to be terminated by a new line character ('\n'). Most statsd clients only add new lines between stats within a single UDP packet, and omit the final trailing new line. This means you will likely need to modify your client to use this feature.


## TLS encryption and authentication

If you specify the `tls_key` and `tls_certificate` options, Veneur will only accept TLS connections on its TCP port. This allows the metrics sent to Veneur to be encrypted.

If you specify the `tls_authority_certificate` option, Veneur will require clients to present a client certificate, signed by this authority. This ensures that only authenticated clients can connect.

You can generate your own set of keys using openssl:

```
# Generate the authority key and certificate (2048-bit RSA signed using SHA-256)
openssl genrsa -out cakey.pem 2048
openssl req -new -x509 -sha256 -key cakey.pem -out cacert.pem -days 1095 -subj "/O=Example Inc/CN=Example Certificate Authority"

# Generate the server key and certificate, signed by the authority
openssl genrsa -out serverkey.pem 2048
openssl req -new -sha256 -key serverkey.pem -out serverkey.csr -days 1095 -subj "/O=Example Inc/CN=veneur.example.com"
openssl x509 -sha256 -req -in serverkey.csr -CA cacert.pem -CAkey cakey.pem -CAcreateserial -out servercert.pem -days 1095

# Generate a client key and certificate, signed by the authority
openssl genrsa -out clientkey.pem 2048
openssl req -new -sha256 -key clientkey.pem -out clientkey.csr -days 1095 -subj "/O=Example Inc/CN=Veneur client key"
openssl x509 -req -in clientkey.csr -CA cacert.pem -CAkey cakey.pem -CAcreateserial -out clientcert.pem -days 1095
```

Set `tcp_address`, `tls_key`, `tls_certificate`, and `tls_authority_certificate`:

```
tcp_address: "localhost:8129"
tls_certificate: |
  -----BEGIN CERTIFICATE-----
  MIIC8TCCAdkCCQDc2V7P5nCDLjANBgkqhkiG9w0BAQsFADBAMRUwEwYDVQQKEwxC
  ...
  -----END CERTIFICATE-----
tls_key: |
    -----BEGIN RSA PRIVATE KEY-----
  MIIEpAIBAAKCAQEA7Sntp4BpEYGzgwQR8byGK99YOIV2z88HHtPDwdvSP0j5ZKdg
  ...
  -----END RSA PRIVATE KEY-----
tls_authority_certificate: |
  -----BEGIN CERTIFICATE-----
  ...
  -----END CERTIFICATE-----
```

### Performance implications of TLS

Establishing a TLS connection is fairly expensive, so you should reuse connections as much as possible. RSA keys are also far more expensive than using ECDH keys. Using localhost on a machine with one CPU, Veneur was able to establish ~700 connections/second using ECDH `prime256v1` keys, but only ~110 connections/second using RSA 2048-bit keys. According to the Go profiling for a Veneur instance using TLS with RSA keys, approximately 25% of the CPU time was in the TLS handshake, and 13% was decrypting data.

# Name

The [veneur](https://en.wikipedia.org/wiki/Grand_Huntsman_of_France) is a person acting as superintendent of the chase and especially
of hounds in French medieval venery and being an important officer of the royal household. In other words, it is the master of dogs. :)
