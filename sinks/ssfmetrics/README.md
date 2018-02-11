# SSFMetrics Sink

This sink creates timer metrics from SSF indicator spans.

# Status

**This sink is experimental**. Names and tags are likely to change.

# Capabilities

## Metrics

If an SSF span's `indicator` flag is true, then a timer will be added for
Veneur's next flush. The `indicator_span_timer_name` controls the name used. The
following tags are also set:

* SSF field `Name` is mapped to the tag `span_name`
* SSF field `Service` is mapped to the tag `service`
* SSF field `Error` is mapped to the tag `error` with a value of `true` or `false`
* The unit of the metric is nanoseconds

# Configuration

The `indicator_span_timer_name` controls the generated metric name.
