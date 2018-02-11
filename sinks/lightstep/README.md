# LightStep Sink

This sink sends Veneur spans to [LightStep](https://lightstep.com).

# Status

**This sink is production ready**.

# Capabilities

## Spans

Enabled if `lightstep_access_token` is set to non-empty value.

The following rules manage how [SSF](https://github.com/stripe/veneur/tree/master/ssf)
spans and tags are mapped to LightStep spans:

* The SSF `indicator` field is converted a tag.
* The SSF `service` field is mapped to LightStep's `component` field.

# Collector Distribution

Veneur can create `lightstep_num_clients` number of connections to LightStep
collectors. Veneur will then distribute spans to each of these connections. This
is useful if you don't have a appropriate load-balancer for balancing gRPC
connections. You can also set the `lightstep_reconnect_period` to change how
often each of these connections reconnect. Using these together can help facilitate
an even distribution of spans across collectors.

# Configuration

See the various `lightstep_*` keys in [example.yaml](https://github.com/stripe/veneur/blob/master/example.yaml) for all available configuration options.
