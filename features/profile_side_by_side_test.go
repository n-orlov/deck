package features

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cucumber/godog"
)

// registerProfileSideBySideSteps wires SPEC §3.4's R156 scenario: two named
// profiles, each with a session of the same name, running at the same time
// on their OWN derived tmux socket ("deck-<name>", not a private
// per-scenario name like every other scenario in this package uses) and
// their own state.db. Unlike registerProfileHookIsolationSteps (task 014,
// R154), which deliberately overrides DECK_TMUX_SOCKET to a private name to
// keep the isolation proof about DECK_PROFILE alone, this scenario's own
// success criteria name the literal derived sockets ("deck-<a>"/"deck-<b>")
// as the thing being queried -- so DECK_TMUX_SOCKET is cleared instead of
// overridden, exactly like StartNamedClientForNewProfile (task 011) already
// does, letting config.go's own "deck-"+profile derivation run for real.
// Both sockets are ephemeral, sibling-container-local tmux servers (see
// ci/run.sh: only the workspace and the go-cache volume are bind-mounted;
// /tmp is fresh per `docker run --rm`), never the operator's own machine, so
// this never touches anything the standing rules protect.
func registerProfileSideBySideSteps(sc *godog.ScenarioContext) {
	sc.Step(`^deck client "([^"]+)" is started for the existing profile "([^"]+)" on its own derived socket$`, startClientForExistingProfileOnDerivedSocket)
	sc.Step(`^the profile "([^"]+)" session "([^"]+)" is a live tmux session on socket "([^"]+)"$`, profileSessionIsLiveTMuxSessionOnSocket)
	sc.Step(`^deck client "([^"]+)" sidebar lists exactly one session row, named "([^"]+)"$`, clientSidebarListsExactlyOneSessionRowNamed)
	sc.Step(`^the profile "([^"]+)" state database holds exactly one session row, named "([^"]+)"$`, profileStateDatabaseHoldsExactlyOneSessionRowNamed)
}

// startClientForExistingProfileOnDerivedSocket pre-creates profile's own
// data directory directly (as startClientForExistingProfile does, for the
// same reason: the directory's existence is what avoids the typo-guard
// prompt this scenario is not testing), then starts a named client with
// DECK_PROFILE=profile and DECK_TMUX_SOCKET explicitly cleared -- the one
// difference from every "existing profile" starter elsewhere in this
// package -- so config.go falls through to its own "deck-"+profile
// derivation instead of the harness's usual private per-scenario socket.
// The derived socket is registered in h.extraSockets so Close kills and
// probes it exactly like every other socket this harness owns.
//
// DECK_ASCII is also explicitly cleared to "0" here: the harness default
// (DECK_ASCII=1, set for every other scenario in this package) renders the
// header's separator as the ascii fallback " - " (m.glyph's own asciiKey),
// while this scenario's own success criteria name the literal unicode
// glyph "profile: <a> \u00b7 socket: deck-<a>" -- so this scenario, alone,
// needs the non-ascii render to make that string appear on screen at all.
func startClientForExistingProfileOnDerivedSocket(ctx context.Context, name, profile string) error {
	h, err := scenarioHarness(ctx)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(h.Home, "profiles", profile), 0o700); err != nil {
		return fmt.Errorf("pre-create profile %q directory: %w", profile, err)
	}
	socket := "deck-" + profile
	h.extraSockets = append(h.extraSockets, socket)
	client, err := h.StartNamedClient(ctx, name, "DECK_PROFILE="+profile, "DECK_TMUX_SOCKET=", "DECK_ASCII=0")
	if err != nil {
		return err
	}
	// The title separator is itself one of m.glyph's own pairs (tui.go's
	// windowTitle: `"deck" + m.glyph(" — ", " - ") + "sessions"`), so with
	// DECK_ASCII=0 the literal ASCII "deck - sessions" every other starter in
	// this package waits for never appears; the em dash render does.
	return client.WaitForFrame(ctx, false, "deck — sessions")
}

