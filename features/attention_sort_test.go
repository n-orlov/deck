package features

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/cucumber/godog"
)

// registerAttentionSortSteps backs features/attention_sort.feature (task
// 026, requirements 28-32, 40): engineering deterministic status/workspace
// preconditions directly in state.db (the same established convention as
// features/agent_steps_test.go's launch-lease setup, since the sort itself
// is a pure function of stored rows -- there is no user action that
// produces "idle" or "error" quickly and deterministically), then asserting
// the released binary's own render order, grouping/collapse, the collapsed
// strip's attention count, and `space`'s non-vacuous, status-preserving walk.
func registerAttentionSortSteps(sc *godog.ScenarioContext) {
	sc.Step(`^the state database session "([^"]+)" has status "([^"]+)" ([0-9]+) seconds ago$`, setSessionStatusSecondsAgo)
	sc.Step(`^the state database session "([^"]+)" has workspace "([^"]+)"$`, setSessionWorkspace)
	sc.Step(`^deck client "([^"]+)" selects session "([^"]+)"$`, clientSelectsSessionByName)
	sc.Step(`^deck client "([^"]+)" has session "([^"]+)" selected$`, clientHasSessionSelected)
	sc.Step(`^deck client "([^"]+)" screen shows sessions in this order:$`, clientShowsSessionsInOrder)
	sc.Step(`^deck client "([^"]+)" collapsed strip shows attention count ([0-9]+)$`, clientCollapsedStripShowsAttentionCount)
	sc.Step(`^the state database status rows are captured as "([^"]+)"$`, captureStatusRowsAs)
	sc.Step(`^the state database status rows still match "([^"]+)"$`, statusRowsStillMatch)
}

// setSessionStatusSecondsAgo poses a session at an exact rank/tie-break
// position for the attention sort (requirements 28, 29) by writing status
// and status_at directly, mirroring the precedent already set by
// features/agent_steps_test.go's sessionChangesToNonLeasableError. This is
// safe against internal/service.reconcile's own promotion of a live shell
// pane (status "starting" -> "running"): every status this step is ever
// asked to write is not "starting" for a shell session (the harness's
// attention_sort.feature scenarios represent "starting" with a claude/pi
// session left untouched instead), so nothing reconciliation does
// afterwards can revert this write.
func setSessionStatusSecondsAgo(ctx context.Context, name, status string, secondsAgo int) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	db, err := openObservedDatabase(h)
	if err != nil {
		return err
	}
	defer db.Close()
	at := time.Now().Add(-time.Duration(secondsAgo) * time.Second).UnixMilli()
	result, err := db.ExecContext(ctx, `
		UPDATE sessions
		SET status = ?, status_reason = NULL, status_source = 'feature-test', status_at = ?
		WHERE name = ?`, status, at, name)
	if err != nil {
		return fmt.Errorf("set session %q status %q: %w", name, status, err)
	}
	if err := requireOneRowAffected(result, "set session %q status %q", name, status); err != nil {
		return err
	}
	return nil
}

// setSessionWorkspace poses SPEC requirement 30's grouping key directly,
// since the create modal (the only user-facing way to set a session's cwd)
// gives every session created within one scenario the same cwd and hence
// the same default workspace -- there is no other black-box way to put two
// sessions in different workspace groups within a single scenario.
func setSessionWorkspace(ctx context.Context, name, workspace string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	db, err := openObservedDatabase(h)
	if err != nil {
		return err
	}
	defer db.Close()
	result, err := db.ExecContext(ctx, `UPDATE sessions SET workspace = ? WHERE name = ?`, workspace, name)
	if err != nil {
		return fmt.Errorf("set session %q workspace %q: %w", name, workspace, err)
	}
	if err := requireOneRowAffected(result, "set session %q workspace %q", name, workspace); err != nil {
		return err
	}
	return nil
}

func requireOneRowAffected(result interface{ RowsAffected() (int64, error) }, format string, args ...any) error {
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf(format+": check rows affected: %w", append(args, err)...)
	}
	if affected != 1 {
		return fmt.Errorf(format+": %d rows affected, want 1", append(args, affected)...)
	}
	return nil
}

