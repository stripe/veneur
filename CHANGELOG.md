# 3.0.0, in development

## Incompatible changes
* These deprecated configuration keys are no longer supported and will cause an error on startup if used in a veneur config file:
** `api_hostname` - replaced in 1.5.0 by `datadog_api_hostname`
** `key` - replaced in 1.5.0 by `datadog_api_key`
** `trace_address` - replaced in 1.7.0 by `ssf_listen_addresses`
** `trace_api_address` - replaced in 1.5.0 by `datadog_trace_api_address`
** `ssf_address` - replaced in 1.7.0 by `ssf_listen_addresses`
** `tcp_address` and `udp_address` - replaced in 1.7.0 by `statsd_listen_addresses`
* These metrics have changed names:
** Datadog, MetricExtraction, and SignalFx sinks now emit `veneur.sink.metric_flush_total_duration_ns` for metric flush duration and tag it with `sink`
** Datadog, Kafka, MetricExtraction, and SignalFx sinks now emits `sink.metrics_flushed_total` for metric flush counts and tag it with `sink`
** Datadog and LightStep sinks now emit `veneur.sink.span_flush_total_duration_ns` for span flush duration and tag it with `sink`
** Datadog, Kafka, MetricExtraction, and LightStep sinks now emit `sink.spans_flushed_total` for metric flush counts and tag it with `sink`
* Veneur's internal metrics are no longer tagged with `veneurlocalonly`. This means that percentile metrics (such as timers) will now be aggregated globally.

