package mock

import (
	"github.com/golang/mock/gomock"
	"github.com/sirupsen/logrus"
	"github.com/stripe/veneur/v14"
	"github.com/stripe/veneur/v14/sinks"
)

type MockMetricSinkFactory struct {
	Controller *gomock.Controller
	Sinks      map[string]*MockMetricSink
}

func (factory *MockMetricSinkFactory) CreateMetricSink(
	server *veneur.Server, name string, logger *logrus.Entry,
	config veneur.Config, sinkConfig veneur.MetricSinkConfig,
) (sinks.MetricSink, error) {
	sink := NewMockMetricSink(factory.Controller)
	sink.EXPECT().Name().AnyTimes().Return(name)
	factory.Sinks[name] = sink
	return sink, nil
}

type MockSpanSinkFactory struct {
	Controller *gomock.Controller
	Sinks      map[string]*MockSpanSink
}

func (factory *MockSpanSinkFactory) CreateSpanSink(
	server *veneur.Server, name string, logger *logrus.Entry,
	config veneur.Config, sinkConfig veneur.SpanSinkConfig,
) (sinks.SpanSink, error) {
	sink := NewMockSpanSink(factory.Controller)
	sink.EXPECT().Name().AnyTimes().Return(name)
	factory.Sinks[name] = sink
	return sink, nil
}
