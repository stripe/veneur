# SignalFx Sink

This sink sends Veneur metrics to [SignalFx](https://signalfx.com/).

# Status

**This sink is experimental**. This sink does not yet provide all of the functionality that SignalFx allows. Some of Veneur's primitives still need to be mapped to those of SignalFx.

# Configuration

* `signalfx_api_key` to set the API key.
* `signalfx_endpoint_base` to adjust where metrics are sent. If you use a proxy or something in front of SignalFx's API this is the place! Do not add a path (i.e. `https://ingest.signalfx.com` is right!)
* `signalfx_hostname_tag` to adjust the tag in which the hostname of this instance will be added. SignalFx doesn't have a first-class hostname field, so this tag handles that.

# TODO

* Does not handle events correctly yet, only copies timestamp, title and tags
