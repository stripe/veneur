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

# Status

Veneur is currently a work in progress and thus should not be relied on for production traffic.
