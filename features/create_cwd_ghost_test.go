package features

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
	sc.Step(`^deck client "([^"]+)" cwd field shows no ghost text$`, clientCWDFieldShowsNoGhostText)
	sc.Step(`^deck client "([^"]+)" cwd field shows ghost text$`, clientCWDFieldShowsGhostText)
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

// clientCWDFieldShowsNoGhostText backs task 011's negative proof (SPEC
// requirement 15): with several ambiguous directory matches, the field
// must ghost NOTHING at all -- not the alphabetically-first candidate, not
// any other arbitrary one. The ghost is the only text this package ever
// renders in the `dimmed` token ON THE CWD FIELD'S OWN ROWS, so scanning
// every cell of those rows for a single dimmed foreground -- rather than
// checking one candidate substring is absent -- also catches a ghost of
// some OTHER, unexpected text this scenario's author didn't think to name.
// This is a cell-grid assertion, not an absence-of-error check: it reads
// real Style.Fg values via CellAt, exactly as the per-cell steps in
// features/cell_attributes_test.go do.
//
// The scan is bounded to the field's own rows (its label/value row plus any
// physical continuation rows, ending before its one-line help text)
// because task 016 gave the modal its §11.6 tokens: SPEC.md:1355 puts every
// field's HELP line in `dimmed` too, so "no dimmed cell anywhere on screen"
// -- what this step asserted while the modal was uncoloured -- now says
// "the modal is unthemed", which is the opposite of what the spec asks for.
// The cwd rows are exactly where a ghost could ever appear, so bounding the
// scan there loses no ghost this step could previously have caught. Finding
// F27 (docs/reports/phase3g-findings.md) records why this narrowing, though
// correct, cannot also satisfy a PRD condition that the pre-016 whole-grid
// version stay byte-unchanged; clientCWDFieldShowsGhostText below is this
// narrowing's positive control, sharing cwdFieldRowBounds/cwdFieldDimmedCell
// so the two directions of the same check can never silently diverge.
func clientCWDFieldShowsNoGhostText(ctx context.Context, name string) error {
	client, err := assertionClient(ctx, name)
	if err != nil {
		return err
	}
	hint, err := resolveScenarioTokenHex(ctx, "hint")
	if err != nil {
		return err
	}
	found, row, col, content, first, last, err := cwdFieldDimmedCell(client, hint)
	if err != nil {
		return fmt.Errorf("client %q %w", name, err)
	}
	if found {
		return fmt.Errorf("client %q has a dimmed-token cell %q at row %d column %d (inside the cwd field's own rows %d-%d), want no ghost text there", name, content, row, col, first, last)
	}
	return nil
}

// cwdFieldRowBounds locates the create modal's cwd field's own rows on the
// client's current grid: its label/value row (found by "Working
// directory:") through any physical continuation rows, up to but not
// including its one-line help text (found by "the session's cwd"). This is
// the exact bound-finding logic clientCWDFieldShowsNoGhostText used inline
// before task 203, now shared with cwdFieldDimmedCell below.
func cwdFieldRowBounds(client *ScreenDriver) (first, last int, err error) {
	cols, rows := client.GridSize()
	rowText := func(y int) string {
		var b strings.Builder
		for x := 0; x < cols; x++ {
			if cell := client.CellAt(x, y); cell != nil {
				b.WriteString(cell.Content)
			}
		}
		return b.String()
	}
	first = -1
	for y := 0; y < rows; y++ {
		if strings.Contains(rowText(y), "Working directory:") {
			first = y
			break
		}
	}
	if first < 0 {
		return 0, 0, fmt.Errorf("shows no \"Working directory:\" row -- the create modal is not open, so this step cannot assert anything")
	}
	// The field's help line ("... the session's cwd; must exist ...",
	// possibly prefixed with "N matches — tab to list") ends the field's
	// own rows; every row before it belongs to the value.
	last = first
	for y := first + 1; y < rows; y++ {
		if strings.Contains(rowText(y), "the session's cwd") {
			break
		}
		last = y
	}
	return first, last, nil
}

