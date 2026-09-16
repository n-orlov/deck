package agent

import (
	"os"
	"sort"
	"strings"
	"testing"
)

func TestCodex_Capabilities(t *testing.T) {
	caps := Codex{}.Capabilities()
	if caps.AssignsConversationID {
		t.Fatalf("codex Caps.AssignsConversationID = true, want false (codex mints its own id)")
	}
	if !caps.Resumable {
		t.Fatalf("codex Caps.Resumable = false, want true")
	}
	if !caps.HasTranscript {
		t.Fatalf("codex Caps.HasTranscript = false, want true")
	}
	if caps.Executable != "codex" {
		t.Fatalf("codex Caps.Executable = %q, want %q", caps.Executable, "codex")
	}
	for _, profile := range []string{"safe", "edits", "yolo"} {
		if !caps.SupportsProfile(profile) {
			t.Fatalf("codex Caps.SupportsProfile(%q) = false, want true", profile)
		}
	}
	if caps.SupportsProfile("plan") {
		t.Fatalf("codex Caps.SupportsProfile(%q) = true, want false — codex has no plan mode", "plan")
	}
	if len(caps.Profiles) != 3 {
		t.Fatalf("codex Caps.Profiles = %v, want exactly [safe edits yolo]", caps.Profiles)
	}
}

func TestCodex_LaunchArgv(t *testing.T) {
	cases := []struct {
		profile string
		want    []string
	}{
		{"safe", []string{"codex", "-a", "on-request", "-s", "workspace-write"}},
		{"edits", []string{"codex", "-a", "never", "-s", "workspace-write"}},
		{"yolo", []string{"codex", "-a", "never", "-s", "danger-full-access"}},
	}

	codex := Codex{}
	for _, tc := range cases {
		t.Run(tc.profile, func(t *testing.T) {
			argv, err := codex.Launch(LaunchInput{
				CWD:       "/tmp/proj",
				Profile:   tc.profile,
				ExtraArgs: []string{"--extra"},
			})
			if err != nil {
				t.Fatalf("Launch: %v", err)
			}
			want := append(append([]string{}, tc.want...), "--extra")
			if !equalArgv(argv, want) {
				t.Fatalf("Launch(%q) argv = %v, want %v", tc.profile, argv, want)
			}
			for _, a := range argv {
				if a == "resume" {
					t.Fatalf("Launch(%q) argv %v contains a resume/id argument, want none", tc.profile, argv)
				}
			}
		})
	}
}

func TestCodex_ResumeArgv(t *testing.T) {
	cases := []struct {
		profile string
		want    []string
	}{
		{"safe", []string{"codex", "resume", "conv-123", "-a", "on-request", "-s", "workspace-write"}},
		{"edits", []string{"codex", "resume", "conv-123", "-a", "never", "-s", "workspace-write"}},
		{"yolo", []string{"codex", "resume", "conv-123", "-a", "never", "-s", "danger-full-access"}},
	}

	codex := Codex{}
	for _, tc := range cases {
		t.Run(tc.profile, func(t *testing.T) {
			argv, err := codex.Resume(ResumeInput{
				CWD:            "/tmp/proj",
				ConversationID: "conv-123",
				Profile:        tc.profile,
				ExtraArgs:      []string{"--extra"},
			})
			if err != nil {
				t.Fatalf("Resume: %v", err)
			}
			want := append(append([]string{}, tc.want...), "--extra")
			if !equalArgv(argv, want) {
				t.Fatalf("Resume(%q) argv = %v, want %v", tc.profile, argv, want)
			}
		})
	}
}

func TestCodex_UnknownProfile(t *testing.T) {
	codex := Codex{}
	if _, err := codex.Launch(LaunchInput{Profile: "plan"}); err == nil {
		t.Fatalf("Launch with profile %q: want error, got nil", "plan")
	}
	if _, err := codex.Resume(ResumeInput{ConversationID: "conv-123", Profile: "bogus"}); err == nil {
		t.Fatalf("Resume with profile %q: want error, got nil", "bogus")
	}
}

func TestCodex_LaunchSucceedsWithEmptyConversationID(t *testing.T) {
	codex := Codex{}
	argv, err := codex.Launch(LaunchInput{CWD: "/tmp/proj", Profile: "safe", ConversationID: ""})
	if err != nil {
		t.Fatalf("Launch with empty ConversationID: want success, got error: %v", err)
	}
	for _, a := range argv {
		if a == "" {
			t.Fatalf("Launch argv %v contains an empty argument", argv)
		}
	}
}

func TestCodex_ResumeRefusesEmptyConversationID(t *testing.T) {
	codex := Codex{}
	if _, err := codex.Resume(ResumeInput{CWD: "/tmp/proj", Profile: "safe", ConversationID: ""}); err == nil {
		t.Fatalf("Resume with empty ConversationID: want error, got nil")
	}
}

