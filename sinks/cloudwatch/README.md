# CloudWatch

This sink sends metrics to AWS CloudWatch.

## Configuration

```yaml
metric_sinks:
  - kind: cloudwatch
    name: cloudwatch
    config:
      aws_region: us-east-1
      cloudwatch_namespace: veneur
```

### Additional options

* `cloudwatch_endpoint`: Specify a custom endpoint to use in place of the standard [monitoring endpoint](https://docs.aws.amazon.com/general/latest/gr/cw_region.html)
* `cloudwatch_standard_unit_tag_name` (default: `cloudwatch_standard_unit`): Specify a [standard unit](https://pkg.go.dev/github.com/aws/aws-sdk-go-v2/service/cloudwatch@v1.17.0/types?utm_source=gopls#StandardUnit) on a per-metric-datum basis, e.g. "Seconds", "Bytes"
* `remote_timeout` (default: `30s`): Specify an HTTP client timeout, after which the sink will fail if it has not received a PutMetricData response

## Status

This sink is experimental. It is functional, but use caution for production workloads.
