package veneur

//go:generate protoc --gogofaster_out=Mssf/sample.proto=github.com/stripe/veneur/v14/ssf,plugins=grpc:. sinks/falconer/grpc_sink.proto
//go:generate protoc --gogofaster_out=plugins=grpc:. ssf/grpc.proto
//go:generate protoc --gogofaster_out=plugins=grpc:. protocol/dogstatsd/grpc.proto
//go:generate protoc --gogofaster_out=. ssf/sample.proto
//go:generate protoc -I=. -I=$GOPATH/pkg/mod -I=$GOPATH/pkg/mod/github.com/gogo/protobuf@v1.2.1/protobuf --gogofaster_out=. tdigest/tdigest.proto
//go:generate protoc -I=. -I=$GOPATH/pkg/mod -I=$GOPATH/pkg/mod/github.com/gogo/protobuf@v1.2.1/protobuf --gogofaster_out=Mtdigest/tdigest.proto=github.com/stripe/veneur/v14/tdigest:. samplers/metricpb/metric.proto
//go:generate protoc -I=. -I=$GOPATH/pkg/mod -I=$GOPATH/pkg/mod/github.com/gogo/protobuf@v1.2.1/protobuf --gogofaster_out=Mtdigest/tdigest.proto=github.com/stripe/veneur/v14/tdigest,Msamplers/metricpb/metric.proto=github.com/stripe/veneur/v14/samplers/metricpb,Mgoogle/protobuf/empty.proto=github.com/golang/protobuf/ptypes/empty,plugins=grpc:. forwardrpc/forward.proto
//go:generate stringer -type MetricType ./samplers
//TODO(aditya) reenable go:generate gojson -input fixtures/datadog_trace.json -o datadog_trace_span.go -fmt json -pkg veneur -name DatadogTraceSpan
//go:generate mockgen -source=sinks/sinks.go -destination=sinks/mock/mock.go -package=mock
//go:generate mockgen -source=sources/sources.go -destination=sources/mock/mock.go -package=mock
