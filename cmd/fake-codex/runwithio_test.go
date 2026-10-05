package main

import (
	"bytes"
	"strings"
	"testing"
)

func noEnv(string) string { return "" }

func TestHelpFlagPrintsUsageWithoutLaunching(t *testing.T) {
	for _, flag := range []string{"--help", "-h"} {
		var stdout bytes.Buffer
		code, err := runWithIO([]string{flag}, strings.NewReader(""), &stdout, &bytes.Buffer{}, noEnv)
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
		"-a":           false,
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
	code, err := runWithIO([]string{"--full-auto"}, strings.NewReader(""), &stdout, &bytes.Buffer{}, noEnv)
	if err == nil || !strings.Contains(err.Error(), "--full-auto") || code != 0 {
		t.Fatalf("runWithIO = (%d, %v), want the --full-auto rejection", code, err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("a rejected invocation printed %q before failing", stdout.String())
	}
}