## Bugfixes
* LightStep sink was hardcoded to use plaintext, now adjusts based on URL scheme (http versus https). Thanks [gphat](https://github.com/gphat)!

## Added
* `veneur-emit` now infers parent and trace IDs from the environment (using the variables `VENEUR_EMIT_TRACE_ID` and `VENEUR_EMIT_PARENT_SPAN_ID`) and sets these environment variables from its `-trace_id` and `parent_span_id` when timing commands, allowing for convenient construction of trace trees if traced programs call `veneur-emit` themselves. Thanks, [antifuchs](https://github.com/antifuchs)
* The Kafka sink for spans can now sample spans (at a rate determined by `kafka_span_sample_rate_percent`) based off of traceIDs (by default) or a tag's values (configurable via `kafka_span_sample_tag`) to consistently sample spans related to each other. Thanks, [rhwlo](https://github.com/rhwlo)!
* Improvements in SSF metrics reporting - thanks, [antifuchs](https://github.com/antifuchs)!
** Function `ssf.RandomlySample` that takes an array of samples and a sample rate and randomly drops those samples, adjusting the kept samples' rates
** New variable `ssf.NamePrefix` that can be used to prepend a common name prefix to SSF sample names.
** Package `trace/metrics`, containing functions that allow reporting metrics through a trace client.
** New type `ssf.Samples` holding a batch of samples which can be submitted conveniently through `trace/metrics`.
** Method `trace.(*Trace).Add`, which allows adding metrics to a trace span.
* `veneur-proxy` has a new configuration option `forward_timeout` which allows specifying how long forwarding a batch to global veneur servers may take in total. Thanks, [antifuchs](https://github.com/antifuchs)!
* Add native support for running Veneur within Kubernetes. Thanks, [aditya](https://github.com/chimeracoder)!

## Improvements
* Updated Datadog span sink to latest version in Datadog tracing agent. Thanks, [gphat](https://github.com/gphat)!

# 2.0.0, 2018-01-09

## Incompatible changes
* The semantics around `veneur-emit` command timing have changed: `-shellCommand` argument has been renamed to `-command`, and `-command` is now gone. The only way to time a command is to provide the command and its arguments as separate arguments, the method of passing in a shell-escaped string is no longer supported.

## Added
* A [SignalFx sink](https://github.com/stripe/veneur/tree/master/sinks/signalfx) has been added for flushing metrics to [SignalFx](https://signalfx.com/). Thanks, [gphat](https://github.com/gphat)!
* A [Kafka sink](https://github.com/stripe/veneur/tree/master/sinks/kafka) has been added for publishing spans or metrics. Thanks, [parabuzzle](https://github.com/parabuzzle) and [gphat](https://github.com/gphat)!
* Buffered trace clients in `github.com/stripe/veneur/trace` now have a new option to automatically flush them in a periodic interval. Thanks, [antifuchs](https://github.com/antifuchs)!
* Gauges can now be marked as `veneurglobalonly` to be globally "last write wins". Thanks [gphat](https://github.com/gphat)!
* `veneur-emit` now takes `-span_service`, `-trace_id`, `-parent_span_id`, and `-indicator` arguments. These (combined with `-ssf`) allow submitting spans for tracing when recording timing data for commands. In addition, `-timing` with `-ssf` now works, too.
* The `github.com/stripe/veneur/ssf` package now has a few helper functions to create samples that can be attached to spans: `Count`, `Gauge`, `Histogram`, `Timing`. In addition, veneur now has a span sink that extracts these samples and treats them as metrics. Thanks, [antifuchs](https://github.com/antifuchs)
* Spans sent to Lightstep now have an `indicator` tag set, indicating whether the span is an indicator span or not. Thanks, [aditya](https://github.com/chimeracoder)!
* `veneur-emit -command` now streams output from the invoked program's stdout/stderr to its own stdout/stderr. Thanks, [antifuchs](https://github.com/antifuchs)!
* Veneur now supports the tag `veneursinkonly:<sink_name>` on metrics, which routes the metric to only the sink specified. See [the docs](README.md#routing-metrics) here. Thanks, [antifuchs](https://github.com/antifuchs)!

## Improvements
* Veneur now emits a timer metric giving the duration (in nanoseconds) of every "indicator" span that it receives, if you configure the setting `indicator_span_timer_name`. Thanks, [antifuchs](https://github.com/antifuchs)!
* All sinks have been moved to their own packages for smaller code and better interfaces. Thanks [gphat](https://github.com/gphat)!
* Removed noisy Sentry events that duplicated Datadog error reporting. Thanks, [aditya](https://github.com/chimeracoder)!
* Veneur now reuses HTTP connections for forwarding and Datadog flushes. Furthermore each phase of the HTTP request is traced and can be seen using the trace sink of your choice. This should drastically improve performance and reliability for installs with large numbers of instances. Thanks [gphat](https://github.com/gphat)!

# 1.8.1, 2017-12-05

## Improvements
* Veneur now tracks statsd metrics for SSF spans concerning its own operation. This means that the `veneur.ssf.spans.received_total` counter and the `veneur.ssf.packet_size` histogram again reflect trace spans routed internally. Thanks, [antifuchs](https://github.com/antifuchs)!

# 1.8.0, 2017-11-29

## Added
* New 'blackhole' sink for testing and benchmark purposes. Thanks [gphat](https://github.com/gphat)!

## Improvements
* Veneur no longer **requires** the use of Datadog as a target for flushes. Veneur can now use one or more of any of its supported sinks as a backend. This realizes our desire for Veneur to be fully vendor agnostic. Thanks [gphat](https://github.com/gphat)!
* The package `github.com/stripe/veneur/trace` now depends on fewer other packages across veneur, making it easier to pull in `trace` as a dependency. Thanks [antifuchs](https://github.com/antifuchs)!
* A Veneur server with tracing enabled now submits traces and spans concerning its own operation to itself internally without sending them over UDP. See the "Upgrade Notes" section below for metrics affected by this change. Thanks [antifuchs](https://github.com/antifuchs)!
* veneur-prometheus and veneur-proxy executables are now included in the docker images. Thanks [jac](https://github.com/jac-stripe)
* All Veneur executables are now in $PATH in the docker images. Thanks [jac](https://github.com/jac-stripe)
* When using Lightstep as a tracing sink, spans can be load-balanced more evenly across collectors by configuring the `trace_lightstep_num_clients` option to multiplex across multiple clients. Thanks [aditya](https://github.com/chimeracoder)!
* sync_with_interval is a new configuration option! If enabled, when starting, Veneur will now delay its first metric to be aligned with an `interval` boundary on the local clock. This will effectively "synchronize" Veneur instances across your deployment assuming reasonable clock behavior. The result is a metric timestamps in your TSDB that mostly line up improving bucketing behavior. Thanks [gphat](https://github.com/gphat)!
* Cleaned up some linter warnings. Thanks [gphat](https://github.com/gphat)!
* Tests no longer depend on implicit presence of a Datadog metric or span sink. Thanks [gphat](https://github.com/gphat)!
* Refactor internal HTTP helper into its own package fix up possible circular deps. Thanks [gphat](https://github.com/gphat)!

## Bugfixes
* Fix a panic when using `veneur-emit` to emit metrics via `-ssf` when no tags are specified. Thanks [myndzi](https://github.com/myndzi)
* Remove spurious warnings about unset configuration settings. Thanks [antifuchs](https://github.com/antifuchs)

## Removals
* Removed the InfluxDB plugin as it was experimental and wasn't working. We can make a sink for it in the future if desired. Thanks [gphat](https://github.com/gphat)!

# 1.7.0, 2017-10-19

## Notes for upgrading from previous versions
* The `set` data structure serialization format for communiation with a global Veneur server has changed in an incompatible way. If your infrastructure relies on a global Veneur installation, they will drop `set` data from non-matching versions until the entire fleet and the global Veneur are all at the same version.
* The metrics for SSF packets (and spans) received have changed names: They used to be `veneur.packet.received_total` and `veneur.packet.spans.received_total`, respectively, and they are now named `veneur.ssf.received_total` and `veneur.ssf.spans.received_total`.

## Added
* New [configuration option](https://github.com/stripe/veneur/pull/233) `statsd_listen_addresses`, a list of URIs indicating on which ports (and protocols) Veneur should listen on for statsd metrics. This deprecates both the `udp_address` and `tcp_address` settings. Thanks [antifuchs](https://github.com/antifuchs)!
* New package `github.com/stripe/veneur/protocol`, containing a wire protocol for sending/reading SSF over a streaming connection. Thanks [antifuchs](https://github.com/antifuchs)!
* `github.com/veneur/trace` now contains [customizable `Client`s](https://github.com/stripe/veneur/pull/262) that support streaming connections.
* [veneur-prometheus](https://github.com/stripe/veneur/tree/master/cmd/veneur-prometheus) now has a `-p` option for specifying a prefix for all metrics. Thanks [gphat](https://github.com/gphat)!
* New metrics `ssf.spans.tags_per_span` and `ssf.packet_size` track the distribution of tags per span and total packet sizes, respectively.
* Our super spiffy logo, designed by [mercedes](https://github.com/mercedes-stripe), is now included at the top of the README!
* Refactor internals to use a new intermediary metric struct to facilitate new plugins. Thanks [gphat](https://github.com/gphat)!
* The BUILD_DATE and VERSION variables can be set at link-time and are now exposed by the `/builddate` and `/version` endpoints.

## Improvements
* [A new HyperLogLog implementation](https://github.com/stripe/veneur/pull/190) means `set`s are faster and allocate less memory. Thanks, [martinpinto](https://github.com/martinpinto) and [seiflotfy](https://github.com/seiflotfy)!
* Introduced a new `metricSink` which provides a common interface for metric backends. In an upcoming release all plugins will be converted to this interface. Thanks [gphat](https://github.com/gphat)!

## Deprecations
* `veneur-emit` no longer supports the `-config` argument, as it's no longer possible to reliably detect which statsd host/port to connect to. The `-hostport` option now takes a URL of the same form `statsd_listen_addresses` takes to explicitly tell it what address it should send to.
* `SSFSpanCollection` got removed, as it is superseded by the wire protocol. If you need to send multiple spans in bulk, we recommend setting up a buffered `trace.Client`!

# 1.6.0, 2017-08-29

## Added
* Veneur-emit [can now time any shell command](https://github.com/stripe/veneur/pull/222) and emit its duration as a Timing metric. Thanks [redsn0w422](https://github.com/redsn0w422)!
* Config options can now be provided via environment variables using [envconfig](https://github.com/kelseyhightower/envconfig) for Veneur and veneur-proxy. Thanks [gphat](https://github.com/gphat)!
* [SSF](https://github.com/stripe/veneur/tree/master/ssf) now includes a boolean `indicator` field for signaling that this span is useful as a [Service Level Indicator](https://en.wikipedia.org/wiki/Service_level_indicator) for its service.
* A type `SSFSpanCollection` has been added but is not yet used.
* The `veneur-prometheus` command can be used to [scrape prometheus endpoints and emit those metrics to Veneur](https://github.com/stripe/veneur/pull/221). Thanks [gphat](https://github.com/gphat) and [jvns](https://github.com/jvns)

## Improvements

As a result of the efficiency improvements in this release, we've seen ~50% reduction in memory usage by way of measuring the allocated heap.

Secondly, the shift in *not* buffering spans on their way to LightStep should be noted. This changes behavior in Veneur which has traditionally done everything in 10s increments.

* If possible, initialization errors when starting Veneur [will now be reported to Sentry](https://github.com/stripe/veneur/pull/173). Thanks [chimeracoder](https://github.com/chimeracoder)!
* [Check return value of LightStep flush](https://github.com/stripe/veneur/pull/209). Thanks [chimeracoder](https://github.com/chimeracoder)!
* No longer using a fork of [Logrus](https://github.com/sirupsen/logrus) that fixed a race condition. Thanks [chimeracoder](https://github.com/chimeracoder)!
* Updated to latest version of [LightStep's tracing library](https://github.com/lightstep/lightstep-tracer-go), which drastically improves the success of spans on hosts with high span rates. Thanks [gphat](https://github.com/gphat)!
* [No longer buffers spans for LightStep](https://github.com/stripe/veneur/pull/227), they are dispatched directly to the LightStep client. Thanks [gphat](https://github.com/gphat)!
* [Reuse an existing buffer when parsing incoming spans](https://github.com/stripe/veneur/pull/234), reducing allocations. Thanks [gphat](https://github.com/gphat)!
* Use [gogo protobuf](https://github.com/gogo/protobuf) for [code generation of SSF's protobuf](https://github.com/stripe/veneur/pull/236), resulting in faster and less memory span ingestion. Thanks [gphat](https://github.com/gphat)!

## Bugfixes
* veneur-proxy no longer balks at using static hosts for tracing and metrics. Thanks [gphat](https://github.com/gphat)!

## Deprecations
* SSF's `operation` field has been deprecated in favor of the field `name`.
* SSF spans with a tag `name` will have that name placed into the SSF span `name` field until 2.0 is released.

# 1.5.2, 2017-08-15

## Bugfixes
* Correctly parse `Set` metrics if sent via SSF. Thanks [redsn0w422](https://github.com/redsn0w422)!
* Return correct array of tags after parsing an SSF metric. Thanks [redsn0w422](https://github.com/redsn0w422)!
* Don't panic if a packet doesn't have tags. Thanks [redsn0w422](https://github.com/redsn0w422)!
* Fix a typo in the link to veneur-emit in the readme. Thanks [vasi](https://github.com/vasi)!

## Added
* Adds [events](https://docs.datadoghq.com/guides/dogstatsd/#events-1) and [service checks](https://docs.datadoghq.com/guides/dogstatsd/#service-checks-1) to `veneur-emit`. Thanks [redsn0w422](https://github.com/redsn0w422)!
* Switch to [dep](https://github.com/golang/dep/) for managing the `vendor` directory. Thanks [chimeracoder](https://github.com/chimeracoder)!
* Remove support for `govendor`. Thanks [chimeracoder](https://github.com/chimeracoder)!
* Emit GC metrics at flush time. This feature is best used with Go 1.9 as previous versions cause GC pauses when collecting this information from go. Thanks [gphat](https://github.com/gphat)!
* Allow configuration of LightStep's reconnect period via `trace_lightstep_reconnect_period` and the maximum number of spans it will buffer via `trace_lightstep_maximum_spans`. Thanks [gphat](https://github.com/gphat)!
* Switch to [dep](https://github.com/golang/dep/) for managing the `vendor` directory. Thanks [chimeracoder](https://github.com/chimeracoder)!
* Remove support for `govendor`. Thanks [chimeracoder](https://github.com/chimeracoder)!
* Added [harmonic mean](https://en.wikipedia.org/wiki/Harmonic_mean) as an optional aggregate type. Use `hmean` as an option to the `aggregates` config option to enable. Thanks [non](https://github.com/non)!

## Improvements
* Added tests for `parseMetricSSF`. Thanks [redsn0w422](https://github.com/redsn0w422)!
* Refactored `veneur-emit` flag usage to make testing easier. Thanks [redsn0w422](https://github.com/redsn0w422)!
* Minor text fixes in the README. Thanks [an-stripe](https://github.com/an-stripe)!
* Restructured SSF parsing logic and added more tests. Thanks [redsn0w422](https://github.com/redsn0w422)!
* Tag `packet.spans.received_total` with `service` from the span. Thanks [chimeracoder](https://github.com/chimeracoder)!

# 1.5.1, 2017-07-18

## Bugfixes
* Flush the lightstep tracer before closing it. Thanks [gphat](https://github.com/gphat) with assist from [stangles](https://github.com/joshu-stripe)!

## Improvements
* Better document how to configure Veneur as a DogStatsD replacement. Thanks [gphat](https://github.com/gphat) with assist from [stangles](https://github.com/stangles)!

# 1.5.0, 2017-07-13

## Bugfixes
* Fixed an error in graceful shutdown of the TCP listener. Thanks [evanj](https://github.com/evanj)!
* Don't hang if we call `log.Fatal` and we aren't hooked up to a Sentry. Thanks [evanj](https://github.com/evanj)!
* Fix flusher_test being called more than once resulting in flappy failure. Thanks [evanj](https://github.com/evanj)!
* Improve flusher test to not start Veneur, fixing flapping test. Thanks [evanj](https://github.com/evanj)!

## Added
* `veneur-emit` can now emit metrics using the [SSF protocol](https://github.com/stripe/veneur/tree/master/ssf#readme). Thanks [redsn0w422](https://github.com/redsn0w422)!
* Documentation for SSF. Thanks [gphat](https://github.com/gphat)!

## Improvements
* It is no longer required to emit a `sum` to get an `avg` when configuring what aggregations to emit for a histogram. Thanks [cgilling](https://github.com/cgilling)!
* Tags added in the `tags` config key are now applied to trace spans. Thanks [chimeracoder](https://github.com/chimeracoder)!
* Additional documentation for `veneur-proxy`. Thanks [gphat](https://github.com/gphat)!
* Revamped configuration file organization and comments. Thanks [gphat](https://github.com/gphat)!
* Changed some config keys to have more specific names to facilitate future refactoring. Thanks [gphat](https://github.com/gphat)!
* Adjust the flush loop to listen for server shutdown to improve test consistency. Thanks [evanj](https://github.com/evanj)!
* Veneur can now, experimentally, ingest metrics using the [SSF protocol](https://github.com/stripe/veneur/tree/master/ssf#readme). Thanks [redsn0w422](https://github.com/redsn0w422)!
* Reresolve the LightStep trace flusher on each flush, accomodating Consul-based DNS use and preventing stale sinks. Thanks [chimeracoder](https://github.com/chimeracoder)!

## Deprecations
* The following configuration keys are deprecated and will be removed in version 2.0 of Veneur:
  * `datadog_api_key` replaces `key`
  * `datadog_api_hostname` replaces `api_hostname`
  * `datadog_trace_api_address` replaces `trace_api_address`
  * `ssf_address` replaces `trace_address`

# 1.4.0, 2017-06-09

## Changes
* Require Go 1.8+ and stop building against 1.7 Thanks Thanks [chimeracoder](https://github.com/chimeracoder)!

# 1.3.1, 2017-06-06

## Bugfixes
* Decrease logging level for proxy's "forwarded" messages. Thanks [gphat](https://github.com/gphat)!
* Failed discovery refreshes now log the service name. Thanks [gphat](https://github.com/gphat)!

## Improvements
* Proxy no longer requires a trace service name, since it's not wired up. Thanks [gphat](https://github.com/gphat)!

# 1.3.0, 2017-05-19

## Bugfixes
* No longer allow clients to pass in `nan`, `+inf` or `-inf` as a value for a metric, as this caused errors on flush. Thanks [gphat](https://github.com/gphat)!

## Added
* Added `veneur-proxy` to provide HA features with consistent hashing. See the [Proxy section of the README](https://github.com/stripe/veneur#proxy)

# 1.2.0, 2017-04-24

## Bugfixes
* Fix flusher_test to properly shutdown HTTP after handling. Thanks [evanj](https://github.com/evanj)!
* Verify that `trace_max_length_bytes` is properly set. Thanks [evanj](https://github.com/evanj)!
* Fix some race conditions in testing.

## Improvements
* Document performance cost of TLS with RSA and ECDH keys. Thanks [evanj](https://github.com/evanj)!
* Reduce logging of tracing information to `debug` level to decrease unnecessary logging.
* Reduce common TCP error logs to `info` level. Thanks [evanj](https://github.com/evanj)!
* Deal with server shutdown without inspecting errors strings. Thanks [evanj](https://github.com/evanj)!
* Decrease the number of things we send to Sentry as "errors".
* Detect and emit a metric `veneur.packet.error_total` tagged `reason:toolong` for metrics that exceed the metric max length.
* Emit a metric `veneur.packet.error_total` tagged `reason:zerolength` for metrics have no contents.
* Correct unnecessary allocation / goroutine in TCP connections that was leaking memory. Thanks [evanj](https://github.com/evanj)!
* Close idle TCP connections after 10 minutes. Thanks [evanj](https://github.com/evanj)!
* Fixed a lot of go lint errors.

## Added
* Add a metric `veneur.sentry.errors_total` for number of errors we send to Sentry.
* New plugin `flush_file` for writing metrics to a flat file.
* New `/healthcheck/tracing` endpoint that returns 200 if this Veneur instance is accepting traces.

# 1.1.0, 2017-03-02

## Changes
* Refactor tests to use a more shareable test fixture. Thanks [evanj](https://github.com/evanj)!
* Refactor `Server`'s constructor to not start any goroutines and add a `Start()` that takes care of that, making for easier tests.

## Bugfixes
* Hostname and device name tags are now omitted from JSON generated for transmission to Datadog at flush time. Thanks [evanj](https://github.com/evanj)!
* Fix panic when an error is generated and Sentry is not configured. Thanks [evanj](https://github.com/evanj)!
* Fix typos in README

## Improvements
* Add `omit_empty_hostname` option. If true and `hostname` tag is set to empty, Veneur will not add a host tag to its own metrics. Thanks [evanj](https://github.com/evanj)!
* Support "all interfaces" addresses (`:1234`) for listening configuration. Thanks [evanj](https://github.com/evanj)!
* Add support for receiving statsd packets over authenticated TLS connections. Thanks [evanj](https://github.com/evanj)!
* [EXPERIMENTAL] Add [InfluxDB](https://www.influxdata.com) support.
* [EXPERIMENTAL] Add support for ingesting traces and sending to Datadog's APM agent.
