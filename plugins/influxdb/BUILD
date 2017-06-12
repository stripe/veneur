load("@io_bazel_rules_go//go:def.bzl", "go_library")

go_library(
    name = "go_default_library",
    srcs = ["influxdb.go"],
    visibility = ["//visibility:public"],
    deps = [
        "//plugins:go_default_library",
        "//samplers:go_default_library",
        "//vendor:github.com/DataDog/datadog-go/statsd",
        "//vendor:github.com/Sirupsen/logrus",
    ],
)
