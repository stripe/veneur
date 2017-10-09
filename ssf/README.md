# Sensor Sensibility Format

The Sensor Sensibility Format — or SSF for short — is a language agnostic format for transmitting observability data such as trace spans, metrics, events and more.

# Why?

SSF is based on prior art of metrics formats and protocols mentioned in the [inspiration section](#inspiration). Unlike each of these wonderful formats, which we have used for years and benefited greatly from, SSF is a binary format utilizing [Protocol Buffers](https://developers.google.com/protocol-buffers/). It is emission-, collection- and storage-agnostic. It is merely a protocol for transmitting information. It is developed as part of a suite of technologies around [Veneur](https://github.com/stripe/veneur).

## Why A New Format?

Because we want to:

* leverage protobuf so we no longer have to write buggy, string-based marshaling and unmarshaling code in clients libraries and server implementations
* benefit from the efficiency of protobuf
* collect and combine the great ideas from our [inspiration](https://github.com/stripe/veneur/tree/master/ssf#inspiration)
* add some of [our own ideas](https://github.com/stripe/veneur/tree/master/ssf#philosophy)

# Philosophy

We've got some novel ideas that we've put in to SSF. It might help to be familiar with the concepts in our [inspiration](https://github.com/stripe/veneur/tree/master/ssf#inspiration). Here is our founding philosophy of SSF:

* a timer is a span
* a log line is a span, especially if it's [structured](https://www.thoughtworks.com/radar/techniques/structured-logging)
* events are also spans
* therefore, the core unit of all observability data is a span, or a unit of a trace.
  * spans might be single units, or units of a larger whole
* other point metrics (e.g. counters and gauges) can be constituents of a span
  * it's more valuable to know the depth of a queue in the context of a span than alone
  * improve the context of counters and gauges, as they are part of a span
* provide a format containing the superset of many backend's features

# Inspiration

We build on the shoulders of giants, and are proud to have used and been inspired by these marvelous tools:

* [DogStatsD](http://docs.datadoghq.com/guides/dogstatsd/#datagram-format)
* [Metrics 2.0](http://metrics20.org)
* [OpenTracing](http://opentracing.io)
* [StatsD](https://github.com/b/statsd_spec)
