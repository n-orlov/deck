package features

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/cucumber/godog"
)

// registerCreateSessionCWDPrefillSteps backs requirement 12: the create
// modal's cwd field opens pre-filled with the most recent §11.7 recent_cwds
// entry (labelled "last used"), or with no history the directory deck
// itself was started in, and the first edit replaces that prefill wholesale
// rather than appending to it (task 008).
func registerCreateSessionCWDPrefillSteps(sc *godog.ScenarioContext) {
	sc.Step(`^deck client "([^"]+)" is started in a fresh directory labelled "([^"]+)"$`, clientStartedInFreshDirectoryLabelled)
	sc.Step(`^deck client "([^"]+)" opens the create modal$`, clientOpensCreateModal)
	sc.Step(`^deck client "([^"]+)" screen contains the directory labelled "([^"]+)"$`, clientScreenContainsDirectoryLabelled)
	sc.Step(`^deck client "([^"]+)" creates shell session "([^"]+)" with a fresh working directory labelled "([^"]+)"$`, clientCreatesShellSessionWithFreshCWDLabelled)
	sc.Step(`^deck client "([^"]+)" creates shell session "([^"]+)" typing over the prefilled working directory with the directory labelled "([^"]+)"$`, clientCreatesShellSessionTypingOverPrefillWithLabelled)
	sc.Step(`^the state database session "([^"]+)" has cwd exactly the directory labelled "([^"]+)"$`, sessionHasCWDExactlyLabelled)
	sc.Step(`^deck client "([^"]+)" presses "(up|down)" in the cwd field (\d+) times?$`, clientPressesArrowInCWDFieldNTimes)
	sc.Step(`^deck client "([^"]+)" tabs to the cwd field$`, clientTabsToCWDField)
}

// namedDirectory returns the real path a prior step registered under
// label, in h.namedDirectories -- the same map task 002's fingerprint
// harness populates, reused here for a directory a session's own cwd or a
// client's own process cwd is pinned to, rather than introducing a second,
// parallel bookkeeping map for the same "label -> real path" shape.
func namedDirectory(h *ScenarioHarness, label string) (string, error) {
	path, ok := h.namedDirectories[label]
	if !ok {
		return "", fmt.Errorf("directory %q has not been registered", label)
	}
	return path, nil
}

func registerNamedDirectory(h *ScenarioHarness, label, path string) {
	if h.namedDirectories == nil {
		h.namedDirectories = make(map[string]string)
	}
	h.namedDirectories[label] = path
}

// clientStartedInFreshDirectoryLabelled creates a new, empty directory
// under the scenario's own DECK_HOME (torn down with everything else),
// labels it, and starts a named client with the released binary's own
// process working directory pinned to it -- the "directory deck was
// started in" a no-history create modal falls back to.
func clientStartedInFreshDirectoryLabelled(ctx context.Context, name, label string) error {
	h, err := scenarioHarness(ctx)
	if err != nil {
		return err
	}
	dir := filepath.Join(h.Home, "create-session-"+label)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create directory labelled %q: %w", label, err)
	}
	registerNamedDirectory(h, label, dir)
	client, err := h.StartNamedClientInDir(ctx, name, dir)
	if err != nil {
		return err
	}
	return client.WaitForFrame(ctx, false, "deck - sessions")
}

// clientOpensCreateModal sends "n" and waits for the create modal's own
// title, without asserting anything about which fields are pre-filled --
// callers state what they expect separately.
func clientOpensCreateModal(ctx context.Context, name string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	client, err := h.Client(name)
	if err != nil {
		return err
	}
	if err := client.Send("n"); err != nil {
		return err
	}
	return client.WaitForFrame(ctx, false, "Create shell session")
}

// clientScreenContainsDirectoryLabelled asserts the currently rendered
// frame contains the real path a prior step registered under label,
// reusing clientScreenContains rather than re-implementing its polling.
func clientScreenContainsDirectoryLabelled(ctx context.Context, name, label string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	dir, err := namedDirectory(h, label)
	if err != nil {
		return err
	}
	return clientScreenContains(ctx, name, dir)
}

