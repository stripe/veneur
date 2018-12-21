package metricingester_test

import "github.com/stripe/veneur/metricingester"

func metrics(ms ...metricingester.Metric) []metricingester.Metric {
	return ms
}

type metricConfig struct {
	tags       []string
	hostname   string
	samplerate float32
}

type metricOpt func(*metricConfig)

func hn(hostname string) metricOpt {
	return func(cfg *metricConfig) {
		cfg.hostname = hostname
	}
}

func tags(tags ...string) metricOpt {
	return func(cfg *metricConfig) {
		cfg.tags = tags
	}
}

func samplerate(samplerate float32) metricOpt {
	return func(cfg *metricConfig) {
		cfg.samplerate = samplerate
	}
}

func getConfig(opts []metricOpt) metricConfig {
	cfg := metricConfig{
		samplerate: 1.0,
	}
	for _, opt := range opts {
		opt(&cfg)
	}
	return cfg
}

func set(name string, v string, opts ...metricOpt) metricingester.Metric {
	cfg := getConfig(opts)
	return metricingester.NewSet(name, v, cfg.tags, cfg.samplerate, cfg.hostname)
}

func histo(name string, v float64, opts ...metricOpt) metricingester.Metric {
	cfg := getConfig(opts)
	return metricingester.NewHisto(name, v, cfg.tags, cfg.samplerate, cfg.hostname)
}

func mixedhisto(name string, v float64, opts ...metricOpt) metricingester.Metric {
	cfg := getConfig(opts)
	return metricingester.NewMixedHisto(name, v, cfg.tags, cfg.samplerate, cfg.hostname)
}

func statusCheck(name string, value float64, msg string, opts ...metricOpt) metricingester.Metric {
	cfg := getConfig(opts)
	return metricingester.NewStatusCheck(name, value, msg, cfg.tags, cfg.samplerate, cfg.hostname)
}

func counter(name string, v int64, opts ...metricOpt) metricingester.Metric {
	cfg := getConfig(opts)
	return metricingester.NewCounter(name, v, cfg.tags, cfg.samplerate, cfg.hostname)
}
