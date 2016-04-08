[![Build Status](https://travis-ci.org/gphat/veneur.svg?branch=master)](https://travis-ci.org/gphat/veneur)

Veneur (venn-urr) is a server implementation of the [DogStatsD protocol](http://docs.datadoghq.com/guides/dogstatsd/#datagram-format), which is a superset of the StatsD protocol.

# Motivation

Veneur's intended use is as a standalone server to which multiple DogStatsD clients report, such that the metrics are *global*
rather than host-local. This is particularly useful for histograms and timers, as in their normal, per-host configuration the
percentiles for histograms can be less effective or even meaningless.

Global \*StatsD installations can be problematic, as they either require client-side or proxy sharding behavior to prevent an
instance being a Single Point of Failure (SPoF) for all metrics. Veneur isn't much different, but attempts to lessen the risk
by being simple and fast. It is advised that you only use Veneur for metric types for which it is beneficial (i.e. histograms and timers)
even though it supports other metric types.

# Features

## Internal Metrics

Veneur assumes you have a running DogStatsD on the localhost and emits metrics to it's default port of 8125. Those metrics are:

* `veneur.packet.error_total` - Number of packets that Veneur could not parse.
* `veneur.packet.received_total` - Number of packets received. Sample rate configurable.
* `veneur.flush.error_total` - Number of errors when attempting to POST metrics to Datadog.
* `veneur.flush.metrics_total` - Total number of metrics flushed at each flush time.
* `veneur.flush.transaction_duration_ns` - Time taken to POST metrics to Datadog.
* `veneur.flush.worker_duration_ns` - Per-worker timing — tagged with `worker` - for flush.

# Status

Veneur is currently a work in progress and thus should not yet be relied on for production traffic.

# TODO

* Add option to flush histograms at each flush?
* Config file
  * Configuration of percentiles for histograms
  * Global tags, added to all metrics

# Usage
```
Usage of veneur:
  -apiurl string
    	The URL to which Metrics will be posted (default "https://app.datadoghq.com")
  -buffersize int
    	The size of the buffer of work for the worker pool (default 4096)
  -d	Enable debug logging
  -expiry duration
    	The duration metrics will be retained if they stop showing up, see go's ParseDuration (default 5m0s)
  -i duration
    	The interval at which to flush metrics, see go's ParseDuration (default 10s)
  -key string
    	Your Datadog API Key (default "fart")
  -listen string
    	Address to listen for UDP requests on (default ":8126")
  -n int
    	The number of workers to start (default 4)
  -stats string
    	Address of DogStatsD instance to send internal metrics (default "localhost:8125")
```

# How Veneur Is Different Than Official DogStatsD

Veneur is different for a few reasons. They enumerated here.

## Approximate Histograms

Because Veneur is built to handle lots and lots of data, it uses approximate histograms.

Specifically the [forward-decaying priority reservoir](http://www.research.att.com/people/Cormode_Graham/library/publications/CormodeShkapenyukSrivastavaXu09.pdf)
 implementation from [rcrowley/metrics-go](https://github.com/rcrowley/go-metrics/). Metrics are consistently routed to the same worker to distribute load and to be added to the same histogram. There is [documentation for it's memory usage](https://github.com/rcrowley/go-metrics/blob/master/memory.md#50000-histograms-with-a-uniform-sample-size-of-1028) as well.

 Per [Dropwizard's documentation](https://dropwizard.github.io/metrics/3.1.0/apidocs/com/codahale/metrics/ExponentiallyDecayingReservoir.html), the reservoir size defaults to 1028 with an alpha of 0.015:

 > which offers a 99.9% confidence level with a 5% margin of error assuming a normal distribution, … which heavily biases the reservoir to the past 5 minutes of measurements.

Datadog's DogStatsD — and StatsD — uses an exact histogram which retains all samples and is reset every flush period. This means that there is a loss of precision when using Veneur, but
the resulting percentile values are meant to be more representative of a global view.

## Lack of Host Tags

By definition the hostname is not applicable to metrics that Veneur processes. Note that if you
do include a hostname tag, Veneur will **not** strip it for you.

# Name

The [veneur](https://en.wikipedia.org/wiki/Grand_Huntsman_of_France) is a person acting as superintendent of the chase and especially
of hounds in French medieval venery and being an important officer of the royal household. In other words, it is the master of dogs. :)
