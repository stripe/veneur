# Kafka Plugin

The Kafka plugin sends flushed metrics to a [Kafka](https://kafka.apache.org/) topic.

**Note: This plugin is still in an experimental state.**

# Configuration

See the various `kafka_*` keys in [example.yaml](https://github.com/stripe/veneur/blob/master/example.yaml) for all available configuration options. This sink supports the following features:

* batching
* ack requirements
* publishing of Protobuf or JSON formatted messages