// profileSessionIsLiveTMuxSessionOnSocket asserts profile's session name is
// a real, live session on socket, queried directly with `tmux -L socket
// has-session` -- never through profile A's own derived socket for profile
// B, so the Gherkin's own literal socket names ("deck-a"/"deck-b") are what
// gets checked, not merely the socket the harness happened to compute. The
// tmux session's target name is profile's own state.db slug for name
// (openProfileDatabase/profileSessionRow, shared with
// registerProfileHookIsolationSteps), never assumed to equal name itself.
func profileSessionIsLiveTMuxSessionOnSocket(ctx context.Context, profile, name, socket string) error {
	h, err := scenarioHarness(ctx)
	if err != nil {
		return err
	}
	if want := "deck-" + profile; socket != want {
		return fmt.Errorf("scenario wiring error: socket %q named for profile %q, want %q", socket, profile, want)
	}
	_, slug, _, _, _, err := profileSessionRow(ctx, h, profile, name)
	if err != nil {
		return err
	}
	target := "deck_" + slug
	commandCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	output, err := exec.CommandContext(commandCtx, "tmux", "-L", socket, "has-session", "-t", target).CombinedOutput()
	if err != nil {
		return fmt.Errorf("tmux -L %s has-session -t %s: %w: %s", socket, target, err, strings.TrimSpace(string(output)))
	}
	return nil
}

