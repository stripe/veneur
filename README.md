[![Build Status](https://travis-ci.org/stripe/veneur.svg?branch=master)](https://travis-ci.org/stripe/veneur)
[![GoDoc](https://godoc.org/github.com/stripe/veneur?status.svg)](http://godoc.org/github.com/stripe/veneur)

Veneur (venn-urr) is a server implementation of the [DogStatsD protocol](http://docs.datadoghq.com/guides/dogstatsd/#datagram-format), which is a superset of the StatsD protocol.

# Motivation

Veneur's intended use is as a standalone server to which multiple DogStatsD clients report, such that the metrics are *global*
rather than host-local. This is particularly useful for histograms, timers and sets, as in their normal, per-host configuration the
percentiles for histograms can be less effective or even meaningless. Per-host unique sets are also often not what's desired.

Global \*StatsD installations can be problematic, as they either require client-side or proxy sharding behavior to prevent an
instance being a Single Point of Failure (SPoF) for all metrics. Veneur isn't much different, but attempts to lessen the risk
by being simple and fast. It is advised that you only use Veneur for metric types for which it is beneficial (i.e. histograms, timers
and sets) even though it supports other metric types.

# Features

## Internal Metrics

Veneur assumes you have a running DogStatsD on the localhost and emits metrics to it's default port of 8125. Those metrics are:

* `veneur.packet.error_total` - Number of packets that Veneur could not parse due to some sort of formatting error by the client.
* `veneur.flush.error_total` - Number of errors when attempting to POST metrics to Datadog.
* `veneur.flush.metrics_total` - Total number of metrics flushed at each flush time, tagged by `metric_type`.
* `veneur.flush.transaction_duration_ns` - Time taken to POST metrics to Datadog.
* `veneur.flush.worker_duration_ns` - Per-worker timing — tagged by `worker` - for flush. This is important as it is the time in which the worker holds a lock and is unavailable for other work.
* `veneur.worker.metrics_processed_total` - Total number of metrics processed between flushes by workers, tagged by `worker`. This helps you find hot spots where a single worker is handling a lot of metrics.

# Status

Veneur is currently a work in progress and thus should not yet be relied on for production traffic.

# Usage
```
venuer -f example.yaml
```

See example.yaml for a sample config. Be sure and set your Datadog API `key`!

# Configuration

Veneur expects to have a config file supplied via `-f PATH`. The include `example.yaml` outlines the options:

* `api_hostname` - The Datadog API URL to post to. Probably `https://app.datadoghq.com`.
* `metric_max_length` - How big a buffer to allocate for incoming metric lengths. Metrics longer than this will get truncated!
* `debug` - Should we output lots of debug info? :)
* `hostname` - The hostname to be used with each metric sent. Defaults to `os.Hostname()`
* `interval` - How often to flush. Something like 10s seems good.
* `key` - Your Datadog API key
* `percentiles` - The percentiles to generate from our timers and histograms. Specified as array of float64s
* `publish_histogram_counters` - Veneur can publish a counter, `$name.count`, for how many samples a histogram has received. Note that this counter, like every other metric passing through veneur, will be attached to Veneur's own host.
* `udp_address` - The address on which to listen for metrics. Probably `:8126` so as not to interfere with normal DogStatsD.
* `num_workers` - The number of worker goroutines to start.
* `read_buffer_size_bytes` - The size of the receive buffer for the UDP socket. Defaults to 2MB, as having a lot of buffer prevents packet drops during flush!
* `set_size` - The cardinality of the set you'll using with sets. Too small will cause decreased accuracy.
* `set_accuracy` - The approximate accuracy of set's approximations. More accuracy uses more memory.
* `stats_address` - The address to send internally generated metrics. Probably `127.0.0.1:8125` to send to a local DogStatsD
* `tags` - Tags to add to every metric that is sent to Veneur. Expects an array of strings!

# How Veneur Is Different Than Official DogStatsD

Veneur is different for a few reasons. They enumerated here.

## Approximate Histograms

Because Veneur is built to handle lots and lots of data, it uses approximate histograms.

Specifically the streaming approximate histograms
 implementation from [VividCortex/metrics-go](https://github.com/VividCortex/gohistogram). Metrics are consistently routed to the same worker to distribute load and to be added to the same histogram.

Datadog's DogStatsD — and StatsD — uses an exact histogram which retains all samples and is reset every flush period. This means that there is a loss of precision when using Veneur, but
the resulting percentile values are meant to be more representative of a global view.

## Approximate Sets

Veneur uses [Bloom filters](https://github.com/willf/bloom) for approximate unique sets. Configured via `set_size` and `set_accuracy`
discussed above an approximate unique count is generated at each flush.

## Lack of Host Tags

By definition the hostname is not applicable to metrics that Veneur processes. Note that if you
do include a hostname tag, Veneur will **not** strip it for you. Veneur will add it's own hostname as configured to metrics sent to Datadog.

## Expiration

Veneur expires all metrics on each flush. If a metric is no longer being sent (or is sent sparsely) Veneur will not send it as zeros! This was chosen because the combination of the approximation's features and the additional hysteresis imposed by *retaining* these approximations over time was deemed more complex than desirable.

# Performance

Processing packets quickly is the name of the game.

## Benchmarks

Veneur aims to be highly performant. In local testing running on a 8-core i7-2600K
with 16GB of RAM and `workers: 96` in it's config, Veneur was able to sustain ~150k metrics processed per second with no drops on the loopback interface,
with flushes every 10 seconds. Tests used ~24,000 metric name & tag combinations.

This chart shows the number of packets processed per second as well as a nice flat zero for the number of errors per second in dropped packets.

![Benchmark](/benchmark.png?raw=true "Benchmark")

Box load was around 8, memory usage can be seen here from `htop`:

![Memory Usage](/memory.png?raw=true "Memory Usage")

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

# Name

The [veneur](https://en.wikipedia.org/wiki/Grand_Huntsman_of_France) is a person acting as superintendent of the chase and especially
of hounds in French medieval venery and being an important officer of the royal household. In other words, it is the master of dogs. :)
