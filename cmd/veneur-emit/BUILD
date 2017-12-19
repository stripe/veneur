load("@io_bazel_rules_go//go:def.bzl", "go_binary", "go_library", "go_test")

go_library(
    name = "go_default_library",
    srcs = ["main.go"],
    visibility = ["//visibility:private"],
    deps = [
        "//:go_default_library",
        "//vendor:github.com/DataDog/datadog-go/statsd",
        "//vendor:github.com/Sirupsen/logrus",
    ],
)

go_binary(
    name = "veneur-emit",
    library = ":go_default_library",
    visibility = ["//visibility:public"],
)

go_test(
    name = "go_default_test",
    srcs = ["main_test.go"],
    library = ":go_default_library",
    deps = [
        "//:go_default_library",
        "//vendor:github.com/Sirupsen/logrus",
    ],
)
