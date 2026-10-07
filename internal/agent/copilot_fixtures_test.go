package agent

import (
	"bytes"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// copilotFixtureDir holds real captures from copilot 1.0.93; see
// testdata/probes/copilot-PROVENANCE.md for how each was taken and the edits
// made to the captured bytes (none).
var copilotFixtureDir = filepath.Join("testdata", "probes", "copilot")

// copilotFixtures names every pane capture the R219 rules are fitted to, with
// the anchor substrings the requirement says classify it. The expectation is
// written from the requirement, not from probe.go, so a copilot rule can be
// built against these files without being able to rewrite what they must show.
var copilotFixtures = []struct {
	file string
	// has are substrings that must all be present.
	has []string
	// lacks are substrings that must all be absent.
	lacks []string
	// linePrefix, when set, must start at least one line (after leading blanks).
	linePrefix string
}{
	{file: "trust.txt", has: []string{"Confirm folder trust", "Do you trust the files in this folder?"}},
	{file: "permission.txt", has: []string{"Do you want to ", "↑/↓ to navigate · enter to select · esc to cancel"}, lacks: []string{"Do you trust the files"}},
	{file: "question.txt", has: []string{"Copilot needs information."}, lacks: []string{"Do you want to "}},
	{file: "working.txt", has: []string{"Working", "esc interrupt"}},
	{file: "working-edit-prompt.txt", has: []string{"Working", "esc edit prompt"}},
	{file: "error.txt", has: []string{"· / commands"}, lacks: []string{"Working"}, linePrefix: "✗ "},
	{file: "warning.txt", has: []string{"· / commands"}, lacks: []string{"Working", "✗ "}, linePrefix: "! "},
	{file: "idle.txt", has: []string{"· / commands"}, lacks: []string{"Working", "@ files"}},
	{file: "idle-draft.txt", has: []string{"@ files · # issues"}, lacks: []string{"Working", "/ commands"}},
	{file: "idle-wrapped-80.txt", has: []string{"· / commands"}, lacks: []string{"Working", "← open sidebar · Interactive · Manual Approval"}},
	{file: "allow-all-footer.txt", has: []string{"Allow All", "· / commands"}, lacks: []string{"Manual Approval"}},
}

func TestCopilotPaneFixturesCarryTheirAnchors(t *testing.T) {
	for _, fx := range copilotFixtures {
		t.Run(fx.file, func(t *testing.T) {
			path := filepath.Join(copilotFixtureDir, fx.file)
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read captured copilot pane fixture %s: %v", path, err)
			}
			pane := string(raw)
			for _, want := range fx.has {
				if !strings.Contains(pane, want) {
					t.Errorf("%s lacks anchor %q", path, want)
				}
			}
			for _, bad := range fx.lacks {
				if strings.Contains(pane, bad) {
					t.Errorf("%s must not contain %q", path, bad)
				}
			}
			if fx.linePrefix != "" && !hasLineWithPrefix(pane, fx.linePrefix) {
				t.Errorf("%s has no line starting %q", path, fx.linePrefix)
			}
		})
	}
}

func hasLineWithPrefix(pane, prefix string) bool {
	for _, line := range strings.Split(pane, "\n") {
		if strings.HasPrefix(strings.TrimLeft(line, " "), prefix) {
			return true
		}
	}
	return false
}

func TestCopilotWrappedFooterIsTheEightyColumnCapture(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(copilotFixtureDir, "idle-wrapped-80.txt"))
	if err != nil {
		t.Fatalf("read wrapped footer fixture: %v", err)
	}
	lines := strings.Split(strings.TrimRight(string(raw), "\n"), "\n")
	var footerRows int
	for i, line := range lines {
		if w := len([]rune(line)); w > 80 {
			t.Errorf("line %d is %d columns wide, want <= 80", i+1, w)
		}
		if i > 0 && strings.HasPrefix(strings.TrimLeft(lines[i-1], " "), "─") && strings.Contains(line, "open sidebar") {
			footerRows = len(lines) - i
		}
	}
	if footerRows < 3 {
		t.Errorf("footer spans %d rows, want it wrapped onto at least 3", footerRows)
	}
}

func TestCopilotRawStreamCoversOneStartOneTurnOneExit(t *testing.T) {
	path := filepath.Join(copilotFixtureDir, "stream.raw")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read raw copilot byte stream %s: %v", path, err)
	}
	// In stream order: the startup queries and alt-screen enter, the turn
	// (a BEL-terminated OSC 0 title carrying the prompt text, then the reply),
	// and the exit (alt-screen leave after mouse and focus modes are reset).
	ordered := []string{
		"\x1b[?u", "\x1b[?1049h", "\x1b[?1004h", "\x1b[?1003h", "\x1b[?1006h",
		"\x1b[?12$p", "\x1b[?1007$p", "\x1b]10;?\x1b\\", "\x1b]11;?\x1b\\",
		"\x1b]0;GitHub Copilot\x07", "\x1b[?996n", "\x1b[>q",
		"\x1b]0;say hello – café ✓ - GitHub Copilot\x07",
		"Stand-in reply",
		"\x1b[?1003l", "\x1b[?1004l", "\x1b[?1049l",
	}
	pos := 0
	for _, want := range ordered {
		i := bytes.Index(raw[pos:], []byte(want))
		if i < 0 {
			t.Fatalf("raw stream lacks %q after offset %d", want, pos)
		}
		pos += i + len(want)
	}
	if n := bytes.Count(raw, []byte("\x1b[?1049h")); n != 1 {
		t.Errorf("stream enters the alternate screen %d times, want one start", n)
	}
	if n := bytes.Count(raw, []byte("\x1b[?1049l")); n != 1 {
		t.Errorf("stream leaves the alternate screen %d times, want one exit", n)
	}
}

func TestCopilotFixtureDirHasExactlyTheDocumentedFiles(t *testing.T) {
	want := []string{"stream.raw"}
	for _, fx := range copilotFixtures {
		want = append(want, fx.file)
	}
	entries, err := os.ReadDir(copilotFixtureDir)
	if err != nil {
		t.Fatalf("read %s: %v", copilotFixtureDir, err)
	}
	var got []string
	for _, e := range entries {
		got = append(got, e.Name())
	}
	sort.Strings(got)
	sort.Strings(want)
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("fixture files = %v, documented = %v", got, want)
	}
	prov, err := os.ReadFile(filepath.Join("testdata", "probes", "copilot-PROVENANCE.md"))
	if err != nil {
		t.Fatalf("read provenance: %v", err)
	}
	if !strings.Contains(string(prov), "1.0.93") {
		t.Errorf("provenance does not state the CLI version 1.0.93")
	}
	for _, name := range want {
		if !strings.Contains(string(prov), name) {
			t.Errorf("provenance does not mention %s", name)
		}
	}
}

func TestCopilotFixturesCarryNoCredential(t *testing.T) {
	needles := []string{"ghp_", "gho_", "github_pat_", "Bearer"}
	files, err := filepath.Glob(filepath.Join(copilotFixtureDir, "*"))
	if err != nil || len(files) == 0 {
		t.Fatalf("glob %s: %v (%d files)", copilotFixtureDir, err, len(files))
	}
	files = append(files, filepath.Join("testdata", "probes", "copilot-PROVENANCE.md"))
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("read %s: %v", f, err)
		}
		for _, n := range needles {
			if bytes.Contains(b, []byte(n)) {
				t.Errorf("%s contains %q", f, n)
			}
		}
	}
}
