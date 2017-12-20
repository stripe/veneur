package safeexec

import (
	"bytes"
	"os/exec"

	"strings"

	"github.com/signalfx/golib/errors"
)

// Execute a command, passing in stdin, returning stdout, stderr, and nil, only if the command
// finishes with a non zero error code
func Execute(name string, stdin string, args ...string) (string, string, error) {
	cmd := exec.Command(name, args...)
	cmd.Stdin = strings.NewReader(stdin)
	stdout, stderr := new(bytes.Buffer), new(bytes.Buffer)
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	err := cmd.Run()
	return stdout.String(), stderr.String(), errors.Annotatef(err, "cannot run command %s", name)
}
