# Kafka Sink

The Kafka sink allows flushing of metrics or spans to to a [Kafka](https://kafka.apache.org/) topic.

# Status

**This sink is experimental**.

# Configuration

See the various `kafka_*` keys in [example.yaml](https://github.com/stripe/veneur/blob/master/example.yaml) for all available configuration options. This sink supports the following features:

* batching
* ack requirements
* publishing of Protobuf or JSON formatted messages

# Format

Metrics or spans may be published in one of either
