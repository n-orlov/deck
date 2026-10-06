package features

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/cucumber/godog"
)

// staleHookBinaries is the two-binary fixture behind
// features/stale_hook_binding.feature (R204, #56). Binary A is the real
// cmd/deck built with the test-only `deckoldschema` tag and an ldflag that
// lowers the schema it claims to support, so it stands in for an older deck
// without ever being a real old release. Binary B is a byte copy of the
// current build.
type staleHookBinaries struct {
	current    string // the scenario's released binary, before A replaced it
	a, b       string
	aSchema    int
	lastOutput string
	lastErr    error
}

func registerStaleHookBindingSteps(sc *godog.ScenarioContext) {
	sc.Step(`^the scenario deck binary is the old build "A" claiming schema ([0-9]+)$`, scenarioDeckBinaryIsOldBuildA)
	sc.Step(`^the scenario deck binary is "B", a copy of the current build$`, scenarioDeckBinaryIsCopyB)
	sc.Step(`^the state database schema is newer than binary "A"'s$`, databaseSchemaNewerThanBinaryA)
	sc.Step(`^the state database session "([^"]+)" is bound to the hook executable of binary "A"$`, sessionBoundToBinaryA)
	sc.Step(`^the state database session "([^"]+)" is bound to the hook executable of binary "B"$`, sessionBoundToBinaryB)
	sc.Step(`^a long-running fake "pi" binary is on PATH for future deck clients$`, longRunningFakePiOnPATHForFutureClients)
	sc.Step(`^fake Pi session "([^"]+)" loaded the deck extension from the data root$`, fakePiLoadedDeckExtension)
	sc.Step(`^fake Pi session "([^"]+)" fires "([^"]+)" through its installed extension$`, fakePiFiresThroughInstalledExtension)
	sc.Step(`^binary "B" is no longer executable$`, binaryBIsNoLongerExecutable)
	sc.Step(`^binary "A" runs _hook for session "([^"]+)" with a "([^"]+)" payload$`, binaryARunsHook)
	sc.Step(`^that hook run succeeded and printed nothing$`, lastHookSucceededSilently)
	sc.Step(`^that hook run failed and its output names binary "A" and both schemas and says to restart the session from deck$`, lastHookPrintedRestartMessage)
	sc.Step(`^session "([^"]+)" has ([0-9]+) "([^"]+)" events?$`, sessionHasEventCount)
	sc.Step(`^deck client "([^"]+)" screen shows the hint that session binds to binary "A"$`, clientScreenShowsStaleBindingHint)
}

func staleHookState(ctx context.Context) (*ScenarioHarness, *staleHookBinaries, error) {
	h, err := assertionHarness(ctx)
	if err != nil {
		return nil, nil, err
	}
	return h, &h.staleHook, nil
}

func scenarioDeckBinaryIsOldBuildA(ctx context.Context, schema int) error {
	h, st, err := staleHookState(ctx)
	if err != nil {
		return err
	}
	root, err := repositoryRoot()
	if err != nil {
		return err
	}
	dir := filepath.Join(h.Home, "deck-a")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	binary := filepath.Join(dir, "deck")
	ldflags := fmt.Sprintf("-X main.version=v0.0.1-old -X github.com/n-orlov/deck/internal/store.oldSchemaVersion=%d", schema)
	build := exec.CommandContext(ctx, "go", "build", "-tags", "deckoldschema", "-ldflags", ldflags, "-o", binary, "github.com/n-orlov/deck/cmd/deck")
	build.Dir = root
	if output, err := build.CombinedOutput(); err != nil {
		return fmt.Errorf("build old-schema deck A: %w\n%s", err, output)
	}
	st.current, st.a, st.aSchema = h.Binary, binary, schema
	h.Binary = binary
	return nil
}

