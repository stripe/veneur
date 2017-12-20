package logkey

import "testing"

func TestKeys(t *testing.T) {
	ignored()
	if Func.String() != "func" {
		t.Error("unexpected func")
	}
	// Nothing to test.  Just to signal that we have coverage in this directory
}
