package main

import (
	"context"
	"errors"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/agent"
	"github.com/n-orlov/deck/internal/store"
)

// hookHarness is one agent kind's R204 test subject: the adapter whose launch
// path builds the hook command, and a payload that harness really sends.
type hookHarness struct {
	name    string
	adapter agent.Adapter
	payload string
	kind    string // the event kind the payload records once it lands
	reason  string // and the reason that event carries
}

// R204 (#56) covers Claude, Codex and Pi. Pi's adapter builds no hook command
// (Instrument is empty until Pi has a verified event source, SPEC §8.1), so
// the case is: a `_hook` that reaches the store for a Pi row, whichever way it
// was started, takes exactly the same newer-schema paths.
var hookHarnesses = []hookHarness{
	{"claude", agent.Claude{}, `{"hook_event_name":"SessionEnd","session_id":"conversation-1","reason":"logout"}`, "session_end", "logout"},
	{"codex", agent.Codex{}, `{"hook_event_name":"PermissionRequest","session_id":"conversation-1","tool_name":"shell"}`, "permission_request", "shell"},
	{"pi", agent.Pi{}, `{"hook_event_name":"SessionEnd","session_id":"conversation-1","reason":"logout"}`, "session_end", "logout"},
}

// launchHookCommand returns the exact shell command line the harness's agent
// would run for a hook, built by that harness's real launch path from deck
// (Claude: inside --settings JSON; Codex: inside a -c TOML override). Pi has
// none, so the hook is started as the plain `<deck> _hook` argv instead.
func launchHookCommand(t *testing.T, h hookHarness, deck string) string {
	t.Helper()
	argv, _ := h.adapter.Instrument(agent.LaunchInput{Profile: "safe", DeckExecutable: deck})
	switch h.adapter.(type) {
	case agent.Claude:
		var settings struct {
			Hooks map[string][]struct {
				Hooks []struct{ Command string } `json:"hooks"`
			} `json:"hooks"`
		}
		if len(argv) != 2 {
			t.Fatalf("claude Instrument argv = %#v", argv)
		}
		if err := json.Unmarshal([]byte(argv[1]), &settings); err != nil {
			t.Fatal(err)
		}
		return settings.Hooks["SessionEnd"][0].Hooks[0].Command
	case agent.Codex:
		pattern := regexp.MustCompile(`^hooks\.PermissionRequest=\[\{hooks=\[\{type="command",command="((?:[^"\\]|\\.)*)"\}\]\}\]$`)
		for i := 0; i+1 < len(argv); i++ {
			if match := pattern.FindStringSubmatch(argv[i+1]); argv[i] == "-c" && match != nil {
				return strings.NewReplacer(`\\`, `\`, `\"`, `"`).Replace(match[1])
			}
		}
		t.Fatalf("codex Instrument embedded no PermissionRequest hook: %#v", argv)
	}
	if len(argv) != 0 {
		t.Fatalf("%s unexpectedly instruments hooks: %#v", h.name, argv)
	}
	return "'" + strings.ReplaceAll(deck, "'", `'"'"'`) + "' _hook"
}

// quirkyDeck copies binary to a path holding a space and a single quote, so
// the launch path's shell quoting is exercised by a real exec, and the hook
// still has to name this exact path in its message.
func quirkyDeck(t *testing.T, binary string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "deck builds", "it's")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(dir, "deck")
	in, err := os.Open(binary)
	if err != nil {
		t.Fatal(err)
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY, 0o755)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(out, in); err != nil {
		t.Fatal(err)
	}
	if err := out.Close(); err != nil {
		t.Fatal(err)
	}
	return dst
}

// runLaunchCommand runs the harness's hook command line the way the agent
// does (`sh -c`), with the payload on stdin and the state home in DECK_HOME.
func runLaunchCommand(t *testing.T, command, home, payload string) (string, int) {
	t.Helper()
	cmd := exec.Command("sh", "-c", command)
	cmd.Stdin = strings.NewReader(payload)
	cmd.Env = append(os.Environ(), "DECK_HOME="+home, "DECK_TMUX_SOCKET=priv-hook-harness")
	output, err := cmd.CombinedOutput()
	code := 0
	var exit *exec.ExitError
	switch {
	case errors.As(err, &exit):
		code = exit.ExitCode()
	case err != nil:
		t.Fatalf("run %q: %v", command, err)
	}
	return string(output), code
}

// R204.1 and R204.4 for every harness: an older `_hook`, launched through the
// harness's own hook command, against a newer database prints the restart
// message naming its executable, version and both schemas, and never says
// "upgrade deck".
func TestOlderHookMessageForEveryHarness(t *testing.T) {
	old := quirkyDeck(t, buildOldSchemaDeck(t))
	for _, h := range hookHarnesses {
		t.Run(h.name, func(t *testing.T) {
			home, dbSchema := newerStateHome(t)
			out, code := runLaunchCommand(t, launchHookCommand(t, h, old), home, h.payload)
			if code == 0 {
				t.Fatalf("hook exit = 0, want non-zero; output %q", out)
			}
			for _, want := range []string{old, "v0.0.1-old", fmt.Sprintf("schema %d", oldHookSchema), fmt.Sprintf("schema %d", dbSchema), "Restart the session from deck (R)"} {
				if !strings.Contains(out, want) {
					t.Errorf("hook output lacks %q: %q", want, out)
				}
			}
			if strings.Contains(strings.ToLower(out), "upgrade deck") {
				t.Errorf("hook output says to upgrade deck: %q", out)
			}
		})
	}
}

// R204.2 for every harness: a recorded writer that can take over is re-exec'd
// exactly once with the payload intact, the harness's status update lands on
// its own session row, and the writer's exit code is the hook's.
func TestOlderHookHealsForEveryHarness(t *testing.T) {
	old, current := quirkyDeck(t, buildOldSchemaDeck(t)), buildCurrentDeck(t)
	for _, h := range hookHarnesses {
		t.Run(h.name, func(t *testing.T) {
			dir := t.TempDir()
			log, payload := filepath.Join(dir, "argv.log"), filepath.Join(dir, "payload")
			wrapper := writeScript(t, fmt.Sprintf(`if [ "$1" = _hook ]; then echo "$*|marker=$DECK_HOOK_REEXEC" >> %[2]s; cat > %[3]s; exec %[1]s "$@" < %[3]s; fi
exec %[1]s "$@"
`, current, log, payload))
			home, paths := stateHomeFor(t, h.name, store.SchemaVersion, wrapper, true)
			out, code := runLaunchCommand(t, launchHookCommand(t, h, old), home, h.payload)
			if code != 0 {
				t.Fatalf("old hook did not heal: exit %d: %s", code, out)
			}
			if got := readFileOrEmpty(t, log); got != "_hook|marker=1\n" {
				t.Fatalf("writer invocations = %q, want exactly one `_hook` re-exec carrying the loop-guard marker", got)
			}
			if got := readFileOrEmpty(t, payload); got != h.payload {
				t.Fatalf("payload at the writer = %q, want %q", got, h.payload)
			}
			db, err := store.Open(paths)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			// The payload landed on the harness's own row: the hook event is in
			// its log with the payload's reason. (A non-SessionEnd hook is followed
			// by a liveness pass, which finds this pane-less fixture row gone, so
			// the event, not the row's final status, is what proves the landing.)
			events, err := db.ListEvents(context.Background(), 20)
			if err != nil {
				t.Fatal(err)
			}
			landed := false
			for _, event := range events {
				landed = landed || (event.SessionID == "row-1" && event.Kind == h.kind && event.Reason == h.reason)
			}
			if !landed {
				t.Fatalf("no %s event with reason %q on row-1 after the healed hook: %#v", h.kind, h.reason, events)
			}
		})
	}
}

// R204.2 for every harness: each failed condition prints the R204.1 message,
// does not re-exec, and never runs the writer's `_hook`.
func TestOlderHookDoesNotReexecForEveryHarness(t *testing.T) {
	old := quirkyDeck(t, buildOldSchemaDeck(t))
	notExec, notExecLog, _ := recordingWriter(t, store.SchemaVersion+1, 0)
	if err := os.Chmod(notExec, 0o644); err != nil {
		t.Fatal(err)
	}
	older, olderLog, _ := recordingWriter(t, oldHookSchema, 0)
	cases := []struct{ name, writer, log string }{
		{"writer missing", filepath.Join(t.TempDir(), "gone"), ""},
		{"writer not executable", notExec, notExecLog},
		{"writer is the hook itself", old, ""},
		{"writer reports a schema older than the database", older, olderLog},
		{"no writer recorded", "", ""},
	}
	for _, h := range hookHarnesses {
		for _, tc := range cases {
			t.Run(h.name+"/"+tc.name, func(t *testing.T) {
				home, _ := stateHomeFor(t, h.name, store.SchemaVersion+1, tc.writer, true)
				out, code := runLaunchCommand(t, launchHookCommand(t, h, old), home, h.payload)
				if code != 1 || !strings.Contains(out, "Restart the session from deck (R)") || strings.Contains(out, "upgrade deck") {
					t.Fatalf("hook exit = %d, output %q; want exit 1 and the R204.1 message", code, out)
				}
				if tc.log != "" && readFileOrEmpty(t, tc.log) != "" {
					t.Fatalf("writer ran %q, want no re-exec", readFileOrEmpty(t, tc.log))
				}
			})
		}
	}
}

// R204.2 loop guard for every harness: the recorded writer is a wrapper that
// runs the same old hook again; the second process carries the marker and
// prints the message instead of re-execing, so the writer is entered once.
func TestReexecLoopGuardForEveryHarness(t *testing.T) {
	old := quirkyDeck(t, buildOldSchemaDeck(t))
	for _, h := range hookHarnesses {
		t.Run(h.name, func(t *testing.T) {
			log := filepath.Join(t.TempDir(), "argv.log")
			wrapper := writeScript(t, fmt.Sprintf(`if [ "$1" = _schema ]; then echo 99; exit 0; fi
echo "$*" >> %[2]s
if [ "$(wc -l < %[2]s)" -gt 5 ]; then echo runaway >&2; exit 99; fi
exec %[1]q "$@"
`, old, log))
			home, _ := stateHomeFor(t, h.name, store.SchemaVersion, wrapper, true)
			out, code := runLaunchCommand(t, launchHookCommand(t, h, old), home, h.payload)
			if code == 0 || !strings.Contains(out, "Restart the session from deck (R)") || strings.Contains(out, "runaway") {
				t.Fatalf("hook exit = %d, output %q; want the R204.1 message from the second process", code, out)
			}
			if got := readFileOrEmpty(t, log); got != "_hook\n" {
				t.Fatalf("writer entered %q, want exactly once", got)
			}
		})
	}
}
