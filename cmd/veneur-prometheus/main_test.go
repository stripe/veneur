package main

import (
	"regexp"
	"testing"

	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
)

func TestGetTags(t *testing.T) {
	label1Name := "label1Name"
	label1Value := "label1Value"
	label1Pair := &dto.LabelPair{
		Name:  &label1Name,
		Value: &label1Value,
	}

	label2Name := "label2Name"
	label2Value := "label2Value"
	label2Pair := &dto.LabelPair{
		Name:  &label2Name,
		Value: &label2Value,
	}

	label3Name := "label3Name"
	label3Value := "label3Value"
	label3Pair := &dto.LabelPair{
		Name:  &label3Name,
		Value: &label3Value,
	}

	labels := []*dto.LabelPair{
		label1Pair, label2Pair, label3Pair,
	}

	ignoredLabels := []*regexp.Regexp{
		regexp.MustCompile(".*5.*"),
		regexp.MustCompile(".*abel1.*"),
	}

	tags := getTags(labels, ignoredLabels)
	expectedTags := []string{
		"label2Name:label2Value",
		"label3Name:label3Value",
	}

	assert.Equal(t, expectedTags, tags)
}

func TestShouldExportMetric(t *testing.T) {
	metric1Name := "metric1Name"

	mf := dto.MetricFamily{
		Name: &metric1Name,
	}

	ignoredMetrics1 := []*regexp.Regexp{
		regexp.MustCompile(".*0.*"),
		regexp.MustCompile(".*1.*"),
	}
	ignoredMetrics2 := []*regexp.Regexp{regexp.MustCompile(".*2.*")}

	assert.False(t, shouldExportMetric(mf, ignoredMetrics1))
	assert.True(t, shouldExportMetric(mf, ignoredMetrics2))
}
