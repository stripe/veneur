load("@io_bazel_rules_go//go:def.bzl", "go_library", "go_test")

go_library(
    name = "go_default_library",
    srcs = [
        "opentracing.go",
        "trace.go",
    ],
    visibility = ["//visibility:public"],
    deps = [
        "//ssf:go_default_library",
        "//vendor:github.com/Sirupsen/logrus",
        "//vendor:github.com/golang/protobuf/proto",
        "//vendor:github.com/opentracing/opentracing-go",
        "//vendor:github.com/opentracing/opentracing-go/log",
    ],
)

go_test(
    name = "go_default_test",
    srcs = [
        "opentracing_test.go",
        "trace_test.go",
    ],
    library = ":go_default_library",
    deps = [
        "//ssf:go_default_library",
        "//vendor:github.com/golang/protobuf/proto",
        "//vendor:github.com/opentracing/opentracing-go",
        "//vendor:github.com/stretchr/testify/assert",
    ],
)
