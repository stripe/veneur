package tagging

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

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
			extraTags := NewExtendTags(test.implicitTags)

			actualTags := extraTags.Extend(test.explicitTags)
			assert.Equal(t, test.expectedTags, actualTags)

			mappedTags := extraTags.ExtendMap(ParseTagSliceToMap(test.explicitTags))
			assert.Equal(t, ParseTagSliceToMap(test.expectedTags), mappedTags)
		})
	}
}

// var letters = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")

// func randSeq(n int) string {
// 	b := make([]rune, n)
// 	for i := range b {
// 		b[i] = letters[rand.Intn(len(letters))]
// 	}
// 	return string(b)
// }

// func randTags(n int, tagLen int, prefix string) []string {
// 	tags := make([]string, 0, n)
// 	for i := 0; i < n; i++ {
// 		tags = append(tags, fmt.Sprintf("%s%s", prefix, randSeq(tagLen-len(prefix))))
// 	}
// 	return tags
// }

// func BenchmarkExtend(b *testing.B) {
// 	tagSize := 15
// 	for prefixLen := 0; prefixLen <= 7; prefixLen += 7 {
// 		prefixStr := randSeq(prefixLen)
// 		for sharedQty := 0; sharedQty <= 100; sharedQty += 25 {
// 			shared := randTags(sharedQty, tagSize, prefixStr)
// 			for implicitQty := sharedQty; implicitQty <= 100; implicitQty += 25 {
// 				implicit := append(shared, randTags(implicitQty, tagSize, prefixStr)...)
// 				for explicitQty := sharedQty; explicitQty <= 100; explicitQty += 25 {
// 					explicit := append(shared, randTags(explicitQty, tagSize, prefixStr)...)

// 					extraTags := NewExtraTags(implicit)
// 					b.Run(fmt.Sprintf("Map: pl=%d sq=%d iq=%d eq=%d", prefixLen, sharedQty, implicitQty, explicitQty), func(b *testing.B) {
// 						res := extraTags.ExtendMap(explicit)
// 						assert.NotEqual(b, []string{"a"}, res)
// 					})
// 					b.Run(fmt.Sprintf("Prefix: pl=%d sq=%d iq=%d eq=%d", prefixLen, sharedQty, implicitQty, explicitQty), func(b *testing.B) {
// 						res := extraTags.ExtendPrefix(explicit)
// 						assert.NotEqual(b, []string{"a"}, res)
// 					})
// 				}
// 			}
// 		}
// 	}
// }