func scenarioDeckBinaryIsCopyB(ctx context.Context) error {
	h, st, err := staleHookState(ctx)
	if err != nil {
		return err
	}
	if st.current == "" {
		return fmt.Errorf("binary A has not been built yet")
	}
	dir := filepath.Join(h.Home, "deck-b")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	in, err := os.Open(st.current)
	if err != nil {
		return err
	}
	defer in.Close()
	st.b = filepath.Join(dir, "deck")
	out, err := os.OpenFile(st.b, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	h.Binary = st.b
	return nil
}

func databaseSchemaNewerThanBinaryA(ctx context.Context) error {
	h, st, err := staleHookState(ctx)
	if err != nil {
		return err
	}
	db, err := openObservedDatabase(h)
	if err != nil {
		return err
	}
	defer db.Close()
	var schema int
	if err := db.QueryRowContext(ctx, `SELECT version FROM meta WHERE key = 'schema_version'`).Scan(&schema); err != nil {
		return err
	}
	// binary A answers the hidden `_schema` verb with the schema it supports.
	out, err := exec.CommandContext(ctx, st.a, "_schema").Output()
	if err != nil {
		return fmt.Errorf("binary A _schema: %w", err)
	}
	supported, err := strconv.Atoi(strings.TrimSpace(string(out)))
	if err != nil {
		return fmt.Errorf("binary A _schema = %q: %w", out, err)
	}
	if supported != st.aSchema || schema <= supported {
		return fmt.Errorf("binary A supports schema %d (built for %d), database is at schema %d; want the database newer", supported, st.aSchema, schema)
	}
	return nil
}

func sessionHookExecutable(ctx context.Context, h *ScenarioHarness, name string) (string, error) {
	db, err := openObservedDatabase(h)
	if err != nil {
		return "", err
	}
	defer db.Close()
	var path string
	if err := db.QueryRowContext(ctx, `SELECT COALESCE(hook_executable, '') FROM sessions WHERE name = ?`, name).Scan(&path); err != nil {
		return "", fmt.Errorf("read hook_executable of %q: %w", name, err)
	}
	return path, nil
}

func sessionBoundToBinaryA(ctx context.Context, name string) error {
	h, st, err := staleHookState(ctx)
	if err != nil {
		return err
	}
	got, err := sessionHookExecutable(ctx, h, name)
	if err != nil {
		return err
	}
	if got != st.a {
		return fmt.Errorf("session %q hook_executable = %q, want binary A %q", name, got, st.a)
	}
	return nil
}

func sessionBoundToBinaryB(ctx context.Context, name string) error {
	h, st, err := staleHookState(ctx)
	if err != nil {
		return err
	}
	// The binding is recorded once the relaunched pane is up, a moment after
	// the launch record the scenario waited on.
	deadline := time.Now().Add(5 * time.Second)
	for {
		got, err := sessionHookExecutable(ctx, h, name)
		if err != nil {
			return err
		}
		if got == st.b {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("session %q hook_executable = %q, want binary B %q", name, got, st.b)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func longRunningFakePiOnPATHForFutureClients(ctx context.Context) error {
	return installFakePiOnPATH(ctx, true)
}

// fakePiPane returns the tmux target of the named session's pane and a reader
// for its joined capture.
func fakePiPane(ctx context.Context, name string) (*ScenarioHarness, string, func() (string, error), error) {
	h, err := assertionHarness(ctx)
	if err != nil {
		return nil, "", nil, err
	}
	slug, err := sessionSlugByName(h, name)
	if err != nil {
		return nil, "", nil, err
	}
	target := "deck_" + slug
	capture := func() (string, error) {
		out, err := tmuxOutput(ctx, h, "capture-pane", "-p", "-J", "-S", "-", "-t", target)
		return string(out), err
	}
	return h, target, capture, nil
}

// fakePiLoadedDeckExtension asserts the launched fake pi announced the
// extension file deck's Pi launch named with -e, under the scenario's data
// root: the install is read from the pane the launch produced.
func fakePiLoadedDeckExtension(ctx context.Context, name string) error {
	h, _, capture, err := fakePiPane(ctx, name)
	if err != nil {
		return err
	}
	want := "fake-pi extension: " + filepath.Join(h.Home, "pi", "deck-hook.js")
	deadline := time.Now().Add(5 * time.Second)
	for {
		output, err := capture()
		if err == nil && strings.Contains(output, want) {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("fake pi pane of %q never announced %q (err=%w):\n%s", name, want, err, output)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// fakePiFiresThroughInstalledExtension asks the running fake pi to fire one
// event through the extension its launch installed (cmd/fake-pi's "hook" pane
// command), which runs the hook command the launch put in its environment: the
// scenario never runs a hook binary itself. The pane's own report of that one
// hook run becomes the "last hook run" the shared hook steps assert on: its
// notification text when the hook failed, nothing when it succeeded.
func fakePiFiresThroughInstalledExtension(ctx context.Context, name, event string) error {
	_, st, err := staleHookState(ctx)
	if err != nil {
		return err
	}
	h, target, capture, err := fakePiPane(ctx, name)
	if err != nil {
		return err
	}
	fired, failed := "fake-pi hook fired: "+event, "fake-pi hook failed: "+event
	before, err := capture()
	if err != nil {
		return err
	}
	baseline := strings.Count(before, fired) + strings.Count(before, failed)
	request, err := json.Marshal(map[string]any{"command": "hook", "event": event, "payload": map[string]any{"source": "startup"}})
	if err != nil {
		return err
	}
	if _, err := tmuxOutput(ctx, h, "send-keys", "-t", target, "-l", string(request)); err != nil {
		return fmt.Errorf("send hook command to fake pi pane %q: %w", target, err)
	}
	if _, err := tmuxOutput(ctx, h, "send-keys", "-t", target, "Enter"); err != nil {
		return fmt.Errorf("submit hook command to fake pi pane %q: %w", target, err)
	}
	deadline := time.Now().Add(15 * time.Second)
	for {
		output, err := capture()
		if err == nil && strings.Count(output, fired)+strings.Count(output, failed) > baseline {
			st.lastOutput, st.lastErr = "", nil
			if strings.Count(output, failed) > strings.Count(before, failed) {
				_, notice, _ := strings.Cut(output[strings.LastIndex(output, failed):], "fake-pi notify: ")
				st.lastOutput, st.lastErr = strings.TrimSpace(notice), fmt.Errorf("fake pi hook %s failed", event)
			}
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("fake pi pane %q never reported the %s hook (err=%w):\n%s", target, event, err, output)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func binaryBIsNoLongerExecutable(ctx context.Context) error {
	_, st, err := staleHookState(ctx)
	if err != nil {
		return err
	}
	return os.Chmod(st.b, 0o644)
}

// binaryARunsHook runs binary A's `_hook` exactly as an agent launched under A
// would (injected session identity in the environment, payload on stdin).
func binaryARunsHook(ctx context.Context, name, event string) error {
	h, st, err := staleHookState(ctx)
	if err != nil {
		return err
	}
	id, err := sessionIDByName(h, name)
	if err != nil {
		return err
	}
	payload, err := json.Marshal(map[string]any{"hook_event_name": event, "source": "fresh"})
	if err != nil {
		return err
	}
	commandCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(commandCtx, st.a, "_hook")
	cmd.Env = append(os.Environ(), h.Environment("DECK_SESSION_ID="+id)...)
	cmd.Stdin = bytes.NewReader(payload)
	output, runErr := cmd.CombinedOutput()
	st.lastOutput, st.lastErr = strings.TrimSpace(string(output)), runErr
	return nil
}

func lastHookSucceededSilently(ctx context.Context) error {
	_, st, err := staleHookState(ctx)
	if err != nil {
		return err
	}
	if st.lastErr != nil || st.lastOutput != "" {
		return fmt.Errorf("binary A hook: %w, output %q; want a silent success through the re-exec", st.lastErr, st.lastOutput)
	}
	return nil
}

func lastHookPrintedRestartMessage(ctx context.Context) error {
	h, st, err := staleHookState(ctx)
	if err != nil {
		return err
	}
	if st.lastErr == nil {
		return fmt.Errorf("binary A hook succeeded (output %q); want it to fail without a re-exec", st.lastOutput)
	}
	db, err := openObservedDatabase(h)
	if err != nil {
		return err
	}
	defer db.Close()
	var dbSchema int
	if err := db.QueryRowContext(ctx, `SELECT version FROM meta WHERE key = 'schema_version'`).Scan(&dbSchema); err != nil {
		return err
	}
	for _, want := range []string{
		st.a, "v0.0.1-old",
		fmt.Sprintf("state database (schema %d)", dbSchema),
		fmt.Sprintf("schema %d)", st.aSchema),
		"Restart the session from deck (R)",
	} {
		if !strings.Contains(st.lastOutput, want) {
			return fmt.Errorf("hook message %q lacks %q", st.lastOutput, want)
		}
	}
	if strings.Contains(strings.ToLower(st.lastOutput), "upgrade deck") {
		return fmt.Errorf("hook message %q says to upgrade deck", st.lastOutput)
	}
	return nil
}

func sessionHasEventCount(ctx context.Context, name string, want int, kind string) error {
	h, _, err := staleHookState(ctx)
	if err != nil {
		return err
	}
	db, err := openObservedDatabase(h)
	if err != nil {
		return err
	}
	defer db.Close()
	deadline := time.Now().Add(5 * time.Second)
	for {
		var got int
		if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM events WHERE session_id = (SELECT id FROM sessions WHERE name = ?) AND kind = ?`, name, kind).Scan(&got); err != nil {
			return err
		}
		if got == want {
			return nil
		}
		if want == 0 || time.Now().After(deadline) {
			return fmt.Errorf("session %q has %d %q events, want %d", name, got, kind, want)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// clientScreenShowsStaleBindingHint asserts the R204c line (ASCII glyphs, as
// the scenario runs with DECK_ASCII=1) on the session's detail dialog. The
// dialog wraps the line at its own width, so the frame is compared with box
// borders removed and whitespace collapsed.
func clientScreenShowsStaleBindingHint(ctx context.Context, client string) error {
	h, st, err := staleHookState(ctx)
	if err != nil {
		return err
	}
	driver, err := h.Client(client)
	if err != nil {
		return err
	}
	want := "hooks: bound to " + st.a + " - restart (R) to refresh"
	waitCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	_, err = driver.WaitForFrameFunc(waitCtx, false, func(frame string) bool {
		flat := strings.Join(strings.Fields(strings.ReplaceAll(frame, "|", " ")), " ")
		return strings.Contains(flat, want)
	})
	if err != nil {
		return fmt.Errorf("client %q did not show %q: %w", client, want, err)
	}
	return nil
}
