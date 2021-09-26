package veneur

import "time"

type ComputationRoutingConfig struct {
	Name                    string         `yaml:"name"`
	FlushGroup              string         `yaml:"flush_group"`
	MatcherConfigs          MatcherConfigs `yaml:"match"`
	WorkerInterval          time.Duration  `yaml:"worker_interval"`
	WorkerWatchdogIntervals int            `yaml:"worker_watchdog_intervals"`
	WorkerCount             int            `yaml:"worker_count"`
	ForwardMetrics          bool           `yaml:"forward_metrics"`
}

type SinkRoutingConfig struct {
	Name                    string   `yaml:"name"`
	FlushGroupSubscriptions []string `yaml:"flush_group_subscriptions"`
}
