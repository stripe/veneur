package veneuer

import (
	"strings"
	"testing"
)

func TestParser(t *testing.T) {
	m, _ := ParseMetric([]byte("a.b.c:1|c"))
	if m == nil {
		t.Error("Want metric, got nil!")
	} else {
		if m.Name != "a.b.c" {
			t.Errorf("Expected name, wanted (a.b.c) got (%s)", m.Name)
		}
		if m.Value != 1 {
			t.Errorf("Expected value, wanted (1) got (%d)", m.Value)
		}
		if m.Type != "counter" {
			t.Errorf("Expected type, wanted (counter) got (%s)", m.Type)
		}
	}
}

func TestParserWithTags(t *testing.T) {
	m, _ := ParseMetric([]byte("a.b.c:1|c|#foo:bar,baz:gorch"))
	if m == nil {
		t.Error("Want metric, got nil!")
	} else {
		if m.Name != "a.b.c" {
			t.Errorf("Expected name, wanted (a.b.c) got (%s)", m.Name)
		}
		if m.Value != 1 {
			t.Errorf("Expected value, wanted (1) got (%d)", m.Value)
		}
		if m.Type != "counter" {
			t.Errorf("Expected type, wanted (counter) got (%s)", m.Type)
		}
		if len(m.Tags) != 2 {
			t.Errorf("Expected tags, wanted (2) got (%d)", len(m.Tags))
		}
	}

	v, valueError := ParseMetric([]byte("a.b.c:fart|c"))
	if valueError == nil || !strings.Contains(valueError.Error(), "Invalid integer") {
		t.Errorf("Unexpected success of invalid value (%v)", v)
	}
}

func TestParserWithSampleRate(t *testing.T) {
	m, _ := ParseMetric([]byte("a.b.c:1|c|@0.1"))
	if m == nil {
		t.Error("Want metric, got nil!")
	} else {
		if m.Name != "a.b.c" {
			t.Errorf("Expected name, wanted (a.b.c) got (%s)", m.Name)
		}
		if m.Value != 1 {
			t.Errorf("Expected value, wanted (1) got (%d)", m.Value)
		}
		if m.Type != "counter" {
			t.Errorf("Expected type, wanted (counter) got (%s)", m.Type)
		}
		if m.SampleRate != 0.1 {
			t.Errorf("Expected sample rate, wanted (0.1) got (%f)", m.SampleRate)
		}
	}

	v, valueError := ParseMetric([]byte("a.b.c:fart|c"))
	if valueError == nil || !strings.Contains(valueError.Error(), "Invalid integer") {
		t.Errorf("Unexpected success of invalid value (%v)", v)
	}
}

func TestParserWithSampleRateAndTags(t *testing.T) {
	m, _ := ParseMetric([]byte("a.b.c:1|c|@0.1|#foo:bar,baz:gorch"))
	if m == nil {
		t.Error("Want metric, got nil!")
	} else {
		if m.Name != "a.b.c" {
			t.Errorf("Expected name, wanted (a.b.c) got (%s)", m.Name)
		}
		if m.Value != 1 {
			t.Errorf("Expected value, wanted (1) got (%d)", m.Value)
		}
		if m.Type != "counter" {
			t.Errorf("Expected type, wanted (counter) got (%s)", m.Type)
		}
		if m.SampleRate != 0.1 {
			t.Errorf("Expected sample rate, wanted (0.1) got (%f)", m.SampleRate)
		}
		if len(m.Tags) != 2 {
			t.Errorf("Expected tags, wanted (2) got (%d)", len(m.Tags))
		}
	}

	v, valueError := ParseMetric([]byte("a.b.c:fart|c"))
	if valueError == nil || !strings.Contains(valueError.Error(), "Invalid integer") {
		t.Errorf("Unexpected success of invalid value (%v)", v)
	}
}
