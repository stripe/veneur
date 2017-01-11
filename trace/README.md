Trace
=========

Veneur provides an experimental API for tracing requests between hosts. The API is in an experimental state and is subject to change.

The main Veneur tracing API is defined in the `trace.go` file; however, an OpenTracing compatibility layer is provided for convenience as well. These are functionally equivalent, though it is recommended not to mix the functions. (In other words, if a trace is started using Veneur's tracing API, avoid using the OpenTracing API functions on it or future children).

Eventually, these two interfaces will be consolidated.


