[![Build Status](https://travis-ci.org/stripe/veneur.svg?branch=master)](https://travis-ci.org/stripe/veneur)
[![GoDoc](https://godoc.org/github.com/stripe/veneur?status.svg)](http://godoc.org/github.com/stripe/veneur)

Veneur (venn-urr) is a server implementation of the [DogStatsD protocol](http://docs.datadoghq.com/guides/dogstatsd/#datagram-format) for aggregating metrics and sending them to downstream storage (e.g. Datadog), typically [Datadog](http://datadoghq.com). It can also as a [global aggregator](#global-aggregation) for histograms, sets and counters.

More generically, Veneur is a convenient, host-local sink for various observability primitives.

# Status

Veneur is currently handling all metrics for Stripe and is considered production ready. It is under active development and maintenance!

Building Veneur requires Go 1.7 or later.

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

Via an optional [magic tag](#magic-tag) Veneur will forward counters to a global host for accumulation. This feature was primarily developed to

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
* Gauges: Locally accrued, flushed to Datadog
* Histograms: Locally accrued, count, max and min flushed to Datadog, percentiles forwarded to `forward_address` for global aggregation when set.
* Timers: Locally accrued, count, max and min flushed to Datadog, percentiles forwarded to `forward_address` for global aggregation when set.
* Sets: Locally accrued, forwarded to `forward_address` for global aggregation when set.

# Usage

```
veneur -f example.yaml
```

See example.yaml for a sample config. Be sure and set your Datadog API `key`!

# Plugins

Veneur [includes optional plugins](tree/master/plugins) to extend it's capabilities. These plugins are enabled via configuration options. Please consult each plugin's README for more information:

* [S3 Plugin](plugins/s3) - Emit flushed metrics as a TSV file to Amazon S3
* [InfluxDB Plugin](plugins/influxdb) - Emit flushed metrics to InfluxDB (experimental)

# Setup

Here we'll document some explanations of setup choices you may make when using Veneur.

## Einhorn Usage

When you upgrade Veneur (deploy, stop, start with new binary) there will be a
brief period where Veneur will not be able to handle HTTP requests. At Stripe
we use [Einhorn](https://github.com/stripe/einhorn) as a shared socket manager to
bridge the gap until Veneur is ready to handle HTTP requests again.

You'll need to consult Einhorn's documentation for installation, setup and usage.
But once you've done that you can tell Veneur to use Einhorn by setting `http_address`
to `einhorn@0`. This informs [goji/bind](https://github.com/zenazn/goji/tree/master/bind) to use it's
Einhorn handling code to bind to the file descriptor for HTTP.

## Forwarding

Veneur instances can be configured to forward their global metrics to another Veneur instance. You can use this feature to get the best of both worlds: metrics that benefit from global aggregation can be passed up to a single global Veneur, but other metrics can be published locally with host-scoped information. Note: **Forwarding adds an additional delay to metric availability corresponding to the value of the `interval` configuration option**, as the local veneur will flush it to it's configured upstream, which will then flush any recieved metrics when it's interval expires.

If a local instance receives a histogram or set, it will publish the local parts of that metric (the count, min and max) directly to DataDog, but instead of publishing percentiles, it will package the entire histogram and send it to the global instance. The global instance will aggregate all the histograms together and publish their percentiles to DataDog.

Note that the global instance can also receive metrics over UDP. It will publish a count, min and max for the samples that were sent directly to it, but not counting any samples from other Veneur instances (this ensures that things don't get double-counted). You can even chain multiple levels of forwarding together if you want. This might be useful if, for example, your global Veneur is under too much load. The root of the tree will be the Veneur instance that has an empty `forward_address`. (Do not tell a Veneur instance to forward metrics to itself. We don't support that and it doesn't really make sense in the first place.)

With respect to the `tags` configuration option, the tags that will be added are those of the Veneur that actually publishes to DataDog. If a local instance forwards its histograms and sets to a global instance, the local instance's tags will not be attached to the forwarded structures. It will still use its own tags for the other metrics it publishes, but the percentiles will get extra tags only from the global instance.

### Consul Service Discovery and Consistent Hashing

If you use Consul for service discovery, Veneur can be configured to query it's API for instances of a service using `consul_forward_service_name`. Each **healthy** instance is then entered in to a hash ring. When choosing which host to forward to, Veneur will use a combination of metric name and tags to _consistently_ choose the same host.

Use the `consul_refresh_interval` to specify how often Veneur should refresh it's list.

#### Tracing

Consistent handling of tracing can also be used by setting `consul_trace_service_name`. The trace's ID — not the span, but the overall trace — is used for the choice of destination. So long as the hosts in the service stay consistent, this means that all of a trace's spans should arrive to the same destination host.

#### Concerns

* Veneur locks the list of servers when refreshing and flushing to avoid race conditions. If your retrieval of consul hosts (see metric `veneur.discoverer.update_duration_ns`) or flushes (see metric `veneur.flush.total_duration_ns`) are slow, you see one or the other slow down.
* Veneur uses a [consistent hash ring](https://en.wikipedia.org/wiki/Consistent_hashing) to try and mitigate the impact of changes in Consul's list of healthy nodes. This is not perfect, and you can expect some churn whenever the list of healthy nodes changes in Consul.

### Static Configuration

For static configuration you need one Veneur, which we'll call the _global_ instance, and one or more other Veneurs, which we'll call _local_ instances. The local instances should have their `forward_address` configured to the global instance's `http_address`. The global instance should have an empty `forward_address` (ie just don't set it). You can then report metrics to any Veneur's `udp_address` as usual.

### Magic Tag

If you want a metric to be strictly host-local, you can tell Veneur not to forward it by including a `veneurlocalonly` tag in the metric packet, eg `foo:1|h|#veneurlocalonly`. This tag will not actually appear in DataDog; Veneur removes it.

#### Counters

Relatedly, if you want to forward a counter to the global Veneur instance to reduce tag cardinality, you can tell Veneur to flush it to the global instance by including a `veneurglobalonly` tag in the count's metric packet. This tag will also not appear in Datadog. Note: for global counters to report correctly, the local and global Veneur instances should be configured to have the same flush interval.

#### Hostname and Device

Veneur also honors the same "magic" tags that the dogstatsd daemon includes in the datadog agent. The tag `host` will override `Hostname` in the metric and `device` will override `DeviceName`.

# Configuration

Veneur expects to have a config file supplied via `-f PATH`. The include `example.yaml` outlines the options:

* `api_hostname` - The Datadog API URL to post to. Probably `https://app.datadoghq.com`.
* `metric_max_length` - How big a buffer to allocate for incoming metric lengths. Metrics longer than this will get truncated!
* `flush_max_per_body` - how many metrics to include in each JSON body POSTed to Datadog. Veneur will POST multiple bodies in parallel if it goes over this limit. A value around 5k-10k is recommended; in practice we've seen Datadog reject bodies over about 195k.
* `debug` - Should we output lots of debug info? :)
* `hostname` - The hostname to be used with each metric sent. Defaults to `os.Hostname()`
* `omit_empty_hostname` - If true and `hostname` is empty (`""`) Veneur will *not* add a host tag to its own metrics.
* `interval` - How often to flush. Something like 10s seems good. **Note: If you change this, it breaks all kinds of things on Datadog's side. You'll have to change all your metric's metadata.**
* `key` - Your Datadog API key
* `percentiles` - The percentiles to generate from our timers and histograms. Specified as array of float64s
* `aggregates` - The aggregates to generate from our timers and histograms. Specified as array of strings, choices: min, max, median, avg, count, sum. Default: min, max, count
* `udp_address` - The address on which to listen for metrics. Probably `:8126` so as not to interfere with normal DogStatsD.
* `http_address` - The address to serve HTTP healthchecks and other endpoints. This can be a simple ip:port combination like `127.0.0.1:8127`. If you're under einhorn, you probably want `einhorn@0`.
* `forward_address` - The address of an upstream Veneur to forward metrics to. See below.
* `num_workers` - The number of worker goroutines to start.
* `num_readers` - The number of reader goroutines to start. Veneur supports SO_REUSEPORT on Linux to scale to multiple readers. On other platforms, this should always be 1; other values will probably cause errors at startup. See below.
* `read_buffer_size_bytes` - The size of the receive buffer for the UDP socket. Defaults to 2MB, as having a lot of buffer prevents packet drops during flush!
* `sentry_dsn` A [DSN](https://docs.sentry.io/hosted/quickstart/#configure-the-dsn) for [Sentry](https://sentry.io/), where errors will be sent when they happen.
* `stats_address` - The address to send internally generated metrics. Probably `127.0.0.1:8125`. In practice this means you'll be sending metrics to yourself. This is expected!
* `tags` - Tags to add to every metric that is sent to Veneur. Expects an array of strings!

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

* `veneur.packet.error_total` - Number of packets that Veneur could not parse due to some sort of formatting error by the client.
* `veneur.flush.post_metrics_total` - The total number of time-series points that will be submitted to Datadog via POST. Datadog's rate limiting is roughly proportional to this number.
* `veneur.forward.post_metrics_total` - Indicates how many metrics are being forwarded in a given POST request. A "metric", in this context, refers to a unique combination of name, tags and metric type.
* `veneur.*.content_length_bytes.*` - The number of bytes in a single POST body. Remember that Veneur POSTs large sets of metrics in multiple separate bodies in parallel. Uses a histogram, so there are multiple metrics generated depending on your local DogStatsD config.
* `veneur.flush.duration_ns` - Time taken for a single POST transaction to the Datadog API. Tagged by `part` for each sub-part `marshal` (assembling the request body) and `post` (blocking on an HTTP response).
* `veneur.forward.duration_ns` - Same as `flush.duration_ns`, but for forwarding requests.
* `veneur.flush.total_duration_ns` - Total time spent POSTing to Datadog, across all parallel requests. Under most circumstances, this should be roughly equal to the total `veneur.flush.duration_ns`. If it's not, then some of the POSTs are happening in sequence, which suggests some kind of goroutine scheduling issue.
* `veneur.flush.error_total` - Number of errors received POSTing to Datadog.
* `veneur.forward.error_total` - Number of errors received POSTing to an upstream Veneur. See also `import.request_error_total` below.
* `veneur.flush.worker_duration_ns` - Per-worker timing — tagged by `worker` - for flush. This is important as it is the time in which the worker holds a lock and is unavailable for other work.
* `veneur.worker.metrics_processed_total` - Total number of metric packets processed between flushes by workers, tagged by `worker`. This helps you find hot spots where a single worker is handling a lot of metrics. The sum across all workers should be approximately proportional to the number of packets received.
* `veneur.worker.metrics_flushed_total` - Total number of metrics flushed at each flush time, tagged by `metric_type`. A "metric", in this context, refers to a unique combination of name, tags and metric type. You can use this metric to detect when your clients are introducing new instrumentation, or when you acquire new clients.
* `veneur.worker.metrics_imported_total` - Total number of metrics received via the importing endpoint. A "metric", in this context, refers to a unique combination of name, tags, type _and originating host_. This metric indicates how much of a Veneur instance's load is coming from imports.
* `veneur.import.response_duration_ns` - Time spent responding to import HTTP requests. This metric is broken into `part` tags for `request` (time spent blocking the client) and `merge` (time spent sending metrics to workers).
* `veneur.import.request_error_total` - A counter for the number of import requests that have errored out. You can use this for monitoring and alerting when imports fail.

### Service Discovery

If you use service discovery (e.g. Consul) for forwarding or tracing, these metrics will be useful to you. Each of these is tagged with `service` that has a value matching the service name supplied via the config:

* `veneur.discoverer.destination_number` - A gauge containing the number of hosts Veneur discovered and added to the hash ring.
* `veneur.discoverer.errors` - A counter tracking the number of times the service discovery mechanism has failed to return *any* hosts. Note that Veneur will refuse to update it's list if there are 0 returned hosts and may use stale results until such as as > 1 host is returned.
* `veneur.discoverer.update_duration_ns` - A timer describing the duration of service discovery calls.

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

# Name

The [veneur](https://en.wikipedia.org/wiki/Grand_Huntsman_of_France) is a person acting as superintendent of the chase and especially
of hounds in French medieval venery and being an important officer of the royal household. In other words, it is the master of dogs. :)