// profileStateDatabaseHoldsExactlyOneSessionRowNamed asserts profile's own
// state.db has exactly one row in sessions, AND that row is the one named
// name -- together, this scenario's own "each state.db holds only its own
// row" evidence: a leak that put the OTHER profile's row in this database
// (or that put this profile's row somewhere else entirely) would fail
// either half.
func profileStateDatabaseHoldsExactlyOneSessionRowNamed(ctx context.Context, profile, name string) error {
	h, err := scenarioHarness(ctx)
	if err != nil {
		return err
	}
	db, err := openProfileDatabase(ctx, h, profile)
	if err != nil {
		return err
	}
	defer db.Close()
	var count int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM sessions`).Scan(&count); err != nil {
		return fmt.Errorf("count profile %q sessions: %w", profile, err)
	}
	if count != 1 {
		return fmt.Errorf("profile %q state database has %d session rows, want exactly 1", profile, count)
	}
	if _, _, _, _, _, err := profileSessionRow(ctx, h, profile, name); err != nil {
		return fmt.Errorf("profile %q state database's one row is not named %q: %w", profile, name, err)
	}
	return nil
}

// clientSidebarListsExactlyOneSessionRowNamed is this scenario's own
// "each deck lists only its own session" screen evidence. A bare "screen
// contains <name>" cannot detect a leak here: both profiles deliberately
// share one session name, so a deck that also listed the OTHER profile's
// row would still contain the name. So this step first waits out one full
// reconcile interval (every pass that could pick up a foreign row has run
// and rendered), then reads ONLY the sidebar column of the emulator grid --
// the text between a content row's left border and the sidebar/preview
// divider, so nothing the preview pane echoes can count -- and requires:
//   - exactly one sidebar row whose text (after the "> " selection marker)
//     is the session name followed by its status, and
//   - the group headers' own "(N)" member counts sum to exactly 1, so a
//     foreign row filed under some other group (or any extra session row
//     at all, whatever its name) fails too.
func clientSidebarListsExactlyOneSessionRowNamed(ctx context.Context, clientName, name string) error {
	timer := time.NewTimer(scenarioReconcileInterval + 100*time.Millisecond)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
	}
	h, err := scenarioHarness(ctx)
	if err != nil {
		return err
	}
	client, err := h.Client(clientName)
	if err != nil {
		return err
	}
	frame := client.Frame(false)
	if err := sidebarListsExactlyOneSessionRowNamed(frame, name); err != nil {
		return fmt.Errorf("deck client %q: %w:\n%s", clientName, err, frame)
	}
	return nil
}

// sidebarListsExactlyOneSessionRowNamed is the step's whole verdict on one
// frame: exactly one sidebar row names name, and the group headers count
// exactly one session in total.
func sidebarListsExactlyOneSessionRowNamed(frame, name string) error {
	rows, groupTotal, headers, err := sidebarSessionRowCounts(frame, name)
	if err != nil {
		return err
	}
	if rows != 1 {
		return fmt.Errorf("sidebar lists %d rows named %q, want exactly 1", rows, name)
	}
	if headers == 0 || groupTotal != 1 {
		return fmt.Errorf("sidebar group headers (%d of them) count %d sessions in total, want exactly 1", headers, groupTotal)
	}
	return nil
}

// sidebarSessionRowCounts parses frame's sidebar column (the text between
// each content row's left border and the sidebar/preview divider) and
// returns how many rows name session name, the sum of every group header's
// "(N)" member count, and how many group headers there were.
func sidebarSessionRowCounts(frame, name string) (rows, groupTotal, headers int, err error) {
	for _, line := range strings.Split(frame, "\n") {
		cells := strings.Split(line, "\u2502") // "│": left border, divider, right border
		if len(cells) < 3 {
			continue
		}
		text := strings.TrimSpace(cells[1])
		header, expanded := strings.CutPrefix(text, "\u25be ")           // "▾ "
		collapsedHeader, collapsed := strings.CutPrefix(text, "\u25b8 ") // "▸ "
		if expanded || collapsed {
			if collapsed {
				header = collapsedHeader
			}
			open := strings.LastIndex(header, "(")
			if open < 0 || !strings.HasSuffix(header, ")") {
				return 0, 0, 0, fmt.Errorf("group header %q has no (N) member count", text)
			}
			var n int
			if _, err := fmt.Sscanf(header[open:], "(%d)", &n); err != nil {
				return 0, 0, 0, fmt.Errorf("group header %q: parse member count: %w", text, err)
			}
			headers++
			groupTotal += n
			continue
		}
		entry := stripSidebarRowLead(strings.TrimPrefix(text, "> "))
		if entry == name || strings.HasPrefix(entry, name+" ") {
			rows++
		}
	}
	return rows, groupTotal, headers, nil
}

// TestSidebarSessionRowCountsDetectsAForeignRow pins that R156's sidebar
// step actually fails when a deck lists the OTHER profile's same-named
// session: the base frame is the sidebar a real profile-"a" client renders
// in profile_side_by_side.feature, and each leaked variant adds one foreign
// "shared" row (same group, another group) that a bare "screen contains"
// check could never tell apart. Preview-pane text naming the session must
// not count as a row either.
func TestSidebarSessionRowCountsDetectsAForeignRow(t *testing.T) {
	const (
		top    = "╭ deck — sessions ─────────────────┬──────────────────────╮"
		header = "│ profile: a · socket: deck-a      │ $                    │"
		group  = "│ ▾ default  (%d)                   │                      │"
		row    = "│ %s shared running                 │                      │"
		when   = "│   2s ago                         │                      │"
		blank  = "│                                  │                      │"
		bottom = "╰──────────────────────────────────┴──────────────────────╯"
	)
	join := func(lines ...string) string { return strings.Join(lines, "\n") }
	cases := []struct {
		name                string
		frame               string
		wantRows, wantTotal int
	}{
		{"own row only", join(top, header, fmt.Sprintf(group, 1), fmt.Sprintf(row, ">"), when, blank, bottom), 1, 1},
		{"foreign row in the same group", join(top, header, fmt.Sprintf(group, 2), fmt.Sprintf(row, ">"), when, fmt.Sprintf(row, " "), when, bottom), 2, 2},
		{"foreign row in another group", join(top, header, fmt.Sprintf(group, 1), fmt.Sprintf(row, ">"), when,
			"│ ▸ other  (1)                     │                      │", bottom), 1, 2},
		{"preview names the session", join(top, header, fmt.Sprintf(group, 1), fmt.Sprintf(row, ">"), when,
			"│                                  │ shared running       │", bottom), 1, 1},
	}
	for _, tc := range cases {
		rows, total, headers, err := sidebarSessionRowCounts(tc.frame, "shared")
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if rows != tc.wantRows || total != tc.wantTotal || headers == 0 {
			t.Fatalf("%s: rows=%d total=%d headers=%d, want rows=%d total=%d headers>0", tc.name, rows, total, headers, tc.wantRows, tc.wantTotal)
		}
		err = sidebarListsExactlyOneSessionRowNamed(tc.frame, "shared")
		if wantPass := tc.wantRows == 1 && tc.wantTotal == 1; (err == nil) != wantPass {
			t.Fatalf("%s: step verdict err=%v, want pass=%v", tc.name, err, wantPass)
		}
	}
}
