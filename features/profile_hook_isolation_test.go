package features

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/cucumber/godog"
)

// registerProfileHookIsolationSteps wires SPEC §3.4's "every pane carries
// DECK_PROFILE ... so a pane writes to the database of the profile that
// created it for its whole life" and "a profile never reads another
// profile's state.db" together (task 014, R154): two profiles' decks run at
// once in one scenario's DECK_HOME, a hook fires against a session that
// lives in profile A's own database, and only A's database (never B's) is
// asserted to have moved.
func registerProfileHookIsolationSteps(sc *godog.ScenarioContext) {
	sc.Step(`^deck client "([^"]+)" is started for the existing profile "([^"]+)"$`, startClientForExistingProfile)
	sc.Step(`^an uncontended Claude hook target "([^"]+)" exists in profile "([^"]+)"'s state database$`, uncontendedClaudeHookTargetInProfile)
	sc.Step(`^the profile "([^"]+)" state database is snapshotted as "([^"]+)"$`, snapshotProfileDatabase)
	sc.Step(`^the released hook receiver handles a "([^"]+)" event for "([^"]+)" in profile "([^"]+)"$`, releasedHookHandlesEventInProfile)
	sc.Step(`^the profile "([^"]+)" state database session "([^"]+)" is stopped by session end$`, profileSessionStoppedBySessionEnd)
	sc.Step(`^the profile "([^"]+)" state database still matches snapshot "([^"]+)" in content and session row count$`, profileDatabaseMatchesSnapshot)
}

// profileStateDBPath is SPEC §3.4's DECK_HOME-mode named-profile layout:
// profiles/<name>/state.db nested under the scenario's own data root.
func profileStateDBPath(h *ScenarioHarness, profile string) string {
	return filepath.Join(h.Home, "profiles", profile, "state.db")
}

