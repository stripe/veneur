# The Veneur code structure

If you're reading this, you may be interested in writing code in
veneur! We couldn't be more excited about this & hope you find this
file useful! It's a living document, so we'll update it when veneur's
internals change.

Let's dive right in: Veneur is a distributed, fault-tolerant pipeline
for runtime data. It is organized into some components meant to be
used by people running Veneur:

* A local server that accepts data, splits and forwards that data
  (think local on each box in your infrastructure).
* A set of global servers that aggregates data (think global to your infrastructure)
* A proxy server to ensure aggregate data gets to a stable set of hosts
* A client library for submitting `SSF` data to veneurs.

These parts all make use of some parts of the actual Veneur code base,
and that is what we're going to focus on here. Veneur has some good
godoc [documentation](https://godoc.org/github.com/stripe/veneur/) for
its component functions, and this document hopefully provides a bit of
information on how it all fits together!

## Common parts

All parts of Veneur use its
own [client library](https://godoc.org/github.com/stripe/veneur/trace)
to provide information about its own runtime behavior (by
submitting [ssf](https://godoc.org/github.com/stripe/veneur/ssf) spans
to itself).

## Veneur the server

Veneur's [`Server`](https://godoc.org/github.com/stripe/veneur#Server)
structure is what veneur runs for receiving, submitting and forwarding
metrics along the pipeline.

All of Veneur's setup and startup behavior happens in two
functions:
[`veneur.NewFromConfig`](https://godoc.org/github.com/stripe/veneur#NewFromConfig) and
[`(*Server).Start`](https://godoc.org/github.com/stripe/veneur#Server.Start).

`veneur.NewFromConfig` creates a veneur server instance from the
configuration object (typically parsed from the YAML file). It sets up
sinks and options on the instance so that it's runnable in a way
reflecting the configuration.

`(*Server).Start` starts the veneur machine, starts listening on the
configured ports and kicks off goroutines reading channels on which
metrics and trace spans can come in.

### A local server

The local server listens for metrics on a local instance, and
either submits them to a storage backend directly, or forwards the
data to a global veneur. Typically, a global server will also be a
local server, in order to allow collecting metrics on the global
veneur machine, so you can think of a local server as a strict
subset of a global server.

When a local server's `(*Server).Start` method is called, that kicks
off listening for Statsd metrics and for SSF spans:

* statsd metrics are read by [`(*Server).ReadMetricSocket`](https://godoc.org/github.com/stripe/veneur#Server.ReadMetricSocket) (for UDP) or [`(*Server).ReadTCPSocket`](https://godoc.org/github.com/stripe/veneur#Server.ReadTCPSocket).
* SSF spans on UNIX domain sockets are read by [`ReadSSFStreamSocket`](https://godoc.org/github.com/stripe/veneur#Server.ReadSSFStreamSocket) using the framing protocol in [package `protocol`](https://godoc.org/github.com/stripe/veneur/protocol).
* SSF spans from UDP are read by [`(*Server).ReadSSFPacketSocket`](https://godoc.org/github.com/stripe/veneur#Server.ReadSSFPacketSocket). The packets contain only the protobuf representation of an SSF span.

#### Workers - processing data internally

Once read, metrics and spans are submitted to
a [`Worker`](https://godoc.org/github.com/stripe/veneur#Worker), and
spans are submitted to
a [`SpanWorker`](https://godoc.org/github.com/stripe/veneur#SpanWorker).

These workers encapsulate a goroutine, exposing a channel that can
accept new metrics or spans respectively. In periodic intervals, the
`Server`'s
[`Flush`](https://godoc.org/github.com/stripe/veneur#Server.Flush)
method calls each worker's `Flush` method ---
[`(*Worker).Flush`](https://godoc.org/github.com/stripe/veneur#SpanWorker.Flush) and
[`(*SpanWorker).Flush`](https://godoc.org/github.com/stripe/veneur#SpanWorker.Flush) ---
to collect the data that needs forwarding, then sends that data to
the appropriate Sink.

#### `MetricSink`s - submitting metrics to storage

First, we have to talk about what package `samplers` does:

A metric starts out life as
a [`UDPMetric`](https://godoc.org/github.com/stripe/veneur/samplers#UDPMetric) -
data read from a UDP packet.

The worker converts each `UDPMetric` (based on
its
[`MetricKey`](https://godoc.org/github.com/stripe/veneur/samplers#MetricKey) to
a concrete metric structure -- `Gauge`, `Counter`, `Histo`, `Set` are
currently supported. Each of these concrete metrics has a `Flush` and
an `Export` method that takes the data accumulated during the flush
interval and either sends it directly, or (if so configured) gets
forwarded to a global veneur.

If the metric can be submitted directly, `Flush`
(e.g. [`(*Counter).Flush`](https://godoc.org/github.com/stripe/veneur/samplers#Counter.Flush))
converts the counter to an `InterMetric` array, which can be passed to a concrete
[`MetricSink`](https://godoc.org/github.com/stripe/veneur/sinks#MetricSink) for
submission to each metric storage system.

If the metric should be forwarded to a global veneur (e.g. if it's a
Set or a Histogram), it is converted to
a [`JSONMetric`](https://godoc.org/github.com/stripe/veneur/samplers#JSONMetric) and
then submitted as an HTTP POST.

**Note:** Only the veneur server that ends up submitting a metric to a metric
sink converts that metric to an `InterMetric`, and all `InterMetric`s
are submitted to the sink directly!

Phew! This is all fairly complex wordy, so here's a diagram to maybe
clear up how a UDP counter data flow looks:

```
┌─Veneur server───────────────────────┐
│ ╔══════════Metric Worker══════════╗ │
│ ║     ┌─────────────────────┐     ║ │
│ ║     │      UDPMetric      │     ║ │
│ ║     └─────────────────────┘     ║ │
│ ║                │                ║ │
│ ║                │                ║ │
│ ║                ▼                ║ │
│ ║     ┌─────────────────────┐     ║ │
│ ║     │       Counter       │     ║ │
│ ║     └─────────────────────┘     ║ │
│ ║                │                ║ │
│ ║                │                ║ │
│ ║                ▼                ║ │
│ ║     ┌─────────────────────┐     ║ │
│ ║     │     InterMetric     │     ║ │
│ ║     └─────────────────────┘     ║ │
│ ║                │                ║ │
│ ╚════════════════╬════════════════╝ │
│                  │                  │
│                  ▼                  │
│       ╔═════════════════════╗       │
│       ║     Metric Sink     ║       │
│       ╚═════════════════════╝       │
└─────────────────────────────────────┘
```

There are multiple metric sinks,
with [`datadog.DatadogMetricSink`](https://godoc.org/github.com/stripe/veneur/sinks/datadog#DatadogMetricSink)
being the most prominent one. Each metric sink is responsible for
submitting all the data passed to its `.Flush` method to the metrics
storage.
