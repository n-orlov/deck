package features

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/cucumber/godog"
)

// registerEnvEditorSteps exposes task 020's `e` env editor and task 021's
// own write path (SPEC §6.1/§6.3): opening it for a named session, editing
// the highlighted row, and asserting a session's durable env map, its
// env_dirty flag, tmux's own mirrored environment table, and -- critically
// -- the live pane's own already-running process environment, all directly
// against their real sources rather than deck's own view of them.
func registerEnvEditorSteps(sc *godog.ScenarioContext) {
	sc.Step(`^deck client "([^"]+)" opens the env editor for session "([^"]+)"$`, clientOpensEnvEditorForSession)
	sc.Step(`^deck client "([^"]+)" edits the highlighted env key to "([^"]*)"$`, clientEditsHighlightedEnvKeyToValue)
	sc.Step(`^the state database session "([^"]+)" has no env key "([^"]+)"$`, sessionHasNoEnvKey)
	sc.Step(`^the state database session "([^"]+)" has env key "([^"]+)" with value "([^"]*)"$`, sessionHasEnvKeyWithValue)
	sc.Step(`^the state database session "([^"]+)" is marked env_dirty$`, sessionIsMarkedEnvDirty)
	sc.Step(`^the state database session "([^"]+)" is not marked env_dirty$`, sessionIsNotMarkedEnvDirty)
	sc.Step(`^the live tmux environment for session "([^"]+)" has key "([^"]+)" with value "([^"]*)"$`, liveTmuxEnvironmentHasKeyWithValue)
	sc.Step(`^the live pane process environment for session "([^"]+)" key "([^"]+)" is captured as "([^"]+)"$`, livePaneProcessEnvironmentKeyIsCapturedAs)
	sc.Step(`^the live pane process environment for session "([^"]+)" key "([^"]+)" still matches the captured "([^"]+)"$`, livePaneProcessEnvironmentKeyStillMatchesCaptured)
	sc.Step(`^the live pane process environment for session "([^"]+)" key "([^"]+)" is "([^"]*)"$`, livePaneProcessEnvironmentKeyEquals)
}

// clientOpensEnvEditorForSession selects the named row and sends `e` (SPEC
// §6.1/§6.3), the only way envView opens.
func clientOpensEnvEditorForSession(ctx context.Context, clientName, sessionName string) error {
	if err := clientSelectsSessionByName(ctx, clientName, sessionName); err != nil {
		return err
	}
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	client, err := h.Client(clientName)
	if err != nil {
		return err
	}
	if err := client.Send("e"); err != nil {
		return err
	}
	return client.WaitForFrame(ctx, false, "Environment for "+sessionName)
}

// clientEditsHighlightedEnvKeyToValue sends enter to open the row the
// cursor is already on for editing (task 021: envView's own `>` marker,
// left where a prior step -- opening the editor always starts at row 0 --
// or navigation put it), then the new value's runes, then enter to commit
// it. It relies on the first typed rune replacing the prefilled buffer
// wholesale (updateEnvDialog), so the caller never needs to know the old
// value's length to clear it first.
func clientEditsHighlightedEnvKeyToValue(ctx context.Context, clientName, newValue string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	client, err := h.Client(clientName)
	if err != nil {
		return err
	}
	if err := client.Send("\r"); err != nil {
		return err
	}
	if err := client.WaitForFrame(ctx, false, "Enter saves this key"); err != nil {
		return fmt.Errorf("open the highlighted env key for editing: %w", err)
	}
	if newValue != "" {
		if err := client.Send(newValue); err != nil {
			return err
		}
		if err := client.WaitForFrame(ctx, false, newValue); err != nil {
			return fmt.Errorf("type new env value %q: %w", newValue, err)
		}
	}
	return client.Send("\r")
}

