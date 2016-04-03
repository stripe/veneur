Veneur (venn-urr) is a server implementation of the [DogStatsD protocol](http://docs.datadoghq.com/guides/dogstatsd/#datagram-format), which is a superset of the StatsD protocol.

# Motivation

Veneur's intended use is as a standalone server to which multiple DogStatsD clients report, such that the metrics are *global*
rather than host-local. This is particularly useful for histograms and timers, as in their normal, per-host configuration the
percentiles for histograms can be less effective or even meaningless.

# How It Works

Global \*StatsD installations can be problematic, as they either require client-side or proxy sharding behavior to prevent an
instance being a Single Point of Failure (SPoF) for all metrics. Veneur isn't much different, but attempts to lessen the risk
by being simple and fast. It is advised that you only use Veneur for metric types for which it is beneficial (histograms) even
though it supports other metric types.

## Histogram Implementation

Veneur uses [rcrowley/metrics-go](https://github.com/rcrowley/go-metrics/)'s histogram, specifically the [forward-decaying
priority reservoir](http://www.research.att.com/people/Cormode_Graham/library/publications/CormodeShkapenyukSrivastavaXu09.pdf)
 implementation. Metrics are consistently routed to the same worker to distribute load and to be added to the same histogram.

# Usage
```
Usage of /Users/gphat/src/veneur/bin/veneur:
  -http string
    	Address to listen for UDP requests on (default ":8125")
  -i int
    	The interval in seconds at which to flush metrics (default 10)
  -key string
    	Your Datadog API Key (default "fart")
  -n int
    	The number of workers to start (default 4)
```

# Status

Veneur is currently a work in progress and thus should not be relied on for production traffic.

# Name

The [veneur](https://en.wikipedia.org/wiki/Grand_Huntsman_of_France) is a person acting as superintendent of the chase and especially
of hounds in French medieval venery and being an important officer of the royal household. In other words, it is the master of dogs. :)
