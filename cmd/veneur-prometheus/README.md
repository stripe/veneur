`veneur-prometheus` is a command line utility for periodically polling
[a Prometheus metrics endpoint](https://prometheus.io/docs/instrumenting/exposition_formats/)
and passing those metrics along to an instance of Veneur.

## SSF

At present these metrics are emitting as DogStatsD style metrics. We will add SSF at a later date.

## Prometheus Counters vs Statsd Counters

Prometheus counters continue to increase over the life time of the monitored service.  Conversely, statsd counters are only the counts of events that have happened since the last time they were reported.  Therefore it is incorrect to naively read from a Prometheus endpoint and just pass the values returned for counters to statsd.  Instead you need to send the difference between the last observation point and the current one to get an accurate mapping.

This leads to the following conditions:
- we've made no observations to cache and compare against.  We won't report any metrics to statsd until a second observation allows us to diff.
- we have some observations in our cache, but a particular metric does not exist there (common for vector types to appear over time). In this case, we can implicitly compare that metric to 0.
- our cache version is smaller than the Prometheus metric count.  This is the normal case and indicates that there have ben events on that metric.  We report the difference.
- our cache version is bigger than the Prometheus metric count.  This indicates a restart on the monitored service.  The best we can do is report the events since that restart.

This leads to the following cases where we will miss metrics:
- `veneur-prometheus` is restarted.  All counts while `veneur-prometheus` is down, plus those that happen between the first and second observation, are missed.
- the monitored service is restarted.  Any events that happen after the `veneur-prometheus` observation and before the monitored service restart are missed.

### Prometheus Histograms are (mostly) Counters

Prometheus implements its histograms as additive counters.  That is if you have a set of buckets like 1 second, 2 seconds, 5 seconds, when an event that is 1.8 seconds long is observed the count on the 2 second bucket, the 5 second bucket and the infinity bucket are incremented.  These buckets are just Prometheus counters so all of the cases above apply to each of them.  Further the histogram keeps a count as well which is impacted.

Our rules above get particularly problematic when dealing with histogram metrics where the buckets have changed on a monitored service restart.  That is if it previously had 1, 2, 5 second buckets and after a restart has 1, 2, 6 second buckets.  The end result is that our translation of Prometheus histograms around monitored restarts are likely not ever going to provide extremely accurate information.

*Note* Prometheus Summaries also have a count sub-component that is a counter and impacted by the above.  The majority of information in Summaries are gauges though.

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
