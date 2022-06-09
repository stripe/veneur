#!/usr/bin/env bash
#
# Generate all protobuf bindings.

PATH=$PATH:$(go env GOPATH)/bin protoc -I=. -I=$(go env GOPATH)/src/github.com/gogo/protobuf -I=$(go env GOPATH)/src/github.com/gogo/protobuf/protobuf --gogofast_out=plugins=grpc:. *.proto
sed -i.bak -E '/import _ \"gogoproto\"/d' *.pb.go