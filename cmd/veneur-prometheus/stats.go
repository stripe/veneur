package main

import (
	"sort"
)

type stat struct {
	Name string
	Tags []string
}

func (s stat) Same(o stat) bool {
	if s.Name != o.Name {
		return false
	}

	if len(s.Tags) != len(o.Tags) {
		return false
	}

	sort.Strings(s.Tags)
	sort.Strings(o.Tags)

	for i, a := range s.Tags {
		if a != o.Tags[i] {
			return false
		}
	}

	return true
}

type count struct {
	stat
	Value int64
}

type gauge struct {
	stat
	Value float64
}
