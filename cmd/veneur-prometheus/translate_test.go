package main

import (
	"regexp"
	"testing"

	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
)

func TestTranslateTags(t *testing.T) {
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

	tags := translator(ignoredLabels).Tags(labels)
	expectedTags := []string{
		"label2Name:label2Value",
		"label3Name:label3Value",
	}

	assert.Equal(t, expectedTags, tags)
}
