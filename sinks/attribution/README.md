# Attribution

`attribution` is a sink used to profile the makeup of metrics processed by Veneur. This includes qualitative information such as what service (application) owners emit metrics as well as quantitative (the cardinality of metric namespaces/tags, the rate at which datapoints are flushed from sinks).

Using the attribution sink will likely look something like this for your organization:

![Example Attribution Sink Topology](./example_topology.png)

Although this diagram makes reference to a Veneur > Spark > Presto pipeline, Spark and Presto are provided as examples of what one could do with data flushed from the attribution sink. There is no code in this PR to set up such a pipeline.

## Development Status

This sink is experimental and makes opinionated assumptions about your organization that may not apply to all organizations.

- Your organization does not care about attributing metrics in real time, and is willing to use the experimental Metrics Sink Routing feature to tally and compute attribution metrics out of band from metrics being forwarded to their final destination
- Your organization does not care about attributing spans
- Your organization uses AWS S3

This sink is experimental; use with caution and be mindful of non-backwards-compatible changes that may occur even with a Veneur major version bump.

## Usage

```yaml
metric_sinks:
  - kind: attribution
    name: attribution
    config:
      veneur_instance_id: workerbox-1234.northwest.qa.paymentscompany.com
      aws_region: us-west-2
      s3_bucket: veneur-attribution
```
