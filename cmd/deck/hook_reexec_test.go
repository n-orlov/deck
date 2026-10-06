package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
)

const reexecPayload = `{"hook_event_name":"SessionEnd","session_id":"conversation-1","reason":"logout"}`

// stateHomeAt creates a state database at the given schema whose recorded
// writer binary is writer, and returns its home. When seed is set it holds one
// session for reexecPayload to land on.
func stateHomeAt(t *testing.T, schema int, writer string, seed bool) (string, config.Paths) {
	t.Helper()
	return stateHomeFor(t, "claude", schema, writer, seed)
}

// stateHomeFor is stateHomeAt for a session of the given agent kind.
func stateHomeFor(t *testing.T, agentKind string, schema int, writer string, seed bool) (string, config.Paths) {
	t.Helper()
	home := t.TempDir()
	paths := config.Paths{Home: home, DataDir: home, ConfigFile: filepath.Join(home, "config.toml"), LogDir: filepath.Join(home, "log"), StateDB: filepath.Join(home, "state.db")}
	db, err := store.Open(paths)
	if err != nil {
		t.Fatal(err)
	}
	if seed {
		if _, err := db.CreateSession(context.Background(), store.CreateSessionInput{
			ID: "row-1", Name: "reexec target", CWD: home, Agent: agentKind, CapturedPath: "/bin",
			Status: "running", StatusSource: "hook", StatusAt: 1000, CreatedAt: 1000, ConversationID: "conversation-1",
		}); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.SetWriterBinary(writer); err != nil {
		t.Fatal(err)
	}
	if _, err := db.DB().Exec(`UPDATE meta SET version = ? WHERE key = 'schema_version'`, schema); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	return home, paths
}

func writeScript(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "writer")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nPATH=/usr/bin:/bin\n"+body), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

// recordingWriter is a stand-in writer binary: it answers `_schema` with
// schema, and for `_hook` appends its argv and the loop-guard marker to log,
// copies stdin to payload and exits with code.
func recordingWriter(t *testing.T, schema, code int) (writer, log, payload string) {
	t.Helper()
	dir := t.TempDir()
	log, payload = filepath.Join(dir, "argv.log"), filepath.Join(dir, "payload")
	writer = writeScript(t, fmt.Sprintf(`if [ "$1" = _schema ]; then echo %d; exit 0; fi
echo "$*|marker=$DECK_HOOK_REEXEC" >> %s
cat > %s
exit %d
`, schema, log, payload, code))
	return writer, log, payload
}

func runHookInProcess(t *testing.T, home string) (int, string) {
	t.Helper()
	isolateDeckEnv(t)
	t.Setenv("DECK_HOME", home)
	code, _, stderr := runCapture(t, reexecPayload, "_hook")
	return code, stderr
}

func readFileOrEmpty(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	return string(data)
}

// R204.2 (#56): an older _hook that meets a newer schema re-execs the recorded
// writer exactly once, with the same argv and the stdin payload intact.
func TestOlderHookReexecsRecordedWriterOnceWithPayload(t *testing.T) {
	writer, log, payload := recordingWriter(t, store.SchemaVersion+1, 0)
	home, _ := stateHomeAt(t, store.SchemaVersion+1, writer, false)
	code, stderr := runHookInProcess(t, home)
	if code != 0 || stderr != "" {
		t.Fatalf("hook exit = %d, stderr %q; want a silent success through the re-exec", code, stderr)
	}
	if got := readFileOrEmpty(t, log); got != "_hook|marker=1\n" {
		t.Fatalf("writer invocations = %q, want exactly one `_hook` re-exec carrying the loop-guard marker", got)
	}
	if got := readFileOrEmpty(t, payload); got != reexecPayload {
		t.Fatalf("writer stdin = %q, want the payload %q", got, reexecPayload)
	}
}

// The re-exec'd writer's own exit code is the hook's, and the R204 message is
// not printed over it.
func TestReexecPropagatesWriterExitCode(t *testing.T) {
	writer, _, _ := recordingWriter(t, store.SchemaVersion+1, 3)
	home, _ := stateHomeAt(t, store.SchemaVersion+1, writer, false)
	code, stderr := runHookInProcess(t, home)
	if code != 3 || strings.Contains(stderr, "Restart the session") {
		t.Fatalf("hook exit = %d, stderr %q; want the writer's exit 3 and no R204 message", code, stderr)
	}
}

// R204.2: every condition that fails prints the R204.1 message, does not
// re-exec, and never runs the writer's `_hook`.
func TestOlderHookDoesNotReexecWhenWriterIsUnusable(t *testing.T) {
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	notExec, notExecLog, _ := recordingWriter(t, store.SchemaVersion+1, 0)
	if err := os.Chmod(notExec, 0o644); err != nil {
		t.Fatal(err)
	}
	older, olderLog, _ := recordingWriter(t, store.SchemaVersion, 0)
	cases := []struct {
		name, writer, log string
	}{
		{"writer missing", filepath.Join(t.TempDir(), "gone"), ""},
		{"writer not executable", notExec, notExecLog},
		{"writer is the hook itself", self, ""},
		{"writer reports a schema older than the database", older, olderLog},
		{"no writer recorded", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			home, _ := stateHomeAt(t, store.SchemaVersion+1, tc.writer, false)
			code, stderr := runHookInProcess(t, home)
			if code != 1 || !strings.Contains(stderr, "Restart the session from deck (R)") || strings.Contains(stderr, "upgrade deck") {
				t.Fatalf("hook exit = %d, stderr %q; want exit 1 and the R204.1 message", code, stderr)
			}
			if tc.log != "" {
				if got := readFileOrEmpty(t, tc.log); got != "" {
					t.Fatalf("writer ran %q, want no re-exec", got)
				}
			}
		})
	}
}

