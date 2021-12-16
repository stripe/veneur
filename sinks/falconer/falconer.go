package falconer

import (
	"context"

	"github.com/stripe/veneur/v14"
	"github.com/stripe/veneur/v14/sinks"
	"github.com/stripe/veneur/v14/util"

	"github.com/sirupsen/logrus"
	"github.com/stripe/veneur/v14/sinks/grpsink"
)

type FalconerSpanSinkConfig struct {
	Target string `yaml:"target"`
}

func MigrateConfig(conf *veneur.Config) error {
	if conf.FalconerAddress == "" {
		return nil
	}
	conf.SpanSinks = append(conf.SpanSinks, veneur.SinkConfig{
		Kind: "falconer",
		Name: "falconer",
		Config: FalconerSpanSinkConfig{
			Target: conf.FalconerAddress,
		},
	})
	return nil
}

func Create(
	server *veneur.Server, name string, logger *logrus.Entry,
	config veneur.Config, sinkConfig veneur.SpanSinkConfig, ctx context.Context,
) (sinks.SpanSink, error) {
	return grpsink.Create(server, name, logger, config, sinkConfig, ctx)
}

// ParseConfig decodes the map config for a Falconer sink into a FalconerSpanSinkConfig
// struct.
func ParseConfig(
	name string, config interface{},
) (veneur.SpanSinkConfig, error) {
	falconerConfig := FalconerSpanSinkConfig{}
	err := util.DecodeConfig(name, config, &falconerConfig)
	if err != nil {
		return nil, err
	}
	return falconerConfig, nil
}
