package tagging

import (
	"sort"
	"strings"
)

// ExtendTags holds the pre-calculated data to apply the
// "extend tags" operation on a slice of strings representing
// some tags
type ExtendTags struct {
	extraTags       []string
	extraTagsMap    map[string]string
	dropPrefixesMap map[string]struct{}
}

// NewExtendTags creates a new ExtraTags struct, including
// the pre-calculation of prefixes to drop
func NewExtendTags(tags []string) ExtendTags {
	extraTags := make([]string, 0, len(tags))
	extraTagsMap := ParseTagSliceToMap(tags)
	dropPrefixesMap := make(map[string]struct{})
	for _, tag := range tags {
		if tag != "" {
			extraTags = append(extraTags, tag)
			pre := strings.SplitN(tag, ":", 2)[0]
			dropPrefixesMap[pre] = struct{}{}
		}
	}
	return ExtendTags{
		extraTags:       extraTags,
		extraTagsMap:    extraTagsMap,
		dropPrefixesMap: dropPrefixesMap,
	}
}

// Extend takes as input a slice of tags as a string, and returns
// a sorted slice of tags including those specified by ExtraTags.
// Tags present in ExtraTags will override any present in the input
// based on their key component (the text before any `:`)
func (et *ExtendTags) Extend(tags []string) []string {
	// one of the two sides is empty: just sort
	if len(et.extraTags) == 0 || len(tags) == 0 {
		ret := make([]string, 0, len(tags)+len(et.extraTags))
		ret = append(ret, tags...)
		ret = append(ret, et.extraTags...)
		sort.Strings(ret)
		return ret
	}

	// we are adding at least one tag: remove any conflicting
	// tags, append the additional tags, and sort
	ret := make([]string, 0, len(tags)+len(et.extraTags))
	for _, tag := range tags {
		if tag != "" {
			pre := strings.SplitN(tag, ":", 2)[0]
			if _, ok := et.dropPrefixesMap[pre]; !ok {
				ret = append(ret, tag)
			}
		}
	}
	ret = append(ret, et.extraTags...)
	sort.Strings(ret)
	return ret
}

func (et *ExtendTags) ExtendMap(tags map[string]string) map[string]string {
	ret := make(map[string]string, len(tags)+len(et.extraTags))
	for key, value := range tags {
		ret[key] = value
	}
	for key, value := range et.extraTagsMap {
		ret[key] = value
	}
	return ret
}

// func (et *ExtraTags) ExtendMap(tags []string) []string {
// 	// we are adding at least one tag: remove any conflicting
// 	// tags, append the additional tags, and sort
// 	ret := make([]string, 0, len(tags)+len(et.extraTags))
// 	for _, tag := range tags {
// 		if tag != "" {
// 			pre := strings.SplitN(tag, ":", 2)[0]
// 			if _, ok := et.dropPrefixesMap[pre]; !ok {
// 				ret = append(ret, tag)
// 			}
// 		}
// 	}
// 	ret = append(ret, et.extraTags...)
// 	sort.Strings(ret)
// 	return ret
// }

// func (et *ExtraTags) ExtendPrefix(tags []string) []string {
// 	// we are adding at least one tag: remove any conflicting
// 	// tags, append the additional tags, and sort
// 	ret := make([]string, 0, len(tags)+len(et.extraTags))
// outer:
// 	for _, tag := range tags {
// 		if tag != "" {
// 			pre := strings.SplitN(tag, ":", 2)[0]

// 			for _, dropPrefix := range et.dropPrefixes {
// 				if pre == dropPrefix {
// 					continue outer
// 				}
// 			}
// 			ret = append(ret, tag)
// 		}
// 	}
// 	ret = append(ret, et.extraTags...)
// 	sort.Strings(ret)
// 	return ret
// }
