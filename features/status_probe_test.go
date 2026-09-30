package features

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"

	"github.com/cucumber/godog"

	"github.com/n-orlov/deck/internal/racebuild"
	"github.com/n-orlov/deck/internal/theme"
)

const probeClock = "2025-01-02T03:04:05Z"

func registerProbeStatusSteps(sc *godog.ScenarioContext) {
	sc.Step(`^probe fixture agents and an advanceable frozen clock are configured$`, configureProbeScenario)
	sc.Step(`^fake agent session "([^"]+)" renders golden fixture "([^"]+)"$`, renderGoldenFixture)
	sc.Step(`^fake agent session "([^"]+)" renders these exact golden fixtures:$`, renderGoldenFixtures)
	sc.Step(`^deck client "([^"]+)" creates persistent shell session "([^"]+)"$`, createPersistentShell)
	sc.Step(`^the probe event count for session "([^"]+)" is ([0-9]+)$`, probeEventCount)
	sc.Step(`^the state database session "([^"]+)" has status "([^"]+)" from "([^"]+)"$`, databaseSessionStatusSource)
	sc.Step(`^the state database session "([^"]+)" has probe status "([^"]+)" with reason "([^"]+)"$`, databaseSessionProbeStatus)
	sc.Step(`^session "([^"]+)" has one losing "([^"]+)" event$`, sessionHasOneLosingProbeEvent)
	sc.Step(`^within one configured reconcile interval deck client "([^"]+)" row "([^"]+)" contains "([^"]+)"$`, clientRowContainsWithinReconcile)
	sc.Step(`^the frozen clock advances across stale_after while a fresh hook races the next probe of "([^"]+)" from "([^"]+)"$`, raceFreshHookAgainstProbe)
}

func configureProbeScenario(ctx context.Context) error {
	h, err := scenarioHarness(ctx)
	if err != nil {
		return err
	}
	if err := installFakeClaudeOnPATH(ctx, true); err != nil {
		return err
	}
	root, err := repositoryRoot()
	if err != nil {
		return err
	}
	pi := exec.CommandContext(ctx, "go", "build", "-o", filepath.Join(h.agentPATHDir, "pi"), "./cmd/fake-pi")
	pi.Dir = root
	if output, err := pi.CombinedOutput(); err != nil {
		return fmt.Errorf("build fake pi fixture: %w\n%s", err, output)
	}
	fixtureDir := filepath.Join(root, "internal", "agent", "testdata", "probes")
	config := fmt.Sprintf("stale_after = \"45s\"\n[env]\nFAKE_PI_COMMANDS = \"1\"\nFAKE_AGENT_FIXTURE_DIR = %q\n", fixtureDir)
	if err := os.WriteFile(filepath.Join(h.Home, "config.toml"), []byte(config), 0o600); err != nil {
		return fmt.Errorf("write probe config: %w", err)
	}

	// Only the released deck process sees this wrapper. Harness tmux commands
	// still use the real binary, which lets the race step observe and release a
	// capture at the precise precedence boundary without importing service code.
	realTMux, err := exec.LookPath("tmux")
	if err != nil {
		return err
	}
	arm := filepath.Join(h.Home, "probe-capture.arm")
	started := filepath.Join(h.Home, "probe-capture.started")
	release := filepath.Join(h.Home, "probe-capture.release")
	wrapper := fmt.Sprintf(`#!/bin/sh
arm=%q
started=%q
release=%q
capture=0
match=0
target=""
[ ! -f "$arm" ] || target=$(cat "$arm")
for arg in "$@"; do
  [ "$arg" != capture-pane ] || capture=1
  [ -z "$target" ] || [ "$arg" != "$target" ] || match=1
done
if [ "$capture" = 1 ] && [ "$match" = 1 ]; then
  : > "$started"
  i=0
  while [ ! -f "$release" ] && [ "$i" -lt 300 ]; do sleep 0.01; i=$((i+1)); done
fi
exec %q "$@"
`, arm, started, release, realTMux)
	if err := os.WriteFile(filepath.Join(h.agentPATHDir, "tmux"), []byte(wrapper), 0o700); err != nil {
		return fmt.Errorf("write probe capture wrapper: %w", err)
	}
	h.clientEnv = []string{"DECK_CLOCK=" + probeClock, "DECK_CLOCK_STEP=45s"}
	return nil
}

