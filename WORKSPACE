git_repository(
    name = "io_bazel_rules_go",
    remote = "https://github.com/bazelbuild/rules_go.git",
    tag = "0.4.4",
)

load("@io_bazel_rules_go//go:def.bzl", "go_repositories", "new_go_repository")

go_repositories()

new_go_repository(
    name = "com_github_aws_aws_sdk_go",
    commit = "5717392a1fc0a2cc1e35b28eef3154ce318b18f4",
    importpath = "github.com/aws/aws-sdk-go",
)

new_go_repository(
    name = "com_github_jmespath_go_jmespath",
    commit = "bd40a432e4c76585ef6b72d3fd96fb9b6dc7b68d",
    importpath = "github.com/jmespath/go-jmespath",
)

new_go_repository(
    name = "com_github_go_ini_ini",
    commit = "6e4869b434bd001f6983749881c7ead3545887d8",
    importpath = "github.com/go-ini/ini",
)