// createShellSessionInLabelledCWD drives the real create modal to a
// successful shell-session creation in a brand-new, existing directory
// under the scenario's DECK_HOME: send "n", type the session name, tab to
// the cwd field, type dir over whatever the field already held (its own
// §11.7 prefill included -- typing never clears first, by design; see
// task 008) and submit. Shared by both create-session steps below, which
// differ only in *why* the scenario calls it (seeding recent_cwds history
// vs. proving the wholesale-replace rule), not in what it does.
func createShellSessionInLabelledCWD(ctx context.Context, h *ScenarioHarness, clientName, sessionName, label string) error {
	client, err := h.Client(clientName)
	if err != nil {
		return err
	}
	dir := filepath.Join(h.Home, "create-session-"+label)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create directory labelled %q: %w", label, err)
	}
	registerNamedDirectory(h, label, dir)
	if err := client.Send("n"); err != nil {
		return err
	}
	if err := client.WaitForFrame(ctx, false, "Create shell session"); err != nil {
		return err
	}
	if err := client.Send(sessionName); err != nil {
		return err
	}
	time.Sleep(75 * time.Millisecond)
	if err := client.Send("\t" + dir + "\r"); err != nil {
		return err
	}
	return client.WaitForFrame(ctx, false, "starting")
}

// clientCreatesShellSessionWithFreshCWDLabelled seeds §11.7's recent_cwds
// history: per task 007, a successful create promotes the resolved
// absolute path to the front of it, so a later "opens the create modal"
// step can assert it is the one that pre-fills.
func clientCreatesShellSessionWithFreshCWDLabelled(ctx context.Context, clientName, sessionName, label string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	return createShellSessionInLabelledCWD(ctx, h, clientName, sessionName, label)
}

// clientCreatesShellSessionTypingOverPrefillWithLabelled opens the create
// modal (so the cwd field carries whatever prefill task 008 gave it -- a
// recent_cwds entry if an earlier step seeded one) and types a second,
// different directory over it without ever clearing the field by hand. If
// the wholesale-replace rule regressed to appending instead, the field
// would hold the prefill concatenated with this directory -- a path that
// does not exist -- and the create would be rejected rather than reaching
// "starting", so a passing scenario here already proves the replace, not
// only the later cwd-value assertion.
func clientCreatesShellSessionTypingOverPrefillWithLabelled(ctx context.Context, clientName, sessionName, label string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	return createShellSessionInLabelledCWD(ctx, h, clientName, sessionName, label)
}

// clientTabsToCWDField sends a single tab, moving create-modal focus from
// the name field (0, where "n" leaves it) to the cwd field (1) --
// clientPressesArrowInCWDFieldNTimes below needs the cwd field actually
// focused, since up/down are a no-op on every other create-modal field
// (task 009: only field 1 binds them).
func clientTabsToCWDField(ctx context.Context, name string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	client, err := h.Client(name)
	if err != nil {
		return err
	}
	if err := client.Send("\t"); err != nil {
		return err
	}
	time.Sleep(60 * time.Millisecond)
	return nil
}

// clientPressesArrowInCWDFieldNTimes drives task 009's shell-history-style
// up/down cycling: the caller must already have tabbed focus to the cwd
// field (clientTabsToCWDField) since up/down are a no-op everywhere else in
// the create modal. Sends the real terminal escape sequence for up
// (\x1b[A) or down (\x1b[B) n times, pausing between presses exactly as
// clientPressesKeyNTimes (features/layout_modes_test.go) does for any
// other repeated-keystroke step, since the same pty-coalescing gotcha
// applies to any raw byte sent back to back.
func clientPressesArrowInCWDFieldNTimes(ctx context.Context, name, direction string, n int) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	client, err := h.Client(name)
	if err != nil {
		return err
	}
	seq := "\x1b[A"
	if direction == "down" {
		seq = "\x1b[B"
	}
	for i := 0; i < n; i++ {
		if err := client.Send(seq); err != nil {
			return err
		}
		time.Sleep(60 * time.Millisecond)
	}
	return nil
}

// sessionHasCWDExactlyLabelled asserts the sessions table's cwd column for
// name is byte-identical to the real path registered under label -- a
// stricter check than substring-on-screen, catching a leftover prefill
// prefix or suffix a screen assertion alone would miss.
func sessionHasCWDExactlyLabelled(ctx context.Context, name, label string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	want, err := namedDirectory(h, label)
	if err != nil {
		return err
	}
	db, err := openObservedDatabase(h)
	if err != nil {
		return err
	}
	defer db.Close()
	var got string
	if err := db.QueryRowContext(ctx, "SELECT cwd FROM sessions WHERE name = ?", name).Scan(&got); err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("no session named %q in the state database", name)
		}
		return fmt.Errorf("observe cwd for session %q: %w", name, err)
	}
	if got != want {
		return fmt.Errorf("session %q cwd = %q, want exactly %q", name, got, want)
	}
	return nil
}