func renderGoldenFixtures(ctx context.Context, session string, table *godog.Table) error {
	for _, row := range table.Rows {
		if len(row.Cells) != 1 {
			return fmt.Errorf("fixture row has %d cells, want one", len(row.Cells))
		}
		if err := renderGoldenFixture(ctx, session, row.Cells[0].Value); err != nil {
			return err
		}
	}
	return nil
}

func renderGoldenFixture(ctx context.Context, session, fixture string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	slug, err := sessionSlugByName(h, session)
	if err != nil {
		return err
	}
	request, _ := json.Marshal(map[string]string{"command": "fixture", "name": fixture})
	target := "deck_" + slug
	if _, err := tmuxOutput(ctx, h, "send-keys", "-t", target, "-l", string(request)); err != nil {
		return err
	}
	if _, err := tmuxOutput(ctx, h, "send-keys", "-t", target, "Enter"); err != nil {
		return err
	}
	root, err := repositoryRoot()
	if err != nil {
		return err
	}
	golden, err := os.ReadFile(filepath.Join(root, "internal", "agent", "testdata", "probes", fixture))
	if err != nil {
		return err
	}
	want := strings.ReplaceAll(string(golden), "\r\n", "\n")
	deadline := time.Now().Add(3 * time.Second)
	for {
		captured, captureErr := tmuxOutput(ctx, h, "capture-pane", "-p", "-J", "-S", "-", "-t", target)
		if captureErr == nil && strings.Contains(strings.ReplaceAll(string(captured), "\r\n", "\n"), want) {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("pane %q did not render exact golden bytes for %s; capture error=%v\npane:\n%s", target, fixture, captureErr, captured)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func createPersistentShell(ctx context.Context, clientName, name string) error {
	_, client, err := positionCreateModalOnProfileField(ctx, clientName, "shell", name, "safe")
	if err != nil {
		return err
	}
	// The ordinary interactive /bin/sh fixture can legitimately consume EOF
	// under a heavily repeated PTY suite. Pin this eligibility row to an
	// explicit long-running shell command instead.
	if err := client.Send("\x1b[B[\"-c\",\"while :; do sleep 3600; done\"]\r"); err != nil {
		return err
	}
	return client.WaitForFrame(ctx, false, "starting")
}

func probeEventCount(ctx context.Context, session string, want int) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	db, err := openObservedDatabase(h)
	if err != nil {
		return err
	}
	defer db.Close()
	var got int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM events WHERE session_id = (SELECT id FROM sessions WHERE name = ?) AND kind LIKE 'probe.%'`, session).Scan(&got); err != nil {
		return err
	}
	if got != want {
		return fmt.Errorf("session %q probe event count = %d, want %d", session, got, want)
	}
	return nil
}

func databaseSessionStatusSource(ctx context.Context, name, status, source string) error {
	return waitForDatabaseVerdict(ctx, name, status, source, "", false)
}

func databaseSessionProbeStatus(ctx context.Context, name, status, reason string) error {
	return waitForDatabaseVerdict(ctx, name, status, "probe", reason, true)
}

func waitForDatabaseVerdict(ctx context.Context, name, wantStatus, wantSource, wantReason string, checkReason bool) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	db, err := openObservedDatabase(h)
	if err != nil {
		return err
	}
	defer db.Close()
	deadline := time.Now().Add(3 * time.Second)
	for {
		var status, source, reason string
		err := db.QueryRowContext(ctx, `SELECT status, status_source, COALESCE(status_reason, '') FROM sessions WHERE name = ?`, name).Scan(&status, &source, &reason)
		if err == nil && status == wantStatus && source == wantSource && (!checkReason || reason == wantReason) {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("session %q verdict = %q/%q reason %q, want %q/%q reason %q (err=%v)", name, status, source, reason, wantStatus, wantSource, wantReason, err)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func sessionHasOneLosingProbeEvent(ctx context.Context, name, kind string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	db, err := openObservedDatabase(h)
	if err != nil {
		return err
	}
	defer db.Close()
	deadline := time.Now().Add(3 * time.Second)
	for {
		var count int
		if err := db.QueryRowContext(ctx, `SELECT count(*) FROM events WHERE session_id = (SELECT id FROM sessions WHERE name = ?) AND kind = ?`, name, kind).Scan(&count); err != nil {
			return err
		}
		var source string
		if err := db.QueryRowContext(ctx, `SELECT status_source FROM sessions WHERE name = ?`, name).Scan(&source); err != nil {
			return err
		}
		if count == 1 && source == "hook" {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("session %q has %d %q events and source %q, want one losing-probe evidence event with durable hook source", name, count, kind, source)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// frameSidebarRowContains reports whether the named session's OWN sidebar
// row in frame (a plain-text client frame) shows want as one of its own
// status badges -- the status word itself, its hook/probe quality word
// ("live"/"sampled"), its unseen glyph ("●"/"!") or its archived badge.
// It is frameSessionRowShows with no other session names known; see that
// function for the exact matching rule.
func frameSidebarRowContains(frame, rowName, want string) bool {
	return frameSessionRowShows(frame, rowName, want, nil)
}

// frameSessionRowShows is the one matcher every registered named status
// callback (clientRowContainsWithinReconcile,
// clientRowContainsWithinThreeSeconds,
// clientRowContainsAcrossSeveralProbeCycles) settles on. A named wait must
// verify the NAMED SESSION's own status, never any other text that happens
// to share the frame (R150, cure-01-01-2/3), so it matches no substring at
// all:
//
//   - Only the sidebar panel's own cell of each terminal line is read (the
//     text between the line's first and second vertical border glyph), never
//     the whole line: the preview panel shares every terminal line with the
//     sidebar, and a session's pane output printing "running" there is not
//     its status (cure-01-01-2).
//   - A group header cell is never a session row (sidebarCellIsGroupHeader):
//     group "alpha-running" says nothing about session alpha
//     (cure-01-01-3).
//   - A session row's first line is exactly the rendering sidebarRowLines
//     (internal/tui/tui.go) produces: the selection gutter ("> " or two
//     spaces), the session's name, one space, then the badge run
//     `[unseen] [quality] <status> [archived]`. The cell must START with
//     rowName followed by a space -- so "alpha-helper running" and
//     "running-alpha starting" are simply not row "alpha" -- and the rest
//     of the cell must parse as exactly that badge run (sidebarRowBadges),
//     so a name's own words are never read as a status: "running-alpha
//     starting" shows "starting" for row "running-alpha", nothing else.
//   - Session names may contain spaces, so a cell reading "alpha sampled
//     running" could be session "alpha" (quality sampled, status running)
//     or session "alpha sampled" (status running) -- the frame alone cannot
//     tell them apart. knownNames (the store's own session names) settles
//     it: the LONGEST known name the cell's text parses under owns the
//     cell, and the cell counts for rowName only when rowName is that
//     owner.
func frameSessionRowShows(frame, rowName, want string, knownNames []string) bool {
	if rowName == "" || want == "" {
		return false
	}
	for _, line := range strings.Split(frame, "\n") {
		cell, ok := sidebarCell(line)
		if !ok || sidebarCellIsGroupHeader(cell) {
			continue
		}
		text := strings.TrimLeft(cell, " ")
		text = stripSidebarRowLead(strings.TrimPrefix(text, "> "))
		badges, ok := sidebarRowBadges(text, rowName)
		if !ok {
			continue
		}
		owned := true
		for _, other := range knownNames {
			if len(other) > len(rowName) && other != rowName {
				if _, parses := sidebarRowBadges(text, other); parses {
					owned = false
					break
				}
			}
		}
		if !owned {
			continue
		}
		for _, b := range badges {
			if b == want {
				return true
			}
		}
	}
	return false
}

// sidebarRowLeadGlyphs is every status glyph sidebarRowLines
// (internal/tui/tui.go, sidebarStatusGlyph) leads a session row's line 1
// with, in both glyph modes: SPEC §11's `●◐○◌■✗` and their DECK_ASCII
// fallbacks `?~o.#x`.
var sidebarRowLeadGlyphs = []string{"\u25cf", "\u25d0", "\u25cb", "\u25cc", "\u25a0", "\u2717", "?", "~", "o", ".", "#", "x"}

// stripSidebarRowLead removes a session row's line-1 lead -- SPEC §11's
// fixed order is gutter, status glyph, pin marker (`✦`, ASCII `*`, pinned
// rows only), name -- from text whose gutter is already gone, leaving the
// row's own "<name> <badge run>". Text without a leading status glyph is
// returned unchanged (line 2, a group header, or a synthetic test frame).
func stripSidebarRowLead(text string) string {
	for _, g := range sidebarRowLeadGlyphs {
		if rest, ok := strings.CutPrefix(text, g+" "); ok {
			for _, pin := range []string{"\u2726 ", "* "} {
				if r, ok := strings.CutPrefix(rest, pin); ok {
					return r
				}
			}
			return rest
		}
	}
	return text
}

// frameHasSelectedRowNamed reports whether frame's sidebar holds a
// selected ("> "-gutter) session row whose name starts with name, once
// its status glyph and pin marker (stripSidebarRowLead) are skipped --
// the row-lead-aware replacement for a bare "> "+name substring search.
func frameHasSelectedRowNamed(frame, name string) bool {
	if name == "" {
		return false
	}
	for _, line := range strings.Split(frame, "\n") {
		cell, ok := sidebarCell(line)
		if !ok {
			continue
		}
		rest, selected := strings.CutPrefix(strings.TrimLeft(cell, " "), "> ")
		if selected && strings.HasPrefix(stripSidebarRowLead(rest), name) {
			return true
		}
	}
	return false
}

// sidebarRowBadges parses text (a sidebar cell with its leading pad and
// selection gutter already removed) as session name's own first row line,
// "<name> <badge run>", and returns the badge run's COMPLETE fields. It
// fails unless text starts with name plus a space and the remainder is
// exactly sidebarRowLines' badge run: an optional unseen glyph ("●" or
// "!"), an optional quality word ("live"/"sampled", statusSourceQuality),
// exactly one status word (one of theme.StatusTokens, SPEC §7's seven),
// and an optional archived badge ("▣" or "[archived]"), in that order and
// nothing else.
//
// A row too wide for the sidebar is ellipsised at its end (elideToWidth's
// "…"/"..." marker, internal/tui/tui.go): the run then stops at a last
// field ending in that marker, whose visible prefix must still be a prefix
// of a token allowed at that position. Only the fields BEFORE it count as
// badges -- so "codex-purge-remove live run..." shows its own quality badge
// "live" but no readable status, and a status wait on it keeps waiting.
func sidebarRowBadges(text, name string) ([]string, bool) {
	if !strings.HasPrefix(text, name+" ") {
		return nil, false
	}
	fields := strings.Fields(text[len(name)+1:])
	statuses := make([]string, 0, len(theme.StatusTokens))
	for _, st := range theme.StatusTokens {
		statuses = append(statuses, string(st))
	}
	positions := []struct {
		allowed  []string
		required bool
	}{
		{[]string{"\u25cf", "!"}, false},
		{[]string{"live", "sampled"}, false},
		{statuses, true},
		{[]string{"\u25a3", "[archived]"}, false},
	}
	truncatedPrefix := func(field string) (string, bool) {
		for _, marker := range []string{"\u2026", "..."} {
			if strings.HasSuffix(field, marker) {
				return strings.TrimSuffix(field, marker), true
			}
		}
		return "", false
	}
	i := 0
	for p, pos := range positions {
		if i == len(fields) {
			if pos.required {
				return nil, false
			}
			continue
		}
		if prefix, cut := truncatedPrefix(fields[i]); cut && i == len(fields)-1 {
			// The ellipsis ends the visible run: its prefix must fit some
			// token allowed from this position on.
			for _, later := range positions[p:] {
				for _, a := range later.allowed {
					if strings.HasPrefix(a, prefix) {
						return fields[:i], true
					}
				}
			}
			return nil, false
		}
		matched := false
		for _, a := range pos.allowed {
			if fields[i] == a {
				matched = true
				break
			}
		}
		if matched {
			i++
		} else if pos.required {
			return nil, false
		}
	}
	if i != len(fields) {
		return nil, false
	}
	return fields, true
}

// waitForClientSessionRow polls client clientName's frame until
// session rowName's own sidebar row shows want (frameSessionRowShows,
// disambiguated against every session name the state database holds at
// each poll) or window elapses; the error names within, the caller's own
// description of that window.
func waitForClientSessionRow(ctx context.Context, clientName, rowName, want string, window time.Duration, within string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	client, err := h.Client(clientName)
	if err != nil {
		return err
	}
	db, err := openObservedDatabase(h)
	if err != nil {
		return err
	}
	defer db.Close()
	deadline := time.Now().Add(window)
	for {
		names, nameErr := storeSessionNames(ctx, db)
		if nameErr == nil && frameSessionRowShows(client.Frame(false), rowName, want, names) {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("client %q row %q did not contain %q %s (session names err=%v)\nframe:\n%s", clientName, rowName, want, within, nameErr, client.Frame(false))
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// storeSessionNames returns every session name the state database holds.
func storeSessionNames(ctx context.Context, db *sql.DB) ([]string, error) {
	rows, err := db.QueryContext(ctx, `SELECT name FROM sessions`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		names = append(names, name)
	}
	return names, rows.Err()
}

// sidebarGroupHeaderSuffixRe matches groupHeaderText's own trailing member
// count (internal/tui/group.go: "<chevron> <name>  (<n>)", SPEC §11's
// "every header carries its member count, including (0)") -- a
// whitespace-then-parenthesised-integer tail that no SESSION ROW's own
// rendered text (sidebarRowLines) ever produces: badges use brackets
// (profileBadgeSegment's "[profile]"), never parens, and neither the
// status word nor the relative age ever ends in "(<n>)". One or two
// spaces before the paren both match (groupHeaderText renders two; a
// synthetic test frame may render one), since the count itself, not its
// exact spacing, is what makes a line a header.
var sidebarGroupHeaderSuffixRe = regexp.MustCompile(`\s\(\d+\)\s*$`)

// sidebarCellIsGroupHeader reports whether cell (as extracted by
// sidebarCell) renders a group header line rather than a session row --
// see sidebarGroupHeaderSuffixRe's own doc for the discriminator this
// relies on.
func sidebarCellIsGroupHeader(cell string) bool {
	return sidebarGroupHeaderSuffixRe.MatchString(cell)
}

// sidebarCell returns the text between line's first and second vertical
// border glyph, and false when the line has fewer than two.
func sidebarCell(line string) (string, bool) {
	isBorder := func(r rune) bool { return r == '|' || r == '│' }
	start := strings.IndexFunc(line, isBorder)
	if start < 0 {
		return "", false
	}
	_, w := utf8.DecodeRuneInString(line[start:])
	rest := line[start+w:]
	end := strings.IndexFunc(rest, isBorder)
	if end < 0 {
		return "", false
	}
	return rest[:end], true
}

func clientRowContainsWithinReconcile(ctx context.Context, clientName, rowName, want string) error {
	return waitForClientSessionRow(ctx, clientName, rowName, want, reconcileIntervalPollDeadline(scenarioReconcileInterval, racebuild.Enabled), "within reconcile interval")
}

func raceFreshHookAgainstProbe(ctx context.Context, victim, emitter string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	victimSlug, err := sessionSlugByName(h, victim)
	if err != nil {
		return err
	}
	paneIDRaw, err := tmuxOutput(ctx, h, "display-message", "-p", "-t", "deck_"+victimSlug, "#{pane_id}")
	if err != nil {
		return err
	}
	client, err := h.Client("A")
	if err != nil {
		return err
	}
	// The live preview (tasks 017-021) captures whichever row is currently
	// selected using the exact same capture-pane-by-pane-ID technique the
	// probe capture below uses, and the arm/release wrapper below cannot
	// distinguish the two callers -- it just pauses the next capture-pane
	// invocation naming this pane. If the victim's own row stayed selected
	// (its natural position after creation), the preview's own frequent
	// ticks -- not the reconciler's stale-probe capture this step means to
	// intercept -- would satisfy the "started" wait below prematurely, and
	// the reconciler's own probe might never run before the hook resolves
	// the race, leaving zero losing-probe evidence. Move selection onto the
	// durable "probe shell" row first so the preview targets a pane the
	// wrapper never arms, and the arm/release handshake below can only ever
	// observe the reconciler's own probe-eligible capture of the victim.
	// selectRowByName (features/navigation_settle_test.go) resets to the
	// top itself before walking down, so no separate rewind is needed here.
	if err := selectRowByName(ctx, client, "probe shell"); err != nil {
		return fmt.Errorf("move selection off probe victim %q before arming: %w", victim, err)
	}
	arm := filepath.Join(h.Home, "probe-capture.arm")
	started := filepath.Join(h.Home, "probe-capture.started")
	release := filepath.Join(h.Home, "probe-capture.release")
	_ = os.Remove(started)
	_ = os.Remove(release)
	if err := os.WriteFile(arm, []byte(strings.TrimSpace(string(paneIDRaw))), 0o600); err != nil {
		return err
	}
	// Reach stale_after through the released client's on-demand increment. This
	// deliberately does not calculate or write an absolute clock.now value.
	if err := client.cmd.Process.Signal(syscall.SIGUSR1); err != nil {
		return fmt.Errorf("signal frozen probe clock step: %w", err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		if _, err := os.Stat(started); err == nil {
			break
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("released client did not begin targeted probe capture for %q", victim)
		}
		time.Sleep(10 * time.Millisecond)
	}

	conversation, err := sessionConversationID(h, victim)
	if err != nil {
		return err
	}
	request, _ := json.Marshal(map[string]any{"command": "hook", "event": "SessionStart", "payload": map[string]string{"session_id": conversation, "source": "fresh"}})
	emitterSlug, err := sessionSlugByName(h, emitter)
	if err != nil {
		return err
	}
	if _, err := tmuxOutput(ctx, h, "send-keys", "-t", "deck_"+emitterSlug, "-l", string(request)); err != nil {
		return err
	}
	if _, err := tmuxOutput(ctx, h, "send-keys", "-t", "deck_"+emitterSlug, "Enter"); err != nil {
		return err
	}
	if err := waitForDatabaseVerdict(ctx, victim, "running", "hook", "", false); err != nil {
		return err
	}
	if err := os.WriteFile(release, []byte("release\n"), 0o600); err != nil {
		return err
	}
	return nil
}
