FROM golang:1.9
MAINTAINER The Stripe Observability Team <support@stripe.com>

RUN mkdir -p /build
ENV GOPATH=/go
RUN apt-get update
RUN apt-get install -y zip
RUN go get -u -v github.com/ChimeraCoder/gojson/gojson
RUN go get -u -v github.com/golang/protobuf/protoc-gen-go
RUN go get -u -v github.com/gogo/protobuf/protoc-gen-gofast
RUN cd $GOPATH/src/github.com/gogo/protobuf/ && git checkout v0.5
RUN go install github.com/gogo/protobuf/protoc-gen-gofast
RUN go get -u github.com/golang/dep/cmd/dep
RUN go get -u -v golang.org/x/tools/cmd/stringer
RUN wget https://github.com/google/protobuf/releases/download/v3.1.0/protoc-3.1.0-linux-x86_64.zip
RUN unzip protoc-3.1.0-linux-x86_64.zip
RUN cp bin/protoc /usr/bin/protoc
RUN chmod 777 /usr/bin/protoc

WORKDIR /go/src/github.com/stripe/veneur
ADD . /go/src/github.com/stripe/veneur


# If running locally, ignore any changes since
# the last commit
RUN git reset --hard HEAD && git status

# Unlike the travis build file, we do NOT need to
# ignore changes to protobuf-generated output
# because we are guaranteed only one version of Go
# used to build protoc-gen-go
RUN go generate
RUN dep ensure -v
RUN gofmt -w .

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


RUN go test -race -v -timeout 60s ./...
CMD cp -r henson /build/ && env GOBIN=/build go install -a -v -ldflags "-X github.com/stripe/veneur.VERSION=$(git rev-parse HEAD)" ./cmd/...