func TestCodex_InstrumentEncodesInlineHooksNoIO(t *testing.T) {
	deckExecutable := "/opt/deck/bin/deck"
	argv, env := (Codex{}).Instrument(LaunchInput{
		CWD:            "/tmp/proj",
		Profile:        "safe",
		DeckExecutable: deckExecutable,
		DeckSessionID:  "deck-row-1",
	})

	wantEvents := []string{"SessionStart", "UserPromptSubmit", "PermissionRequest", "Stop", "SessionEnd"}
	if len(argv) != len(wantEvents)*2+1 {
		t.Fatalf("Instrument argv = %#v, want %d entries (one -c pair per event plus the bypass flag)", argv, len(wantEvents)*2+1)
	}
	gotEvents := make([]string, 0, len(wantEvents))
	for i := 0; i+1 < len(argv); i += 2 {
		if argv[i] != "-c" {
			t.Fatalf("Instrument argv[%d] = %q, want %q", i, argv[i], "-c")
		}
		value := argv[i+1]
		event := strings.TrimPrefix(strings.SplitN(value, "=", 2)[0], "hooks.")
		gotEvents = append(gotEvents, event)
		want := `hooks.` + event + `=[{hooks=[{type="command",command="/opt/deck/bin/deck _hook"}]}]`
		if value != want {
			t.Fatalf("Instrument -c value for %s = %q, want %q", event, value, want)
		}
	}
	sort.Strings(gotEvents)
	sortedWant := append([]string{}, wantEvents...)
	sort.Strings(sortedWant)
	if !equalArgv(gotEvents, sortedWant) {
		t.Fatalf("Instrument events = %v, want %v", gotEvents, wantEvents)
	}
	if argv[len(argv)-1] != "--dangerously-bypass-hook-trust" {
		t.Fatalf("Instrument argv last entry = %q, want %q", argv[len(argv)-1], "--dangerously-bypass-hook-trust")
	}
	if env != nil {
		t.Fatalf("Instrument env = %#v, want nil (no LaunchGeneration set)", env)
	}
}

func TestCodex_InstrumentEmitsGenerationEnvOnlyWhenLeaseHeld(t *testing.T) {
	argvNoLease, envNoLease := (Codex{}).Instrument(LaunchInput{Profile: "yolo", DeckExecutable: "/deck"})
	if envNoLease != nil {
		t.Fatalf("Instrument env with no LaunchGeneration = %#v, want nil", envNoLease)
	}
	_ = argvNoLease

	argvLeased, envLeased := (Codex{}).Instrument(LaunchInput{Profile: "yolo", DeckExecutable: "/deck", LaunchGeneration: "gen-7"})
	if envLeased == nil || envLeased[LaunchGenerationEnv] != "gen-7" {
		t.Fatalf("Instrument env with LaunchGeneration = %#v, want {%s: gen-7}", envLeased, LaunchGenerationEnv)
	}
	_ = argvLeased
}

// TestCodex_InstrumentEncoderHandlesQuoteBackslashSpace guards the narrow
// -c encoder against exactly the three bytes a real executable path can
// carry that TOML's basic string syntax treats specially or that a naive
// encoder might mishandle: a double quote, a backslash and a plain space.
func TestCodex_InstrumentEncoderHandlesQuoteBackslashSpace(t *testing.T) {
	deckExecutable := `/opt/deck's "builds"\deck bin`
	command := codexHookCommand(deckExecutable)
	value := codexHookOverride("SessionStart", command)

	want := `hooks.SessionStart=[{hooks=[{type="command",command="/opt/deck's \"builds\"\\deck bin _hook"}]}]`
	if value != want {
		t.Fatalf("codexHookOverride with quote/backslash/space executable = %q, want %q", value, want)
	}

	// Also exercised end to end through Instrument, so a future refactor
	// that bypasses codexHookOverride cannot silently regress this.
	argv, _ := (Codex{}).Instrument(LaunchInput{Profile: "safe", DeckExecutable: deckExecutable})
	found := false
	for _, a := range argv {
		if strings.HasPrefix(a, "hooks.SessionStart=") {
			found = true
			if a != want {
				t.Fatalf("Instrument's SessionStart -c value = %q, want %q", a, want)
			}
		}
	}
	if !found {
		t.Fatalf("Instrument argv %v carries no hooks.SessionStart entry", argv)
	}
}

// TestCodex_InstrumentWritesNoFilesAnywhereUnderCodexHome guards SPEC
// §8.2's "nothing is written to disk" guarantee: deck writes no
// hooks.json, no config.toml and no temp file under $CODEX_HOME (or
// anywhere else) to instrument a codex launch — the hooks are inline argv
// only.
func TestCodex_InstrumentWritesNoFilesAnywhereUnderCodexHome(t *testing.T) {
	codexHome := t.TempDir()
	t.Setenv("CODEX_HOME", codexHome)

	_, _ = (Codex{}).Instrument(LaunchInput{Profile: "safe", DeckExecutable: "/opt/deck"})

	entries, err := os.ReadDir(codexHome)
	if err != nil {
		t.Fatalf("read CODEX_HOME %s: %v", codexHome, err)
	}
	if len(entries) != 0 {
		t.Fatalf("Instrument wrote under $CODEX_HOME %s: %v", codexHome, entries)
	}
}

// TestCodex_InstrumentCommandCarriesNoSessionSpecificSubstring guards SPEC
// §8.2's rule that per-session facts never go in the hook *command*
// (codex's trust hash covers the command string): the composed command
// must be the fixed "<deck> _hook" and nothing else, even when the caller
// hands Instrument session-identifying values.
func TestCodex_InstrumentCommandCarriesNoSessionSpecificSubstring(t *testing.T) {
	argv, _ := (Codex{}).Instrument(LaunchInput{
		CWD:              "/tmp/session-cwd-marker",
		ConversationID:   "conv-marker-abc123",
		Profile:          "edits",
		DeckExecutable:   "/opt/deck",
		DeckSessionID:    "deck-row-marker-42",
		LaunchGeneration: "gen-marker-99",
		DeckHome:         "/tmp/deck-home-marker",
	})
	joined := strings.Join(argv, "\x00")
	for _, marker := range []string{
		"session-cwd-marker",
		"conv-marker-abc123",
		"deck-row-marker-42",
		"gen-marker-99",
		"deck-home-marker",
	} {
		if strings.Contains(joined, marker) {
			t.Fatalf("Instrument argv %v unexpectedly contains session-specific substring %q", argv, marker)
		}
	}
}
