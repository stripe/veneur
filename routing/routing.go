package routing

import "time"

type ComputationRoutingConfig struct {
	FlushGroup              string         `yaml:"flush_group"`
	MatcherConfigs          MatcherConfigs `yaml:"match"`
	WorkerInterval          time.Duration  `yaml:"worker_interval"`
	WorkerWatchdogIntervals int            `yaml:"worker_watchdog_intervals"`
	WorkerCount             int            `yaml:"worker_count"`
	ForwardMetrics          bool           `yaml:"forward_metrics"`
}

type SinkRoutingConfig struct {
	SinkName                string   `yaml:"sink_name"`
	FlushGroupSubscriptions []string `yaml:"flush_group_subscriptions"`
}
