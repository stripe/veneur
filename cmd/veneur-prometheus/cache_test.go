package main

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGaugesDontCacheTranslate(t *testing.T) {
	in := newGauge("foo", nil, 10.2)
	out := in.Translate(new(countCache))
	diffed, is := out.(gauge)
	require.True(t, is)
	assert.True(t, same(in.statID, diffed.statID))
	assert.Equal(t, in.Value, diffed.Value)
}

func TestCountCacheTranslate(t *testing.T) {
	name := "foo"
	var tags []string

	base := newPrometheusCount(name, tags, 10)
	less := newPrometheusCount(name, tags, 8)
	more := newPrometheusCount(name, tags, 12)

	zero := newStatsdCount(name, tags, 0)
	equal := newStatsdCount(name, tags, 10)
	diff := newStatsdCount(name, tags, 2)

	key := cacheKey(base)

	for _, difftest := range []struct {
		name     string
		cache    *countCache
		expected statsdCount
	}{
		{"no-previous", new(countCache), zero},
		{"less", newCache(map[string]prometheusCount{key: less}), diff},
		{"same", newCache(map[string]prometheusCount{key: base}), zero},
		{"more", newCache(map[string]prometheusCount{key: more}), equal},
	} {
		t.Run(difftest.name, func(t *testing.T) {
			out := base.Translate(difftest.cache)
			c, is := out.(statsdCount)
			require.True(t, is)
			assert.True(t, same(difftest.expected.statID, c.statID))
			assert.Equal(t, difftest.expected.Value, c.Value)

			cached, ok := difftest.cache.next[key]
			assert.True(t, ok)
			assert.Equal(t, base.Value, cached.Value)
		})
	}
}

func TestDiffIsDifferentForEmptyCacheVsNewMetrics(t *testing.T) {
	//if this is the first observations set of the cache then all metrics
	//must be considered no traffic as we have no way of determining if they are old or not
	//so we output zero traffic for those
	//
	//conversely if we've made observations before and the metric itself is new
	//we can presume that it's previous observation was a zero.
	//new metrics can appear either because labels get observed or a restart
	//of the monitoring service
	a := newPrometheusCount("a", nil, 3)
	b := newPrometheusCount("b", nil, 3)
	c := newPrometheusCount("c", nil, 4)

	cache := new(countCache)

	//empty cache, no idea where those counts came from time wise
	assert.Equal(t, int64(0), a.diff(cache))
	assert.Equal(t, int64(0), b.diff(cache))

	cache.Done()

	a.Value = 6

	//3 since last observation period we can trust those 3 to send along
	assert.Equal(t, int64(3), a.diff(cache), fmt.Sprintf("%v", cache))
	//this is a new metric since last observation, that means we can assume its count is from this time period
	assert.Equal(t, int64(4), c.diff(cache), fmt.Sprintf("%v", cache))

	cache.Done()

	//b came, left, came again thats weird behavior but effectively the same as new metric we've observed
	assert.Equal(t, int64(3), b.diff(cache))
}

func newCache(startingWith map[string]prometheusCount) *countCache {
	return &countCache{last: startingWith}
}
