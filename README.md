<div align="center">
  <img src="https://raw.githubusercontent.com/stripe/veneur/gh-pages/veneur_logo.svg?sanitize=true">
</div>

[![Build Status](https://travis-ci.org/stripe/veneur.svg?branch=master)](https://travis-ci.org/stripe/veneur)
[![GoDoc](https://godoc.org/github.com/stripe/veneur?status.svg)](http://godoc.org/github.com/stripe/veneur)

# Table of Contents

   * [What Is Veneur?](#what-is-veneur)
   * [Status](#status)
   * [Features](#features)
      * [Vendor And Backend Agnostic](#vendor-and-backend-agnostic)
      * [Modern Metrics Format (Or Others!)](#modern-metrics-format-or-others)
      * [Global Aggregation](#global-aggregation)
      * [Approximate Histograms](#approximate-histograms)
      * [Approximate Sets](#approximate-sets)
      * [Global Counters](#global-counters)
   * [Concepts](#concepts)
      * [By Metric Type Behavior](#by-metric-type-behavior)
      * [Expiration](#expiration)
      * [Other Notes](#other-notes)
   * [Usage](#usage)
   * [Setup](#setup)
      * [Clients](#clients)
      * [Einhorn Usage](#einhorn-usage)
      * [Forwarding](#forwarding)
         * [Proxy](#proxy)
         * [Static Configuration](#static-configuration)
         * [Magic Tag](#magic-tag)
            * [Global Counters And Gauges](#global-counters-and-gauges)
            * [Routing metrics](#routing-metrics)
   * [Configuration](#configuration)
      * [Configuration via Environment Variables](#configuration-via-environment-variables)
   * [Monitoring](#monitoring)
      * [At Local Node](#at-local-node)
         * [Forwarding](#forwarding-1)
      * [At Global Node](#at-global-node)
      * [Metrics](#metrics)
      * [Error Handling](#error-handling)
   * [Performance](#performance)
      * [Benchmarks](#benchmarks)
      * [SO_REUSEPORT](#so_reuseport)
      * [TCP connections](#tcp-connections)
      * [TLS encryption and authentication](#tls-encryption-and-authentication)
         * [Performance implications of TLS](#performance-implications-of-tls)
   * [Name](#name)

# What Is Veneur?

Veneur  (`/vɛnˈʊr/`, rhymes with “assure”) is a distributed, fault-tolerant pipeline for runtime data. It provides a server implementation of the [DogStatsD protocol](http://docs.datadoghq.com/guides/dogstatsd/#datagram-format) or [SSF](https://github.com/stripe/veneur/tree/master/ssf) for aggregating metrics and sending them to downstream storage to one or more supported sinks. It can also act as a [global aggregator](#global-aggregation) for histograms, sets and counters.

More generically, Veneur is a convenient sink for various observability primitives with lots of outputs!

See also:

* A unified, standard format for observability primitives, the [SSF](https://github.com/stripe/veneur/tree/master/ssf/#readme)
* A proxy for resilient distributed aggregation, [veneur-proxy](https://github.com/stripe/veneur/tree/master/cmd/veneur-proxy/#readme)
* A command line tool for emitting metrics, [veneur-emit](https://github.com/stripe/veneur/tree/master/cmd/veneur-emit/#readme)
* A poller for scraping Prometheus metrics, [veneur-prometheus](https://github.com/stripe/veneur/tree/master/cmd/veneur-prometheus/#readme)
* The [sinks supported by Veneur](https://github.com/stripe/veneur/tree/master/sinks#readme)

We wanted percentiles, histograms and sets to be global. We wanted to unify our observability clients, be vendor agnostic and build automatic features like SLI measurement. Veneur helps us do all this and more!

# Status

Veneur is currently handling all metrics for Stripe and is considered production ready. It is under active development and maintenance! Starting with v1.6, Veneur operates on a six-week release cycle, and all releases are tagged in git. If you'd like to contribute, see [CONTRIBUTING](https://github.com/stripe/veneur/blob/master/CONTRIBUTING.md)!

Building Veneur requires Go 1.8 or later.

# Features

## Vendor And Backend Agnostic

Veneur has many [sinks](https://github.com/stripe/veneur/tree/master/sinks#readme) such that your data can be sent one or more vendors, TSDBs or tracing stores!

## Modern Metrics Format (Or Others!)

Unify metrics, spans and logs via the [Sensor Sensibility Format](https://github.com/stripe/veneur/tree/master/ssf). Also works with [DogStatsD](https://github.com/stripe/veneur/tree/master/sinks/datadog#readme), StatsD and [Prometheus](https://github.com/stripe/veneur/tree/master/cmd/veneur-prometheus/#readme).

## Global Aggregation

If configured to do so, Veneur can selectively aggregate global metrics to be cumulative across all instances that report to a central Veneur, allowing global percentile calculation, global counters or global sets.

For example, say you emit a timer `foo.bar.call_duration_ms` from 20 hosts that are configured to forward to a central Veneur. You'll see the following:

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

Datadog's DogStatsD — and StatsD — uses an exact histogram which retains all samples and is reset every flush period. This means that there is a loss of precision when using Veneur, but the resulting percentile values are meant to be more representative of a global view.

## Approximate Sets

Veneur uses [HyperLogLogs](https://github.com/clarkduvall/hyperloglog) for approximate unique sets. These are a very efficient unique counter with fixed memory consumption.

## Global Counters

Via an optional [magic tag](#magic-tag) Veneur will forward counters to a global host for accumulation. This feature was primarily developed to control tag cardinality. Some counters are valuable but do not require per-host tagging.

# Concepts

* Global metrics are those that benefit from being aggregated for chunks — or all — of your infrastructure. These are histograms (including the percentiles generated by timers) and sets.
* Metrics that are sent to another Veneur instance for aggregation are said to be "forwarded". This terminology helps to decipher configuration and metric options below.
* Flushed, in Veneur, means metrics or spans processed by a sink.

## By Metric Type Behavior

To clarify how each metric type behaves in Veneur, please use the following:
* Counters: Locally accrued, flushed to sinks (see [magic tags](#magic-tag) for global version)
* Gauges: Locally accrued, flushed to sinks  (see [magic tags](#magic-tag) for global version)
* Histograms: Locally accrued, count, max and min flushed to sinks, percentiles forwarded to `forward_address` for global aggregation when set.
* Timers: Locally accrued, count, max and min flushed to sinks, percentiles forwarded to `forward_address` for global aggregation when set.
* Sets: Locally accrued, forwarded to `forward_address` for sinks aggregation when set.

## Expiration

Veneur expires all metrics on each flush. If a metric is no longer being sent (or is sent sparsely) Veneur will not send it as zeros! This was chosen because the combination of the approximation's features and the additional hysteresis imposed by *retaining* these approximations over time was deemed more complex than desirable.

## Other Notes

* Veneur aligns its flush timing with the local clock. For the default interval of `10s` Veneur will generally emit metrics at 00, 10, 20, 30, … seconds after the minute.
* Veneur will delay it's first metric emission to align the clock as stated above. This may result in a brief quiet period on a restart at worst < `interval` seconds long.

# Usage

```
veneur -f example.yaml
```

See example.yaml for a sample config. Be sure to set your `datadog_api_key`!

# Setup

Here we'll document some explanations of setup choices you may make when using Veneur.

## Clients

Veneur is capable of ingesting:

* [DogStatsD](https://docs.datadoghq.com/guides/dogstatsd/) including events and service checks
* [SSF](https://github.com/stripe/veneur/tree/master/ssf)
* StatsD as a subset of DogStatsD, but this may cause trouble depending on where you store your metrics.

To use clients with Veneur you need only configure your client of choice to the proper host and port combination. This port should match one of:

* `statsd_listen_addresses` for UDP- and TCP-based clients
* `ssf_listen_addresses` for SSF-based clients using UDP or UNIX domain sockets.

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

For static configuration you need one Veneur, which we'll call the _global_ instance, and one or more other Veneurs, which we'll call _local_ instances. The local instances should have their `forward_address` configured to the global instance's `http_address`. The global instance should have an empty `forward_address` (ie just don't set it). You can then report metrics to any Veneur's `statsd_listen_addresses` as usual.

### Magic Tag

If you want a metric to be strictly host-local, you can tell Veneur not to forward it by including a `veneurlocalonly` tag in the metric packet, eg `foo:1|h|#veneurlocalonly`. This tag will not actually appear in DataDog; Veneur removes it.

#### Global Counters And Gauges

Relatedly, if you want to forward a counter or gauge to the global Veneur instance to reduce tag cardinality, you can tell Veneur to flush it to the global instance by including a `veneurglobalonly` tag in the metric's packet. This `veneurglobalonly` tag is stripped and will not be passed on to sinks.

**Note**: For global counters to report correctly, the local and global Veneur instances should be configured to have the same flush interval.

**Note**: Global gauges are "random write wins" since they are merged in a non-deterministic order at the global Veneur.

#### Routing metrics

Veneur supports specifying that metrics should only be routed to a specific metric sink, with the `veneursinkonly:<sink_name>` tag. The `<sink_name>` value can be any configured metric sink. Currently, that's `datadog`, `kafka`, `signalfx`. It's possible to specify multiple sink destination tags on a metric, which will cause the metric to be routed to each sink specified.

# Configuration

Veneur expects to have a config file supplied via `-f PATH`. The included [example.yaml](https://github.com/stripe/veneur/blob/master/example.yaml) explains all the options!

## Configuration via Environment Variables

Veneur and veneur-proxy each allow configuration via environment variables using [envconfig](https://github.com/kelseyhightower/envconfig). Options provided via environment variables take precedent over those in config. This allows stuff like:

```
VENEUR_DEBUG=true veneur -f someconfig.yml
```

**Note**: The environment variables used for configuration map to the field names in [config.go](https://github.com/stripe/veneur/blob/master/config.go), capitalized, with the prefix `VENEUR_`. For example, the environment variable equivalent of `datadog_api_hostname` is `VENEUR_DATADOGAPIHOSTNAME`.

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

Set `statsd_listen_addresses`, `tls_key`, `tls_certificate`, and `tls_authority_certificate`:

```
statsd_listen_addresses:
  - "tcp://localhost:8129"
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
