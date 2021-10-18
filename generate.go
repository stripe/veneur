package veneur

//go:generate protoc --gogofaster_out=Mssf/sample.proto=github.com/stripe/veneur/v14/ssf,plugins=grpc:. sinks/grpsink/grpc_sink.proto
//go:generate protoc --gogofaster_out=plugins=grpc:. ssf/grpc.proto
//go:generate protoc --gogofaster_out=plugins=grpc:. protocol/dogstatsd/grpc.proto
//go:generate protoc --gogofaster_out=. ssf/sample.proto
//go:generate protoc -I=. -I=$GOPATH/pkg/mod -I=$GOPATH/pkg/mod/github.com/gogo/protobuf@v1.2.1/protobuf --gogofaster_out=. tdigest/tdigest.proto
//go:generate protoc -I=. -I=$GOPATH/pkg/mod -I=$GOPATH/pkg/mod/github.com/gogo/protobuf@v1.2.1/protobuf --gogofaster_out=Mtdigest/tdigest.proto=github.com/stripe/veneur/v14/tdigest:. samplers/metricpb/metric.proto
//go:generate protoc -I=. -I=$GOPATH/pkg/mod -I=$GOPATH/pkg/mod/github.com/gogo/protobuf@v1.2.1/protobuf --gogofaster_out=Mtdigest/tdigest.proto=github.com/stripe/veneur/v14/tdigest,Msamplers/metricpb/metric.proto=github.com/stripe/veneur/v14/samplers/metricpb,Mgoogle/protobuf/empty.proto=github.com/golang/protobuf/ptypes/empty,plugins=grpc:. forwardrpc/forward.proto
//go:generate gojson -input example_proxy.yaml -o config_proxy.go -fmt yaml -pkg veneur -name ProxyConfig
//go:generate stringer -type MetricType ./samplers
//TODO(aditya) reenable go:generate gojson -input fixtures/datadog_trace.json -o datadog_trace_span.go -fmt json -pkg veneur -name DatadogTraceSpan
