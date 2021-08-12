module github.com/stripe/veneur/v14

go 1.15

require (
	github.com/DataDog/datadog-go v3.7.2+incompatible
	github.com/Shopify/sarama v1.15.0
	github.com/Shopify/toxiproxy v2.1.4+incompatible // indirect
	github.com/araddon/dateparse v0.0.0-20180318191655-f58c961370c5
	github.com/aws/aws-sdk-go v1.31.13
	github.com/axiomhq/hyperloglog v0.0.0-20171114175703-8300947202c9
	github.com/dgryski/go-bits v0.0.0-20160601073636-2ad8d707cc05 // indirect
	github.com/dgryski/go-metro v0.0.0-20170608043646-0f6473574cdf // indirect
	github.com/dropbox/godropbox v0.0.0-20200228041828-52ad444d3502 // indirect
	github.com/eapache/go-resiliency v1.0.0 // indirect
	github.com/eapache/go-xerial-snappy v0.0.0-20160609142408-bb955e01b934 // indirect
	github.com/eapache/queue v1.1.0 // indirect
	github.com/facebookgo/stack v0.0.0-20160209184415-751773369052 // indirect
	github.com/facebookgo/stackerr v0.0.0-20150612192056-c2fcf88613f4 // indirect
	github.com/getsentry/sentry-go v0.6.2-0.20200616133211-abb91bfdb057
	github.com/gogo/protobuf v1.3.1
	github.com/golang/protobuf v1.4.2
	github.com/golang/snappy v0.0.0-20180518054509-2e65f85255db // indirect
	github.com/googleapis/gnostic v0.2.0 // indirect
	github.com/gregjones/httpcache v0.0.0-20180305231024-9cad4c3443a7 // indirect
	github.com/hashicorp/consul/api v1.1.0
	github.com/hashicorp/go-msgpack v0.5.4 // indirect
	github.com/hashicorp/memberlist v0.1.4 // indirect
	github.com/kelseyhightower/envconfig v1.3.0
	github.com/lightstep/lightstep-tracer-go v0.13.1-0.20170818234450-ea8cdd9df863
	github.com/mailru/easyjson v0.0.0-20180606163543-3fdea8d05856 // indirect
	github.com/mitchellh/mapstructure v1.4.1
	github.com/newrelic/newrelic-client-go v0.39.0
	github.com/newrelic/newrelic-telemetry-sdk-go v0.4.0
	github.com/opentracing/opentracing-go v1.0.2
	github.com/peterbourgon/diskv v2.0.1+incompatible // indirect
	github.com/pierrec/lz4 v1.0.2-0.20171218195038-2fcda4cb7018 // indirect
	github.com/pierrec/xxHash v0.1.1 // indirect
	github.com/pkg/errors v0.9.1
	github.com/pkg/profile v1.2.1
	github.com/prometheus/client_golang v1.1.0
	github.com/prometheus/client_model v0.0.0-20190812154241-14fe0d1b01d4
	github.com/prometheus/common v0.6.0
	github.com/prometheus/procfs v0.0.5 // indirect
	github.com/rcrowley/go-metrics v0.0.0-20171128170426-e181e095bae9
	github.com/satori/go.uuid v1.2.1-0.20180103174451-36e9d2ebbde5
	github.com/segmentio/fasthash v1.0.0
	github.com/signalfx/com_signalfx_metrics_protobuf v0.0.0-20170330202426-93e507b42f43 // indirect
	github.com/signalfx/gohistogram v0.0.0-20160107210732-1ccfd2ff5083 // indirect
	github.com/signalfx/golib v2.0.0+incompatible
	github.com/simplereach/timeutils v1.2.0 // indirect
	github.com/sirupsen/logrus v1.6.0
	github.com/stretchr/testify v1.6.1
	github.com/theckman/go-flock v0.0.0-20170522022801-6de226b0d5f0
	github.com/zenazn/goji v0.9.1-0.20160507202103-64eb34159fe5
	goji.io v2.0.2+incompatible
	golang.org/x/mod v0.4.0
	golang.org/x/net v0.0.0-20200707034311-ab3426394381
	golang.org/x/sys v0.0.0-20200625212154-ddb9806d33ae
	google.golang.org/grpc v1.29.1
	gopkg.in/inf.v0 v0.9.1 // indirect
	gopkg.in/logfmt.v0 v0.3.0 // indirect
	gopkg.in/stack.v1 v1.6.0 // indirect
	gopkg.in/yaml.v2 v2.3.0
	gopkg.in/yaml.v3 v3.0.0-20200605160147-a5ece683394c
	k8s.io/api v0.0.0-20190325185214-7544f9db76f6
	k8s.io/apimachinery v0.0.0-20190223001710-c182ff3b9841
	k8s.io/client-go v8.0.0+incompatible
	stathat.com/c/consistent v1.0.0
)

replace github.com/hashicorp/consul => github.com/hashicorp/consul v1.5.1
