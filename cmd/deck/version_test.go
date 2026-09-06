package main

import (
	"bytes"
	"runtime"
	"strings"
	"testing"
)

// TestVersionFlagAnswersBeforeConfig pins the contract install.sh and the
// release workflow rely on: `deck --version` prints one line to stdout and
// exits 0 without loading configuration, touching the state directory or
// needing tmux -- so a DECK_HOME pointing at an unwritable path must not
// matter.
func TestVersionFlagAnswersBeforeConfig(t *testing.T) {
	t.Setenv("DECK_HOME", "/proc/deck-does-not-exist")
	for _, flag := range []string{"--version", "-version", "-v", "version"} {
		var stdout, stderr bytes.Buffer
		if code := run([]string{"deck", flag}, strings.NewReader(""), &stdout, &stderr); code != 0 {
			t.Fatalf("%s: exit %d, stderr %q", flag, code, stderr.String())
		}
		got := stdout.String()
		want := runtime.GOOS + "/" + runtime.GOARCH + "\n"
		if !strings.HasPrefix(got, "deck ") || !strings.HasSuffix(got, want) || strings.Count(got, "\n") != 1 {
			t.Fatalf("%s: stdout %q, want one line \"deck <version> %s\"", flag, got, strings.TrimSpace(want))
		}
		if stderr.Len() != 0 {
			t.Fatalf("%s: unexpected stderr %q", flag, stderr.String())
		}
	}
	// Anything longer than the bare flag is not a version request: a hook
	// invocation or a stray second argument must fall through unchanged.
	if isVersionRequest([]string{"deck", "--version", "extra"}) || isVersionRequest([]string{"deck", "_hook"}) {
		t.Fatal("isVersionRequest accepted arguments that are not a bare version request")
	}
}
