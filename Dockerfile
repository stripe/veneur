# For running Veneur under Docker, you probably want either the pre-built images
# published at https://hub.docker.com/r/stripe/veneur/
# or the Dockerfiles in https://github.com/stripe/veneur/tree/master/public-docker-images
FROM golang:1.17
LABEL maintainer="The Stripe Observability Team <support@stripe.com>"

ENV GOPATH=/go
ENV GO111MODULE=on
RUN apt-get update
RUN apt-get install -y zip
RUN go get -u -v github.com/ChimeraCoder/gojson/gojson
RUN go get -u -v github.com/gogo/protobuf/protoc-gen-gogofaster@v1.2.1
RUN go get -u -v golang.org/x/tools/cmd/stringer
WORKDIR /protoc
RUN wget https://github.com/google/protobuf/releases/download/v3.1.0/protoc-3.1.0-linux-x86_64.zip
RUN unzip protoc-3.1.0-linux-x86_64.zip
RUN cp bin/protoc /usr/bin/protoc
RUN chmod 777 /usr/bin/protoc

WORKDIR /veneur
ADD . /veneur

# If running locally, ignore any changes since
# the last commit
RUN git reset --hard HEAD && git status

# Unlike the travis build file, we do NOT need to
# ignore changes to protobuf-generated output
# because we are guaranteed only one version of Go
# used to build protoc-gen-go
RUN go generate
# Exclude vendor from gofmt checks.
RUN mv vendor ../ && gofmt -w . && mv ../vendor .

# Stage any changes caused by go generate and gofmt,
# then confirm that there are no staged changes.
#
# If `go generate` or `gofmt` yielded any changes,
# this will fail with an error message like "too many arguments"
# or "M: binary operator expected"
# Due to overlayfs peculiarities, running git diff-index without --cached
# won't work, because it'll compare the mtimes (which have changed), and
# therefore reports that the file may have changed (ie, a series of 0s)
# See https://github.com/stripe/veneur/pull/110#discussion_r92843581
RUN git add .
# The output will be empty unless the build fails, in which case this
# information is helpful in debugging
RUN git diff --cached
RUN git diff-index --cached --exit-code HEAD

RUN mkdir -p /build
RUN go test -race -v -timeout 60s -ldflags "-X github.com/stripe/veneur/v14.VERSION=$(git rev-parse HEAD) -X github.com/stripe/veneur/v14.BUILD_DATE=$(date +%s)" ./...
CMD cp -r henson /build/ && env GOBIN=/build go install -a -v -ldflags "-X github.com/stripe/veneur/v14.VERSION=$(git rev-parse HEAD) -X github.com/stripe/veneur/v14.BUILD_DATE=$(date +%s)" ./cmd/...
