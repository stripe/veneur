package(default_visibility = ["//:__subpackages__"])

load("@io_bazel_rules_go//go:def.bzl", "go_library")

go_library(
    name = "github.com/DataDog/datadog-go/statsd",
    srcs = glob(
        ["github.com/DataDog/datadog-go/statsd/*.go"],
        exclude = ["**/*_test.go"],
    ),
)

go_library(
    name = "github.com/davecgh/go-spew/spew",
    srcs = glob(
        ["github.com/davecgh/go-spew/spew/*.go"],
        exclude = ["**/*_test.go"],
    ),
)

go_library(
    name = "github.com/pmezard/go-difflib/difflib",
    srcs = glob(
        ["github.com/pmezard/go-difflib/difflib/*.go"],
        exclude = ["**/*_test.go"],
    ),
)

go_library(
    name = "github.com/stretchr/testify/assert",
    srcs = glob(
        ["github.com/stretchr/testify/assert/*.go"],
        exclude = ["**/*_test.go"],
    ),
    deps = [
        ":github.com/davecgh/go-spew/spew",
        ":github.com/pmezard/go-difflib/difflib",
    ],
)

go_library(
    name = "github.com/clarkduvall/hyperloglog",
    srcs = glob(
        ["github.com/clarkduvall/hyperloglog/*.go"],
        exclude = ["**/*_test.go"],
    ),
)

go_library(
    name = "github.com/Sirupsen/logrus",
    srcs = glob(
        ["github.com/Sirupsen/logrus/*.go"],
        exclude = ["**/*_test.go"],
    ),
)

go_library(
    name = "github.com/golang/protobuf/proto",
    srcs = glob(
        ["github.com/golang/protobuf/proto/*.go"],
        exclude = ["**/*_test.go"],
    ),
)

go_library(
    name = "github.com/opentracing/opentracing-go",
    srcs = glob(
        ["github.com/opentracing/opentracing-go/*.go"],
        exclude = ["**/*_test.go"],
    ),
    deps = [
        ":github.com/opentracing/opentracing-go/log",
        ":golang.org/x/net/context",
    ],
)

go_library(
    name = "github.com/opentracing/opentracing-go/log",
    srcs = glob(
        ["github.com/opentracing/opentracing-go/log/*.go"],
        exclude = ["**/*_test.go"],
    ),
)

go_library(
    name = "github.com/getsentry/raven-go",
    srcs = glob(
        ["github.com/getsentry/raven-go/*.go"],
        exclude = ["**/*_test.go"],
    ),
)

go_library(
    name = "github.com/hashicorp/consul/api",
    srcs = glob(
        ["github.com/hashicorp/consul/api/*.go"],
        exclude = ["**/*_test.go"],
    ),
    deps = [
        ":github.com/hashicorp/go-cleanhttp",
        ":github.com/hashicorp/serf/coordinate",
    ],
)

go_library(
    name = "github.com/hashicorp/go-cleanhttp",
    srcs = glob(
        ["github.com/hashicorp/go-cleanhttp/*.go"],
        exclude = ["**/*_test.go"],
    ),
)

go_library(
    name = "github.com/pkg/profile",
    srcs = glob(
        ["github.com/pkg/profile/*.go"],
        exclude = ["**/*_test.go"],
    ),
)

go_library(
    name = "github.com/zenazn/goji/bind",
    srcs = glob(
        ["github.com/zenazn/goji/bind/*.go"],
        exclude = ["**/*_test.go"],
    ),
)

go_library(
    name = "github.com/zenazn/goji/graceful",
    srcs = glob(
        ["github.com/zenazn/goji/graceful/*.go"],
        exclude = ["**/*_test.go"],
    ),
    deps = [":github.com/zenazn/goji/graceful/listener"],
)

go_library(
    name = "github.com/zenazn/goji/graceful/listener",
    srcs = glob(
        ["github.com/zenazn/goji/graceful/listener/*.go"],
        exclude = ["**/*_test.go"],
    ),
)

go_library(
    name = "goji.io",
    srcs = glob(
        ["goji.io/*.go"],
        exclude = ["**/*_test.go"],
    ),
    deps = [
        ":goji.io/internal",
        ":golang.org/x/net/context",
    ],
)

go_library(
    name = "goji.io/internal",
    srcs = glob(
        ["goji.io/internal/*.go"],
        exclude = ["**/*_test.go"],
    ),
    deps = [":golang.org/x/net/context"],
)

go_library(
    name = "goji.io/pat",
    srcs = glob(
        ["goji.io/pat/*.go"],
        exclude = ["**/*_test.go"],
    ),
    deps = [
        ":goji.io/internal",
        ":goji.io/pattern",
        ":golang.org/x/net/context",
    ],
)

go_library(
    name = "goji.io/pattern",
    srcs = glob(
        ["goji.io/pattern/*.go"],
        exclude = ["**/*_test.go"],
    ),
    deps = [
        ":goji.io/internal",
        ":golang.org/x/net/context",
    ],
)

go_library(
    name = "golang.org/x/net/context",
    srcs = glob(
        ["golang.org/x/net/context/*.go"],
        exclude = ["**/*_test.go"],
    ),
)

go_library(
    name = "golang.org/x/sys/unix",
    srcs = glob(
        ["golang.org/x/sys/unix/*.go"],
        exclude = ["**/*_test.go"],
    ),
)

go_library(
    name = "gopkg.in/yaml.v2",
    srcs = glob(
        ["gopkg.in/yaml.v2/*.go"],
        exclude = ["**/*_test.go"],
    ),
)

go_library(
    name = "stathat.com/c/consistent",
    srcs = glob(
        ["stathat.com/c/consistent/*.go"],
        exclude = ["**/*_test.go"],
    ),
)

go_library(
    name = "github.com/hashicorp/serf/coordinate",
    srcs = glob(
        ["github.com/hashicorp/serf/coordinate/*.go"],
        exclude = ["**/*_test.go"],
    ),
)
