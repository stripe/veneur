package s3

import (
	"encoding/csv"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/stripe/veneur/samplers"
)

const PartitionDateFormat = "20060102"
const RedshiftDateFormat = "2006-01-02 03:04:05"

type tsvField int

const (
	// the order in which these appear determines the
	// order of the fields in the resultant TSV
	TsvName tsvField = iota
	TsvTags
	TsvMetricType

	// The hostName attached to the metric
	TsvHostname

	// The hostName of the server flushing the data
	TsvVeneurHostname

	TsvDeviceName
	TsvInterval

	TsvTimestamp
	TsvValue

	// This is the _partition field
	// required by the Redshift IncrementalLoader.
	// For our purposes, the current date is a good partition.
	TsvPartition
)

var tsvSchema = [...]string{
	TsvName:           "Name",
	TsvTags:           "Tags",
	TsvMetricType:     "MetricType",
	TsvHostname:       "Hostname",
	TsvDeviceName:     "DeviceName",
	TsvInterval:       "Interval",
	TsvVeneurHostname: "VeneurHostname",
	TsvTimestamp:      "Timestamp",
	TsvValue:          "Value",
	TsvPartition:      "Partition",
}

// EncodeDDMetricCSV generates a newline-terminated CSV row that describes
// the data represented by the DDMetric.
// The caller is responsible for setting w.Comma as the appropriate delimiter.
// For performance, encodeCSV does not flush after every call; the caller is
// expected to flush at the end of the operation cycle
func EncodeDDMetricCSV(d samplers.DDMetric, w *csv.Writer, partitionDate *time.Time, hostName string) error {

	timestamp := d.Value[0][0]
	value := strconv.FormatFloat(d.Value[0][1], 'f', -1, 64)
	interval := strconv.Itoa(int(d.Interval))

	// TODO(aditya) some better error handling for this
	// to guarantee that the result is proper JSON
	tags := "{" + strings.Join(d.Tags, ",") + "}"

	fields := [...]string{
		// the order here doesn't actually matter
		// as long as the keys are right
		TsvName:           d.Name,
		TsvTags:           tags,
		TsvMetricType:     d.MetricType,
		TsvHostname:       d.Hostname,
		TsvDeviceName:     d.DeviceName,
		TsvInterval:       interval,
		TsvVeneurHostname: hostName,
		TsvValue:          value,

		TsvTimestamp: time.Unix(int64(timestamp), 0).UTC().Format(RedshiftDateFormat),

		// TODO avoid edge case at midnight
		TsvPartition: partitionDate.UTC().Format(PartitionDateFormat),
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
