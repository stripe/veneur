package newrelic

import (
	"errors"
	"strconv"
	"strings"

	"github.com/newrelic/newrelic-client-go/pkg/config"
	"github.com/newrelic/newrelic-client-go/pkg/nrdb"
	"github.com/newrelic/newrelic-client-go/pkg/region"
	"github.com/newrelic/newrelic-telemetry-sdk-go/telemetry"
	"github.com/sirupsen/logrus"
)

const (
	DefaultRegion               = region.US
	DefaultEventType            = "veneur"
	DefaultStatusCheckEventType = "statusChecks"
)

func newEventClient(insertKey string, reg string, log *logrus.Logger) (*nrdb.Nrdb, error) {
	var err error
	regName := DefaultRegion

	// Configure what region we are reporting to
	if reg != "" {
		regName, err = region.Parse(reg)
		if err != nil {
			log.WithError(err).WithFields(logrus.Fields{
				"configured": reg,
				"default":    DefaultRegion,
			}).Warn("failed to parse New Relic region, using default")

			regName = DefaultRegion
		}
	}

	nrRegion, err := region.Get(regName)
	if err != nil {
		return nil, err
	}

	nrdbConfig := config.New()
	nrdbConfig.InsightsInsertKey = insertKey
	nrdbConfig.ServiceName = "veneur"
	nrdbConfig.SetRegion(nrRegion)

	n := nrdb.New(nrdbConfig)

	return &n, nil
}

// newHarvester creates a New Relic telemetry harvester for sending
// Metric and/or Span data
func newHarvester(insertKey string, log *logrus.Logger, tags []string, spanURL string) (*telemetry.Harvester, error) {
	nrCfg := []func(*telemetry.Config){
		telemetry.ConfigHarvestPeriod(0), // Never harvest automatically
	}

	// API Key is required
	if insertKey == "" {
		return nil, errors.New("insert key required for New Relic sink")
	}
	nrCfg = append(nrCfg, telemetry.ConfigAPIKey(insertKey))

	// Add logger
	logger := log
	if logger == nil {
		logger = logrus.StandardLogger()
	}

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
		log.WithField("spanURL", spanURL).Info("using custom span destination url")
	}

	ret, err := telemetry.NewHarvester(nrCfg...)
	if err != nil {
		log.WithError(err).Error("unable to create New Relic harvester")
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
