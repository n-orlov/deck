package agent

import (
	"encoding/json"
	"regexp"
	"strings"
	"testing"
)

// hookCommandOf returns the single hook command string a harness's Instrument
// embeds for event, decoded back from the launch argv it produces (the form
// the agent's own shell finally runs), or "" when it embeds none.
func hookCommandOf(t *testing.T, adapter Adapter, deckExecutable, event string) string {
	t.Helper()
	argv, _ := adapter.Instrument(LaunchInput{Profile: "safe", DeckExecutable: deckExecutable})
	switch adapter.(type) {
	case Claude:
		var settings claudeHookSettings
		if len(argv) != 2 || argv[0] != "--settings" {
			t.Fatalf("claude Instrument argv = %#v", argv)
		}
		if err := json.Unmarshal([]byte(argv[1]), &settings); err != nil {
			t.Fatal(err)
		}
		return settings.Hooks[event][0].Hooks[0].Command
	case Codex:
		pattern := regexp.MustCompile(`^hooks\.` + event + `=\[\{hooks=\[\{type="command",command="((?:[^"\\]|\\.)*)"\}\]\}\]$`)
		for i := 0; i+1 < len(argv); i++ {
			if argv[i] != "-c" {
				continue
			}
			if match := pattern.FindStringSubmatch(argv[i+1]); match != nil {
				return strings.NewReplacer(`\\`, `\`, `\"`, `"`).Replace(match[1])
			}
		}
	case Pi:
		// Pi's hook command rides in the environment its deck-owned extension
		// reads, and the extension is named in the launch argv.
		argv, env := adapter.Instrument(LaunchInput{Profile: "safe", DeckExecutable: deckExecutable, DeckHome: "/deck-home"})
		if len(argv) != 2 || argv[0] != "-e" || argv[1] != PiExtensionPath("/deck-home") {
			t.Fatalf("pi Instrument argv = %#v", argv)
		}
		return env[PiHookCommandEnv]
	}
	return ""
}

// R204 (#56), the launch-path read for all three harnesses: the hook command a
// session's agent keeps for its whole life is the single-quoted absolute path
// of the deck binary that launched it plus ` _hook`, for Claude, Codex and Pi
// alike, whatever the path holds. Claude and Codex embed it in their launch
// argv, Pi hands it to its deck-owned extension through the environment.
func TestHookCommandNamesTheLaunchingDeckExecutableForEveryHarness(t *testing.T) {
	paths := map[string]string{
		"/opt/deck/bin/deck":            `'/opt/deck/bin/deck' _hook`,
		"/opt/deck builds/deck":         `'/opt/deck builds/deck' _hook`,
		`/opt/deck's "bin"/deck`:        `'/opt/deck'"'"'s "bin"/deck' _hook`,
		`/opt/back\slash/deck`:          `'/opt/back\slash/deck' _hook`,
		"/-leading-dash-is-not-flag/dk": `'/-leading-dash-is-not-flag/dk' _hook`,
	}
	harnesses := []struct {
		name    string
		adapter Adapter
		events  []string
	}{
		{"claude", Claude{}, claudeHookEvents},
		{"codex", Codex{}, codexHookEvents},
		{"pi", Pi{}, PiHookEvents},
	}
	for _, h := range harnesses {
		for path, want := range paths {
			for _, event := range h.events {
				if got := hookCommandOf(t, h.adapter, path, event); got != want {
					t.Errorf("%s %s hook command for %q = %q, want %q", h.name, event, path, got, want)
				}
			}
		}
	}
}

// The command string is constant per install: the same executable yields the
// identical hook command on every launch, create or resume, with or without a
// lease generation, so a session's recorded binding is a function of the path
// alone (R204c compares exactly that path).
func TestHookCommandDependsOnTheExecutablePathAlone(t *testing.T) {
	for name, adapter := range map[string]Adapter{"claude": Claude{}, "codex": Codex{}, "pi": Pi{}} {
		event := "SessionEnd"
		first := hookCommandOf(t, adapter, "/opt/deck/a", event)
		argv, _ := adapter.Instrument(LaunchInput{Profile: "yolo", DeckExecutable: "/opt/deck/a", LaunchGeneration: "gen-9", DeckSessionID: "row-1", ConversationID: "c"})
		if len(argv) == 0 || first == "" {
			t.Fatalf("%s instrumented nothing", name)
		}
		if again := hookCommandOf(t, adapter, "/opt/deck/a", event); again != first {
			t.Errorf("%s hook command differs between launches: %q vs %q", name, first, again)
		}
		if other := hookCommandOf(t, adapter, "/opt/deck/b", event); other == first {
			t.Errorf("%s hook command ignores the executable path: %q", name, other)
		}
	}
}