// openProfileDatabase opens profile's own state.db with the same
// busy-policy every other black-box observer in this package uses
// (openObservedDatabase), so an external read never trips over a real
// released client's own concurrent WAL writer.
func openProfileDatabase(ctx context.Context, h *ScenarioHarness, profile string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", profileStateDBPath(h, profile))
	if err != nil {
		return nil, fmt.Errorf("open profile %q state database: %w", profile, err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	if _, err := db.ExecContext(ctx, `PRAGMA busy_timeout=5000`); err != nil {
		db.Close()
		return nil, fmt.Errorf("configure profile %q state database: %w", profile, err)
	}
	return db, nil
}

// startClientForExistingProfile pre-creates profile's own data directory
// directly (SPEC §3.4: "whether a profile exists is a directory scan" --
// there is no registry to satisfy, only the directory itself), then starts
// a named client with DECK_PROFILE=profile. Because the directory already
// exists this never reaches the typo-guard prompt: it is deliberately the
// "profile B, already known" half of this scenario, so only profile A's
// launch exercises the not-yet-existing/prompt path that
// StartNamedClientForNewProfile (task 011) already covers on its own.
func startClientForExistingProfile(ctx context.Context, name, profile string) error {
	h, err := scenarioHarness(ctx)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(h.Home, "profiles", profile), 0o700); err != nil {
		return fmt.Errorf("pre-create profile %q directory: %w", profile, err)
	}
	client, err := h.StartNamedClient(ctx, name, "DECK_PROFILE="+profile)
	if err != nil {
		return err
	}
	return client.WaitForFrame(ctx, false, "deck - sessions")
}

// uncontendedClaudeHookTargetInProfile seeds one Claude-agent session row
// directly into profile's own state.db (the schema already exists: by the
// time this step runs, that profile's own client has already opened and
// migrated it, exactly like hook_contract_test.go's schema-bootstrap
// fixture, just scoped to a named profile's database rather than the
// default one).
func uncontendedClaudeHookTargetInProfile(ctx context.Context, name, profile string) error {
	h, err := scenarioHarness(ctx)
	if err != nil {
		return err
	}
	db, err := openProfileDatabase(ctx, h, profile)
	if err != nil {
		return err
	}
	defer db.Close()
	_, err = db.ExecContext(ctx, `INSERT INTO sessions
		(id, name, slug, cwd, agent, captured_path, conversation_id,
		 status, status_source, status_at, created_at)
		VALUES (?, ?, ?, ?, 'claude', ?, ?, 'starting', 'user', 1, 1)`,
		"hook-"+name, name, "hook-"+name, h.Home, os.Getenv("PATH"), "conversation-"+name)
	if err != nil {
		return fmt.Errorf("insert Claude hook target %q into profile %q: %w", name, profile, err)
	}
	return nil
}

// profileDatabaseSnapshot pairs a profile's state.db raw bytes with its own
// sessions row count, taken together so a later comparison can assert both
// "byte-identical" and "same row count" against one earlier moment.
type profileDatabaseSnapshot struct {
	content     []byte
	sessionRows int
}

func readProfileDatabaseSnapshot(ctx context.Context, h *ScenarioHarness, profile string) (profileDatabaseSnapshot, error) {
	content, err := os.ReadFile(profileStateDBPath(h, profile))
	if err != nil {
		return profileDatabaseSnapshot{}, fmt.Errorf("read profile %q state database: %w", profile, err)
	}
	db, err := openProfileDatabase(ctx, h, profile)
	if err != nil {
		return profileDatabaseSnapshot{}, err
	}
	defer db.Close()
	var rows int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM sessions`).Scan(&rows); err != nil {
		return profileDatabaseSnapshot{}, fmt.Errorf("count profile %q sessions: %w", profile, err)
	}
	return profileDatabaseSnapshot{content: content, sessionRows: rows}, nil
}

// snapshotProfileDatabase records profile's current state.db bytes and
// session row count under label, keyed by profile so two profiles can each
// use the same label without colliding.
func snapshotProfileDatabase(ctx context.Context, profile, label string) error {
	h, err := scenarioHarness(ctx)
	if err != nil {
		return err
	}
	snapshot, err := readProfileDatabaseSnapshot(ctx, h, profile)
	if err != nil {
		return err
	}
	if h.profileDatabaseSnapshots == nil {
		h.profileDatabaseSnapshots = make(map[string]profileDatabaseSnapshot)
	}
	h.profileDatabaseSnapshots[profile+"/"+label] = snapshot
	return nil
}

// profileDatabaseMatchesSnapshot asserts profile's state.db is, right now,
// byte-identical to the recorded snapshot AND has the same sessions row
// count -- the two together are this scenario's "profile B's database is
// unchanged" evidence named by the task's own success criteria.
func profileDatabaseMatchesSnapshot(ctx context.Context, profile, label string) error {
	h, err := scenarioHarness(ctx)
	if err != nil {
		return err
	}
	before, ok := h.profileDatabaseSnapshots[profile+"/"+label]
	if !ok {
		return fmt.Errorf("no snapshot %q recorded for profile %q", label, profile)
	}
	after, err := readProfileDatabaseSnapshot(ctx, h, profile)
	if err != nil {
		return err
	}
	if after.sessionRows != before.sessionRows {
		return fmt.Errorf("profile %q sessions row count = %d, want unchanged %d", profile, after.sessionRows, before.sessionRows)
	}
	if !bytes.Equal(after.content, before.content) {
		return fmt.Errorf("profile %q state database content changed since snapshot %q", profile, label)
	}
	return nil
}

// releasedHookHandlesEventInProfile fires the released _hook command with
// DECK_PROFILE=profile in its environment -- exactly the one deck-owned
// variable SPEC §3.4 says every pane carries and _hook resolves its paths
// from -- plus DECK_SESSION_ID naming the row seeded above, mirroring
// hook_contract_test.go's own releasedHookHandlesEvent (same payload
// shapes) with the one addition this scenario is actually about.
func releasedHookHandlesEventInProfile(ctx context.Context, event, name, profile string) error {
	h, err := scenarioHarness(ctx)
	if err != nil {
		return err
	}
	payload := map[string]string{
		"hook_event_name": event,
		"session_id":      "conversation-" + name,
	}
	switch event {
	case "Notification":
		payload["notification_type"] = "permission_prompt"
	case "SessionEnd":
		payload["reason"] = "logout"
	default:
		return fmt.Errorf("unsupported profile-hook-isolation fixture event %q", event)
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, h.Binary, "_hook")
	cmd.Env = append(os.Environ(), h.Environment("DECK_SESSION_ID=hook-"+name, "DECK_PROFILE="+profile)...)
	cmd.Stdin = strings.NewReader(string(encoded))
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("released hook receiver for profile %q: %w: %s", profile, err, strings.TrimSpace(string(output)))
	}
	return nil
}

// profileSessionStoppedBySessionEnd mirrors hook_contract_test.go's own
// sessionStoppedBySessionEnd, scoped to a named profile's own state.db: the
// row moved from its seeded starting/user to stopped/hook/logout, and
// exactly one session_end event was recorded for it -- this scenario's "A's
// row changed" evidence.
func profileSessionStoppedBySessionEnd(ctx context.Context, profile, name string) error {
	h, err := scenarioHarness(ctx)
	if err != nil {
		return err
	}
	db, err := openProfileDatabase(ctx, h, profile)
	if err != nil {
		return err
	}
	defer db.Close()
	var status, source, reason string
	if err := db.QueryRowContext(ctx, `SELECT status, status_source, status_reason FROM sessions WHERE name = ?`, name).Scan(&status, &source, &reason); err != nil {
		return fmt.Errorf("read ended session %q in profile %q: %w", name, profile, err)
	}
	if status != "stopped" || source != "hook" || reason != "logout" {
		return fmt.Errorf("profile %q ended session %q = status %q source %q reason %q, want stopped/hook/logout", profile, name, status, source, reason)
	}
	var eventCount int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM events WHERE session_id = ? AND kind = 'session_end'`, "hook-"+name).Scan(&eventCount); err != nil {
		return err
	}
	if eventCount != 1 {
		return fmt.Errorf("profile %q session %q session_end event count = %d, want 1", profile, name, eventCount)
	}
	return nil
}
