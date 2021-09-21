Local File Sink
===============

The Local File Sink appends each flush as TSV data to a specified file on the local system.  Since the file path is not parametrized with regards to date or time, the file with the TSV data should be rotated, processed, or removed to avoid problems with filling the disk.

# Configuration

```yaml
metric_sinks:
  - kind: localfile
    name: localfile
    config:
      # The file to flush metrics to
      flush_file: /path/to/file
```

The `flush_file` path must be writeable by Veneur, and if the file does not exist, Veneur will try to create it.