// R204.2 loop guard: a process that carries the env marker is a re-exec and
// never re-execs again, even with a usable writer recorded.
func TestReexecMarkerStopsAFurtherReexec(t *testing.T) {
	writer, log, _ := recordingWriter(t, store.SchemaVersion+1, 0)
	home, _ := stateHomeAt(t, store.SchemaVersion+1, writer, false)
	isolateDeckEnv(t)
	t.Setenv("DECK_HOME", home)
	t.Setenv(hookReexecEnv, "1")
	code, _, stderr := runCapture(t, reexecPayload, "_hook")
	if code != 1 || !strings.Contains(stderr, "Restart the session from deck (R)") {
		t.Fatalf("hook exit = %d, stderr %q; want the R204.1 message", code, stderr)
	}
	if got := readFileOrEmpty(t, log); got != "" {
		t.Fatalf("a marked process re-exec'd: %q", got)
	}
}

func buildCurrentDeck(t *testing.T) string {
	t.Helper()
	root, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(t.TempDir(), "deck-current")
	build := exec.Command("go", "build", "-o", binary, ".")
	build.Dir = root
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build current deck: %v: %s", err, output)
	}
	return binary
}

// End to end with real binaries: an old-schema hook against a database at the
// current schema re-execs the recorded writer (a wrapper that logs and then
// runs the current deck), and the status update lands in the database.
func TestOldBuildHookHealsThroughRecordedCurrentBuild(t *testing.T) {
	old, current := buildOldSchemaDeck(t), buildCurrentDeck(t)
	dir := t.TempDir()
	log, payload := filepath.Join(dir, "argv.log"), filepath.Join(dir, "payload")
	wrapper := writeScript(t, fmt.Sprintf(`if [ "$1" = _hook ]; then echo "$*" >> %[2]s; cat > %[3]s; exec %[1]s "$@" < %[3]s; fi
exec %[1]s "$@"
`, current, log, payload))
	home, paths := stateHomeAt(t, store.SchemaVersion, wrapper, true)
	out, err := runOldDeck(t, old, home, reexecPayload, "_hook")
	if err != nil {
		t.Fatalf("old hook did not heal: %v: %s", err, out)
	}
	if got := readFileOrEmpty(t, log); got != "_hook\n" {
		t.Fatalf("writer invocations = %q, want exactly one `_hook`", got)
	}
	if got := readFileOrEmpty(t, payload); got != reexecPayload {
		t.Fatalf("payload at the writer = %q", got)
	}
	db, err := store.Open(paths)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	row, err := db.GetSession(context.Background(), "row-1")
	if err != nil || row.Status != "stopped" || row.StatusSource != "hook" {
		t.Fatalf("row after healed hook = %#v, %v; want stopped by hook", row, err)
	}
}

