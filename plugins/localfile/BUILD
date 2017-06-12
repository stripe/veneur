load("@io_bazel_rules_go//go:def.bzl", "go_library", "go_test")

go_library(
    name = "go_default_library",
    srcs = ["localfile.go"],
    visibility = ["//visibility:public"],
    deps = [
        "//plugins:go_default_library",
        "//plugins/s3:go_default_library",
        "//samplers:go_default_library",
        "//vendor:github.com/Sirupsen/logrus",
    ],
)

go_test(
    name = "go_default_test",
    srcs = ["localfile_test.go"],
    library = ":go_default_library",
    deps = [
        "//samplers:go_default_library",
        "//vendor:github.com/Sirupsen/logrus",
        "//vendor:github.com/stretchr/testify/assert",
    ],
)
