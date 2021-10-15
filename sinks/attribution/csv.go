package attribution

import (
	"encoding/csv"
	"errors"
	"fmt"
	"strconv"
)

// ErrUnknownSchemaVersion is an error returned when the config specifies a
// schema version that does not exist
var ErrUnknownSchemaVersion = errors.New("specified schema version does not exist")

const (
	SchemaGloballyUniqueMTS = iota + 1 // iota + 1 to assert that a schema version is explicitly specified
	SchemaNonUniqueMTSWithDigests
)

// encodeTimeseriesGroupCSV generates a newline-terminated CSV row that
// describes the data represented by the TimeseriesGroup.
//
// The caller is responsible for setting w.Comma as the appropriate delimiter.
// For performance, encodeCSV does not flush after every call; the caller is
// expected to flush at the end of the operation cycle
func encodeTimeseriesGroupCSV(g *TimeseriesGroup, w *csv.Writer, schemaVersion int) error {
	if schemaVersion == SchemaGloballyUniqueMTS {
		row := [...]string{
			g.Name,
			g.Owner,
			fmt.Sprintf("%d", g.Sketch.Estimate()),
		}
		w.Write(row[:])
		err := w.Error()
		if err != nil {
			return err
		}
	} else if schemaVersion == SchemaNonUniqueMTSWithDigests {
		for _, digest := range g.Digests {
			row := [...]string{
				g.Name,
				g.Owner,
				strconv.FormatUint(uint64(digest), 10),
			}
			w.Write(row[:])
			err := w.Error()
			if err != nil {
				return err
			}
		}
	} else {
		return ErrUnknownSchemaVersion
	}
	return nil
}
