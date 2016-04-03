package main

import "testing"

func TestSamplers(t *testing.T) {
	// m, _ := ParseMetric("a.b.c|1|c")
	// if m == nil {
	// 	t.Error("Want metric, got nil!")
	// } else {
	// 	if m.Name != "a.b.c" {
	// 		t.Errorf("Expected name, wanted (a.b.c) got (%s)", m.Name)
	// 	}
	// 	if m.Value != 1 {
	// 		t.Errorf("Expected value, wanted (1) got (%d)", m.Value)
	// 	}
	// 	if m.Type != "c" {
	// 		t.Errorf("Expected type, wanted (c) got (%s)", m.Name)
	// 	}
	// }
	//
	// _, valueError := ParseMetric("a.b.c|fart|c")
	// if valueError == nil || !strings.Contains(valueError.Error(), "Invalid float") {
	// 	t.Error("Unexpected success of invalid value")
	// }
}
