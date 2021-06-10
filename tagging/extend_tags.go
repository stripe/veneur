package tagging

import (
	"sort"
	"strings"
)

// ExtendTags holds the pre-calculated data to apply the
// "extend tags" operation on a slice of strings representing
// some tags
type ExtendTags struct {
	extraTags        []string
	extraTagsMap     map[string]string
	extraTagPrefixes []string
}

// NewExtendTags creates a new ExtraTags struct, including
// the pre-calculation of prefixes to drop. Empty tags are
// ignored.
func NewExtendTags(tags []string) ExtendTags {
	extraTags := make([]string, 0, len(tags))
	extraTagsMap := ParseTagSliceToMap(tags)
	extraTagPrefixes := make([]string, 0, len(tags))
	// in most contexts, we'll receive a slice of tags, formatted
	// like "tag" or "tag:value". to avoid extra processing in the
	// hot path, we precalculate the tag key / prefix, which is used
	// to merge the explicit tags and the implicit tags.
	//
	// in `shouldDropTag` below, we use the prefixes to drop conflicting
	// tag keys from the _explicit_ tags; this behavior is similar to
	// building a map[string]string where implicit tags override the
	// explicit ones.
	//
	// implicitly configured tags overriding the explicitly specified
	// ones is pre-existing behavior, so we're selecting this ordering
	// because it matches what Veneur already does.
	//
	// we exclude empty tags as a non-fatal config error: there's no
	// point in replacing an empty string with an empty string, so we
	// can just keep the explicitly specified one if there was a conflict.
	//
	// there's no apparent purpose to allowing a use-case of "define an
	// empty tag that is added to all metrics", so it's simply not supported
	for _, tag := range tags {
		if tag != "" {
			extraTags = append(extraTags, tag)
			pre := strings.SplitN(tag, ":", 2)[0]
			extraTagPrefixes = append(extraTagPrefixes, pre)
		}
	}
	sort.Strings(extraTags)
	return ExtendTags{
		extraTags:        extraTags,
		extraTagsMap:     extraTagsMap,
		extraTagPrefixes: extraTagPrefixes,
	}
}

// shouldDropTag performs a naive loop over the tag prefixes to drop;
// this list is not expected to be large. a map lookup might be more
// efficient should we need to support very large lists of implicit
// prefixes. A map lookup requires a string slice by ":" to get the
// key that should be looked up, though.
func (et *ExtendTags) shouldDropTag(tag string) bool {
	for _, pre := range et.extraTagPrefixes {
		// prefix length greater than the entire tag? can't match
		if len(pre) > len(tag) {
			continue
		}
		// prefix length equal to the tag? direct comparison; will
		// never match if the tag has a value ("key:value")
		if len(pre) == len(tag) && pre == tag {
			return true
		}
		// len(pre) must be less than len(tag). the tag must have
		// a value (separated by ":") to match the prefix, and the
		// prefix must match everything before the ':'
		if pre == tag[0:len(pre)] && tag[len(pre)] == ':' {
			return true
		}
	}

	return false
}

// Extend takes as input a slice of tags as a string, and returns
// a sorted slice of tags including those specified by ExtraTags.
// Tags present in ExtraTags will override any present in the input
// based on their key component (the text before any `:`)
func (et *ExtendTags) Extend(tags []string, mutate bool) []string {
	// both sides are empty, return empty slice
	if len(tags) == 0 && len(et.extraTags) == 0 {
		return []string{}
	}

	// if `tags` is empty but `extraTags` is not, all we need to do
	// is return a copy of `extraTags`. The tags are pre-sorted at
	// initialization.
	if len(tags) == 0 {
		// make a copy so nothing mutates our expected state
		ret := make([]string, len(et.extraTags))
		copy(ret, et.extraTags)
		return ret
	}

	// caller has specified some tags. if they tell us we can mutate
	// it, we'll sort it directly; otherwise, we'll make a copy
	var callerTags []string
	if mutate {
		callerTags = tags
	} else {
		callerTags = make([]string, len(tags))
		copy(callerTags, tags)
	}

	// if `extraTags` is empty, all we need to do is return `tags`,
	// but sorted. Because we _must_ sort in the final case, _all_
	// sorting was moved out of the parsing code into this function.
	// as a result, this function must always be called to have
	// correct tags, and shouldn't be "optimized out" of calling
	// code paths
	if len(et.extraTags) == 0 {
		sort.Strings(callerTags)
		return callerTags
	}

	// we are adding at least one tag: remove any conflicting
	// tags, append the additional tags, and sort
	ret := make([]string, 0, len(tags)+len(et.extraTags))

	for _, tag := range tags {
		if tag != "" {
			if !et.shouldDropTag(tag) {
				ret = append(ret, tag)
			}
		} else {
			// maintain explicit empty tags. not sure why, but we have
			// a test that this works this way
			ret = append(ret, "")
		}
	}
	ret = append(ret, et.extraTags...)
	sort.Strings(ret)
	return ret
}

// ExtendMap merges the extra tags into the provided map
func (et *ExtendTags) ExtendMap(tags map[string]string, mutate bool) map[string]string {
	var ret map[string]string
	if mutate {
		// if the caller has specified that we can mutate the input, do so
		ret = tags
	} else {
		// otherwise, make a copy
		ret = make(map[string]string, len(tags)+len(et.extraTags))
		for key, value := range tags {
			ret[key] = value
		}
	}

	for key, value := range et.extraTagsMap {
		ret[key] = value
	}
	return ret
}
