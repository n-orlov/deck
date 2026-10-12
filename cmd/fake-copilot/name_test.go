package main

import (
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/agent"
)

func readWorkspaceName(t *testing.T, h *harness) (string, bool, bool) {
	t.Helper()
	return agent.NewCopilot().ReadName(agent.NameProbeInput{
		ConversationID: testSessionID,
		Env:            map[string]string{"COPILOT_HOME": h.home},
	})
}

// R260: an explicit -n writes name: and user_named: true, and the real
// adapter's reader sees it as explicit, quoted characters intact.
func TestExplicitNameIsWrittenAsUserNamed(t *testing.T) {
	for _, name := range []string{"my review", `fix: it's "odd" # ünï 🚀`} {
		h := newHarness(t)
		if got := h.run([]string{"--session-id", testSessionID, "-n", name}, strings.NewReader(""), nil); got.code != 0 {
			t.Fatalf("exit %d: %s", got.code, got.stderr)
		}
		read, explicit, ok := readWorkspaceName(t, h)
		if !ok || read != name || !explicit {
			t.Errorf("workspace.yaml name = (%q, explicit %v, ok %v), want (%q, true, true)", read, explicit, ok, name)
		}
	}
}

// R260: a session started without -n has no name and user_named: false until
// the first prompt generates one, which stays user_named: false.
func TestFirstPromptOnAnUnnamedSessionGeneratesAName(t *testing.T) {
	h := newHarness(t)
	cmd, stdin := h.startProcess(t, "--session-id", testSessionID)
	waitFor(t, "workspace.yaml", func() bool { return h.exists(h.sessionDir(testSessionID), "workspace.yaml") })
	if name, _, ok := readWorkspaceName(t, h); ok {
		t.Fatalf("an unnamed session already has the name %q", name)
	}
	if _, err := stdin.Write([]byte(`{"command":"prompt","text":"fix the login bug in the parser please"}` + "\n")); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "generated name", func() bool { _, _, ok := readWorkspaceName(t, h); return ok })
	if err := cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = cmd.Wait()
	name, explicit, ok := readWorkspaceName(t, h)
	if !ok || name != "fix the login bug in" || explicit {
		t.Fatalf("generated name = (%q, explicit %v, ok %v), want (\"fix the login bug in\", false, true)", name, explicit, ok)
	}
}

// R260: a later prompt never retitles, and an explicit name is never replaced
// by a first prompt.
func TestPromptsNeverReplaceAnExistingName(t *testing.T) {
	h := newHarness(t)
	h.run([]string{"--session-id", testSessionID, "-n", "chosen"}, strings.NewReader(`{"command":"prompt","text":"something else entirely"}`+"\n"), map[string]string{commandsEnvironment: "1"})
	name, explicit, ok := readWorkspaceName(t, h)
	if !ok || name != "chosen" || !explicit {
		t.Fatalf("name after a prompt = (%q, %v, %v), want the explicit one", name, explicit, ok)
	}
}

// R264: a /rename typed into the pane (the line deck pushes, and the one a
// human types, with its /session alias) writes the name with user_named: true,
// replaces a generated title, and sets the pane title "<name> - GitHub Copilot".
func TestTypedRenameIsPersistedAsUserNamedAndTitlesThePane(t *testing.T) {
	for _, line := range []string{"/rename Inside Copilot", "/session rename Inside Copilot"} {
		h := newHarness(t)
		input := `{"command":"prompt","text":"fix the login bug in the parser please"}` + "\n" + line + "\n"
		got := h.run([]string{"--session-id", testSessionID}, strings.NewReader(input), map[string]string{commandsEnvironment: "1"})
		if got.code != 0 {
			t.Fatalf("%q: exit %d: %s", line, got.code, got.stderr)
		}
		name, explicit, ok := readWorkspaceName(t, h)
		if !ok || name != "Inside Copilot" || !explicit {
			t.Errorf("%q: workspace.yaml name = (%q, explicit %v, ok %v), want (\"Inside Copilot\", true, true)", line, name, explicit, ok)
		}
		for _, title := range []string{"\x1b]0;fix the login bug in - GitHub Copilot\a", "\x1b]0;Inside Copilot - GitHub Copilot\a"} {
			if !strings.Contains(got.stdout, title) {
				t.Errorf("%q: output lacks the pane title %q", line, title)
			}
		}
	}
}

// R264: a session launched with -n shows its name in the pane title from the
// start; an unnamed one sets no named title.
func TestLaunchNameTitlesThePane(t *testing.T) {
	named := newHarness(t).run([]string{"--session-id", testSessionID, "-n", "chosen"}, strings.NewReader(""), nil)
	if !strings.Contains(named.stdout, "\x1b]0;chosen - GitHub Copilot\a") {
		t.Errorf("a named launch never titled the pane; output %q", named.stdout)
	}
	unnamed := newHarness(t).run([]string{"--session-id", testSessionID}, strings.NewReader(""), nil)
	if strings.Contains(unnamed.stdout, "\x1b]0;") {
		t.Errorf("an unnamed launch titled the pane; output %q", unnamed.stdout)
	}
}

// R264: a bare or blank /rename is not a rename and names nothing.
func TestBlankRenameNamesNothing(t *testing.T) {
	if name, ok := typedRename("/rename   "); ok {
		t.Fatalf("a blank /rename was taken as the name %q", name)
	}
}
