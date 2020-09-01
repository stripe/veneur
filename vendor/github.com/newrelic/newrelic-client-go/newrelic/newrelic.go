package newrelic

import (
	"errors"
	"net/http"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/newrelic/newrelic-client-go/internal/logging"
	"github.com/newrelic/newrelic-client-go/pkg/accounts"
	"github.com/newrelic/newrelic-client-go/pkg/alerts"
	"github.com/newrelic/newrelic-client-go/pkg/apiaccess"
	"github.com/newrelic/newrelic-client-go/pkg/apm"
	"github.com/newrelic/newrelic-client-go/pkg/config"
	"github.com/newrelic/newrelic-client-go/pkg/dashboards"
	"github.com/newrelic/newrelic-client-go/pkg/edge"
	"github.com/newrelic/newrelic-client-go/pkg/entities"
	"github.com/newrelic/newrelic-client-go/pkg/events"
	"github.com/newrelic/newrelic-client-go/pkg/eventstometrics"
	"github.com/newrelic/newrelic-client-go/pkg/logs"
	"github.com/newrelic/newrelic-client-go/pkg/nerdgraph"
	"github.com/newrelic/newrelic-client-go/pkg/nerdstorage"
	"github.com/newrelic/newrelic-client-go/pkg/nrdb"
	"github.com/newrelic/newrelic-client-go/pkg/plugins"
	"github.com/newrelic/newrelic-client-go/pkg/region"
	"github.com/newrelic/newrelic-client-go/pkg/synthetics"
	"github.com/newrelic/newrelic-client-go/pkg/workloads"
)

// NewRelic is a collection of New Relic APIs.
type NewRelic struct {
	Accounts        accounts.Accounts
	Alerts          alerts.Alerts
	APIAccess       apiaccess.APIAccess
	APM             apm.APM
	Dashboards      dashboards.Dashboards
	Edge            edge.Edge
	Entities        entities.Entities
	Events          events.Events
	EventsToMetrics eventstometrics.EventsToMetrics
	Logs            logs.Logs
	NerdGraph       nerdgraph.NerdGraph
	NerdStorage     nerdstorage.NerdStorage
	Nrdb            nrdb.Nrdb
	Plugins         plugins.Plugins
	Synthetics      synthetics.Synthetics
	Workloads       workloads.Workloads

	config config.Config
}

// New returns a collection of New Relic APIs.
func New(opts ...ConfigOption) (*NewRelic, error) {
	config := config.New()

	// Loop through config options
	for _, fn := range opts {
		if nil != fn {
			if err := fn(&config); err != nil {
				return nil, err
			}
		}
	}

	if config.PersonalAPIKey == "" && config.AdminAPIKey == "" && config.InsightsInsertKey == "" {
		return nil, errors.New("must use at least one of: ConfigPersonalAPIKey, ConfigAdminAPIKey, ConfigInsightsInsertKey")
	}

	nr := &NewRelic{
		config: config,

		Accounts:        accounts.New(config),
		Alerts:          alerts.New(config),
		APIAccess:       apiaccess.New(config),
		APM:             apm.New(config),
		Dashboards:      dashboards.New(config),
		Edge:            edge.New(config),
		Entities:        entities.New(config),
		Events:          events.New(config),
		EventsToMetrics: eventstometrics.New(config),
		Logs:            logs.New(config),
		NerdGraph:       nerdgraph.New(config),
		NerdStorage:     nerdstorage.New(config),
		Nrdb:            nrdb.New(config),
		Plugins:         plugins.New(config),
		Synthetics:      synthetics.New(config),
		Workloads:       workloads.New(config),
	}

	return nr, nil
}

// ConfigOption configures the Config when provided to NewApplication.
// https://docs.newrelic.com/docs/apis/get-started/intro-apis/types-new-relic-api-keys
type ConfigOption func(*config.Config) error

// ConfigPersonalAPIKey sets the New Relic Admin API key this client will use.
// One of ConfigPersonalAPIKey or ConfigAdminAPIKey must be used to create a client.
// https://docs.newrelic.com/docs/apis/get-started/intro-apis/types-new-relic-api-keys
func ConfigPersonalAPIKey(apiKey string) ConfigOption {
	return func(cfg *config.Config) error {
		cfg.PersonalAPIKey = apiKey
		return nil
	}
}

// ConfigInsightsInsertKey sets the New Relic Insights insert key this client will use.
// https://docs.newrelic.com/docs/apis/get-started/intro-apis/types-new-relic-api-keys
func ConfigInsightsInsertKey(insightsInsertKey string) ConfigOption {
	return func(cfg *config.Config) error {
		cfg.InsightsInsertKey = insightsInsertKey
		return nil
	}
}

