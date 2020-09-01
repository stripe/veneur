package newrelic

import (
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

const (
	testNewRelicAccount          = 0
	testNewRelicApiKey           = "INVALID_API_KEY"
	testNewRelicEventType        = "testVeneur"
	testNewRelicRegion           = "US"
	testNewRelicSpanURL          = ""
	testNewRelicServiceEventType = "testServiceCheck"
)

func TestNewHarvester(t *testing.T) {
	t.Parallel()

	log := logrus.New()
	h, err := newHarvester("", log, []string{}, "")

	assert.Nil(t, h)
	assert.Error(t, err)

}

func TestTagsToKeyValue(t *testing.T) {
	t.Parallel()

	testSet := []struct {
		tags    []string
		results map[string]interface{}
	}{
		{
			tags:    []string{"key1:value1", "key2:2", "key3:3.14", "key4"},
			results: map[string]interface{}{"key1": "value1", "key2": 2.0, "key3": 3.14, "key4": "true"},
		},
	}

	for _, x := range testSet {
		res := tagsToKeyValue(x.tags)

		for k, v := range x.results {
			assert.Equal(t, v, res[k])
		}
	}
}
