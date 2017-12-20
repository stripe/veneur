package safeexec

import (
	"testing"

	"runtime"

	"github.com/stretchr/testify/assert"
)

var Bob = "abc"

func TestEchoExec(t *testing.T) {
	if runtime.GOOS != "windows" {
		stdout, stderr, err := Execute("echo", "", "hi")
		assert.NoError(t, err)
		assert.Equal(t, stdout, "hi\n")
		assert.Equal(t, stderr, "")
	}
}

func TestNotHereExec(t *testing.T) {
	_, _, err := Execute("asdgfhnjasdgnadsjkgnjadhfgnjkadf", "", "hi")
	assert.Error(t, err)
}