// sessionHasNoEnvKey reads the sessions.env JSON column directly (never
// Go internals) and asserts the named key is absent, proving an env
// editor that never writes anything (task 020 is read-only; task 021
// adds the write path) left the session's own env map untouched.
func sessionHasNoEnvKey(ctx context.Context, name, key string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	db, err := openObservedDatabase(h)
	if err != nil {
		return err
	}
	defer db.Close()
	var envJSON string
	if err := db.QueryRowContext(ctx, `SELECT env FROM sessions WHERE name = ?`, name).Scan(&envJSON); err != nil {
		return fmt.Errorf("observe session %q env: %w", name, err)
	}
	var env map[string]string
	if err := json.Unmarshal([]byte(envJSON), &env); err != nil {
		return fmt.Errorf("parse session %q env JSON %q: %w", name, envJSON, err)
	}
	if _, present := env[key]; present {
		return fmt.Errorf("session %q env unexpectedly has key %q (value %q) after an env-editor-only interaction", name, key, env[key])
	}
	return nil
}

// sessionEnvValue is sessionHasNoEnvKey's own env-column read, factored out
// so the with-value and env_dirty steps below share it rather than each
// reimplementing the same JSON column read.
func sessionEnvValue(ctx context.Context, name, key string) (string, bool, error) {
	h, err := assertionHarness(ctx)
	if err != nil {
		return "", false, err
	}
	db, err := openObservedDatabase(h)
	if err != nil {
		return "", false, err
	}
	defer db.Close()
	var envJSON string
	if err := db.QueryRowContext(ctx, `SELECT env FROM sessions WHERE name = ?`, name).Scan(&envJSON); err != nil {
		return "", false, fmt.Errorf("observe session %q env: %w", name, err)
	}
	var env map[string]string
	if err := json.Unmarshal([]byte(envJSON), &env); err != nil {
		return "", false, fmt.Errorf("parse session %q env JSON %q: %w", name, envJSON, err)
	}
	value, present := env[key]
	return value, present, nil
}

// sessionHasEnvKeyWithValue proves an env-editor commit (task 021) landed
// in the sessions.env JSON column itself, not merely on screen.
func sessionHasEnvKeyWithValue(ctx context.Context, name, key, want string) error {
	value, present, err := sessionEnvValue(ctx, name, key)
	if err != nil {
		return err
	}
	if !present {
		return fmt.Errorf("session %q env has no key %q, want value %q", name, key, want)
	}
	if value != want {
		return fmt.Errorf("session %q env key %q = %q, want %q", name, key, value, want)
	}
	return nil
}

// sessionIsMarkedEnvDirty proves a committed edit (task 021) set the
// sessions.env_dirty column, the same flag the sidebar's `env↻` badge and
// task 022's restart-to-apply flow both read.
func sessionIsMarkedEnvDirty(ctx context.Context, name string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	db, err := openObservedDatabase(h)
	if err != nil {
		return err
	}
	defer db.Close()
	var dirty int
	if err := db.QueryRowContext(ctx, `SELECT env_dirty FROM sessions WHERE name = ?`, name).Scan(&dirty); err != nil {
		return fmt.Errorf("observe session %q env_dirty: %w", name, err)
	}
	if dirty == 0 {
		return fmt.Errorf("session %q env_dirty = 0, want 1 (nonzero) after a committed env edit", name)
	}
	return nil
}

// sessionIsNotMarkedEnvDirty is sessionIsMarkedEnvDirty's inverse: task
// 022's `R` restart is the only path that clears env_dirty back to false
// once a relaunch has actually carried the edit to the new pane.
func sessionIsNotMarkedEnvDirty(ctx context.Context, name string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	db, err := openObservedDatabase(h)
	if err != nil {
		return err
	}
	defer db.Close()
	var dirty int
	if err := db.QueryRowContext(ctx, `SELECT env_dirty FROM sessions WHERE name = ?`, name).Scan(&dirty); err != nil {
		return fmt.Errorf("observe session %q env_dirty: %w", name, err)
	}
	if dirty != 0 {
		return fmt.Errorf("session %q env_dirty = %d, want 0 after a restart applies the edit", name, dirty)
	}
	return nil
}

// liveTmuxEnvironmentHasKeyWithValue asks tmux itself, via `show-environment
// -t`, never deck's own store, proving a committed edit (task 021) really
// reached tmux's own per-session environment table -- the same table new
// panes in this session inherit from, not the already-running one.
func liveTmuxEnvironmentHasKeyWithValue(ctx context.Context, name, key, want string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	slug, err := sessionSlugByName(h, name)
	if err != nil {
		return err
	}
	target := "deck_" + slug
	output, err := tmuxOutput(ctx, h, "show-environment", "-t", target, key)
	if err != nil {
		return fmt.Errorf("read tmux environment for session %q key %q: %w", name, key, err)
	}
	line := strings.TrimSpace(string(output))
	if line != key+"="+want {
		return fmt.Errorf("tmux environment for session %q = %q, want %q", name, line, key+"="+want)
	}
	return nil
}

