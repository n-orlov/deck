package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/n-orlov/deck/internal/agent"
	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/hookrecv"
	"github.com/n-orlov/deck/internal/store"
	"github.com/n-orlov/deck/internal/tmux"
)

// eventHookFixture is one temp state home holding one session row, a deck
// configuration with an event hook set and the directory its scripts write to.
type eventHookFixture struct {
	settings config.Settings
	paths    config.Paths
	out      string
}

// newEventHookFixture seeds one row of the given agent kind whose conversation
// is "conv-1", with every offered kind enabled for it.
func newEventHookFixture(t *testing.T, agentKind, status string) eventHookFixture {
	t.Helper()
	t.Setenv("DECK_SESSION_ID", "")
	settings, paths := hookCapSettings(t)
	dir := t.TempDir()
	settings.Socket = "priv-eh-" + filepath.Base(filepath.Dir(dir)) + "-" + filepath.Base(dir)
	settings.Reconcile = 500 * time.Millisecond
	settings.EventHookDefault = true
	settings.EventHookEvents = config.EventHookKinds
	settings.EventHookTimeout = 3 * time.Second
	db, err := store.Open(paths)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	row, err := db.CreateSession(context.Background(), store.CreateSessionInput{
		ID: "row-1", Name: "api", CWD: paths.Home, Agent: agentKind, CapturedPath: "/bin",
		Status: status, StatusSource: "user", StatusAt: 1000, CreatedAt: 1000,
		ConversationID: "conv-1", Env: map[string]string{"API_TOKEN": "s3cret-value"},
	})
	if err != nil {
		t.Fatal(err)
	}
	// The row's tmux session is alive on the fixture's private socket, so the
	// post-hook liveness pass sees no process death: a test that asserts what
	// the payload alone offered is not answered by a disappearance's `ended`.
	keepAlive := tmux.Client{Socket: settings.Socket}
	t.Cleanup(func() { _ = exec.Command("tmux", "-L", settings.Socket, "kill-server").Run() })
	if _, err := keepAlive.Create(context.Background(), tmux.Launch{Slug: row.Slug, CWD: paths.Home,
		Command: []string{"/bin/sh", "-c", "sleep 600"}}); err != nil {
		t.Fatal(err)
	}
	return eventHookFixture{settings: settings, paths: paths, out: t.TempDir()}
}

// script installs an executable event hook running body and points the
// settings at it (plus any fixed arguments).
func (f *eventHookFixture) script(t *testing.T, body string, fixed ...string) {
	t.Helper()
	path := filepath.Join(f.out, "hook.sh")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nPATH=/usr/bin:/bin\n"+body), 0o755); err != nil {
		t.Fatal(err)
	}
	f.settings.EventHook = strings.Join(append([]string{path}, fixed...), " ")
}

// capture is a script that records argv[1], DECK_EVENT_KIND and stdin, and
// snapshots the state database as it is when the script starts.
func (f *eventHookFixture) capture(t *testing.T, tail string) {
	t.Helper()
	f.script(t, fmt.Sprintf(`out=%[1]q
mkdir -p "$out/snap"
cp %[2]q* "$out/snap/"
printf '%%s' "$1" > "$out/argv1"
printf '%%s' "$DECK_EVENT_KIND" > "$out/env_kind"
printf '%%s' "$DECK_EVENT_MESSAGE" > "$out/env_message"
cat > "$out/stdin"
%[3]s
`, f.out, f.paths.StateDB, tail))
}

func (f eventHookFixture) read(t *testing.T, name string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(f.out, name))
	if err != nil {
		t.Fatalf("the script did not record %s: %v", name, err)
	}
	return string(raw)
}

func (f eventHookFixture) run(payload string) (int, string) {
	var stderr bytes.Buffer
	code := runHookCommand(f.settings, strings.NewReader(payload), &stderr)
	return code, stderr.String()
}

