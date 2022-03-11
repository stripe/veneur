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

* `cloudwatch_standard_unit_tag_name` (default: `cloudwatch_standard_unit`): Specify a [standard unit](https://pkg.go.dev/github.com/aws/aws-sdk-go-v2/service/cloudwatch@v1.17.0/types?utm_source=gopls#StandardUnit) on a per-metric-datum basis, e.g. "Seconds", "Bytes"

## Status

This sink is experimental. It is functional, but use caution for production workloads.
