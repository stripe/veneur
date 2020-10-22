package veneur

//go:generate protoc --go_opt=paths=source_relative --go_out=. --go-grpc_out=. --go-grpc_opt=paths=source_relative sinks/grpsink/grpc_sink.proto
//go:generate protoc --go_opt=paths=source_relative --go_out=. tdigest/gogoproto/gogo.proto
//go:generate protoc --go_opt=paths=source_relative --go_out=. ssf/sample.proto
//go:generate protoc --go_opt=paths=source_relative --go_out=. -I=$GOPATH/src -I=. tdigest/tdigest.proto
//go:generate protoc --go_opt=paths=source_relative --go_out=. -I=$GOPATH/src -I=. samplers/metricpb/metric.proto
//go:generate protoc --go_opt=paths=source_relative --go_out=. forwardrpc/forward.proto
//go:generate gojson -input example.yaml -o config.go -fmt yaml -pkg veneur -name Config
//go:generate gojson -input example_proxy.yaml -o config_proxy.go -fmt yaml -pkg veneur -name ProxyConfig
//go:generate stringer -type MetricType samplers
//TODO(aditya) reenable go:generate gojson -input fixtures/datadog_trace.json -o datadog_trace_span.go -fmt json -pkg veneur -name DatadogTraceSpan
