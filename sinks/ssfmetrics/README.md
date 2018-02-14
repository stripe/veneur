# SSFMetrics Sink

This sink extracts metrics from [SSF](https://github.com/stripe/veneur/tree/master/ssf#readme) spans and creates SLI metrics from indicator spans.

# Configuration

The `indicator_span_timer_name` controls the generated metric name.

# Status

**This sink is stable**. Some some encoding or options may change, as it is in active development.

# Capabilities

SSF spans contain a `Metric` field which can contain many SSF metrics. This sink adds those metrics to Veneur's aggregators.

### Indicators

Additionally, if a SSF span's `indicator` flag is true, then a timer will be added for
Veneur's next flush. The `indicator_span_timer_name` controls the name used. The
following tags are also set:

* SSF field `name` is mapped to the tag `span_name`
* SSF field `service` is mapped to the tag `service`
* SSF field `error` is mapped to the tag `error` with a value of `true` or `false`
* The unit of the metric is nanoseconds
