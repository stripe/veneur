FROM golang:1.7
MAINTAINER The Stripe Observability Team <support@stripe.com>

RUN mkdir -p /build
ENV GOPATH=/go
RUN go get -u -v github.com/kardianos/govendor
WORKDIR /go/src/github.com/stripe/veneur
ADD . /go/src/github.com/stripe/veneur
CMD cp -r henson /build/ && govendor test -v -timeout 10s +local && go build -a -v -ldflags "-X github.com/stripe/veneur.VERSION=$(git rev-parse HEAD)" -o /build/veneur ./cmd/veneur
