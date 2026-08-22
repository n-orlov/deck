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

// registerCreateCWDGhostSteps backs requirement 14 (§11.7 ghost completion,
// task 010): with the cursor at the end of the create modal's cwd field, a
// UNIQUE directory match is shown inline in the theme's `dimmed` token and
// `right`/`end` accept it, completing to the match plus a trailing `/`;
// files are never candidates; a hidden directory is a candidate only when
// the segment being completed itself starts with `.`; a leading `~`
// expands for scanning without being rewritten in what is typed or shown.
func registerCreateCWDGhostSteps(sc *godog.ScenarioContext) {
	sc.Step(`^deck client "([^"]+)" is started with colour enabled and HOME set to the scenario home$`, startNamedClientWithColourAndScenarioHome)
	sc.Step(`^a scratch directory labelled "([^"]+)" exists$`, scratchDirectoryLabelledExists)
	sc.Step(`^a directory named "([^"]+)" exists in the scratch directory labelled "([^"]+)"$`, directoryExistsInScratch)
	sc.Step(`^a file named "([^"]+)" exists in the scratch directory labelled "([^"]+)"$`, fileExistsInScratch)
	sc.Step(`^a directory named "([^"]+)" exists under the scenario home$`, directoryExistsUnderScenarioHome)
	sc.Step(`^deck client "([^"]+)" types the scratch directory labelled "([^"]+)" followed by "([^"]*)" into the cwd field$`, clientTypesScratchDirPlusSegment)
	sc.Step(`^deck client "([^"]+)" presses "(right|end)" in the cwd field$`, clientPressesKeyInCWDField)
	sc.Step(`^deck client "([^"]+)" types "([^"]+)" as the session name$`, clientTypesSessionName)
	sc.Step(`^deck client "([^"]+)" submits the create modal$`, clientSubmitsCreateModal)
	sc.Step(`^the state database session "([^"]+)" has cwd exactly the scratch directory labelled "([^"]+)" plus "([^"]*)"$`, sessionHasCWDExactlyScratchDirPlus)
}

// scratchDirectoryLabelledExists creates an empty directory under the
// scenario's own DECK_HOME and registers it under label (reusing task 002's
// "label -> real path" bookkeeping, namedDirectory/registerNamedDirectory,
// already populated the same way by requirement 12's steps) so a later step
// can type its real, absolute path into the cwd field.
func scratchDirectoryLabelledExists(ctx context.Context, label string) error {
	h, err := scenarioHarness(ctx)
	if err != nil {
		return err
	}
	dir := filepath.Join(h.Home, "cwd-ghost-"+label)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create scratch directory labelled %q: %w", label, err)
	}
	registerNamedDirectory(h, label, dir)
	return nil
}

func directoryExistsInScratch(ctx context.Context, name, label string) error {
	h, err := scenarioHarness(ctx)
	if err != nil {
		return err
	}
	dir, err := namedDirectory(h, label)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(dir, name), 0o700); err != nil {
		return fmt.Errorf("create directory %q in scratch directory labelled %q: %w", name, label, err)
	}
	return nil
}

func fileExistsInScratch(ctx context.Context, name, label string) error {
	h, err := scenarioHarness(ctx)
	if err != nil {
		return err
	}
	dir, err := namedDirectory(h, label)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte("not a directory\n"), 0o600); err != nil {
		return fmt.Errorf("create file %q in scratch directory labelled %q: %w", name, label, err)
	}
	return nil
}

// startNamedClientWithColourAndScenarioHome combines
// startNamedClientWithColour's colour override with
// startNamedClientWithScenarioHome's HOME override (both
// features/cell_attributes_test.go and features/create_tilde_test.go), for
// the leading-`~` ghost scenario, which needs both at once: colour to
// assert the dimmed token per-cell, and HOME so "~/..." resolves under a
// directory this scenario controls and tears down.
func startNamedClientWithColourAndScenarioHome(ctx context.Context, name string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	client, err := h.StartNamedClient(ctx, name, "NO_COLOR=", "HOME="+h.Home)
	if err != nil {
		return err
	}
	return client.WaitForFrame(ctx, false, "deck - sessions")
}

// directoryExistsUnderScenarioHome backs the leading-`~` scenario: the
// directory is created directly under the scenario's DECK_HOME, which
// startNamedClientWithScenarioHome (features/create_tilde_test.go) points a
// client's HOME at, so "~/<name>" resolves to it for scanning without this
// step needing to know anything about tilde expansion itself.
func directoryExistsUnderScenarioHome(ctx context.Context, name string) error {
	h, err := scenarioHarness(ctx)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(h.Home, name), 0o700); err != nil {
		return fmt.Errorf("create directory %q under scenario home: %w", name, err)
	}
	return nil
}

// clientTypesScratchDirPlusSegment sends the scratch directory's real,
// absolute path followed by "/" and segment as one keystroke burst -- the
// caller must already have tabbed focus to the cwd field
// (clientTabsToCWDField). segment may be "" (types just the trailing "/",
// e.g. to prove a hidden directory is excluded before a "." is typed).
func clientTypesScratchDirPlusSegment(ctx context.Context, name, label, segment string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	client, err := h.Client(name)
	if err != nil {
		return err
	}
	dir, err := namedDirectory(h, label)
	if err != nil {
		return err
	}
	if err := client.Send(dir + "/" + segment); err != nil {
		return err
	}
	time.Sleep(60 * time.Millisecond)
	return nil
}

// clientPressesKeyInCWDField sends the real terminal escape sequence for
// the ghost-completion acceptance keys task 010 declares: right (\x1b[C)
// and end (\x1b[F, the xterm/lxterm form key.go pins DECK_UNDO_MS-era
// bubbletea to for KeyEnd). The caller must already have tabbed focus to
// the cwd field.
func clientPressesKeyInCWDField(ctx context.Context, name, key string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	client, err := h.Client(name)
	if err != nil {
		return err
	}
	seq := "\x1b[C"
	if key == "end" {
		seq = "\x1b[F"
	}
	if err := client.Send(seq); err != nil {
		return err
	}
	time.Sleep(60 * time.Millisecond)
	return nil
}

// clientTypesSessionName types name while the create modal's Name field
// (field 0, where opening the modal leaves focus) is still focused --
// callers must call this BEFORE tabbing to the cwd field, exactly as
// createShellSessionInLabelledCWD (features/create_session_test.go) always
// types the name first for the same reason.
func clientTypesSessionName(ctx context.Context, clientName, name string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	client, err := h.Client(clientName)
	if err != nil {
		return err
	}
	if err := client.Send(name); err != nil {
		return err
	}
	time.Sleep(60 * time.Millisecond)
	return nil
}

// clientSubmitsCreateModal presses enter, which SPEC §11.4's shared dialog
// contract submits from whichever field currently has focus -- these
// scenarios always submit with the cwd field itself still focused, right
// after accepting or typing its ghost completion.
func clientSubmitsCreateModal(ctx context.Context, clientName string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	client, err := h.Client(clientName)
	if err != nil {
		return err
	}
	return client.Send("\r")
}

// sessionHasCWDExactlyScratchDirPlus asserts the sessions table's cwd
// column for name is byte-identical to the scratch directory labelled
// label joined with suffix (e.g. "/uniqueproj") -- proving right/end
// actually accepted the completion and it reached the store, not merely
// that something was rendered.
func sessionHasCWDExactlyScratchDirPlus(ctx context.Context, name, label, suffix string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	dir, err := namedDirectory(h, label)
	if err != nil {
		return err
	}
	want := dir + suffix
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
