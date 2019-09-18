package main

import (
	"fmt"
	"strings"
)

type countCache struct {
	last map[string]prometheusCount
	next map[string]prometheusCount
}

//GetAndSwap will return the previous count for this metric and add the passed in one for future use
//bool indicates if Get was successful
func (c *countCache) GetAndSwap(n prometheusCount) (prev prometheusCount, ok bool) {
	if c == nil {
		c = &countCache{}
	}

	if c.next == nil {
		c.next = make(map[string]prometheusCount)
	}

	key := cacheKey(n)
	c.next[key] = n

	if c.FirstObservation() {
		return
	}

	prev, ok = c.last[key]
	return
}

func (c *countCache) FirstObservation() bool {
	return c == nil || c.last == nil
}

//indicates that a single observations sweep is done.
//this is important for determining if 'we' are new or if 'they' are
func (c *countCache) Done() {
	if c == nil {
		return
	}

	//double map approach allows us to distinquish between no observations
	//and new metric and keep memory from growing in the face of metrics
	//coming and going so seems worth the complication
	c.last = c.next
	c.next = make(map[string]prometheusCount)
}

func cacheKey(n prometheusCount) string {
	return fmt.Sprintf("%s-%s", n.Name, strings.Join(n.Tags, "-"))
}
