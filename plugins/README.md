Plugins
================
[![GoDoc](http://img.shields.io/badge/godoc-reference-5272B4.svg?style=flat-square)](https://godoc.org/github.com/stripe/veneur/plugins)

Veneur supports the use of plugins to flush data to multiple destinations. For example, the S3 plugin can be used to archive data to S3 while also flushing to Datadog.

Plugins may not carry the same stability guarantees as the rest of Veneur. For information on a specific plugin, consult the documentation for that particular plugin.


For more information on writing your own flushing plugin for Veneur, see the [package documentation](https://godoc.org/github.com/stripe/veneur/plugins).