// cwdFieldLabelEndCol locates, on row y, the column right after the
// field's own literal "Working directory: " label text (colon and its
// one trailing space) -- i.e. where the value/ghost portion of the row
// begins. It returns 0 (no skip) when the label text is not found on
// that row, which only happens for a wrapped continuation row that never
// carries the label. SPEC.md:1355 puts a field's label in the `hint`
// token unconditionally, the same token task 005 re-points the ghost
// itself onto (R95), so cwdFieldDimmedCell below must skip exactly the
// label's own columns on the label row -- otherwise every field, ghosted
// or not, would show a `hint` cell there and the negative proof
// (clientCWDFieldShowsNoGhostText) could never pass.
func cwdFieldLabelEndCol(client *ScreenDriver, y int) int {
	cols, _ := client.GridSize()
	var b strings.Builder
	for x := 0; x < cols; x++ {
		if cell := client.CellAt(x, y); cell != nil {
			b.WriteString(cell.Content)
		}
	}
	const label = "Working directory: "
	idx := strings.Index(b.String(), label)
	if idx < 0 {
		return 0
	}
	return idx + len(label)
}

// cwdFieldDimmedCell scans exactly the bounded rows cwdFieldRowBounds
// returns -- skipping the label row's own "Working directory: " columns
// (cwdFieldLabelEndCol) -- for the first cell whose foreground equals the
// hint token (already resolved to hex), reading real Style.Fg values via
// CellAt, exactly as the per-cell steps in features/cell_attributes_test.go
// do. found is false when no such cell exists in bounds. Shared by
// clientCWDFieldShowsNoGhostText (the negative proof) and
// clientCWDFieldShowsGhostText (task 203's positive control) so both
// directions of the same check use one scan.
func cwdFieldDimmedCell(client *ScreenDriver, dimmed string) (found bool, row, col int, content string, first, last int, err error) {
	first, last, err = cwdFieldRowBounds(client)
	if err != nil {
		return false, 0, 0, "", 0, 0, err
	}
	labelEnd := cwdFieldLabelEndCol(client, first)
	cols, _ := client.GridSize()
	for y := first; y <= last; y++ {
		startX := 0
		if y == first {
			startX = labelEnd
		}
		for x := startX; x < cols; x++ {
			cell := client.CellAt(x, y)
			if cell == nil || cell.Style.Fg == nil {
				continue
			}
			if colorHex(cell.Style.Fg) == dimmed {
				return true, y, x, cell.Content, first, last, nil
			}
		}
	}
	return false, 0, 0, "", first, last, nil
}

// clientCWDFieldShowsGhostText is task 203's positive control for the
// narrowing clientCWDFieldShowsNoGhostText applies above: it proves the
// bounded scan still DETECTS a ghost, sharing the identical helper
// (cwdFieldDimmedCell/cwdFieldRowBounds), so the negative proof is not
// vacuously true merely because the scan window shrank. It is wired into
// the unique-match scenario (create_cwd_ghost.feature), right where a
// ghost is expected on screen; a mutation run that disables ghost
// rendering entirely turns this step red -- see
// docs/reports/phase3g-203-r82-assertion-conflict/.
func clientCWDFieldShowsGhostText(ctx context.Context, name string) error {
	client, err := assertionClient(ctx, name)
	if err != nil {
		return err
	}
	hint, err := resolveScenarioTokenHex(ctx, "hint")
	if err != nil {
		return err
	}
	found, _, _, _, first, last, err := cwdFieldDimmedCell(client, hint)
	if err != nil {
		return fmt.Errorf("client %q %w", name, err)
	}
	if !found {
		return fmt.Errorf("client %q has no dimmed-token cell inside the cwd field's own rows %d-%d, want a ghost there", name, first, last)
	}
	return nil
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
