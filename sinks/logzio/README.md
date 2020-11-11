# Logzio Sink

This sink sends Veneur metrics to [Logz.io](https://www.logz.io).

# Configuration

See the various `logzio_*` keys in [example.yaml](https://github.com/stripe/veneur/blob/master/example.yaml) for all available configuration options.

# Status

**This sink is in active development**

# Capabilities

Veneur (/vɛnˈʊr/, rhymes with “assure”) is a distributed, fault-tolerant pipeline for runtime data.<br>
It provides a server implementation of the DogStatsD protocol or SSF for aggregating metrics and sending them to downstream storage to one or more supported sinks. It can also act as a global aggregator for histograms, sets and counters.<br>
It's a convenient sink for various observability primitives with lots of outputs.
Veneur is also a StatsD or DogStatsD protocol transport, forwarding the locally collected metrics over more reliable TCP implementations.

## Metrics

Enabled if `logzio_shipping_token` and `logzio_listener` are set to non-empty
values.<br>
Conversions:
* Tags will be converted to appear under dimensions.