package attribution

import (
	"encoding/csv"
	"fmt"
	"strconv"
	"strings"
)

type tsvField int

const (
	// the order in which these appear determines the
	// order of the fields in the resultant TSV
	TsvName tsvField = iota
	TsvCardinality
	TsvDigests
	TsvOwner
)

var tsvSchema = [...]string{
	TsvName:        "Name",
	TsvCardinality: "Value",
	TsvDigests:     "Digests",
	TsvOwner:       "Owner",
}

// EncodeInterMetricCSV generates a newline-terminated CSV row that describes
// the data represented by the InterMetric.
// The caller is responsible for setting w.Comma as the appropriate delimiter.
// For performance, encodeCSV does not flush after every call; the caller is
// expected to flush at the end of the operation cycle
func encodeInterMetricCSV(g *TimeseriesGroup, w *csv.Writer, includeDigests bool) error {
	digests := ""
	if includeDigests {
		var strDigests []string
		for _, digest := range g.Digests {
			strDigests = append(strDigests, strconv.FormatUint(uint64(digest), 10))
		}
		digests = strings.Join(strDigests, ",")
	}

	fields := [...]string{
		TsvName:        g.Name,
		TsvCardinality: fmt.Sprintf("%d", g.Sketch.Estimate()),
		TsvDigests:     digests,
		TsvOwner:       g.Owner,
	}

	w.Write(fields[:])
	return w.Error()
}

// String returns the field Name.
// eg tsvName.String() returns "Name"
func (f tsvField) String() string {
	return fmt.Sprintf(strings.Replace(tsvSchema[f], "tsv", "", 1))
}

// each key in tsvMapping is guaranteed to have a unique value
var tsvMapping = map[string]int{}

func init() {
	for i, field := range tsvSchema {
		tsvMapping[field] = i
	}
}
