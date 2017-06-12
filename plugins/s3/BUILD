load("@io_bazel_rules_go//go:def.bzl", "go_library", "go_test")

go_library(
    name = "go_default_library",
    srcs = [
        "csv.go",
        "s3.go",
    ],
    visibility = ["//visibility:public"],
    deps = [
        "//plugins:go_default_library",
        "//samplers:go_default_library",
        "//vendor:github.com/Sirupsen/logrus",
        "@com_github_aws_aws_sdk_go//aws:go_default_library",
        "@com_github_aws_aws_sdk_go//service/s3:go_default_library",
        "@com_github_aws_aws_sdk_go//service/s3/s3iface:go_default_library",
    ],
)

go_test(
    name = "go_default_test",
    srcs = [
        "csv_test.go",
        "s3_test.go",
    ],
    data = ["//fixtures:test_data"],
    library = ":go_default_library",
    deps = [
        "//plugins/s3/mock:go_default_library",
        "//samplers:go_default_library",
        "//testhelpers:go_default_library",
        "//vendor:github.com/Sirupsen/logrus",
        "//vendor:github.com/stretchr/testify/assert",
        "@com_github_aws_aws_sdk_go//aws:go_default_library",
        "@com_github_aws_aws_sdk_go//service/s3:go_default_library",
    ],
)
