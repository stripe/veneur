# Changelog
## v0.13.0
* BasicTracer has been removed.
* Tracer now takes a SpanRecorder as an option.
* Tracer interface now includes Close and Flush.
* Tests redone with ginkgo/gomega.

## v0.12.0 
* Added CloseTracer function to flush and close a lightstep recorder.

## v0.11.0 
* Thrift transport is now deprecated, gRPC is the default.