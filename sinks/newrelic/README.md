# **** Work In Progress, all pricing examples are a best guess and not official ****


# New Relic Sink

This sink sends Veneur Metrics, Events, and Distributed Traces to [New Relic](https://newrelic.com)

# Configuration

Se the various `newrelic_*` keys in keys in [example.yaml](https://github.com/stripe/veneur/blob/master/example.yaml) for all available configuration options.

# Status

**This sink is in active development**

# Capabilities
Veneur (/vɛnˈʊr/, rhymes with “assure”) is a distributed, fault-tolerant pipeline for runtime data. It provides a server implementation of the DogStatsD protocol or SSF for aggregating metrics and sending them to downstream storage to one or more supported sinks. It can also act as a global aggregator for histograms, sets and counters.

Here are some examples of why Stripe and other companies are using Veneur today:

* reducing cost by pre-aggregating metrics such as timers into percentiles
* creating a vendor-agnostic metric collection pipeline
consolidating disparate observability data (from trace spans to metrics, and more!)
* improving efficiency over other metric aggregator implementations
* improving reliability by building a more resilient forwarding system over single points of failure

Global \*StatsD installations can be problematic, as they either require client-side or proxy sharding behavior to prevent an
instance being a Single Point of Failure (SPoF) for all metrics. Veneur aims to solve this problem. Non-global
metrics like counters and gauges are collected by a local Veneur instance and sent to storage at flush time. Global metrics (histograms and sets)
are forwarded to a central Veneur instance for aggregation before being sent to storage.

## Metrics
New Relic's Telemetry Data Platform’s free tier allows for ingesting up to 100GB per month (total) at no cost, regardless of the source or type of data. Leveraging Veneur allows you to easily switch between Metric consumers.

## Events
Events allow you to take a high cardinality data point such as `userName`. New Relic's Telemetry Data Platform’s free tier allows for ingesting up to 100GB per month (total) at no cost, regardless of the source or type of data.

## Trace Spans
Distributed Tracing allows you to track the entire customer journey, allowing you to easily determine root cause of an issue across complex microservice environments. New Relic's Telemetry Data Platform’s free tier allows for ingesting up to 100GB per month (total) at no cost, regardless of the source or type of data. New Relic Infinite Tracing observes 100% of all trace data while only storing errors for long-term analysis.