// ConfigAdminAPIKey sets the New Relic Admin API key this client will use.
// One of ConfigAPIKey or ConfigAdminAPIKey must be used to create a client.
// https://docs.newrelic.com/docs/apis/get-started/intro-apis/types-new-relic-api-keys
func ConfigAdminAPIKey(adminAPIKey string) ConfigOption {
	return func(cfg *config.Config) error {
		cfg.AdminAPIKey = adminAPIKey
		return nil
	}
}

// ConfigRegion sets the New Relic Region this client will use.
func ConfigRegion(r string) ConfigOption {
	return func(cfg *config.Config) error {
		// We can ignore this error since we will be defaulting in the next step
		regName, _ := region.Parse(r)

		reg, err := region.Get(regName)
		if err != nil {
			if _, ok := err.(region.UnknownUsingDefaultError); ok {
				// If region wasn't provided, output a warning message
				// indicating the default region "US" is being used.
				log.Warn(err)
				return nil
			}

			return err
		}

		err = cfg.SetRegion(reg)

		return err
	}
}

// ConfigHTTPTimeout sets the timeout for HTTP requests.
func ConfigHTTPTimeout(t time.Duration) ConfigOption {
	return func(cfg *config.Config) error {
		var timeout = &t
		cfg.Timeout = timeout
		return nil
	}
}

// ConfigHTTPTransport sets the HTTP Transporter.
func ConfigHTTPTransport(transport http.RoundTripper) ConfigOption {
	return func(cfg *config.Config) error {
		if transport != nil {
			cfg.HTTPTransport = transport
			return nil
		}

		return errors.New("HTTP Transport can not be nil")
	}
}

// ConfigUserAgent sets the HTTP UserAgent for API requests.
func ConfigUserAgent(ua string) ConfigOption {
	return func(cfg *config.Config) error {
		if ua != "" {
			cfg.UserAgent = ua
			return nil
		}

		return errors.New("user-agent can not be empty")
	}
}

// ConfigServiceName sets the service name logged
func ConfigServiceName(name string) ConfigOption {
	return func(cfg *config.Config) error {
		if name != "" {
			cfg.ServiceName = name
		}

		return nil
	}
}

// ConfigBaseURL sets the base URL used to make requests to the REST API V2.
func ConfigBaseURL(url string) ConfigOption {
	return func(cfg *config.Config) error {
		if url != "" {
			cfg.Region().SetRestBaseURL(url)
			return nil
		}

		return errors.New("base URL can not be empty")
	}
}

// ConfigInfrastructureBaseURL sets the base URL used to make requests to the Infrastructure API.
func ConfigInfrastructureBaseURL(url string) ConfigOption {
	return func(cfg *config.Config) error {
		if url != "" {
			cfg.Region().SetInfrastructureBaseURL(url)
			return nil
		}

		return errors.New("infrastructure base URL can not be empty")
	}
}

// ConfigSyntheticsBaseURL sets the base URL used to make requests to the Synthetics API.
func ConfigSyntheticsBaseURL(url string) ConfigOption {
	return func(cfg *config.Config) error {
		if url != "" {
			cfg.Region().SetSyntheticsBaseURL(url)
			return nil
		}

		return errors.New("synthetics base URL can not be empty")
	}
}

// ConfigNerdGraphBaseURL sets the base URL used to make requests to the NerdGraph API.
func ConfigNerdGraphBaseURL(url string) ConfigOption {
	return func(cfg *config.Config) error {
		if url != "" {
			cfg.Region().SetNerdGraphBaseURL(url)
			return nil
		}

		return errors.New("nerdgraph base URL can not be empty")
	}
}

// ConfigLogLevel sets the log level for the client.
func ConfigLogLevel(logLevel string) ConfigOption {
	return func(cfg *config.Config) error {
		if logLevel != "" {
			cfg.LogLevel = logLevel
			return nil
		}

		return errors.New("log level can not be empty")
	}
}

// ConfigLogJSON toggles JSON formatting on for the logger if set to true.
func ConfigLogJSON(logJSON bool) ConfigOption {
	return func(cfg *config.Config) error {
		cfg.LogJSON = logJSON
		return nil
	}
}

// ConfigLogger can be used to customize the client's logger.
// Custom loggers must conform to the logging.Logger interface.
func ConfigLogger(logger logging.Logger) ConfigOption {
	return func(cfg *config.Config) error {
		if logger != nil {
			cfg.Logger = logger
			return nil
		}

		return errors.New("logger can not be nil")
	}
}
