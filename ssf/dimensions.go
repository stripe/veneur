package ssf

// The Dimensional interface applies to all SSF data structures that
// have a Dimensions field. They allow adding dimensions and
// retrieving the dimensions that are set on a structure.
type Dimensional interface {
	AddTag(key, value string)
	AddTagPlain(value string)
	AllDimensions() []*Dimension
}

// AddTag adds a dimension as a key/value pair to the SSFSample.
func (s *SSFSample) AddTag(key, value string) {
	s.Dimensions = append(s.Dimensions, &Dimension{Key: key, Value: value})
}

// AddTag adds a dimension as a key/value pair to the SSFSpan.
func (s *SSFSpan) AddTag(key, value string) {
	s.Dimensions = append(s.Dimensions, &Dimension{Key: key, Value: value})
}

// AddTagPlain adds a non-structured dimension to the SSFSample. The
// dimension text can be free-form.
func (s *SSFSample) AddTagPlain(value string) {
	s.Dimensions = append(s.Dimensions, &Dimension{Value: value})
}

// AddTagPlain adds a non-structured dimension to the SSFSpan. The
// dimension text can be free-form.
func (s *SSFSpan) AddTagPlain(value string) {
	s.Dimensions = append(s.Dimensions, &Dimension{Value: value})
}

func allDimensions(tags map[string]string, dimensions []*Dimension) []*Dimension {
	if tags == nil {
		return dimensions
	}
	dims := make([]*Dimension, 0, len(dimensions)+len(tags))
	for k, v := range tags {
		dim := &Dimension{}
		if v == "" {
			// We have a "plain" tag in old SSF parlance
			dim.Value = k
		} else {
			dim.Key = k
			dim.Value = v
		}
		dims = append(dims, dim)
	}
	dims = append(dims, dimensions...)
	return dims
}

// AllDimensions gathers, normalizes and returns all the Dimensions
// present on the SSFSample. If the SSFSample's Tags is nil, the
// returned array is identical to the SSFSample's Dimensions field.
//
// If the SSFSample has non-nil Tags, the returned array contains
// their normalized form first, followed by the contents of the
// Dimensions field.
//
// Normalization of Tags
//
// A Tag with a non-empty key and value is translated 1:1 to a
// Dimension: Its Key is the Tag's key and its Value is the Tag's
// value.
//
// A Tag with an empty value is assumed to be a "plain" tag, and is
// represented as a Dimension with an empty Key and a non-empty Value.
func (s *SSFSample) AllDimensions() []*Dimension {
	return allDimensions(s.Tags, s.Dimensions)
}

// AllDimensions gathers, normalizes and returns all the Dimensions
// present on the SSFSpan. If the SSFSpan's Tags is nil, the
// returned array is identical to the SSFSpan's Dimensions field.
//
// If the SSFSpan has non-nil Tags, the returned array contains
// their normalized form first, followed by the contents of the
// Dimensions field.
//
// Normalization of Tags
//
// A Tag with a non-empty key and value is translated 1:1 to a
// Dimension: Its Key is the Tag's key and its Value is the Tag's
// value.
//
// A Tag with an empty value is assumed to be a "plain" tag, and is
// represented as a Dimension with an empty Key and a non-empty Value.
func (s *SSFSpan) AllDimensions() []*Dimension {
	return allDimensions(s.Tags, s.Dimensions)
}

// DimensionValues retrieves all Dimensions on a data structure with
// dimensions and returns all Values whose key/value pair's key
// matches name, in order (if any Tag matches, it is returned as the
// first element). If no dimensions match, a nil array is returned.
func DimensionValues(d Dimensional, name string) (found []string) {
	for _, d := range d.AllDimensions() {
		if d.Key == name {
			found = append(found, d.Value)
		}
	}
	return
}
