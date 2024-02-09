package tagging

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestShouldDropTag(t *testing.T) {
	tests := []struct {
		name     string
		prefixes []string
		tag      string
		expected bool
	}{
		{
			name:     "No prefixes",
			prefixes: []string{},
			tag:      "foo:bar",
			expected: false,
		},
		{
			name:     "Valueless tag, exact match",
			prefixes: []string{"foo"},
			tag:      "foo",
			expected: true,
		},
		{
			name:     "Tag too short for prefix",
			prefixes: []string{"foo"},
			tag:      "fo",
			expected: false,
		},
		{
			name:     "Valueless tag, prefix match",
			prefixes: []string{"foo"},
			tag:      "foobar",
			expected: false,
		},
		{
			name:     "Tag with value, key match",
			prefixes: []string{"foo"},
			tag:      "foo:bar",
			expected: true,
		},
		{
			name:     "Tag with value, key prefix match",
			prefixes: []string{"foo"},
			tag:      "foobar:baz",
			expected: false,
		},
		{
			name:     "Tag with value, off by -1",
			prefixes: []string{"foo"},
			tag:      "fo:baz",
			expected: false,
		},
		{
			name:     "Tag with value, off by +1",
			prefixes: []string{"foo"},
			tag:      "foob:baz",
			expected: false,
		},
	}
	for _, elt := range tests {
		test := elt
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			extendTags := ExtendTags{extraTagPrefixes: test.prefixes}
			assert.Equal(t, test.expected, extendTags.shouldDropTag(test.tag))
		})
	}
}
func TestExtend(t *testing.T) {
	tests := []struct {
		name         string
		implicitTags []string
		explicitTags []string
		expectedTags []string
	}{
		{
			name:         "all empty",
			implicitTags: []string{},
			explicitTags: []string{},
			expectedTags: []string{},
		},
		{
			name:         "no implicit tags",
			implicitTags: []string{},
			explicitTags: []string{"foo:bar"},
			expectedTags: []string{"foo:bar"},
		},
		{
			name:         "no explicit tags",
			implicitTags: []string{"foo:bar"},
			explicitTags: []string{},
			expectedTags: []string{"foo:bar"},
		},
		{
			name:         "both, non-overlapping",
			implicitTags: []string{"foo:bar"},
			explicitTags: []string{"baz:quux"},
			expectedTags: []string{"baz:quux", "foo:bar"},
		},
		{
			name:         "both, overlapping",
			implicitTags: []string{"foo:bar"},
			explicitTags: []string{"foo:quux"},
			expectedTags: []string{"foo:bar"},
		},
		{
			name:         "both, overlapping, implicit has no value",
			implicitTags: []string{"foo"},
			explicitTags: []string{"foo:quux"},
			expectedTags: []string{"foo"},
		},
		{
			name:         "both, overlapping, explicit has no value",
			implicitTags: []string{"foo:bar"},
			explicitTags: []string{"foo"},
			expectedTags: []string{"foo:bar"},
		},
		{
			name:         "both, some overlap",
			implicitTags: []string{"implicit", "both:keep"},
			explicitTags: []string{"explicit", "both:discard"},
			expectedTags: []string{"both:keep", "explicit", "implicit"},
		},
	}
	for _, elt := range tests {
		test := elt
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			extendTags := NewExtendTags(test.implicitTags)

			actualTags := extendTags.Extend(test.explicitTags, false)
			assert.Equal(t, test.expectedTags, actualTags)

			mappedTags := extendTags.ExtendMap(ParseTagSliceToMap(test.explicitTags), false)
			assert.Equal(t, ParseTagSliceToMap(test.expectedTags), mappedTags)
		})
	}
}
