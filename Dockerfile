FROM golang:1.6
MAINTAINER The Stripe Observability Team <support@stripe.com>

RUN mkdir -p /build
ENV GOPATH=/go
RUN go get -u -v github.com/kardianos/govendor
WORKDIR /go/src/github.com/stripe/veneur
ADD . /go/src/github.com/stripe/veneur
CMD cp -r henson /build/ && govendor test -v -timeout 10s +local && go build -a -v -o /build/veneur ./cmd/veneur
