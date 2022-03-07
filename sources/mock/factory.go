package mock

import (
	"github.com/golang/mock/gomock"
	"github.com/sirupsen/logrus"
	"github.com/stripe/veneur/v14"
	"github.com/stripe/veneur/v14/sources"
)

type MockSourceFactory struct {
	Controller *gomock.Controller
	Sources    map[string]*MockSource
}

func (factory *MockSourceFactory) Create(
	server *veneur.Server, name string, logger *logrus.Entry,
	sourceConfig veneur.ParsedSourceConfig, ingest sources.Ingest,
) (sources.Source, error) {
	source := NewMockSource(factory.Controller)
	// Have the mock Name method always return the passed in name, since each
	// source should have this behavior.
	source.EXPECT().Name().AnyTimes().Return(name)
	factory.Sources[name] = source
	return source, nil
}
