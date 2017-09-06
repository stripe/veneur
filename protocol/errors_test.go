package protocol

import (
	"fmt"
	"testing"

	"os"

	"github.com/stretchr/testify/assert"
)

func TestFramingError(t *testing.T) {
	assert.True(t, IsFramingError(&errFrameVersion{1}))
	assert.True(t, IsFramingError(&errFrameLength{0}))
	assert.True(t, IsFramingError(&errFramingIO{fmt.Errorf("oh hai")}))

	assert.False(t, IsFramingError(os.ErrClosed))
}
