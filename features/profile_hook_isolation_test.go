package features

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cucumber/godog"
)

// registerProfileHookIsolationSteps wires SPEC §3.4's "every pane carries
// DECK_PROFILE ... so a pane writes to the database of the profile that
// created it for its whole life" and "a profile never reads another
// profile's state.db" together (task 014, R154): two profiles' decks run at
// once in one scenario's DECK_HOME, each on its own private tmux socket;
// profile A's deck creates a real fake-Claude pane, that pane's own agent
// fires a hook through the settings deck handed it (so the hook subprocess
// inherits nothing but the pane's own environment), and only A's database
// -- never B's, never the default profile's -- is asserted to have moved.
func registerProfileHookIsolationSteps(sc *godog.ScenarioContext) {
	sc.Step(`^deck client "([^"]+)" is started for the existing profile "([^"]+)" on (the scenario's|its own) private socket$`, startClientForExistingProfile)
	sc.Step(`^the profile "([^"]+)" session "([^"]+)"'s pane carries DECK_PROFILE "([^"]+)" and its own session id$`, profilePaneCarriesDeckProfile)
	sc.Step(`^the profile "([^"]+)" state database session "([^"]+)" settles at status "([^"]+)"$`, profileSessionSettlesAtStatus)
	sc.Step(`^the profile "([^"]+)" state database is snapshotted as "([^"]+)"$`, snapshotProfileDatabase)
	sc.Step(`^fake Claude session "([^"]+)" in profile "([^"]+)" fires "([^"]+)" from its own pane:$`, fakeClaudeInProfileFiresFromOwnPane)
	sc.Step(`^the profile "([^"]+)" state database session "([^"]+)" has hook status "([^"]+)" with reason "([^"]*)" and one "([^"]+)" event$`, profileSessionHasHookStatus)
	sc.Step(`^the profile "([^"]+)" state database still matches snapshot "([^"]+)" in content and session row count$`, profileDatabaseMatchesSnapshot)
	sc.Step(`^the scenario's data root holds no default-profile state database$`, noDefaultProfileStateDatabase)
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
// exists this never reaches the typo-guard prompt (task 011's own scenario
// covers that path). The socket is always a private, per-scenario one --
// never the derived "deck-<profile>" -- so no real profile server is ever
// involved: "the scenario's" is h.Socket itself, and "its own" is a second
// private socket derived from it, registered in h.extraSockets so Close
// kills and probes it exactly like h.Socket. DECK_TMUX_SOCKET wins outright
// over the profile-derived socket (SPEC §3.4), so this is the one variable
// that keeps the two decks' tmux servers apart.
func startClientForExistingProfile(ctx context.Context, name, profile, socketKind string) error {
	h, err := scenarioHarness(ctx)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(h.Home, "profiles", profile), 0o700); err != nil {
		return fmt.Errorf("pre-create profile %q directory: %w", profile, err)
	}
	socket := h.Socket
	if socketKind == "its own" {
		socket = h.Socket + "_" + profile
		h.extraSockets = append(h.extraSockets, socket)
	}
	client, err := h.StartNamedClient(ctx, name, "DECK_PROFILE="+profile, "DECK_TMUX_SOCKET="+socket)
	if err != nil {
		return err
	}
	return client.WaitForFrame(ctx, false, "deck - sessions")
}

// profileSessionRow reads one column set for the session named name from
// profile's own state.db.
func profileSessionRow(ctx context.Context, h *ScenarioHarness, profile, name string) (id, slug, status, source, reason string, err error) {
	db, err := openProfileDatabase(ctx, h, profile)
	if err != nil {
		return "", "", "", "", "", err
	}
	defer db.Close()
	var reasonValue sql.NullString
	err = db.QueryRowContext(ctx, `SELECT id, slug, status, COALESCE(status_source, ''), status_reason FROM sessions WHERE name = ?`, name).
		Scan(&id, &slug, &status, &source, &reasonValue)
	if err != nil {
		return "", "", "", "", "", fmt.Errorf("observe session %q in profile %q: %w", name, profile, err)
	}
	return id, slug, status, source, reasonValue.String, nil
}

// profilePaneCarriesDeckProfile proves the pane profile A's deck created is
// the one about to fire the hook: its tmux session (on h.Socket, the
// socket profile A's deck was started on) carries DECK_PROFILE=profile and
// DECK_SESSION_ID naming the row in profile's OWN state.db -- the pair
// _hook resolves its database and its target from.
func profilePaneCarriesDeckProfile(ctx context.Context, profile, name, wantProfile string) error {
	h, err := scenarioHarness(ctx)
	if err != nil {
		return err
	}
	id, slug, _, _, _, err := profileSessionRow(ctx, h, profile, name)
	if err != nil {
		return err
	}
	target := "deck_" + slug
	for key, want := range map[string]string{"DECK_PROFILE": wantProfile, "DECK_SESSION_ID": id} {
		output, err := tmuxOutput(ctx, h, "show-environment", "-t", target, key)
		if err != nil {
			return fmt.Errorf("read %s from profile %q pane session %q: %w", key, profile, target, err)
		}
		if got := strings.TrimSpace(string(output)); got != key+"="+want {
			return fmt.Errorf("profile %q pane session %q environment %s = %q, want %q", profile, target, key, got, key+"="+want)
		}
	}
	return nil
}

