package main

import (
	"bytes"
	"strings"
	"testing"
)

func piGetenv(string) string { return "" }

func piGetwd(t *testing.T) func() (string, error) {
	t.Helper()
	dir := t.TempDir()
	return func() (string, error) { return dir, nil }
}

func TestHelpFlagPrintsUsageWithoutLaunching(t *testing.T) {
	for _, flag := range []string{"--help", "-h"} {
		var stdout bytes.Buffer
		code, err := runWithIO([]string{flag}, strings.NewReader(""), &stdout, piGetenv, piGetwd(t))
		if err != nil || code != 0 {
			t.Fatalf("runWithIO(%s) = (%d, %v)", flag, code, err)
		}
		if got := stdout.String(); got != helpText {
			t.Fatalf("runWithIO(%s) printed %q, want the help text", flag, got)
		}
	}
}

func TestIsHelpRequestOnlyMatchesALoneHelpFlag(t *testing.T) {
	for args, want := range map[string]bool{
		"--help":       true,
		"-h":           true,
		"":             false,
		"--help extra": false,
		"extra --help": false,
		"--approve":    false,
	} {
		var argv []string
		if args != "" {
			argv = strings.Fields(args)
		}
		if got := isHelpRequest(argv); got != want {
			t.Errorf("isHelpRequest(%q) = %v, want %v", args, got, want)
		}
	}
}

func TestRunWithIOReportsAnArgumentError(t *testing.T) {
	var stdout bytes.Buffer
	code, err := runWithIO([]string{"--no-such-flag"}, strings.NewReader(""), &stdout, piGetenv, piGetwd(t))
	if err == nil || !strings.Contains(err.Error(), "unknown option") || code != 0 {
		t.Fatalf("runWithIO = (%d, %v), want the unknown-option error", code, err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("a rejected invocation printed %q before failing", stdout.String())
	}
}
