package ssf

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

type tagged interface {
	AddTag(key, value string)

	AddTagPlain(value string)

	AllDimensions() []*Dimension
}

func TestAllDimensions(t *testing.T) {
	tests := []struct {
		name   string
		tags   map[string]string
		dims   []*Dimension
		output []*Dimension
	}{
		{
			"tags but nil dimensions",
			map[string]string{"hi": "there"},
			nil,
			[]*Dimension{
				&Dimension{Key: "hi", Value: "there"},
			},
		},
		{
			"dimensions but nil tags",
			nil,
			[]*Dimension{
				&Dimension{Key: "hi", Value: "there"},
			},
			[]*Dimension{
				&Dimension{Key: "hi", Value: "there"},
			},
		},
		{
			"both dimensions and tags",
			map[string]string{"hi": "tagged"},
			[]*Dimension{
				&Dimension{Key: "hi", Value: "there"},
			},
			[]*Dimension{
				&Dimension{Key: "hi", Value: "tagged"},
				&Dimension{Key: "hi", Value: "there"},
			},
		},
	}
	for _, elt := range tests {
		test := elt
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			sample := &SSFSample{
				Tags:       test.tags,
				Dimensions: test.dims,
			}
			assert.Equal(t, test.output, sample.AllDimensions())

			span := &SSFSpan{
				Tags:       test.tags,
				Dimensions: test.dims,
			}
			assert.Equal(t, test.output, span.AllDimensions())
		})
	}
}

func TestAddTag(t *testing.T) {
	type kv struct {
		key   string
		value string
	}
	tests := []struct {
		name    string
		element tagged
		kv      []kv
		output  []*Dimension
	}{
		{
			"one element on a Span",
			&SSFSpan{},
			[]kv{{"hi", "there"}},
			[]*Dimension{&Dimension{"hi", "there"}},
		},
		{
			"one element on a Sample",
			&SSFSample{},
			[]kv{{"hi", "there"}},
			[]*Dimension{&Dimension{"hi", "there"}},
		},
		{
			"multiple elements on a Span",
			&SSFSpan{},
			[]kv{{"hi", "there"}, {"farts", "testing"}, {"", "why"}},
			[]*Dimension{
				&Dimension{"hi", "there"},
				&Dimension{"farts", "testing"},
				&Dimension{"", "why"},
			},
		},
		{
			"multiple elements on a Sample",
			&SSFSample{},
			[]kv{{"hi", "there"}, {"farts", "testing"}, {"", "why"}},
			[]*Dimension{
				&Dimension{"hi", "there"},
				&Dimension{"farts", "testing"},
				&Dimension{"", "why"},
			},
		},
		{
			"existing elements on a Sample",
			&SSFSample{
				Dimensions: []*Dimension{
					&Dimension{"existing", "element"},
				},
			},
			[]kv{{"hi", "there"}, {"farts", "testing"}, {"", "why"}},
			[]*Dimension{
				&Dimension{"existing", "element"},
				&Dimension{"hi", "there"},
				&Dimension{"farts", "testing"},
				&Dimension{"", "why"},
			},
		},
	}
	for _, elt := range tests {
		test := elt
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			for _, kv := range test.kv {
				test.element.AddTag(kv.key, kv.value)
			}
			assert.Equal(t, test.output, test.element.AllDimensions())
		})
	}
}
