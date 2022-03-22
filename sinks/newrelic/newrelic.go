package newrelic

import (
	"errors"
	"strconv"
	"strings"

	"github.com/newrelic/newrelic-client-go/pkg/region"
	"github.com/newrelic/newrelic-telemetry-sdk-go/telemetry"
	"github.com/sirupsen/logrus"
	veneur "github.com/stripe/veneur/v14"
)

const (
	DefaultRegion                = region.US
	DefaultEventType             = "veneur"
	DefaultServiceCheckEventType = "veneurCheck"
)

// TODO(arnavdugar): Remove this once the old configuration format has been
// removed.
func MigrateConfig(conf *veneur.Config) {
	if conf.NewrelicInsertKey.Value != "" && conf.NewrelicAccountID > 0 {
		conf.MetricSinks = append(conf.MetricSinks, veneur.SinkConfig{
			Kind: "newrelic",
			Name: "newrelic",
			Config: NewRelicMetricSinkConfig{
				AccountID:             conf.NewrelicAccountID,
				CommonTags:            conf.NewrelicCommonTags,
				EventType:             conf.NewrelicEventType,
				InsertKey:             conf.NewrelicInsertKey,
				Region:                conf.NewrelicRegion,
				ServiceCheckEventType: conf.NewrelicServiceCheckEventType,
			},
		})
	}
	if conf.NewrelicInsertKey.Value != "" {
		conf.SpanSinks = append(conf.SpanSinks, veneur.SinkConfig{
			Kind: "newrelic",
			Name: "newrelic",
			Config: NewRelicSpanSinkConfig{
				CommonTags:       conf.NewrelicCommonTags,
				InsertKey:        conf.NewrelicInsertKey,
				TraceObserverURL: conf.NewrelicTraceObserverURL,
			},
		})
	}
}

// newHarvester creates a New Relic telemetry harvester for sending
// Metric and/or Span data
func newHarvester(
	insertKey string, logger *logrus.Entry, tags []string, spanURL string,
) (*telemetry.Harvester, error) {
	nrCfg := []func(*telemetry.Config){
		telemetry.ConfigHarvestPeriod(0), // Never harvest automatically
	}

	// API Key is required
	if insertKey == "" {
		return nil, errors.New("insert key required for New Relic sink")
	}
	nrCfg = append(nrCfg, telemetry.ConfigAPIKey(insertKey))

	nrCfg = append(nrCfg,
		telemetry.ConfigBasicErrorLogger(logger.WriterLevel(logrus.ErrorLevel)),
		telemetry.ConfigBasicDebugLogger(logger.WriterLevel(logrus.DebugLevel)),
		telemetry.ConfigBasicAuditLogger(logger.WriterLevel(logrus.TraceLevel)),
	)

	if len(tags) > 0 {
		attrs := tagsToKeyValue(tags)
		if len(attrs) > 0 {
			nrCfg = append(nrCfg, telemetry.ConfigCommonAttributes(attrs))
		}
	}

	if spanURL != "" {
		nrCfg = append(nrCfg, telemetry.ConfigSpansURLOverride(spanURL))
		logger.WithField("spanURL", spanURL).Info("using custom span destination url")
	}

	ret, err := telemetry.NewHarvester(nrCfg...)
	if err != nil {
		logger.WithError(err).Error("unable to create New Relic harvester")
		return nil, err
	}

	return ret, nil
}

// tagsToKeyValue converts string["tag:value"] to map["tag"]  = "value"
func tagsToKeyValue(tags []string) (ret map[string]interface{}) {
	if len(tags) > 0 {
		ret = make(map[string]interface{}, len(tags))

		for _, tag := range tags {
			if strings.Contains(tag, ":") {
				keyvalpair := strings.SplitN(tag, ":", 2)
				parsed, err := strconv.ParseFloat(keyvalpair[1], 64)
				if err != nil || strings.EqualFold(keyvalpair[1], "infinity") {
					ret[keyvalpair[0]] = keyvalpair[1]
				} else {
					ret[keyvalpair[0]] = parsed
				}
			} else {
				ret[tag] = "true"
			}
		}
	}

	return ret
}
