# Kafka Sink

The Kafka sink allows flushing of metrics or spans to to a [Kafka](https://kafka.apache.org/) topic.

# Status

**This sink is experimental**.

## TODO

* Uses the async client, but doesn't currently do anything on failure.
* Does not currently handle writes of events or checks

# Configuration

See the various `kafka_*` keys in [example.yaml](https://github.com/stripe/veneur/blob/master/example.yaml) for all available configuration options. This sink supports the following features:

* batching
* ack requirements
* publishing of Protobuf or JSON formatted messages

## Span Sampling

The Kafka sink supports span sampling: configurable by `kafka_span_sample_tag` and
`kafka_span_sample_rate_percent`. For example, imagine the following configuration:

```
kafka_span_sample_tag: "request_id"
kafka_span_sample_rate_percent: 75
```

With this configuration, spans _without_ the `"request_id"` tag will be rejected,
and spans _with_ the `"request_id"` will be sampled at 75%, based off of a hash
of their `"request_id"` value; in this way, you can sample all values relevant to
a particular tag value.

# Format

Metrics are published in JSON in the form of:

```
{
  "name": "some.metric",
  "timestamp": 1234567, // unix time
  "value": 1.0,
  "tags": [ "tag:value", … ],
  "type": "gauge" // counter, etc
}
```

Spans are published in one of JSON or Protobuf. The form is defined in [SSF's protobuf and codegen output](https://github.com/stripe/veneur/tree/master/ssf). Note that it has a `version` field for compatibility in the future.
