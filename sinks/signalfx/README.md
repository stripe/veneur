# SignalFx Sink

This sink sends Veneur metrics to [SignalFx](https://signalfx.com/).

# Configuration

See the various `signalfx_*` keys in [example.yaml](https://github.com/stripe/veneur/blob/master/example.yaml) for all available configuration options.

# Status

**This sink is stable**. This sink does not yet provide all of the functionality that SignalFx allows. Some of Veneur's primitives still need to be mapped to those of SignalFx.

# Capabilities

## Metrics

Enabled if `signalfx_api_key` is set to a non-empty value.

* Counters are counters.
* Gauges are gauges.

The following tags are mapped to SignalFx fields as follows:

* The configured Veneur `hostname` field is sent to SignalFx as the value from `signalfx_hostname_tag`.

# TODO

* Does not handle events correctly yet, only copies timestamp, title and tags.
* SignalFx does not have a formal concept of per-metric hosts, so `signalfx_hostname_tag` may need some work.
