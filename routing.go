package veneur

type ComputationRoutingConfig struct {
	Name           string         `yaml:"name"`
	FlushGroup     string         `yaml:"flush_group"`
	MatcherConfigs MatcherConfigs `yaml:"match"`
	WorkerInterval int            `yaml:"worker_interval"`
	WorkerCount    int            `yaml:"worker_count"`
}

type SinkRoutingConfig struct {
	Name           string           `yaml:"name"`
	MatcherConfigs MatcherConfigs   `yaml:"match"`
	Sinks          SinkRoutingSinks `yaml:"sinks"`
}

type SinkRoutingSinks struct {
	Matched    []string `yaml:"matched"`
	NotMatched []string `yaml:"not_matched"`
}
