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
* Add `omit_empty_hostname` option. If true and `hostname` tag is set to empty, Veneur will not add a host tag to it's own metrics. Thanks [evanj](https://github.com/evanj)!
* Support "all interfaces" addresses (`:1234`) for listening configuration. Thanks [evanj](https://github.com/evanj)!
* Add support for receiving statsd packets over authenticated TLS connections. Thanks [evanj](https://github.com/evanj)!
* [EXPERIMENTAL] Add [InfluxDB](https://www.influxdata.com) support.
* [EXPERIMENTAL] Add support for ingesting traces and sending to Datadog's APM agent.