// livePaneProcessEnvironmentKey reads the running pane process's own
// /proc/<pid>/environ directly -- the one thing tmux set-environment and
// deck's own SetSessionEnv both deliberately never touch -- rather than
// deck's own store or tmux's mirrored table, either of which a bug in
// task 021's write path could make agree with each other while still
// disagreeing with the actual running process.
func livePaneProcessEnvironmentKey(ctx context.Context, name, key string) (string, error) {
	h, err := assertionHarness(ctx)
	if err != nil {
		return "", err
	}
	slug, err := sessionSlugByName(h, name)
	if err != nil {
		return "", err
	}
	target := "deck_" + slug
	output, err := tmuxOutput(ctx, h, "list-panes", "-t", target, "-F", "#{pane_pid}")
	if err != nil {
		return "", fmt.Errorf("locate live pane process for session %q: %w", name, err)
	}
	pidText := strings.TrimSpace(string(output))
	pid, err := strconv.Atoi(pidText)
	if err != nil {
		return "", fmt.Errorf("parse pane_pid %q for session %q: %w", pidText, name, err)
	}
	raw, err := os.ReadFile(fmt.Sprintf("/proc/%d/environ", pid))
	if err != nil {
		return "", fmt.Errorf("read /proc/%d/environ for session %q's live pane: %w", pid, name, err)
	}
	for _, entry := range strings.Split(string(raw), "\x00") {
		if entry == "" {
			continue
		}
		k, v, ok := strings.Cut(entry, "=")
		if ok && k == key {
			return v, nil
		}
	}
	return "", fmt.Errorf("live pane process environment for session %q has no key %q at all", name, key)
}

// livePaneProcessEnvironmentKeyIsCapturedAs snapshots one key's real,
// host-dependent value (e.g. PATH) under a label, mirroring
// scenarioConfigTOMLIsCapturedAs's own capture-then-compare shape, so a
// later step can assert it is unchanged without hardcoding an expected
// string this scenario has no business predicting.
func livePaneProcessEnvironmentKeyIsCapturedAs(ctx context.Context, name, key, label string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	value, err := livePaneProcessEnvironmentKey(ctx, name, key)
	if err != nil {
		return err
	}
	if h.livePaneEnvSnapshots == nil {
		h.livePaneEnvSnapshots = make(map[string]string)
	}
	h.livePaneEnvSnapshots[label] = value
	return nil
}

// livePaneProcessEnvironmentKeyStillMatchesCaptured proves an env-editor
// commit (task 021) never applied silently to a pane that is already
// running -- only a later restart (task 022) can -- by re-reading the
// same process's /proc/<pid>/environ value and comparing it to the one
// captured before the edit, byte for byte.
func livePaneProcessEnvironmentKeyStillMatchesCaptured(ctx context.Context, name, key, label string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	want, ok := h.livePaneEnvSnapshots[label]
	if !ok {
		return fmt.Errorf("no live pane process environment value was captured as %q", label)
	}
	got, err := livePaneProcessEnvironmentKey(ctx, name, key)
	if err != nil {
		return err
	}
	if got != want {
		return fmt.Errorf("live pane process environment for session %q key %q changed since capture %q: before %q, after %q", name, key, label, want, got)
	}
	return nil
}

// livePaneProcessEnvironmentKeyEquals is
// livePaneProcessEnvironmentKeyStillMatchesCaptured's direct-comparison
// counterpart, for task 022's own restart proof: after `R` relaunches the
// pane, the NEW process's own /proc/<pid>/environ must carry the edited
// value literally, not merely differ from whatever was captured before.
func livePaneProcessEnvironmentKeyEquals(ctx context.Context, name, key, want string) error {
	got, err := livePaneProcessEnvironmentKey(ctx, name, key)
	if err != nil {
		return err
	}
	if got != want {
		return fmt.Errorf("live pane process environment for session %q key %q = %q, want %q", name, key, got, want)
	}
	return nil
}
