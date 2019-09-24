`veneur-prometheus` is a command line utility for periodically polling
[a Prometheus metrics endpoint](https://prometheus.io/docs/instrumenting/exposition_formats/)
and passing those metrics along to an instance of Veneur.

## SSF

At present these metrics are emitting as DogStatsD style metrics. We will add SSF at a later date.

## Prometheus Counters vs Statsd Counters

Prometheus is a pull-based system that uses cumulative counters, which continue to increase over the lifetime of the monitored service. Conversely, statsd counters incrementally track events as they are reported.  Therefore, converting Prometheus counters to statsd requires taking the difference between the previous observation (cached) and the current observation to create an accurate mapping, with four possible states:

1. The cache is empty. When the cache is empty, metrics reported by Prometheus will *not* be passed to statsd, but they will be added to the cache to compare future reports against.
2. The cache is non-empty, but a reported metric is missing from the cache. It's assumed that the metric is being reported for the first time (Prometheus vector types commonly appear over time). In this case, the metric is implicitly compared to 0 and passed directly to statsd as an incremental counter,
3. The cached value of the counter is less than than the Prometheus-reported counter. This is the normal case, indicating that the counter has been incremented application-side since the last report. The difference between the reported counter and the cached counter is passed to statsd as an incremental counter.
4. The cached value of the counter is greater than the Prometheus-reported metric count. This indicates that the monitored service was restarted sometime between the previous report and the current report, with Prometheus reporting the cumulative count since the application was restarted. The current Prometheus-reported metric is implicitly compared to 0 and passed directly to statsd as an incremental counter. Any accumulated values that happened *after* the previous report but *before* the application was restarted cannot be tracked.

Because of these four states, there are two situations which will result in missing metrics.

1. `veneur-prometheus` is restarted.  All counts that accumulate while `veneur-prometheus` is down, plus those that happen between the first and second observation, are missed. (This is analogous to metrics that are missed when push-based metrics are reported over UDP to `veneur-srv`).
2. The monitored service is restarted.  Any counts that accumulate after the previous `veneur-prometheus` observation and before the monitored service restart are missed. (This is unique to pull-based metrics reporting).

### Prometheus Histograms are (mostly) Counters

Prometheus implements histograms as cumulative counters on the upper bound of buckets. Given a set of buckets {1s, 2s, 5s}, an observation of 1.8s will increment the counts on the 2s and 5s bucket, as well as the implicit "infinity" bucket, and an additional counter that tracks the total number of observed data points. Because histogram buckets are treated as Prometheus counters, all of the previous conditions regrading counters apply to histogram buckets as well.

Buckets are defined explicitly by the application. If the definition of bucket boundaries changes within the application, it is recommended to restart `veneur-prometheus` as well. If `veneur-prometheus` is not restarted, the reported histogram metrics will be inaccurate.

*Note*: Prometheus Summaries also have a count sub-component that is a counter and similarly subject to the same conditions that apply to counters. However, the majority of information in Summaries are gauges.

# Usage

```
Usage of ./veneur-prometheus:
  -cacert string
    	The path to a CA cert used to validate the server certificate. Only used if using mTLS.
  -cert string
    	The path to a client cert to present to the server. Only used if using mTLS.
  -d    Enable debug mode
  -h string
    	The full URL — like 'http://localhost:9090/metrics' to query for Prometheus metrics. (default "http://localhost:9090/metrics")
  -i string
    	The interval at which to query. Value must be parseable by time.ParseDuration (https://golang.org/pkg/time/#ParseDuration). (default "10s")
  -ignored-labels string
    	A comma-seperated list of label name regexes to not export
  -ignored-metrics string
    	A comma-seperated list of metric name regexes to not export
  -key string
    	The path to a private key to use for mTLS. Only used if using mTLS.
  -p string
    	A prefix to append to any metrics emitted. Include a trailing period. (e.g. "myservice.")
  -s string
    	The host and port — like '127.0.0.1:8126' — to send our metrics to. (default "127.0.0.1:8126")
```