// Real binaries, loop guard: the recorded writer is a wrapper that runs the
// same old hook again. The second process carries the marker, so it prints the
// message instead of re-execing, and the writer is entered exactly once.
func TestReexecLoopGuardWithRealBinaries(t *testing.T) {
	old := buildOldSchemaDeck(t)
	dir := t.TempDir()
	log := filepath.Join(dir, "argv.log")
	wrapper := writeScript(t, fmt.Sprintf(`if [ "$1" = _schema ]; then echo 99; exit 0; fi
echo "$*" >> %[2]s
if [ "$(wc -l < %[2]s)" -gt 5 ]; then echo runaway >&2; exit 99; fi
exec %[1]s "$@"
`, old, log))
	home, _ := stateHomeAt(t, store.SchemaVersion, wrapper, true)
	out, err := runOldDeck(t, old, home, reexecPayload, "_hook")
	if err == nil || !strings.Contains(out, "Restart the session from deck (R)") || strings.Contains(out, "runaway") {
		t.Fatalf("hook err = %v, output %q; want the R204.1 message from the second process", err, out)
	}
	if got := readFileOrEmpty(t, log); got != "_hook\n" {
		t.Fatalf("writer entered %q, want exactly once", got)
	}
}

func TestSchemaVerbPrintsSupportedSchema(t *testing.T) {
	code, stdout, _ := runCapture(t, "", "_schema")
	if code != 0 || strings.TrimSpace(stdout) != fmt.Sprint(store.SupportedSchemaVersion()) {
		t.Fatalf("_schema = %d, %q", code, stdout)
	}
}

// A writer that is the hook itself is never re-exec'd, even when it is
// executable and reports a new enough schema (the probe would say yes).
func TestReexecRefusesWriterThatIsTheHookItself(t *testing.T) {
	writer, log, _ := recordingWriter(t, store.SchemaVersion+1, 0)
	newer := &store.NewerSchemaError{DB: store.SchemaVersion + 1, Supported: store.SchemaVersion, WriterBinary: writer}
	var out, errOut strings.Builder
	handled, err := reexecHook(context.Background(), newer, writer, []byte(reexecPayload), &out, &errOut)
	if handled || err != nil || readFileOrEmpty(t, log) != "" {
		t.Fatalf("handled = %v, err = %v, writer ran %q; want no re-exec of itself", handled, err, readFileOrEmpty(t, log))
	}
	// The same writer, seen as a different hook, is re-exec'd: the refusal above is the self check.
	handled, err = reexecHook(context.Background(), newer, "/nonexistent-hook", []byte(reexecPayload), &out, &errOut)
	if !handled || err != nil || readFileOrEmpty(t, log) != "_hook|marker=1\n" {
		t.Fatalf("handled = %v, err = %v, log %q; want one re-exec", handled, err, readFileOrEmpty(t, log))
	}
}

// runWriter hands the writer exactly the hook verb, the buffered payload and
// the loop-guard marker, and reports a clean exit as handled.
func TestRunWriterPassesVerbPayloadAndMarker(t *testing.T) {
	writer, log, payload := recordingWriter(t, store.SchemaVersion+1, 0)
	var out, errOut strings.Builder
	handled, err := runWriter(context.Background(), writer, []byte(reexecPayload), &out, &errOut)
	if !handled || err != nil {
		t.Fatalf("handled = %v, err = %v; want handled and clean", handled, err)
	}
	if got := readFileOrEmpty(t, log); got != "_hook|marker=1\n" {
		t.Fatalf("writer saw %q, want the single _hook verb with the marker", got)
	}
	if got := readFileOrEmpty(t, payload); got != reexecPayload {
		t.Fatalf("writer stdin = %q, want the buffered payload", got)
	}
}