// clientSelectsSessionByName exposes the existing down-arrow marker search
// (features/agent_steps_test.go's selectRowByName, already relied on by
// task 027's launch-lease race) as its own Gherkin step, so a scenario can
// position the cursor on a named row without caring where a prior step left
// it -- needed here to put a specific workspace group's row under
// selection before task 039's `g` toggle, and to give `space`'s walk a
// known starting point.
func clientSelectsSessionByName(ctx context.Context, clientName, sessionName string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	client, err := h.Client(clientName)
	if err != nil {
		return err
	}
	return selectRowByName(client, sessionName)
}

// clientHasSessionSelected asserts the selected-row marker
// (selectRowByName's own "> "+name convention) sits on the named session,
// without moving the cursor -- `space`'s own step (requirements 31, 32)
// needs to observe where `space` landed, not search for it.
func clientHasSessionSelected(ctx context.Context, clientName, sessionName string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	client, err := h.Client(clientName)
	if err != nil {
		return err
	}
	marker := "> " + sessionName
	// The keypress that moved selection (e.g. `space`) has already been sent
	// by a prior step, but its render has not necessarily landed in the
	// emulator grid yet -- poll the same way clientScreenContainsBefore does,
	// rather than reading the frame exactly once and racing the render.
	wait, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := client.WaitForFrame(wait, false, marker); err != nil {
		return fmt.Errorf("deck client %q does not have session %q selected: %w", clientName, sessionName, err)
	}
	return nil
}

// clientShowsSessionsInOrder asserts the sidebar's rendered row order
// (requirements 28, 29, 30, 40): each named session must appear at a
// strictly later line than the one named before it, top to bottom,
// regardless of any group header lines interleaved between them.
func clientShowsSessionsInOrder(ctx context.Context, clientName string, table *godog.Table) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	client, err := h.Client(clientName)
	if err != nil {
		return err
	}
	// The status overrides that decide this order are direct SQL writes a
	// still-running reconcile tick can race (a pass already in flight can
	// re-derive a live shell's status from its pane before the next pass
	// re-reads the overridden row), so poll for the order to settle rather
	// than reading the grid exactly once, mirroring
	// clientCollapsedStripShowsAttentionCount's own retry loop.
	deadline := time.Now().Add(3 * time.Second)
	var lastErr error
	for {
		if err := sessionsRenderInOrder(client, table); err != nil {
			lastErr = err
			if time.Now().After(deadline) {
				return lastErr
			}
			time.Sleep(25 * time.Millisecond)
			continue
		}
		return nil
	}
}

func sessionsRenderInOrder(client *ScreenDriver, table *godog.Table) error {
	frame := client.Frame(false)
	lines := strings.Split(frame, "\n")
	prevIndex, prevName := -1, ""
	for _, row := range table.Rows {
		if len(row.Cells) != 1 {
			return fmt.Errorf("session-order row has %d cells, want 1", len(row.Cells))
		}
		name := row.Cells[0].Value
		index := -1
		for i, line := range lines {
			if strings.Contains(line, name) {
				index = i
				break
			}
		}
		if index == -1 {
			return fmt.Errorf("deck client has no rendered row for session %q:\n%s", name, frame)
		}
		if index <= prevIndex {
			return fmt.Errorf("deck client renders session %q at line %d, want strictly after session %q at line %d:\n%s",
				name, index, prevName, prevIndex, frame)
		}
		prevIndex, prevName = index, name
	}
	return nil
}

