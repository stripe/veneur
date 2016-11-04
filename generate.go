package veneur

//go:generate protoc --go_out=ssf sample.proto
//go:generate gojson -input example.yaml -o config.go -fmt yaml -pkg veneur -name Config
