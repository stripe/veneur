package tagging

import "strings"

// ParseTagSliceToMap handles splitting a slice of string tags on `:` and
// creating a map from the parts.
func ParseTagSliceToMap(tags []string) map[string]string {
	mappedTags := make(map[string]string)
	for _, tag := range tags {
		splt := strings.SplitN(tag, ":", 2)
		if len(splt) < 2 {
			mappedTags[splt[0]] = ""
		} else {
			mappedTags[splt[0]] = splt[1]
		}
	}
	return mappedTags
}
