package testhelpers

import (
	"io"
	"io/ioutil"
	"testing"

	"github.com/stretchr/testify/assert"
)

// Helper function for determining that two readers are equal
func AssertReadersEqual(t *testing.T, expected io.Reader, actual io.Reader) {

	// If we can seek, ensure that we're starting at the beginning
	for _, reader := range []io.Reader{expected, actual} {
		if readerSeeker, ok := reader.(io.ReadSeeker); ok {
			readerSeeker.Seek(0, io.SeekStart)
		}
	}

	// do the lazy thing for now
	bts, err := ioutil.ReadAll(expected)
	if err != nil {
		t.Fatal(err)
	}

	bts2, err := ioutil.ReadAll(actual)
	if err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, string(bts), string(bts2))
}
