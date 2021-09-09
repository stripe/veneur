package attribution

import (
	"encoding/csv"
	"fmt"
	"strings"
)

type tsvField int

const (
	// the order in which these appear determines the
	// order of the fields in the resultant TSV
	TsvName tsvField = iota
	TsvOwner
	TsvCardinality
)

var tsvSchema = [...]string{
	TsvName:        "Name",
	TsvOwner:       "Owner",
	TsvCardinality: "Value",
}

// EncodeInterMetricCSV generates a newline-terminated CSV row that describes
// the data represented by the InterMetric.
// The caller is responsible for setting w.Comma as the appropriate delimiter.
// For performance, encodeCSV does not flush after every call; the caller is
// expected to flush at the end of the operation cycle
func encodeInterMetricCSV(ts *Timeseries, w *csv.Writer) error {
	fields := [...]string{
		TsvName:        ts.Metric.Name,
		TsvOwner:       ts.Owner,
		TsvCardinality: fmt.Sprintf("%d", ts.Sketch.Estimate()),
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
