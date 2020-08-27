# 14.0.0, in progress

## Added
* The Datadog sink can now filter metric names by prefix with `datadog_metric_name_prefix_drops`. Thanks, [kaplanelad](https://github.com/kaplanelad)!
* The Datadog sink can now filter tags by metric names prefix with `datadog_exclude_tags_prefix_by_prefix_metric`. Thanks, [kaplanelad](https://github.com/kaplanelad)!
* When specifying the SignalFx key with `signalfx_vary_key_by`, if both the host and the metric provide a value, the metric-provided value will take precedence over the host-provided value. This allows more granular forms of metric organization and attribution. Thanks, [aditya](https://github.com/chimeracoder)!
* Support for listening to abstract statsd metrics on Unix Domain Socket(Datagram type). Thanks, [androohan](https://github.com/androohan)!

## Updated
* Updated the vendored version of DataDog/datadog-go which fixes parsing for abstract unix domain sockets in the statsd client. Thanks, [androohan](https://github.com/androohan)!
* Changed the certificates that veneur tests with to include SANs and no longer rely on Common Names, in order to comply with Go's [upcoming crackdown on CN certificate constraints](https://github.com/stripe/veneur/issues/791). Thanks, [antifuchs](https://github.com/antifuchs)!
* Disabled the dogstatsd client telemetry on the internal statsd client used by Veneur. Thanks, [prudhvi](https://github.com/prudhvi)!
* Migrated from the deprecated Sentry package, raven-go, to sentry-go. Thanks, [yanke](https://github.com/yanske1)!

## Bugfixes
* veneur-emit no longer panics when an empty command is passed. Thanks, [shrivu-stripe](https://github.com/shrivu-stripe)!
* Fixed a bug that caused some veneur-emit builds to not flush metrics to udp. Thanks, [shrivu-stripe](https://github.com/shrivu-stripe)!

# 13.0.0, 2020-01-03

## Added

* The SignalFx sink now supports dynamically fetching per-tag Access tokens from SignalFX. Thanks, [szabado](https://github.com/szabado)!
* The Kafka sink now includes metrics for skipped and dropped spans, as well as a debug log line for flushes. Thanks, [franklinhu](https://github.com/franklinhu)
* A flush "watchdog" controlled by the config setting `flush_watchdog_missed_flushes`. If veneur has not started this many flushes, the watchdog panics and terminates veneur (so it can be restarted by process supervision). Thanks, [antifuchs](https://github.com/antifuchs)!
* Splunk sink: Trace-related IDs are now represented in hexadecimal for cross-tool compatibility and a small byte savings. Thanks, [myndzi](https://github.com/myndzi)
* Splunk sink: Indicator spans are now tagged with `"partial":true` when they would otherwise have been sampled, distinguishing between partial and full traces. Thanks, [myndzi](https://github.com/myndzi)
* New configuration options `veneur_metrics_scopes` and `veneur_metrics_additional_tags`, which allow configuring veneur such that it aggregates its own metrics globally (rather than reporting a set of internal metrics per instance/container/etc). Thanks, [antifuchs](https://github.com/antifuchs)!
* New SSF `sample` field: `scope`. This field lets clients tell Veneur what to do with the sample - it corresponds exactly to the `veneurglobalonly` and `veneurlocalonly` tags that metrics can hold. Thanks, [antifuchs](https://github.com/antifuchs)!
* veneur-prometheus now allows you to specify mTLS configuration for the polling HTTP client. Thanks, [choo-stripe](https://github.com/choo-stripe)!
* The `http_quit` config option enables the `/quitquitquit` endpoint, which can be used to trigger a graceful shutdown using an HTTP POST request. Thanks, [aditya](https://github.com/chimeracoder)!
* New config option `count_unique_timeseries` which is used to emit metric `veneur.flush.unique_timeseries_total`, the HyperLogLog cardinality estimate of the unique timeseries in a flush interval. Thanks, [randallm](https://github.com/randallm)!
* veneur-emit now allows you to mark SSF spans as having errored. Thanks, [randallm](https://github.com/randallm)!
* The Splunk span sink supports tag exclusion, to better manage Splunk indexing volume. If a span contains a tag that's in the excluded set, the Splunk sink will skip sending that span to Splunk. Thanks, [aditya](https://github.com/chimeracoder)!
* veneur-prometheus now supports Prometheus Untyped metrics. Thanks, [kklipsch-stripe](https://github.com/kklipsch-stripe)!
* veneur-prometheus now accepts a socket parameter for proxied requests. Thanks, [kklipsch-stripe](https://github.com/kklipsch-stripe)!
* The Datadog sink can now filter metric names by prefix with `datadog_metric_name_prefix_drops`. Thanks, [kaplanelad](https://github.com/kaplanelad)!

## Updated

* Updated the vendored version of DataDog/datadog-go which adds support for sending metrics to Unix Domain socket. Thanks, [prudhvi](https://github.com/prudhvi)!
* Splunk sink: Downgraded Splunk HEC errors to be logged at warning level, rather than error level. Added a note to clarify that Splunk cluster restarts can cause temporary errors, which are not necessarily problematic. Thanks, [aditya](https://github.com/chimeracoder)!
* Updated the vendored version of github.com/gogo/protobuf which fixes Gopkg.toml conflicts for users of veneur. Thanks, [dtbartle](http://github.com/dtbartle)!
* Updated server.go to use the aws sdk (https://docs.aws.amazon.com/sdk-for-go/api/aws/session/) when the creds are not set in the config.yaml. Thanks, [linuxdynasty](https://github.com/linuxdynasty)!

## Bugfixes
* veneur-prometheus now reports incremental counters instead of cumulative counters. This may cause dramatic differences in the statistics reported by veneur-prometheus.  Thanks, [kklipsch-stripe](https://github.com/kklipsch-stripe)!

## Bugfixes
* Veneur listening on UDS for statsd metrics will respect the `read_buffer_size_bytes` config. Thanks, [prudhvi](https://github.com/prudhvi)!
* The splunk HEC span sink didn't correctly spawn the number of submission workers configured with `splunk_hec_submission_workers`, only spawning one. Now it spawns the number configured. Thanks, [antifuchs](https://github.com/antifuchs)!
* The signalfx sink now correctly constructs ingestion endpoint URLs when given URLs that end in slashes. Thanks, [antifuchs](https://github.com/antifuchs)!
* Veneur now sets a deadline for its flushes: No flush may take longer than the configured server flush interval. Thanks, [antifuchs](https://github.com/antifuchs)!
* The signalfx sink no longer deadlocks the flush process if it receives more than one error per submission. Thanks, [antifuchs](https://github.com/antifuchs)!
* Fixed the README to link to the correct HLL implementation. Thanks, [gphat](https://github.com/gphat)!
* Fixed the BucketRegionError while using the S3 Plugin. Thanks, [linuxdynasty](https://github.com/linuxdynasty)!

## Removed

* A dependency on `github.com/Sirupsen/logrus` from the trace client package `github.com/stripe/veneur/trace`. Thanks, [antifuchs](https://github.com/antifuchs) and [samczsun](https://github.com/samczsun)!

# 12.0.0, 2019-03-06

## Added

* The OpenTracing implementation's `Tracer.Inject` in the `trace` package now sets HTTP headers in a way that tools like [Envoy](https://www.envoyproxy.io/) can propagate on traces. Thanks, [antifuchs](https://github.com/antifuchs)!
* SSF packets are now validated to ensure they contain either a valid span or at least one metric. The metric `veneur.worker.ssf.empty_total` tracks the number of empty SSF packets encountered, which indicates a client error. Thanks, [tummychow](https://github.com/tummychow) and [aditya](https://github.com/chimeracoder)!
* SSF indicator spans can now report an additional "objective" metric, tagged with their service and name. Thanks, [tummychow](https://github.com/tummychow)!
* Support for listening to statsd metrics on Unix Domain Socket(Datagram type). Thanks, [prudhvi](https://github.com/prudhvi)!

## Updated
* The metric `veneur.sink.spans_dropped_total` now includes packets that were skipped due to UDP write errors. Thanks, [aditya](https://github.com/chimeracoder)!
* The `debug` blackhole sink features improved logging output, with more data and better formatting. Thanks, [aditya](https://github.com/chimeracoder)!
* Container images are now built with Go 1.12. Thanks, [aditya](https://github.com/chimeracoder)!

## Bugfixes
* The signalfx client no longer reports a timeout when submission to the datapoint API endpoint encounters an error. Thanks, [antifuchs](https://github.com/antifuchs)!
* SSF packets without a name are no longer considered valid for `protocol.ValidTrace`. Thanks, [tummychow](https://github.com/tummychow)!
* The splunk sink no longer hangs or complains when a HEC endpoint should close the connection. Thanks, [antifuchs](https://github.com/antifuchs)!

## Removed
* Go 1.10 is no longer supported.

# 11.0.0, 2019-01-22

## Added
* Datadog's [distribution](https://docs.datadoghq.com/developers/metrics/distributions/) type for DogStatsD is now supported and treated as a plain histogram for compatibility. Thanks, [gphat](https://github.com/gphat)!
* Add support for `tags_exclude` to the DataDog metrics sink. Thanks, [mhamrah](https://github.com/mhamrah)!
* The `github.com/stripe/veneur/trace` package has brand new and much more extensive [documentation](https://godoc.org/github.com/stripe/veneur/trace)! Thanks, [antifuchs](https://github.com/antifuchs)!
* New configuration setting `signalfx_flush_max_per_body` that allows limiting the payload of HTTP POST bodies containing data points destined for SignalFx. Thanks, [antifuchs](https://github.com/antifuchs)!

# 10.0.0, 2018-12-19

## Added
* The new X-Ray sink provides support for [AWS X-Ray](https://aws.amazon.com/xray/) as a tracing backend. Thanks, [gphat](https://github.com/gphat) and [aditya](https://github.com/chimeracoder)!
* A new package `github.com/stripe/veneur/trace/testbackend` contains two trace client backends that can be used to test the trace data emitted by applications. Thanks, [antifuchs](https://github.com/antifuchs)!

## Updated
* Updated the vendored version of x/net, which picks up a package rename that can lead issues when integrating veneur into other codebases. Thanks, [nicktrav](https://github.com/nicktrav)!
* Updated the vendored versions of x/sys, protobuf, and gRPC. Thanks [nicktrav](https://github.com/nicktrav)!

# 9.0.0, 2018-11-08

## Bugfixes
* The Splunk span sink no longer reports an internal error for timeouts encountered in event submissions; instead, it reports a failure metric with a cause tag set to `submission_timeout`. Thanks, [antifuchs](https://github.com/antifuchs)!
* The Splunk span sink now honors `Connection: keep-alive` from the HEC endpoint and keeps around as many idle HTTP connections in reserve as it has HEC submission workers. Thanks, [antifuchs](https://github.com/antifuchs)!
* The metric `veneur.forward.post_metrics_total` was being emitted both as a gauge and a counter. The errant gauge was removed. Thanks, [gphat](https://github.com/gphat)!

## Added
* The Splunk span sink can be configured with a sample rate for non-indicator spans with the `splunk_span_sample_rate` setting. Thanks, [aditya](https://github.com/chimeracoder)!
* The splunk span sink now has configuration parameters `splunk_hec_max_connection_lifetime` and `splunk_hec_connection_lifetime_jitter` to regulate how long HTTP connections can be kept alive for. Thanks, [antifuchs](https://github.com/antifuchs)!
* The SignalFx sink can now filter metric names by prefix with `signalfx_metric_name_prefix_drops` and tag literals (case-insensitive) with `signalfx_metric_tag_literal_drops`. Thanks [gphat](https://github.com/gphat)!
* Histograms and timers now support global scope. Histograms and timers tagged with "veneurglobalonly" will now emit *all* metrics from the global veneur. The default behavior is to emit aggregates like max, min locally and percentiles globally. Thanks, [clin](https://github.com/clin88)!
* The `ssf.spans.root.received_total` global counter tracks the number of traces (root spans) processed system-wide. Thanks, [aditya](https://github.com/chimeracoder)!

## Updated
* The README's [Metrics section](https://github.com/stripe/veneur#metrics) has been updated, as it referred to some missing metrics. Thanks, [gphat](https://github.com/gphat)!
* Various references to Datadog were removed from the README, Veneur is vendor agnostic. Thanks, [gphat](https://github.com/gphat)!

## Removed
* The metrics `veneur.flush.total_duration_ns` and `veneur.flush.worker_duration_ns` were removed, please use the per-sink `veneur.sink.metric_flush_total_duration_ns` to monitor flush durations.
* The metrics `veneur.gc.GCCPUFraction`, `veneur.gc.alloc_heap_bytes_total`, `veneur.gc.mallocs_objects_total` metrics were removed. Also from veneur proxy. Thanks, [gphat](https://github.com/gphat)!
* The metric `veneur.flush.other_samples_duration_ns` was removed. Thanks, [gphat](https://github.com/gphat)!

# 8.0.0, 2018-09-20

## Added
* Metrics can be forwarded over gRPC using veneur-proxy (and Consul).  Thanks, [noahgoldman](https://github.com/noahgoldman) and [Quantcast](https://github.com/quantcast)!
* Added tracer.InjectHeader convenience function for... convenience! Thanks, [mikeh](https://github.com/mikeh-stripe)!
* Veneur has a new sink that can be configured to send spans as events into a [Splunk HEC](http://dev.splunk.com/view/event-collector/SP-CAAAE6M) endpoint. Thanks, [antifuchs](https://github.com/antifuchs) and [aditya](https://github.com/chimeracoder)!
* Go 1.11 is now supported and used for all public Docker images. Thanks, [aditya](https://github.com/chimeracoder)!
* The `veneur/trace` package now supports setting the indicator bit on a span manually. Thanks, [aditya](https://github.com/chimeracoder)!
* The `-validate-config` and `validate-config-strict` flags will make veneur exit appropriately after checking the specified (`-f`) config file. Thanks, [sdboyer](https://github.com/sdboyer)!
* `veneur-emit` will now exit with an error if no data would have been sent. Thanks, [sdboyer](https://github.com/sdboyer)!

## Bugfixes
* The trace client can now correctly parse trace headers emitted by Envoy. Thanks, [aditya](https://github.com/chimeracoder)!


## Removed
* Go 1.9 is no longer supported.

# 7.0.0, 2018-08-08

## Added
* `veneur-emit` now takes a new option `-span_tags` for tags that should be applied only to spans. This allows span-specific tags that are not applied to other emitted values. Thanks [gphat](https://github.com/gphat)!
* `veneur-emit`'s `-tag` flag now applies the supplied tags to any value emitted, be it a span, metric, service check or event. Use other, mode specific flags (e.g. span_tags) to add tags only to those modes. Thanks [gphat](https://github.com/gphat)!
* Isolated a potential resource starvation issue. Added new configuration options for `veneur-proxy` to configure its [http.Transport](https://golang.org/pkg/net/http/#Transport):
  * `idle_connection_timeout` for controlling how long connections may idle before timing out, corresponds to `IdleConnTimeout`
  * `max_idle_conns` for controlling the maximum number of idle connections in total
  * `max_idle_conns_per_host` for controlling the maximum number of idle connections per host. Not that this now defaults to `100` for safety!
* Added configuration options and improved defaults for the following tracing client parameters:
  * `tracing_client_capacity` for controlling the depth of a buffer that holds tracing spans when they can't be emitted, defaults to `1024`, up from `64`
  * `tracing_client_flush_interval` for controlling how often the tracing client's backing buffer will be emptied (as an alternative to when it is full), defaults to `500ms` from `3s`
  * `tracing_client_metrics_interval` for controlling how often thew tracing client will send metrics about it's own operations, defaults to `1s` and is unchanged

## Bugfixes
* `veneur-prometheus` no longer crashes when the metrics host is unreachable. Thanks, [arjenvanderende](https://github.com/arjenvanderende)!


## Removed
* `veneur-proxy` now only logs forward counts at Debug level, drastically reducing log volume.

# 6.0.0, 2018-06-28

## Added
* Metrics can be imported over gRPC if the `grpc_address` parameter is set.  Thanks, [noahgoldman](https://github.com/noahgoldman) and [Quantcast](https://github.com/quantcast)!
* When timing commands, `veneur-emit` now passes stdin, stdout and stderr through to the child process unmodified. Thanks [antifuchs](https://github.com/antifuchs) and [sdboyer](https://github.com/sdboyer)!
* Metrics can be forwarded over gRPC (currently only to a single Veneur) using `forward_use_grpc`. Thanks, [noahgoldman](https://github.com/noahgoldman) and [Quantcast](https://github.com/quantcast)!
* Two new options, `debug_ingested_spans` and `debug_flushed_metrics` make veneur log (at level DEBUG) information about the metrics and spans it processes. Thanks, [antifuchs](https://github.com/antifuchs)!
* `veneur-emit` now takes a new option `-set` with a string argument, which allows counting how many unique values were reported in veneur's flush interval. Thanks, [antifuchs](https://github.com/antifuchs)!

## Bugfixes
* Fix a possible crash-before-panic when unable to open UDP socket. Thanks, [gphat](https://github.com/gphat)
* The `StartSpan` method on `tracer.Tracer` will default to the provided `operationName` if provided. This function is provided for compatibility with OpenTracing, but the package-level `trace.StartSpanFromContext` function is recommended for new users.
* When creating timer metrics from indicator spans, veneur no longer prefixes `indicator_span_timer_name` with the string `veneur.`. Thanks, [antifuchs](https://github.com/antifuchs)!
* `veneur-prometheus` now exports Histograms properly, with a statsd tag for each bucket
* Environment config for `veneur-proxy` now uses `VENEUR_PROXY_` as a prefix. Previously used `VENEUR_` which was a bug!

## Updated
* Metric sampler parse function now looks for `veneurlocalonly` and `veneurglobalonly` by prefix instead of direct equality for times where value can't/shouldn't be excluded even if it's blank. Thanks [joeybloggs](https://github.com/joeybloggs)
* `veneur-prometheus` now exports a tag for each quartile rather than a seperate metric

## Removed
* The tag `span_name` has been removed from the timer metric generated for indicator spans. Thanks, [aditya](https://github.com/chimeracoder)!

# 5.0.0, 2018-05-17

## Added
* Added a timeout for sink ingestion to all sinks, which prevents a single slow sink from blocking ingestion on other span sinks indefinitely. Thanks, [aditya](https://github.com/chimeracoder)!
* Added `trace.SetDefaultClient` to handle overridding the default trace client, and closing the existing one. Thanks, [franklinhu](https://github.com/franklinhu)

## Improvements
* Veneur's key performance indicator metrics for metrics processing are reported through the statsd client. This way, KPI metrics are affected only by the metrics pipeline, not the tracing pipeline as well.
* SignalFX sink can now handle and convert ssf service checks (represented as a gauge). Thanks, [stealthcode](https://github.com/stealthcode)!
* Converted the grpsink to use unary instead of stream RPCs. Thanks, [sdboyer](https://github.com/sdboyer)!
* Bumped version of [SignalFx go client](https://github.com/signalfx/golib) to prevent accidental removal of `-` from tag keys. Thanks, [gphat](https://github.com/gphat)!
* Switched to a [faster consistent hash implementation](https://github.com/segmentio/fasthash). Thanks, [aditya](https://github.com/chimeracoder) and [noahgoldman](https://github.com/noahgoldman)!
* Reduce mutex contention around the default RNG in the math/rand standard library package. Thanks, [aditya](https://github.com/chimeracoder)!
* Improve performance of `ssf.RandomlySample` by a factor of one million. Thanks, [aditya](https://github.com/chimeracoder)!
* Added additional metrics for internal memory usage. Thanks, [aditya](https://github.com/chimeracoder)!
  * `gc.alloc_heap_bytes_total`
  * `gc.mallocs_objects_total`
  * `gc.GCCPUFraction`

## Bugfixes
* The `ignored-labels` and `ignored-metrics` flags for veneur-prometheus will filter no metrics or labels if no filter is specified. Thanks, [arjenvanderende](https://github.com/arjenvanderende)!
* Fixed problem where all Datadog service checks were set to `OK` instead of the supplied value. Thanks, [gphat](https://github.com/gphat)!
* Added a timeout to the Kafka sink, which prevents the Kafka client from blocking other span sinks. Thanks, [aditya](https://github.com/chimeracoder)!

## Removed
* Official support for building Veneur's binaries with Go 1.8 has been dropped. Supported versions of Go for building Veneur 1.9, 1.10, or tip.
* Veneur's trace client library can still be used in applications that are built with Go 1.8, but it is no longer tested against Go 1.8.
* The `veneur.ssf.received_total` metric has been removed, as it is mostly redundant with `veneur.ssf.spans.received_total`, and was not reported consistently between packet and framed formats.
* The `veneur.ssf.spans.received_total` metric now tracks all SSF data received, in either packet or framed format, whether or not a valid span was extracted.

# 4.0.0, 2018-04-13

## Improvements
* Receiving SSF in UDP packets now happens on `num_readers` goroutines. Thanks, [antifuchs](https://github.com/antifuchs)
* Updated [SignalFx library](https://github.com/signalfx/golib) dependency so that compression is enabled by default, saving significant time on large metric bodies. Thanks, [gphat](https://github.com/gphat)
* Decreased logging output of veneur-proxy. Thanks, [gphat](https://github.com/gphat)!
* Better warnings when invalid flag combinations are passed to `veneur-emit`. Thanks, [sdboyer](https://github.com/sdboyer)!
* Revamped how sinks handle DogStatsD's events and service checks. Thanks, [gphat](https://github.com/gphat)
  * `veneur.worker.events_flushed_total` and `veneur.worker.checks_flushed_total` have been replaced by `veneur.worker.other_samples_flushed_total`
  * `veneur.flush.event_worker_duration_ns` has been replaced by `veneur.flush.other_samples_duration_ns`

## Added

* The new `tags_exclude` parameter can be used to strip tags from all metrics Veneur processes, either for all supported sinks or a subset of sinks. Thanks, [aditya](https://github.com/chimeracoder)!
* The SSF client now defaults to opening 8 connections in parallel to avoid blocking client code. Thanks, [antifuchs](https://github.com/antifuchs)!
* New config settings `num_span_workers` and `span_channel_capacity` that allow you to customize the parallelism of span ingestion. Thanks, [antifuchs](https://github.com/antifuchs)!
* New span sink utilization metrics - Thanks, [antifuchs](https://github.com/antifuchs):
  * `veneur.sink.span_ingest_total_duration_ns` gives the total time per `sink` spent ingesting spans
  * `veneur.worker.span_chan.total_elements` over `veneur.worker.span_chan.total_capacity` gives the utilization of the sink ingestion channel.
* Introduce a generic gRPC streaming backend for trace spans. Thanks, [sdboyer](https://github.com/sdboyer)!
* New config keys `signalfx_vary_key_by` and `signalfx_per_tag_api_keys` which which allow sending signalfx data points with an API key specific to these data points' dimensions. Thanks, [antifuchs](https://github.com/antifuchs)!
* veneur-proxy now reports runtime metrics (with the prefix `veneur-proxy.`) at a configurable interval controlled by `runtime_metrics_interval`. It defaults to 10s. Thanks [gphat](https://github.com/gphat)!
* Allow specifying trace start/end times on `veneur-emit`. Thanks, [sdboyer](https://github.com/sdboyer)!
* Default `span_channel_capacity` to a non-zero value so we don't drop most spans in a minimal configuration. Thanks, [gphat](http://github.com/gphat)!
* Added tests for parsing floating point timers and histograms, just in case! Thanks [gphat](https://github.com/gphat)!
* New `ignored-labels` and `ignored-metrics` flags added to veneur-prometheus to selectively restrict exports to Veneur. Thanks, [yolken](https://github.com/yolken)!

# 3.0.0, 2018-02-27

## Incompatible changes

* These deprecated configuration keys are no longer supported and will cause an error on startup if used in a veneur config file:
  * `api_hostname` - replaced in 1.5.0 by `datadog_api_hostname`
  * `key` - replaced in 1.5.0 by `datadog_api_key`
  * `trace_address` - replaced in 1.7.0 by `ssf_listen_addresses`
  * `trace_api_address` - replaced in 1.5.0 by `datadog_trace_api_address`
  * `ssf_address` - replaced in 1.7.0 by `ssf_listen_addresses`
  * `tcp_address` and `udp_address` - replaced in 1.7.0 by `statsd_listen_addresses`
* These metrics have changed names:
  * Datadog, MetricExtraction, and SignalFx sinks now emit `veneur.sink.metric_flush_total_duration_ns` for metric flush duration and tag it with `sink`
  * Datadog, Kafka, MetricExtraction, and SignalFx sinks now emits `sink.metrics_flushed_total` for metric flush counts and tag it with `sink`
  * Datadog and LightStep sinks now emit `veneur.sink.span_flush_total_duration_ns` for span flush duration and tag it with `sink`
  * Datadog, Kafka, MetricExtraction, and LightStep sinks now emit `sink.spans_flushed_total` for metric flush counts and tag it with `sink`
* Veneur's internal metrics are no longer tagged with `veneurlocalonly`. This means that percentile metrics (such as timers) will now be aggregated globally.

## Bugfixes
* LightStep sink was hardcoded to use plaintext, now adjusts based on URL scheme (http versus https). Thanks [gphat](https://github.com/gphat)!
* The Datadog sink will no longer panic if `flush_max_per_body` is not configured; a default is used instead. Thanks [silverlyra](https://github.com/silverlyra)!
* The statsd source will no longer reject all packets if `metric_max_length` is not configured; a default is used instead. Thanks [silverlyra](https://github.com/silverlyra)!

## Added
* `veneur-emit` now infers parent and trace IDs from the environment (using the variables `VENEUR_EMIT_TRACE_ID` and `VENEUR_EMIT_PARENT_SPAN_ID`) and sets these environment variables from its `-trace_id` and `parent_span_id` when timing commands, allowing for convenient construction of trace trees if traced programs call `veneur-emit` themselves. Thanks, [antifuchs](https://github.com/antifuchs)
* The Kafka sink for spans can now sample spans (at a rate determined by `kafka_span_sample_rate_percent`) based off of traceIDs (by default) or a tag's values (configurable via `kafka_span_sample_tag`) to consistently sample spans related to each other. Thanks, [rhwlo](https://github.com/rhwlo)!
* Improvements in SSF metrics reporting - thanks, [antifuchs](https://github.com/antifuchs)!
  * Function `ssf.RandomlySample` that takes an array of samples and a sample rate and randomly drops those samples, adjusting the kept samples' rates
  * New variable `ssf.NamePrefix` that can be used to prepend a common name prefix to SSF sample names.
  * Package `trace/metrics`, containing functions that allow reporting metrics through a trace client.
  * New type `ssf.Samples` holding a batch of samples which can be submitted conveniently through `trace/metrics`.
  * Method `trace.(*Trace).Add`, which allows adding metrics to a trace span.
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

## Removed
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
