# Datadog Sink

This sink sends Veneur metrics to [Datadog](https://www.datadoghq.com).

# Configuration

See the various `datadog_*` keys in [example.yaml](https://github.com/stripe/veneur/blob/master/example.yaml) for all available configuration options.

# Status

**This sink is stable**.

# Capabilities

Veneur began life as an alternative implementation of [DogStatsD](http://docs.datadoghq.com/guides/dogstatsd/#datagram-format).

Veneur is a DogStatsD implementation that acts as a local collector and — optionally — as an aggregator for
some metric types, such that the metrics are *global* rather than host-local. This is particularly useful for histograms,
timers and sets, as in their normal, per-host configuration the percentiles for histograms can be less effective or even
meaningless. Per-host unique sets are also often not what's desired.

Global \*StatsD installations can be problematic, as they either require client-side or proxy sharding behavior to prevent an
instance being a Single Point of Failure (SPoF) for all metrics. Veneur aims to solve this problem. Non-global
metrics like counters and gauges are collected by a local Veneur instance and sent to storage at flush time. Global metrics (histograms and sets)
are forwarded to a central Veneur instance for aggregation before being sent to storage.

## Replacing Datadog Agent

Veneur can act as a replacement for the stock DogStatsD. In our environment we disable the Datadog DogStatsD port by setting `use_dogstatsd` to false. We run Veneur on an alternate port (8200) so as to not accidentally ingest metrics from legacy StatsD clients and we configure our Datadog agent to use this port by `dogstatsd_port` to 8200 to match.

## How Veneur Is Different Than Official DogStatsD

Veneur is different for a few reasons. They are enumerated here.

### Protocol

Veneur adheres to [the official DogStatsD datagram format](http://docs.datadoghq.com/guides/dogstatsd/#datagram-format) with the exceptions below:

* The tag `veneurlocalonly` is stripped and influences forwarding behavior, as discussed below.
* The tag `veneurglobalonly` is stripped and influences forwarding behavior, as discussed below.

## Lack of Host Tags for Aggregated Metrics

By definition the hostname is not applicable to *global* metrics that Veneur processes. Note that if you
do include a hostname tag, Veneur will **not** strip it for you. Veneur will add its own hostname as configured to metrics sent to Datadog.

## Metrics

Enabled if `datadog_api_hostname` and `datadog_api_key` are set to non-empty
values.

* Counters are converted to second-normalized rates with an interval matching the server's `interval`.
* Gauges are gauges.

The following tags are mapped to Datadog fields as follows:

* The tag `host` to `hostname`
* The tag `device` to `device_name`

### Compressed, Chunked POST

Datadog's API is tuned for small POST bodies from lots of hosts since they work on a per-host basis. Also there are limits on the size of the body that
can be POSTed. As a result Veneur chunks metrics in to smaller bits — governed by `datadog_flush_max_per_body` — and sends them (compressed) concurrently to
Datadog. This is essential for reasonable performance as Datadog's API seems to be somewhat `O(n)` with the size of the body (which is proportional
to the number of metrics).

We've found that our hosts generate around 5k metrics and have reasonable performance, so in our case 5k is used as the `datadog_flush_max_per_body`.

## Spans

Enabled if `datadog_trace_api_address` and `datadog_api_key` are set to non-empty
values.

Spans are sent to [Datadog APM](https://www.datadoghq.com/apm/). The following
rules manage how [SSF](https://github.com/stripe/veneur/tree/master/ssf) spans
and tags are mapped to Datadog [trace fields](https://docs.datadoghq.com/api/#tracing):

* The SSF field `name` is mapped to the trace's `name` field.
* The tag `resource` is removed and mapped to the trace's `resource` field.
* The `type` field is currently hardcoded to "web".
* The SSF field `error` is mapped to the trace's `error` field.
* Remaining tags are mapped to the trace's `meta` dictionary.

### Span Retention

Veneur allocates a ring buffer of `datadog_span_buffer_size` entries which is flushed
every `interval`. Being a ring buffer, the oldest spans will be overwritten
until it `interval` expires and the ring buffer is reset.

## Other

As a side-effect of implementing [DogStatsD](https://docs.datadoghq.com/developers/dogstatsd/)
Veneur parses both [Service Checks](https://docs.datadoghq.com/api/#service-checks)
and [Events](https://docs.datadoghq.com/api/#events).
