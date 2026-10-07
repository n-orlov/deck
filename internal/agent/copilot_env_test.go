package agent

import (
	"sort"
	"strings"
	"testing"
)

// R222 items 2-3: the adapter's instrumentation is a pure function of deck's
// own facts (data root, executable, launch generation, profile). Whatever a
// session's COPILOT_HOME is, the plugin dir stays under the data root, and the
// environment deck adds is exactly DECK_EXE, the generation and -- for yolo
// only -- COPILOT_ALLOW_ALL; none of the operator-owned variables.
func TestCopilotInstrumentOwnsOnlyDeckKeysAndKeepsThePluginUnderTheDataRoot(t *testing.T) {
	userOwned := []string{"COPILOT_GITHUB_TOKEN", "GH_TOKEN", "GITHUB_TOKEN", "COPILOT_MODEL", "HTTPS_PROXY", "COPILOT_HOME"}
	for _, profile := range copilotProfiles {
		argv, env := NewCopilot().Instrument(LaunchInput{
			ConversationID: copilotTestID, Profile: profile, DeckExecutable: "/opt/deck", DeckHome: "/data/deck", LaunchGeneration: "g1",
			ExtraArgs: nil,
		})
		if strings.Join(argv, " ") != "--plugin-dir /data/deck/copilot/plugin" {
			t.Fatalf("%s: argv = %v, want the plugin dir under the data root", profile, argv)
		}
		for _, key := range userOwned {
			if _, ok := env[key]; ok {
				t.Fatalf("%s: instrumentation sets %s", profile, key)
			}
		}
		var keys []string
		for key := range env {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		want := "DECK_EXE DECK_LAUNCH_GENERATION"
		if profile == "yolo" {
			want = "COPILOT_ALLOW_ALL DECK_EXE DECK_LAUNCH_GENERATION"
		}
		if got := strings.Join(keys, " "); got != want {
			t.Fatalf("%s: env keys = %q, want %q", profile, got, want)
		}
	}
}
