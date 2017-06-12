load("@io_bazel_rules_go//go:def.bzl", "go_library", "go_test")

go_library(
    name = "go_default_library",
    srcs = [
        "parser.go",
        "samplers.go",
        "split_bytes.go",
    ],
    visibility = ["//visibility:public"],
    deps = [
        "//tdigest:go_default_library",
        "//vendor:github.com/clarkduvall/hyperloglog",
    ],
)

go_test(
    name = "go_default_test",
    srcs = ["samplers_test.go"],
    library = ":go_default_library",
    deps = ["//vendor:github.com/stretchr/testify/assert"],
)
