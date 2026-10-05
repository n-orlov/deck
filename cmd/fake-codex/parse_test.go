package main

import (
	"strings"
	"testing"
)

func TestParseValuedOptionNeedsAFollowingValue(t *testing.T) {
	for _, option := range []string{"-a", "--ask-for-approval", "-s", "--sandbox", "-c", "--config"} {
		_, err := parse([]string{"prompt", option})
		if err == nil || !strings.Contains(err.Error(), "requires a value") {
			t.Errorf("parse(prompt %s) error = %v, want a requires-a-value error", option, err)
		}
	}
}

func TestParseValuedOptionConsumesExactlyItsValue(t *testing.T) {
	got, err := parse([]string{"-c", "a=1", "word", "--config", "b=2", "-s", "read-only", "tail"})
	if err != nil {
		t.Fatalf("parse error = %v", err)
	}
	if want := []string{"a=1", "b=2"}; strings.Join(got.configOverrides, "|") != strings.Join(want, "|") {
		t.Errorf("configOverrides = %q, want %q", got.configOverrides, want)
	}
	if got.sandbox != "read-only" || got.message != "word tail" {
		t.Errorf("sandbox = %q, message = %q, want read-only and %q", got.sandbox, got.message, "word tail")
	}
}
