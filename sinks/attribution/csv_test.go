package attribution

import (
	"bytes"
	"encoding/csv"
	"testing"

	"github.com/axiomhq/hyperloglog"
	"github.com/stretchr/testify/assert"
)

// TestEncodeGloballyUniqueMTSRow asserts that encodeTimeseriesGroupCSV is able
// to write a row to a CSV in the v1 attribution schema format
func TestEncodeGloballyUniqueMTSRow(t *testing.T) {
	const Comma = '\t'

	tsGroup := TimeseriesGroup{
		Name:  "csv-test.1",
		Owner: "observability",
		Digests: map[uint32]struct{}{
			1: {},
			2: {},
		},
		Sketch: hyperloglog.New(),
	}
	tsGroup.Sketch.Insert([]byte{'a'})
	tsGroup.Sketch.Insert([]byte{'b'})

	b := &bytes.Buffer{}
	w := csv.NewWriter(b)
	w.Comma = '\t'

	encodeTimeseriesGroupCSV(&tsGroup, w, SchemaGloballyUniqueMTS)
	w.Flush()
	assert.NoError(t, w.Error())

	csvr := csv.NewReader(b)
	csvr.Comma = Comma
	records, err := csvr.ReadAll()
	assert.NoError(t, err)

	assert.Equal(t, 1, len(records))
	assert.Equal(t, "csv-test.1", records[0][0])
	assert.Equal(t, "observability", records[0][1])
	assert.Equal(t, "2", records[0][2])
}

// TestNonUniqueMTSWithDigestsRows asserts that encodeTimeseriesGroupCSV is
// able to write rows to a CSV in the v2 attribution schema format
func TestNonUniqueMTSWithDigestsRows(t *testing.T) {
	const Comma = '\t'

	tsGroup := TimeseriesGroup{
		Name:  "csv-test.1",
		Owner: "observability",
		Digests: map[uint32]struct{}{
			1: {},
			2: {},
		},
		Sketch: hyperloglog.New(),
	}
	tsGroup.Sketch.Insert([]byte{'a'})
	tsGroup.Sketch.Insert([]byte{'b'})

	b := &bytes.Buffer{}
	w := csv.NewWriter(b)
	w.Comma = '\t'

	encodeTimeseriesGroupCSV(&tsGroup, w, SchemaNonUniqueMTSWithDigests)
	w.Flush()
	assert.NoError(t, w.Error())

	csvr := csv.NewReader(b)
	csvr.Comma = Comma
	records, err := csvr.ReadAll()
	assert.NoError(t, err)

	assert.Equal(t, 2, len(records))

	assert.Equal(t, "csv-test.1", records[0][0])
	assert.Equal(t, "observability", records[0][1])
	assert.Equal(t, "csv-test.1", records[1][0])
	assert.Equal(t, "observability", records[1][1])

	// Map ordering is not guaranteed
	assert.True(t, (records[0][2] == "1" && records[1][2] == "2") || (records[0][2] == "2" && records[1][2] == "1"))
}