// A writer that exits non-zero yields a *hookReexecExit with that exact code,
// whatever the code is.
func TestRunWriterReportsEveryNonZeroExitCode(t *testing.T) {
	for _, code := range []int{1, 7, 42} {
		writer, _, _ := recordingWriter(t, store.SchemaVersion+1, code)
		var out, errOut strings.Builder
		handled, err := runWriter(context.Background(), writer, []byte(reexecPayload), &out, &errOut)
		var exit *hookReexecExit
		if !handled || !errors.As(err, &exit) || exit.code != code {
			t.Fatalf("code %d: handled = %v, err = %v; want a hookReexecExit with that code", code, handled, err)
		}
	}
}

// A writer that cannot start is not handled, so the caller prints the R204
// message instead of swallowing the hook.
func TestRunWriterNotHandledWhenWriterCannotStart(t *testing.T) {
	var out, errOut strings.Builder
	handled, err := runWriter(context.Background(), filepath.Join(t.TempDir(), "missing"), []byte(reexecPayload), &out, &errOut)
	if handled || err != nil {
		t.Fatalf("handled = %v, err = %v; want not handled and no error", handled, err)
	}
}

// hangingWriter answers `_schema` normally but wedges on `_hook`, in a child
// process that inherits the output pipes (the worst case for a kill).
func hangingWriter(t *testing.T) string {
	t.Helper()
	return writeScript(t, fmt.Sprintf(`if [ "$1" = _schema ]; then echo %d; exit 0; fi
sleep 5
`, store.SchemaVersion+1))
}

func shortenWriterTimeout(t *testing.T, d time.Duration) {
	t.Helper()
	saved := writerRunTimeout
	writerRunTimeout = d
	t.Cleanup(func() { writerRunTimeout = saved })
}

// R212 (#71 item 1): a writer still running at the timeout is killed and
// runWriter returns not-handled and no error, so the caller prints the R204
// restart message, well before the writer would have finished by itself.
func TestRunWriterKillsAWriterThatOutlivesTheTimeout(t *testing.T) {
	shortenWriterTimeout(t, 300*time.Millisecond)
	writer := hangingWriter(t)
	var out, errOut strings.Builder
	start := time.Now()
	handled, err := runWriter(context.Background(), writer, []byte(reexecPayload), &out, &errOut)
	if handled || err != nil {
		t.Fatalf("handled = %v, err = %v; want not handled and no error after the timeout", handled, err)
	}
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Fatalf("runWriter returned after %v, want it bounded by the shortened timeout", elapsed)
	}
}

// R212: through the whole hook, a wedged writer yields the R204 restart
// message and exit 1 within the bound.
func TestHookPrintsRestartMessageWhenWriterHangs(t *testing.T) {
	shortenWriterTimeout(t, 300*time.Millisecond)
	home, _ := stateHomeAt(t, store.SchemaVersion+1, hangingWriter(t), false)
	start := time.Now()
	code, stderr := runHookInProcess(t, home)
	if code != 1 || !strings.Contains(stderr, "Restart the session from deck (R)") {
		t.Fatalf("hook exit = %d, stderr %q; want exit 1 and the R204.1 message", code, stderr)
	}
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Fatalf("hook returned after %v, want it bounded by the shortened timeout", elapsed)
	}
}

// R212: the run timeout is 10 s and the `_schema` probe stays at 3 s.
func TestWriterTimeoutsKeepTheirBounds(t *testing.T) {
	if writerRunTimeout != 10*time.Second || writerProbeTimeout != 3*time.Second {
		t.Fatalf("run timeout %v, probe timeout %v; want 10s and 3s", writerRunTimeout, writerProbeTimeout)
	}
}