func (f eventHookFixture) lastEvent(t *testing.T, paths config.Paths) store.Event {
	t.Helper()
	db, err := store.Open(paths)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	events, err := db.ListEvents(context.Background(), 20)
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range events {
		if event.SessionID == "row-1" {
			return event
		}
	}
	t.Fatalf("no event on row-1 in %s", paths.StateDB)
	return store.Event{}
}

func (f eventHookFixture) hasEvent(t *testing.T, paths config.Paths, kind string) bool {
	t.Helper()
	db, err := store.Open(paths)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	events, err := db.ListEvents(context.Background(), 50)
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range events {
		if event.SessionID == "row-1" && event.Kind == kind {
			return true
		}
	}
	return false
}

// waitFor polls until the file exists and returns it; a detached script is
// not waited for by the hook, so the test waits for it.
func (f eventHookFixture) waitFor(t *testing.T, name string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(filepath.Join(f.out, name)); err == nil {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("%s never appeared", name)
}

// hookPayloadCases are one payload per agent family that reaches `deck _hook`:
// Claude, Codex, Pi and Copilot (whose event name rides in the environment).
var hookPayloadCases = []struct {
	name, agent, payload, copilotEvent string
	wantKind, wantStored               string
}{
	{"claude", "claude", `{"hook_event_name":"Stop","session_id":"conv-1","last_assistant_message":"all done"}`, "", "idle", "stop"},
	{"codex", "codex", `{"hook_event_name":"PermissionRequest","session_id":"conv-1","tool_name":"shell"}`, "", "waiting", "permission_request"},
	{"pi", "pi", `{"hook_event_name":"Notification","session_id":"conv-1","notification_type":"idle_prompt"}`, "", "waiting", "notification"},
	{"copilot", "copilot", `{"sessionId":"conv-1","stopReason":"end_turn"}`, "agentStop", "idle", "stop"},
}

// R232a (1)+(2): the event hook sees argv[1], DECK_EVENT_KIND and the versioned
// payload for every agent family, and by the time it runs the status update
// and the event row are committed.
func TestHookDispatchesEventAfterTheEventRowIsCommitted(t *testing.T) {
	for _, tc := range hookPayloadCases {
		t.Run(tc.name, func(t *testing.T) {
			f := newEventHookFixture(t, tc.agent, "running")
			t.Setenv(agent.CopilotHookEventEnv, tc.copilotEvent)
			f.capture(t, "")
			if code, stderr := f.run(tc.payload); code != 0 {
				t.Fatalf("hook exit = %d, stderr %q", code, stderr)
			}
			if got := f.read(t, "argv1"); got != tc.wantKind {
				t.Errorf("argv[1] = %q, want %q", got, tc.wantKind)
			}
			if got := f.read(t, "env_kind"); got != tc.wantKind {
				t.Errorf("DECK_EVENT_KIND = %q, want %q", got, tc.wantKind)
			}
			var body struct {
				Version int `json:"version"`
				Session struct {
					ID, Name, Agent, Status string
				} `json:"session"`
				Event struct{ Kind, At, Reason, Message string } `json:"event"`
				Deck  struct{ Host, Version string }             `json:"deck"`
			}
			if err := json.Unmarshal([]byte(f.read(t, "stdin")), &body); err != nil {
				t.Fatalf("stdin is not the SPEC §10.1 JSON object: %v", err)
			}
			if body.Version != 1 || body.Session.ID != "row-1" || body.Session.Name != "api" || body.Session.Agent != tc.agent ||
				body.Session.Status != map[string]string{"idle": "idle", "waiting": "waiting"}[tc.wantKind] ||
				body.Event.Kind != tc.wantKind || body.Event.At == "" || body.Deck.Version == "" {
				t.Errorf("payload = %+v", body)
			}
			// The state database as the script saw it already held the event row.
			snap := f.paths
			snap.StateDB = filepath.Join(f.out, "snap", "state.db")
			snap.Home, snap.DataDir = filepath.Join(f.out, "snap"), filepath.Join(f.out, "snap")
			if event := f.lastEvent(t, snap); event.Kind != tc.wantStored {
				t.Errorf("event row at script start = %q, want %q", event.Kind, tc.wantStored)
			}
		})
	}
}

// The stop payload's last message reaches the script as DECK_EVENT_MESSAGE.
func TestHookDispatchPassesTheLastAssistantMessage(t *testing.T) {
	f := newEventHookFixture(t, "claude", "running")
	f.capture(t, "")
	f.run(hookPayloadCases[0].payload)
	if got := f.read(t, "env_message"); got != "all done" {
		t.Errorf("DECK_EVENT_MESSAGE = %q, want the last assistant message", got)
	}
}

// Fixed arguments of an argv event_hook follow the kind (SPEC §10.1).
func TestHookDispatchKeepsKindAsArgv1WithFixedArgumentsAfterIt(t *testing.T) {
	f := newEventHookFixture(t, "claude", "running")
	f.script(t, `printf '%s|%s|%s' "$1" "$2" "$3" > "`+filepath.Join(f.out, "argv")+`"`+"\n", "--fixed", "two")
	f.run(hookPayloadCases[0].payload)
	if got := f.read(t, "argv"); got != "idle|--fixed|two" {
		t.Errorf("argv[1:] = %q, want idle|--fixed|two", got)
	}
}

// Nothing is spawned when no script is set, the kind is not enabled, or the
// pair already fired in this epoch; a second identical hook is one spawn.
func TestHookDispatchFiltersAndDedupes(t *testing.T) {
	counted := func(f *eventHookFixture, t *testing.T) {
		f.script(t, `echo x >> "`+filepath.Join(f.out, "runs")+`"`+"\n")
	}
	runs := func(f eventHookFixture) int {
		raw, _ := os.ReadFile(filepath.Join(f.out, "runs"))
		return strings.Count(string(raw), "x")
	}
	t.Run("inert without a script", func(t *testing.T) {
		f := newEventHookFixture(t, "claude", "running")
		if code, _ := f.run(hookPayloadCases[0].payload); code != 0 {
			t.Fatalf("exit %d", code)
		}
		if _, err := os.Stat(filepath.Join(f.out, "runs")); err == nil {
			t.Fatal("something ran")
		}
	})
	t.Run("kind not in the list", func(t *testing.T) {
		f := newEventHookFixture(t, "claude", "running")
		f.settings.EventHookEvents = []string{"waiting"}
		counted(&f, t)
		f.run(hookPayloadCases[0].payload)
		if got := runs(f); got != 0 {
			t.Fatalf("idle spawned %d times with only waiting enabled", got)
		}
	})
	t.Run("one spawn per epoch", func(t *testing.T) {
		f := newEventHookFixture(t, "claude", "running")
		counted(&f, t)
		f.run(hookPayloadCases[1].payload)
		f.run(hookPayloadCases[1].payload)
		if got := runs(f); got != 1 {
			t.Fatalf("the same prompt in one epoch spawned %d times, want 1", got)
		}
	})
}

// R232a (3): a session-end payload dispatches the hook detached, so `_hook`
// returns while a slow script is still running, and records nothing about it.
func TestHookSessionEndDispatchesDetachedAndDoesNotWait(t *testing.T) {
	f := newEventHookFixture(t, "claude", "running")
	f.settings.EventHookTimeout = 300 * time.Millisecond
	f.capture(t, fmt.Sprintf("echo started > %q\nsleep 2\necho finished > %q", filepath.Join(f.out, "started"), filepath.Join(f.out, "finished")))
	started := time.Now()
	code, stderr := f.run(`{"hook_event_name":"SessionEnd","session_id":"conv-1","reason":"logout"}`)
	elapsed := time.Since(started)
	if code != 0 || stderr != "" {
		t.Fatalf("hook exit = %d, stderr %q", code, stderr)
	}
	// The same bound the released SessionEnd test holds the hook to.
	if elapsed >= time.Second {
		t.Fatalf("session-end _hook took %s while the script slept 2s: it waited", elapsed)
	}
	f.waitFor(t, "started")
	if _, err := os.Stat(filepath.Join(f.out, "finished")); err == nil {
		t.Fatal("the script had already finished: the test cannot show the hook did not wait")
	}
	if got := f.read(t, "argv1"); got != "ended" {
		t.Errorf("argv[1] = %q, want ended", got)
	}
	if got := f.read(t, "env_kind"); got != "ended" {
		t.Errorf("DECK_EVENT_KIND = %q, want ended", got)
	}
	if !strings.Contains(f.read(t, "stdin"), `"kind":"ended"`) {
		t.Errorf("stdin = %q, want the versioned payload with kind ended", f.read(t, "stdin"))
	}
	f.waitFor(t, "finished")
}

// R232a (3): the detached handoff never waits on the script reading its
// stdin, so a session-end payload larger than a pipe buffer (a long reason
// lands in the payload twice) still returns within the same bound.
func TestHookSessionEndWithAPayloadLargerThanAPipeDoesNotWait(t *testing.T) {
	f := newEventHookFixture(t, "claude", "running")
	f.settings.EventHookTimeout = 300 * time.Millisecond
	f.capture(t, fmt.Sprintf("sleep 2\necho finished > %q", filepath.Join(f.out, "finished")))
	reason := strings.Repeat("x", 50000)
	payload := `{"hook_event_name":"SessionEnd","session_id":"conv-1","reason":"` + reason + `"}`
	type outcome struct {
		code   int
		stderr string
	}
	done := make(chan outcome, 1)
	started := time.Now()
	go func() {
		code, stderr := f.run(payload)
		done <- outcome{code, stderr}
	}()
	var got outcome
	select {
	case got = <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("session-end _hook blocked handing a large payload to the event hook")
	}
	if elapsed := time.Since(started); elapsed >= time.Second {
		t.Fatalf("session-end _hook took %s while the script slept 2s: it waited", elapsed)
	}
	if got.code != 0 || got.stderr != "" {
		t.Fatalf("hook exit = %d, stderr %q", got.code, got.stderr)
	}
	if !f.hasEvent(t, f.paths, "session_end") {
		t.Error("the session_end event row is missing")
	}
	f.waitFor(t, "finished")
	if stdin := f.read(t, "stdin"); !strings.Contains(stdin, `"kind":"ended"`) || strings.Count(stdin, reason) < 1 {
		t.Errorf("stdin (%d bytes) lacks the ended kind or the full reason", len(stdin))
	}
}

// R232a (4): a missing, failing or slow script leaves the hook's exit code and
// stderr exactly what they are with no event hook configured.
func TestHookExitAndOutputAreUnchangedByTheEventHook(t *testing.T) {
	payload := hookPayloadCases[0].payload
	baseline := newEventHookFixture(t, "claude", "running")
	wantCode, wantStderr := baseline.run(payload)

	cases := map[string]func(*eventHookFixture, *testing.T){
		"missing": func(f *eventHookFixture, _ *testing.T) { f.settings.EventHook = filepath.Join(f.out, "nope") },
		"not executable": func(f *eventHookFixture, t *testing.T) {
			path := filepath.Join(f.out, "plain")
			if err := os.WriteFile(path, []byte("#!/bin/sh\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			f.settings.EventHook = path
		},
		"failing": func(f *eventHookFixture, t *testing.T) { f.script(t, "echo noise; echo more >&2; exit 7\n") },
		"slow": func(f *eventHookFixture, t *testing.T) {
			f.settings.EventHookTimeout = 300 * time.Millisecond
			f.script(t, "sleep 30\n")
		},
	}
	for name, configure := range cases {
		t.Run(name, func(t *testing.T) {
			f := newEventHookFixture(t, "claude", "running")
			configure(&f, t)
			started := time.Now()
			code, stderr := f.run(payload)
			if code != wantCode || stderr != wantStderr {
				t.Fatalf("exit/stderr = %d/%q, want the no-hook %d/%q", code, stderr, wantCode, wantStderr)
			}
			if elapsed := time.Since(started); elapsed > 5*time.Second {
				t.Fatalf("hook took %s: a slow script must be cut at event_hook_timeout", elapsed)
			}
			if !f.hasEvent(t, f.paths, "stop") {
				t.Fatal("the event was lost")
			}
		})
	}
}

// A hook that records no change of the session offers the script nothing.
func TestHookDispatchSkipsEventsThatChangedNothing(t *testing.T) {
	cases := map[string]string{
		"orphan":           `{"hook_event_name":"Stop","session_id":"other-conversation"}`,
		"in-session end":   `{"hook_event_name":"SessionEnd","session_id":"conv-1","reason":"clear"}`,
		"prompt":           `{"hook_event_name":"UserPromptSubmit","session_id":"conv-1"}`,
		"compaction start": `{"hook_event_name":"SessionStart","session_id":"conv-1","source":"compact"}`,
	}
	for name, payload := range cases {
		t.Run(name, func(t *testing.T) {
			f := newEventHookFixture(t, "claude", "running")
			f.script(t, `echo x > "`+filepath.Join(f.out, "ran")+`"`+"\n")
			f.run(payload)
			if _, err := os.Stat(filepath.Join(f.out, "ran")); err == nil {
				t.Fatalf("%s spawned the hook", name)
			}
		})
	}
}

// A late hook whose status write lost to a stopped row offers nothing.
func TestHookDispatchSkipsAStatusThatWasNotApplied(t *testing.T) {
	f := newEventHookFixture(t, "claude", "stopped")
	f.script(t, `echo x > "`+filepath.Join(f.out, "ran")+`"`+"\n")
	f.run(hookPayloadCases[0].payload)
	if _, err := os.Stat(filepath.Join(f.out, "ran")); err == nil {
		t.Fatal("an idle event for a stopped row spawned the hook")
	}
}

func TestHookResultOffersEvent(t *testing.T) {
	ok := hookrecv.Result{SessionID: "row-1", Status: "idle", Kind: "stop"}
	if !hookResultOffersEvent(ok) {
		t.Fatal("an applied hook offers no event")
	}
	for name, mutate := range map[string]func(*hookrecv.Result){
		"orphan":     func(r *hookrecv.Result) { r.Orphan = true },
		"superseded": func(r *hookrecv.Result) { r.Superseded = true },
		"in-session": func(r *hookrecv.Result) { r.InSession = true },
		"no status":  func(r *hookrecv.Result) { r.Status = "" },
		"no session": func(r *hookrecv.Result) { r.SessionID = "" },
	} {
		result := ok
		mutate(&result)
		if hookResultOffersEvent(result) {
			t.Errorf("%s offers an event", name)
		}
	}
}

// R232b: the hook path stores the script's exit status and output tail against
// the very event row it wrote, and a failing script changes nothing the hook
// prints or returns.
func TestHookDispatchStoresTheResultAgainstTheEvent(t *testing.T) {
	f := newEventHookFixture(t, "claude", "running")
	f.script(t, "echo went wrong >&2\nexit 4\n")
	if code, stderr := f.run(hookPayloadCases[0].payload); code != 0 || stderr != "" {
		t.Fatalf("hook exit = %d, stderr %q; a failing script must change neither", code, stderr)
	}
	db, err := store.Open(f.paths)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	// The post-hook liveness pass may append its own events after the stop.
	events, err := db.ListEvents(context.Background(), 50)
	if err != nil {
		t.Fatal(err)
	}
	var event store.Event
	for _, e := range events {
		if e.SessionID == "row-1" && e.Kind == "stop" {
			event = e
		}
	}
	got, ok, err := db.EventHookResultOf(context.Background(), event.Seq)
	if event.Seq == 0 || err != nil || !ok {
		t.Fatalf("no hook result against the stop event %+v: ok=%v err=%v", event, ok, err)
	}
	if got.Kind != "idle" || got.ExitCode != 4 || got.TimedOut || got.Output != "went wrong\n" {
		t.Fatalf("stored result = %+v, want idle, exit 4, the output tail", got)
	}
}

// SPEC §7 "the next `_hook` invocation" / §10.4: a process death that the
// post-hook liveness pass of `deck _hook` records is offered to the event
// hook by that same `_hook`, so an unattended crash fires with no TUI running.
// The script outlives the pass's reconcile budget: it is bounded by
// event_hook_timeout alone, and its result is stored against the death's row.
func TestHookLivenessPassDispatchesAnUnattendedCrash(t *testing.T) {
	f := newEventHookFixture(t, "claude", "running")
	f.settings.Reconcile = 300 * time.Millisecond
	f.script(t, fmt.Sprintf(`printf '%%s %%s\n' "$1" "$DECK_SESSION_ID" >> %[1]q
sleep 1
echo crash-hook-done
`, filepath.Join(f.out, "calls")))
	db, err := store.Open(f.paths)
	if err != nil {
		t.Fatal(err)
	}
	crashed, err := db.CreateSession(context.Background(), store.CreateSessionInput{
		ID: "row-2", Name: "unattended", CWD: f.paths.Home, Agent: "claude", CapturedPath: "/bin",
		Status: "running", StatusSource: "hook", StatusAt: 1000, CreatedAt: 1001,
	})
	if closeErr := db.Close(); err != nil || closeErr != nil {
		t.Fatal(err, closeErr)
	}
	client := tmux.Client{Socket: f.settings.Socket}
	t.Cleanup(func() { _ = exec.Command("tmux", "-L", f.settings.Socket, "kill-server").Run() })
	if _, err := client.Create(context.Background(), tmux.Launch{Slug: crashed.Slug, CWD: crashed.CWD,
		Command: []string{"/bin/sh", "-c", "printf 'dying words\\n'; exit 9"}}); err != nil {
		t.Fatal(err)
	}
	waitForRetainedDeadPane(t, client, crashed.Slug)

	start := time.Now()
	if code, stderr := f.run(`{"hook_event_name":"Notification","session_id":"conv-1","notification_type":"idle_prompt"}`); code != 0 {
		t.Fatalf("hook exit = %d, stderr %q", code, stderr)
	}
	if elapsed := time.Since(start); elapsed > 2*f.settings.EventHookTimeout+f.settings.Reconcile+2*time.Second {
		t.Fatalf("_hook took %s, past its bounds", elapsed)
	}
	calls := strings.Split(strings.TrimSpace(f.read(t, "calls")), "\n")
	if len(calls) != 2 || calls[0] != "waiting row-1" || calls[1] != "error row-2" {
		t.Fatalf("event hook calls = %q, want the hook's own waiting then the crash's error", calls)
	}
	db, err = store.Open(f.paths)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	run, ok, err := db.LastEventHookResult(context.Background(), "row-2")
	if err != nil || !ok {
		t.Fatalf("no event-hook result on the crashed row: ok=%v err=%v", ok, err)
	}
	if run.Kind != "error" || run.ExitCode != 0 || run.TimedOut || !strings.Contains(run.Output, "crash-hook-done") {
		t.Fatalf("crash hook result = %+v: want error, exit 0, not timed out, the script's output (the reconcile budget must not cut it short)", run)
	}
}

// SPEC §10.4: a tmux session that disappeared cleanly is a process death the
// liveness pass of `deck _hook` records, and that same `_hook` offers `ended`
// for it (after the hook's own event), once, with the result on the event.
func TestHookLivenessPassDispatchesAnUnattendedDisappearance(t *testing.T) {
	f := newEventHookFixture(t, "claude", "running")
	f.script(t, fmt.Sprintf(`printf '%%s %%s\n' "$1" "$DECK_SESSION_ID" >> %[1]q
echo gone-hook-done
`, filepath.Join(f.out, "calls")))
	if err := exec.Command("tmux", "-L", f.settings.Socket, "kill-server").Run(); err != nil {
		t.Fatal(err)
	}
	if code, stderr := f.run(`{"hook_event_name":"Notification","session_id":"conv-1","notification_type":"idle_prompt"}`); code != 0 {
		t.Fatalf("hook exit = %d, stderr %q", code, stderr)
	}
	calls := strings.Split(strings.TrimSpace(f.read(t, "calls")), "\n")
	if len(calls) != 2 || calls[0] != "waiting row-1" || calls[1] != "ended row-1" {
		t.Fatalf("event hook calls = %q, want the hook's own waiting then the disappearance's ended", calls)
	}
	db, err := store.Open(f.paths)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	run, ok, err := db.LastEventHookResult(context.Background(), "row-1")
	if err != nil || !ok || run.Kind != "ended" || run.ExitCode != 0 || !strings.Contains(run.Output, "gone-hook-done") {
		t.Fatalf("stored result = %+v ok=%v err=%v, want ended with the script's output", run, ok, err)
	}
}

// SPEC §3.1: several deaths one liveness pass records cost `_hook` one
// event_hook_timeout together, not one each. Four vanished sessions whose
// script takes a second apiece (one of them failing) leave `_hook` inside the
// hook's own dispatch plus one batch, and every death's result is on its row.
func TestHookLivenessPassHoldsManySlowHooksToOneBound(t *testing.T) {
	f := newEventHookFixture(t, "claude", "running")
	f.script(t, fmt.Sprintf(`printf '%%s %%s\n' "$1" "$DECK_SESSION_ID" >> %[1]q
sleep 1
case "$DECK_SESSION_ID" in row-3) echo failing-hook >&2; exit 5;; esac
echo hook-done
`, filepath.Join(f.out, "calls")))
	db, err := store.Open(f.paths)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"row-2", "row-3", "row-4", "row-5"} {
		if _, err := db.CreateSession(context.Background(), store.CreateSessionInput{
			ID: id, Name: id, CWD: f.paths.Home, Agent: "claude", CapturedPath: "/bin",
			Status: "running", StatusSource: "hook", StatusAt: 1000, CreatedAt: 1001,
		}); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	// row-1 vanishes too: five deaths in one pass.
	if err := exec.Command("tmux", "-L", f.settings.Socket, "kill-server").Run(); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	if code, stderr := f.run(`{"hook_event_name":"Notification","session_id":"conv-1","notification_type":"idle_prompt"}`); code != 0 {
		t.Fatalf("hook exit = %d, stderr %q", code, stderr)
	}
	// The hook's own offer (1 s) plus one batch (1 s); five serial deaths would add 5 s.
	if elapsed := time.Since(start); elapsed > 4*time.Second+f.settings.Reconcile {
		t.Fatalf("_hook took %s with five deaths, want about two script runs", elapsed)
	}
	db, err = store.Open(f.paths)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, id := range []string{"row-1", "row-2", "row-3", "row-4", "row-5"} {
		run, ok, err := db.LastEventHookResult(context.Background(), id)
		if err != nil || !ok || run.Kind != "ended" || run.TimedOut {
			t.Fatalf("%s result = %+v ok=%v err=%v, want a recorded ended", id, run, ok, err)
		}
		wantExit := 0
		if id == "row-3" {
			wantExit = 5
		}
		if run.ExitCode != wantExit {
			t.Fatalf("%s exit = %d, want %d", id, run.ExitCode, wantExit)
		}
	}
}

// waitForRetainedDeadPane waits until tmux keeps slug's pane dead under
// remain-on-exit, the corpse the liveness pass collects.
func waitForRetainedDeadPane(t *testing.T, client tmux.Client, slug string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		live, err := client.List(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		for _, session := range live {
			if session.Name == "deck_"+slug && len(session.Panes) == 1 && session.Panes[0].Dead {
				return
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("crash fixture did not leave a retained dead pane")
}