// clientCollapsedStripShowsAttentionCount reads SPEC requirement 15's 3-
// column collapsed strip directly from the emulator grid: the strip's sole
// content column sits at rune offset 2 of every content row (border,
// padding, content -- internal/tui/panel.go's sidebarContentLine), so the
// marker glyph (m.glyph("»", ">")) anchors the strip's first content row
// and every immediately following row whose offset-2 rune is a digit
// contributes one digit of Model.attentionCount's decimal rendering
// (internal/tui/tui.go's collapsedStripLines, task 015/025). This is the
// one place attention_sort.feature reads a specific screen cell rather than
// a substring, because the digit(s) alone are otherwise indistinguishable
// from incidental digits anywhere else in the frame.
func clientCollapsedStripShowsAttentionCount(ctx context.Context, clientName string, want int) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	client, err := h.Client(clientName)
	if err != nil {
		return err
	}
	// The mode-cycling keypresses that preceded this step are asynchronous
	// (like every other keystroke this suite sends), so poll for the strip
	// to settle rather than reading the grid exactly once and racing the
	// render, mirroring clientScreenContainsBefore's own retry loop.
	deadline := time.Now().Add(5 * time.Second)
	var lastErr error
	var lastFrame string
	for {
		frame := client.Frame(false)
		lastFrame = frame
		lines := strings.Split(frame, "\n")
		markerLine := -1
		for i, line := range lines {
			runes := []rune(line)
			if len(runes) > 2 && (runes[2] == '\u00bb' || runes[2] == '>') {
				markerLine = i
				break
			}
		}
		if markerLine == -1 {
			lastErr = fmt.Errorf("deck client %q shows no collapsed strip marker", clientName)
		} else {
			var digits strings.Builder
			for i := markerLine + 1; i < len(lines); i++ {
				runes := []rune(lines[i])
				if len(runes) <= 2 || runes[2] < '0' || runes[2] > '9' {
					break
				}
				digits.WriteRune(runes[2])
			}
			got, convErr := strconv.Atoi(digits.String())
			switch {
			case convErr != nil:
				lastErr = fmt.Errorf("deck client %q collapsed strip digits %q are not numeric", clientName, digits.String())
			case got != want:
				lastErr = fmt.Errorf("deck client %q collapsed strip attention count = %d, want %d", clientName, got, want)
			default:
				return nil
			}
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("%w:\n%s", lastErr, lastFrame)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(50 * time.Millisecond):
		}
	}
}

// statusColumnsSnapshot is the exact set of columns `space`
// (Model.nextAttentionSelection) must never write to: every column the
// attention sort, NeedsAttention or a user-facing status assertion reads.
func statusColumnsSnapshot(ctx context.Context, h *ScenarioHarness) (string, error) {
	db, err := openObservedDatabase(h)
	if err != nil {
		return "", err
	}
	defer db.Close()
	rows, err := db.QueryContext(ctx, `
		SELECT id, status, status_reason, status_source, status_at, acknowledged, notify_epoch
		FROM sessions ORDER BY id`)
	if err != nil {
		return "", fmt.Errorf("snapshot session status columns: %w", err)
	}
	defer rows.Close()
	var b strings.Builder
	for rows.Next() {
		var id, status, source string
		var reason *string
		var statusAt, epoch int64
		var acknowledged int
		if err := rows.Scan(&id, &status, &reason, &source, &statusAt, &acknowledged, &epoch); err != nil {
			return "", fmt.Errorf("scan session status columns: %w", err)
		}
		reasonText := ""
		if reason != nil {
			reasonText = *reason
		}
		fmt.Fprintf(&b, "%s|%s|%s|%s|%d|%d|%d\n", id, status, reasonText, source, statusAt, acknowledged, epoch)
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	return b.String(), nil
}

// captureStatusRowsAs snapshots every session's status-related columns
// under a label, so a later step can prove repeated `space` presses
// (requirements 31, 32) leave the sessions table byte-identical -- SPEC
// requirement 32's constraint that `space` "changes no status" and never
// triggers §7's attach-clears-waiting transition.
func captureStatusRowsAs(ctx context.Context, label string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	snapshot, err := statusColumnsSnapshot(ctx, h)
	if err != nil {
		return err
	}
	if h.statusRowSnapshots == nil {
		h.statusRowSnapshots = make(map[string]string)
	}
	h.statusRowSnapshots[label] = snapshot
	return nil
}

func statusRowsStillMatch(ctx context.Context, label string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	want, ok := h.statusRowSnapshots[label]
	if !ok {
		return fmt.Errorf("no state database status rows were captured as %q", label)
	}
	got, err := statusColumnsSnapshot(ctx, h)
	if err != nil {
		return err
	}
	if got != want {
		return fmt.Errorf("state database status rows changed since captured as %q:\nwant:\n%s\ngot:\n%s", label, want, got)
	}
	return nil
}
