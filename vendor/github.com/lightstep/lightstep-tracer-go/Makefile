# tools
GO=go

default: build

.PHONY: default build test

# Thrift
ifeq (,$(wildcard $(GOPATH)/src/github.com/lightstep/common-go/crouton.thrift))
lightstep_thrift/constants.go:
else
# LightStep-specific: rebuilds the LightStep thrift protocol files.
# Assumes the command is run within the LightStep development
# environment (i.e., private repos are cloned in GOPATH).
lightstep_thrift/constants.go: $(GOPATH)/src/github.com/lightstep/common-go/crouton.thrift
	docker run --rm -v "$(GOPATH)/src/github.com/lightstep/common-go:/data" -v "$(PWD):/out" thrift:0.9.2 \
	  thrift --gen go:package_prefix='github.com/lightstep/lightstep-tracer-go/',thrift_import='github.com/lightstep/lightstep-tracer-go/thrift_0_9_2/lib/go/thrift' -out /out /data/crouton.thrift
	rm -rf lightstep_thrift/reporting_service-remote
endif

# gRPC
ifeq (,$(wildcard lightstep-tracer-common/collector.proto))
collectorpb/collector.pb.go:
else
collectorpb/collector.pb.go: lightstep-tracer-common/collector.proto
	docker run --rm -v $(shell pwd)/lightstep-tracer-common:/input:ro -v $(shell pwd)/collectorpb:/output \
	  lightstep/protoc:latest \
	  protoc --go_out=plugins=grpc:/output --proto_path=/input /input/collector.proto
endif

# gRPC
ifeq (,$(wildcard lightstep-tracer-common/collector.proto))
lightsteppb/lightstep_carrier.pb.go:
else
lightsteppb/lightstep_carrier.pb.go: lightstep-tracer-common/lightstep_carrier.proto
	docker run --rm -v $(shell pwd)/lightstep-tracer-common:/input:ro -v $(shell pwd)/lightsteppb:/output \
	  lightstep/protoc:latest \
	  protoc --go_out=plugins=grpc:/output --proto_path=/input /input/lightstep_carrier.proto
endif

test: lightstep_thrift/constants.go collectorpb/collector.pb.go lightsteppb/lightstep_carrier.pb.go
	${GO} test $(shell go list ./... | grep -v /vendor/)
	docker run --rm -v $(GOPATH):/input:ro lightstep/noglog:latest noglog github.com/lightstep/lightstep-tracer-go

build: lightstep_thrift/constants.go collectorpb/collector.pb.go lightsteppb/lightstep_carrier.pb.go version.go
	${GO} build github.com/lightstep/lightstep-tracer-go/...

# When releasing significant changes, make sure to update the semantic
# version number in `./VERSION`, merge changes, then run `make release_tag`.
version.go: VERSION
	./tag_version.sh

release_tag:
	git tag -a v`cat ./VERSION`
	git push origin v`cat ./VERSION`
