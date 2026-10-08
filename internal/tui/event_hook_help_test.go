package tui

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// eventHookHelpSection returns the "Event hook" section of the help view, up
// to the next blank line followed by a column-0 heading.
func eventHookHelpSection(t *testing.T) string {
	t.Helper()
	text := helpText(false)
	start := strings.Index(text, "\nEvent hook (")
	if start < 0 {
		t.Fatalf("help view has no Event hook section")
	}
	rest := text[start+1:]
	if end := strings.Index(rest, "\n\n"); end >= 0 {
		rest = rest[:end]
	}
	return rest
}

// TestHelpDocumentsTheEventHookContract is R234's help test: every named part
// of the contract (SPEC §10.1-§10.4) must be in the section, so removing any
// one of them fails its own subtest.
func TestHelpDocumentsTheEventHookContract(t *testing.T) {
	section := eventHookHelpSection(t)
	parts := []struct{ name, want string }{
		{"argv", "argv"},
		{"argv kind position", "argv[1]"},
		{"DECK_EVENT_KIND", "DECK_EVENT_KIND"},
		{"DECK_EVENT_REASON", "DECK_EVENT_REASON"},
		{"DECK_EVENT_MESSAGE", "DECK_EVENT_MESSAGE"},
		{"DECK_EVENT_AT", "DECK_EVENT_AT"},
		{"DECK_SESSION_ prefix", "DECK_SESSION_"},
		{"stdin", "stdin"},
		{"timeout", "timeout"},
		{"timeout setting", "hook timeout setting"},
		{"no retry", "no retry"},
		{"no outbox", "no outbox"},
		{"dedupe", "dedupe"},
		{"dedupe by epoch", "notify_epoch"},
		{"limit 1: no retry", "There is no retry and no outbox"},
		{"limit 2: probe agents need a TUI", "Probe-classified agents"},
		{"limit 2: only while a TUI runs", "only while a deck TUI is running"},
		{"limit 3: process death", "Process death is detected late"},
		{"limit 3: SIGKILL", "SIGKILL"},
	}
	for _, p := range parts {
		t.Run(p.name, func(t *testing.T) {
			if !strings.Contains(section, p.want) {
				t.Errorf("Event hook help section lacks %q:\n%s", p.want, section)
			}
		})
	}
}

// TestHelpEventHookSectionIsReachableInTheHelpView proves the section is part
// of the text the help view renders, not only of helpText's literal.
func TestHelpEventHookSectionIsReachableInTheHelpView(t *testing.T) {
	if !strings.Contains(helpText(false), eventHookHelpSection(t)) {
		t.Fatal("section not in helpText")
	}
}

// readmeEventHookSection returns the README's "## Event hook" section.
func readmeEventHookSection(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	start := strings.Index(text, "\n## Event hook\n")
	if start < 0 {
		t.Fatal("README has no \"## Event hook\" section")
	}
	rest := text[start+1:]
	if end := strings.Index(rest[3:], "\n## "); end >= 0 {
		rest = rest[:end+3]
	}
	return rest
}

// fencedBlocks returns the bodies of the fenced blocks whose info string is lang.
func fencedBlocks(section, lang string) []string {
	var blocks []string
	var cur []string
	in := false
	for _, line := range strings.Split(section, "\n") {
		switch {
		case !in && strings.TrimSpace(line) == "```"+lang:
			in, cur = true, nil
		case in && strings.TrimSpace(line) == "```":
			in = false
			blocks = append(blocks, strings.Join(cur, "\n")+"\n")
		case in:
			cur = append(cur, line)
		}
	}
	return blocks
}

// shSyntax runs `sh -n` over script and returns its combined output and error.
func shSyntax(t *testing.T, script string) (string, error) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "example.sh")
	if err := os.WriteFile(path, []byte(script), 0o600); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command("sh", "-n", path).CombinedOutput()
	return string(out), err
}

// TestReadmeEventHookExamplesPassShSyntaxCheck extracts each README example
// script and runs it through `sh -n` (parse only, nothing is executed).
func TestReadmeEventHookExamplesPassShSyntaxCheck(t *testing.T) {
	scripts := fencedBlocks(readmeEventHookSection(t), "sh")
	if len(scripts) != 2 {
		t.Fatalf("README Event hook section has %d sh scripts, want 2 (Telegram, desktop)", len(scripts))
	}
	for i, want := range []string{"curl", "notify-send"} {
		if !strings.HasPrefix(scripts[i], "#!/bin/sh\n") || !strings.Contains(scripts[i], want) {
			t.Errorf("script %d should be a #!/bin/sh script using %s:\n%s", i, want, scripts[i])
		}
		if out, err := shSyntax(t, scripts[i]); err != nil {
			t.Errorf("script %d (%s) fails sh -n: %v\n%s", i, want, err, out)
		}
	}
}

// TestShSyntaxCheckRejectsABrokenScript keeps the check above honest: the
// same helper must fail on a script with a syntax error.
func TestShSyntaxCheckRejectsABrokenScript(t *testing.T) {
	if _, err := shSyntax(t, "#!/bin/sh\ncase \"$1\" in\n  a) echo a\n"); err == nil {
		t.Fatal("sh -n accepted an unterminated case")
	}
}

// TestReadmeEventHookCarriesTheIdempotencyAndNoRetryNotes pins the two notes
// R234 asks for next to the examples.
func TestReadmeEventHookCarriesTheIdempotencyAndNoRetryNotes(t *testing.T) {
	section := readmeEventHookSection(t)
	for _, want := range []string{"**No retry.**", "**Idempotency.**", "api.telegram.org", "notify-send", "DECK_EVENT_KIND", "stdin"} {
		if !strings.Contains(section, want) {
			t.Errorf("README Event hook section lacks %q", want)
		}
	}
}