// profileSessionSettlesAtStatus waits, bounded, for the durable fact that
// profile's session name reached status in profile's own state.db, so a
// later snapshot of that database is taken after its own launch has
// finished writing rather than mid-transition.
func profileSessionSettlesAtStatus(ctx context.Context, profile, name, want string) error {
	h, err := scenarioHarness(ctx)
	if err != nil {
		return err
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		_, _, status, _, _, err := profileSessionRow(ctx, h, profile, name)
		if err == nil && status == want {
			return nil
		}
		if time.Now().After(deadline) {
			if err != nil {
				return err
			}
			return fmt.Errorf("profile %q session %q status = %q, want %q", profile, name, status, want)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// fakeClaudeInProfileFiresFromOwnPane types the fake-claude fixture's
// "hook" command into the pane profile's deck created for name. The
// fixture then runs the hook command deck itself put in that agent's
// settings (`<deck> _hook`), as a child of the pane's own process, so the
// hook's DECK_PROFILE/DECK_SESSION_ID/DECK_HOME come from nowhere but the
// pane's inherited environment -- the step never supplies any of them.
// It waits, bounded, for the durable fact the hook landed: one more event
// row of the hook's own kind for that session in profile's own state.db
// (never just any event -- a pane that died because its hook failed also
// records one).
func fakeClaudeInProfileFiresFromOwnPane(ctx context.Context, name, profile, event string, table *godog.Table) error {
	h, err := scenarioHarness(ctx)
	if err != nil {
		return err
	}
	payload := make(map[string]any)
	for _, row := range table.Rows {
		if len(row.Cells) != 2 {
			return fmt.Errorf("hook payload row has %d cells, want key and value", len(row.Cells))
		}
		payload[row.Cells[0].Value] = row.Cells[1].Value
	}
	request, err := json.Marshal(map[string]any{"command": "hook", "event": event, "payload": payload})
	if err != nil {
		return err
	}
	id, slug, _, _, _, err := profileSessionRow(ctx, h, profile, name)
	if err != nil {
		return err
	}
	db, err := openProfileDatabase(ctx, h, profile)
	if err != nil {
		return err
	}
	defer db.Close()
	kind := hookEventKind(event)
	var before int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM events WHERE session_id = ? AND kind = ?`, id, kind).Scan(&before); err != nil {
		return err
	}
	paneTarget := "deck_" + slug
	if _, err := tmuxOutput(ctx, h, "send-keys", "-t", paneTarget, "-l", string(request)); err != nil {
		return fmt.Errorf("send %s command to profile %q fake Claude pane %q: %w", event, profile, paneTarget, err)
	}
	if _, err := tmuxOutput(ctx, h, "send-keys", "-t", paneTarget, "Enter"); err != nil {
		return fmt.Errorf("submit %s command to profile %q fake Claude pane %q: %w", event, profile, paneTarget, err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		var count int
		if err := db.QueryRowContext(ctx, `SELECT count(*) FROM events WHERE session_id = ? AND kind = ?`, id, kind).Scan(&count); err != nil {
			return err
		}
		if count > before {
			return nil
		}
		if time.Now().After(deadline) {
			output, _ := tmuxOutput(ctx, h, "capture-pane", "-p", "-S", "-", "-t", paneTarget)
			return fmt.Errorf("profile %q fake Claude pane %q did not persist %s hook; pane:\n%s", profile, paneTarget, event, output)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// hookEventKind maps a Claude hook event name to the events.kind the hook
// receiver records it under: CamelCase to snake_case ("SessionStart" ->
// "session_start", "Notification" -> "notification").
func hookEventKind(event string) string {
	var out strings.Builder
	for i, r := range event {
		if r >= 'A' && r <= 'Z' {
			if i > 0 {
				out.WriteByte('_')
			}
			r += 'a' - 'A'
		}
		out.WriteRune(r)
	}
	return out.String()
}

// profileSessionHasHookStatus is this scenario's "A's row changed"
// evidence: profile's session name now carries the hook's own verdict
// (status/source "hook"/reason) and exactly one event of kind.
func profileSessionHasHookStatus(ctx context.Context, profile, name, wantStatus, wantReason, kind string) error {
	h, err := scenarioHarness(ctx)
	if err != nil {
		return err
	}
	id, _, status, source, reason, err := profileSessionRow(ctx, h, profile, name)
	if err != nil {
		return err
	}
	if status != wantStatus || source != "hook" || reason != wantReason {
		return fmt.Errorf("profile %q session %q = status %q source %q reason %q, want %s/hook/%s", profile, name, status, source, reason, wantStatus, wantReason)
	}
	db, err := openProfileDatabase(ctx, h, profile)
	if err != nil {
		return err
	}
	defer db.Close()
	var count int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM events WHERE session_id = ? AND kind = ?`, id, kind).Scan(&count); err != nil {
		return err
	}
	if count != 1 {
		return fmt.Errorf("profile %q session %q %q event count = %d, want 1", profile, name, kind, count)
	}
	return nil
}

// noDefaultProfileStateDatabase asserts no deck process in this scenario
// -- neither the two named decks nor the hook fired from A's pane -- ever
// fell back to the default profile's database at the data root itself.
func noDefaultProfileStateDatabase(ctx context.Context) error {
	h, err := scenarioHarness(ctx)
	if err != nil {
		return err
	}
	path := filepath.Join(h.Home, "state.db")
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("default-profile state database %s exists (stat err %w), want none", path, err)
	}
	return nil
}

// profileDatabaseSnapshot pairs a profile's state.db (plus -wal) raw bytes with its own
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
	// A WAL-mode writer lands its pages in state.db-wal first; fold that
	// file in (when present) so a write not yet checkpointed into the main
	// file still counts as a content change.
	wal, err := os.ReadFile(profileStateDBPath(h, profile) + "-wal")
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return profileDatabaseSnapshot{}, fmt.Errorf("read profile %q state database WAL: %w", profile, err)
	}
	content = append(append(content, []byte("\x00-wal\x00")...), wal...)
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
