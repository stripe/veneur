FROM golang:1.7
MAINTAINER The Stripe Observability Team <support@stripe.com>

RUN mkdir -p /build
ENV GOPATH=/go
RUN apt-get update
RUN apt-get install -y zip
RUN go get -u -v github.com/kardianos/govendor
RUN go get -u -v github.com/ChimeraCoder/gojson/gojson
RUN go get -u github.com/golang/protobuf/protoc-gen-go
RUN wget https://github.com/google/protobuf/releases/download/v3.1.0/protoc-3.1.0-linux-x86_64.zip
RUN unzip protoc-3.1.0-linux-x86_64.zip
RUN cp bin/protoc /usr/bin/protoc
RUN chmod 777 /usr/bin/protoc

WORKDIR /go/src/github.com/stripe/veneur
ADD . /go/src/github.com/stripe/veneur


# If running locally, ignore any changes since
# the last commit
RUN git reset --hard HEAD && git status
RUN cp -r henson /build/

# Unlike the travis build file, we do NOT need to
# ignore changes to protobuf-generated output
# because we are guaranteed only one version of Go
# used to build protoc-gen-go
RUN go generate
RUN gofmt -w .
# Stage any changes caused by go generate and gofmt,
# then confirm that there are no staged changes.
# TODO figure out why this test is flaky if we don't stage
# and run without --cached.
RUN git add .
RUN git diff-index --cached --exit-code HEAD


RUN govendor test -v -timeout 10s +local
CMD go build -a -v -ldflags "-X github.com/stripe/veneur.VERSION=$(git rev-parse HEAD)" -o /build/veneur ./cmd/veneur
