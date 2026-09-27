package features

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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
