package store

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

func TestOpenInitializesPrivateV1WALStore(t *testing.T) {
	home := filepath.Join(t.TempDir(), "deck")
	path := filepath.Join(home, "state.db")
	store, err := OpenPath(home, path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	assertMode(t, home, 0o700)
	assertMode(t, path, 0o600)
	var journal string
	if err := store.DB().QueryRow(`PRAGMA journal_mode`).Scan(&journal); err != nil || strings.ToLower(journal) != "wal" {
		t.Fatalf("journal mode = %q, %v; want wal", journal, err)
	}
	var synchronous, timeout, foreignKeys, version int
	if err := store.DB().QueryRow(`PRAGMA synchronous`).Scan(&synchronous); err != nil || synchronous != 1 {
		t.Fatalf("synchronous = %d, %v; want NORMAL (1)", synchronous, err)
	}
	if err := store.DB().QueryRow(`PRAGMA busy_timeout`).Scan(&timeout); err != nil || timeout != 5000 {
		t.Fatalf("busy_timeout = %d, %v; want 5000", timeout, err)
	}
	if err := store.DB().QueryRow(`PRAGMA foreign_keys`).Scan(&foreignKeys); err != nil || foreignKeys != 1 {
		t.Fatalf("foreign_keys = %d, %v; want 1", foreignKeys, err)
	}
	if err := store.DB().QueryRow(`SELECT version FROM meta WHERE key = 'schema_version'`).Scan(&version); err != nil || version != SchemaVersion {
		t.Fatalf("schema version = %d, %v; want %d", version, err, SchemaVersion)
	}
	for _, table := range []string{"sessions", "events", "meta", "ui_state"} {
		var name string
		if err := store.DB().QueryRow(`SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?`, table).Scan(&name); err != nil || name != table {
			t.Fatalf("missing %s table: %v", table, err)
		}
	}
}

func TestOpenMigratesSupportedVersionZeroFixture(t *testing.T) {
	home := filepath.Join(t.TempDir(), "deck")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(home, "state.db")
	fixture, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.Exec(`CREATE TABLE meta (key TEXT PRIMARY KEY, version INTEGER NOT NULL); INSERT INTO meta VALUES ('schema_version', 0)`); err != nil {
		t.Fatal(err)
	}
	fixture.Close()

	store, err := OpenPath(home, path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	var version int
	if err := store.DB().QueryRow(`SELECT version FROM meta WHERE key = 'schema_version'`).Scan(&version); err != nil || version != SchemaVersion {
		t.Fatalf("migrated version = %d, %v", version, err)
	}
	var sessions int
	if err := store.DB().QueryRow(`SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = 'sessions'`).Scan(&sessions); err != nil || sessions != 1 {
		t.Fatalf("sessions table after migration = %d, %v", sessions, err)
	}
	assertMode(t, home, 0o700)
	assertMode(t, path, 0o600)
}

func TestOpenRefusesNewerFixtureWithoutMutation(t *testing.T) {
	home := filepath.Join(t.TempDir(), "deck")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(home, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(home, "state.db")
	fixture, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.Exec(`CREATE TABLE meta (key TEXT PRIMARY KEY, version INTEGER NOT NULL); INSERT INTO meta VALUES ('schema_version', 5)`); err != nil {
		t.Fatal(err)
	}
	if err := fixture.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := OpenPath(home, path); err == nil || !strings.Contains(err.Error(), "newer than supported") {
		t.Fatalf("newer fixture error = %v", err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatal("newer fixture was modified")
	}
	assertMode(t, home, 0o755)
	assertMode(t, path, 0o644)
}

func TestCreateSessionIsTargetedAndEnforcesNameAndSlugUniqueness(t *testing.T) {
	home := t.TempDir()
	store, err := OpenPath(home, filepath.Join(home, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	first, err := store.CreateSession(ctx, CreateSessionInput{
		ID: "00000000-0000-4000-8000-000000000001", Name: "Build: API.v1", CWD: "/same/cwd",
		Agent: "shell", CapturedPath: "/bin", StatusAt: 100, CreatedAt: 100,
	})
	if err != nil {
		t.Fatal(err)
	}
	if first.Slug != "build-api-v1" || strings.ContainsAny(first.Slug, ".:") {
		t.Fatalf("slug = %q; want tmux-safe build-api-v1", first.Slug)
	}
	second, err := store.CreateSession(ctx, CreateSessionInput{
		ID: "00000000-0000-4000-8000-000000000002", Name: "Other", CWD: "/same/cwd",
		Agent: "shell", CapturedPath: "/bin", StatusAt: 101, CreatedAt: 101,
	})
	if err != nil {
		t.Fatal(err)
	}
	if second.CWD != first.CWD {
		t.Fatalf("duplicate cwd was not retained: %q != %q", second.CWD, first.CWD)
	}
	for _, input := range []CreateSessionInput{
		{ID: "00000000-0000-4000-8000-000000000003", Name: "Other", CWD: "/x", Agent: "shell", CapturedPath: "/bin", StatusAt: 102, CreatedAt: 102},
		{ID: "00000000-0000-4000-8000-000000000004", Name: "Build API v1", CWD: "/x", Agent: "shell", CapturedPath: "/bin", StatusAt: 103, CreatedAt: 103},
	} {
		if _, err := store.CreateSession(ctx, input); err == nil || !strings.Contains(err.Error(), "already exists") && !strings.Contains(err.Error(), "collides") {
			t.Fatalf("collision error = %v; want useful name or slug collision", err)
		}
	}
	var count int
	if err := store.DB().QueryRow(`SELECT count(*) FROM sessions`).Scan(&count); err != nil || count != 2 {
		t.Fatalf("session count = %d, %v; failed insert must not rewrite rows", count, err)
	}
}

func TestStatusWritersRequireExplicitTimestamps(t *testing.T) {
	home := t.TempDir()
	store, err := OpenPath(home, filepath.Join(home, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	input := CreateSessionInput{ID: "timestamp-required", Name: "timestamp required", CWD: "/work", Agent: "shell", CapturedPath: "/bin"}
	if _, err := store.CreateSession(ctx, input); err == nil || !strings.Contains(err.Error(), "timestamps are required") {
		t.Fatalf("create without clock timestamp = %v", err)
	}
	input.StatusAt, input.CreatedAt = 123, 123
	if _, err := store.CreateSession(ctx, input); err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateSessionStatus(ctx, StatusUpdateInput{SessionID: input.ID, Status: "stopped"}); err == nil || !strings.Contains(err.Error(), "timestamp is required") {
		t.Fatalf("status/event write without clock timestamp = %v", err)
	}
	if err := store.SetPermissionProfile(ctx, input.ID, "safe", "user", 0); err == nil || !strings.Contains(err.Error(), "timestamp is required") {
		t.Fatalf("event mutation without clock timestamp = %v", err)
	}
	var events int
	if err := store.DB().QueryRow(`SELECT count(*) FROM events`).Scan(&events); err != nil || events != 0 {
		t.Fatalf("rejected fallback write produced %d events: %v", events, err)
	}
}

func TestCreateSessionRoundTripsAllPhase1FieldsAcrossReopen(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, "state.db")
	ctx := context.Background()
	input := CreateSessionInput{
		ID: "00000000-0000-4000-8000-000000000010", Name: "Full Agent", CWD: "/work/full",
		Agent: "claude", CapturedPath: "/usr/bin:/bin", StatusAt: 200, CreatedAt: 200,
		LaunchArgs:        []string{"--extra", "flag"},
		Env:               map[string]string{"FOO": "bar", "BAZ": "qux"},
		PreLaunch:         "source secrets.sh",
		LoginShell:        true,
		PermissionProfile: "yolo",
		ConversationID:    "11111111-1111-4111-8111-111111111111",
		ResumePin:         "22222222-2222-4222-8222-222222222222",
		ResumeState:       "pinned",
	}

	store, err := OpenPath(home, path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateSession(ctx, input); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := OpenPath(home, path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()

	assertRoundTrip := func(t *testing.T, session Session) {
		t.Helper()
		if got := strings.Join(session.LaunchArgs, ","); got != "--extra,flag" {
			t.Fatalf("launch args = %q; want --extra,flag", got)
		}
		if len(session.Env) != 2 || session.Env["FOO"] != "bar" || session.Env["BAZ"] != "qux" {
			t.Fatalf("env = %#v; want FOO=bar,BAZ=qux", session.Env)
		}
		if session.PreLaunch != input.PreLaunch {
			t.Fatalf("pre_launch = %q; want %q", session.PreLaunch, input.PreLaunch)
		}
		if !session.LoginShell {
			t.Fatalf("login_shell = false; want true")
		}
		if session.PermissionProfile != input.PermissionProfile {
			t.Fatalf("permission_profile = %q; want %q", session.PermissionProfile, input.PermissionProfile)
		}
		if session.ConversationID != input.ConversationID {
			t.Fatalf("conversation_id = %q; want %q", session.ConversationID, input.ConversationID)
		}
		if session.ResumePin != input.ResumePin {
			t.Fatalf("resume_pin = %q; want %q", session.ResumePin, input.ResumePin)
		}
		if session.ResumeState != input.ResumeState {
			t.Fatalf("resume_state = %q; want %q", session.ResumeState, input.ResumeState)
		}
	}

	got, err := reopened.GetSession(ctx, input.ID)
	if err != nil {
		t.Fatal(err)
	}
	assertRoundTrip(t, got)

	listed, err := reopened.ListSessions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 {
		t.Fatalf("listed sessions = %d; want 1", len(listed))
	}
	assertRoundTrip(t, listed[0])
}

func TestCreateSessionDefaultsLaunchArgsAndEnvToEmptyNotNull(t *testing.T) {
	home := t.TempDir()
	store, err := OpenPath(home, filepath.Join(home, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	if _, err := store.CreateSession(ctx, CreateSessionInput{
		ID: "00000000-0000-4000-8000-000000000011", Name: "Bare Shell", CWD: "/work/bare",
		Agent: "shell", CapturedPath: "/bin", StatusAt: 1, CreatedAt: 1,
	}); err != nil {
		t.Fatal(err)
	}
	got, err := store.GetSession(ctx, "00000000-0000-4000-8000-000000000011")
	if err != nil {
		t.Fatal(err)
	}
	if got.LaunchArgs == nil || len(got.LaunchArgs) != 0 {
		t.Fatalf("launch args = %#v; want non-nil empty slice", got.LaunchArgs)
	}
	if got.Env == nil || len(got.Env) != 0 {
		t.Fatalf("env = %#v; want non-nil empty map", got.Env)
	}
	if got.PermissionProfile != "safe" {
		t.Fatalf("permission_profile = %q; want default safe", got.PermissionProfile)
	}
	if got.ResumeState != "auto" {
		t.Fatalf("resume_state = %q; want default auto", got.ResumeState)
	}
	if got.ConversationID != "" || got.ResumePin != "" || got.PreLaunch != "" {
		t.Fatalf("unset optional fields must be empty strings: conversation_id=%q resume_pin=%q pre_launch=%q", got.ConversationID, got.ResumePin, got.PreLaunch)
	}
}

// TestCapturedPathAdvisoryReflectsLoginShellAndPersistsAcrossReopen proves
// SPEC §6.3's "enabling login_shell marks captured_path advisory" is a
// persisted, queryable marking (the login_shell column, read back through
// CapturedPathAdvisory), not merely a comment: a login_shell=1 row still
// stores its non-empty create-time CapturedPath (it is never cleared), a
// login_shell=0 row reports CapturedPathAdvisory=false, and the marking
// survives a store close/reopen exactly like every other persisted column.
func TestCapturedPathAdvisoryReflectsLoginShellAndPersistsAcrossReopen(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, "state.db")
	ctx := context.Background()
	st, err := OpenPath(home, path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateSession(ctx, CreateSessionInput{
		ID: "00000000-0000-4000-8000-000000000012", Name: "Login Shell Agent", CWD: "/work/login",
		Agent: "claude", CapturedPath: "/usr/bin:/bin", StatusAt: 1, CreatedAt: 1, LoginShell: true,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateSession(ctx, CreateSessionInput{
		ID: "00000000-0000-4000-8000-000000000013", Name: "Plain Agent", CWD: "/work/plain",
		Agent: "claude", CapturedPath: "/usr/bin:/bin", StatusAt: 1, CreatedAt: 1,
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenPath(home, path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()

	advisory, err := reopened.GetSession(ctx, "00000000-0000-4000-8000-000000000012")
	if err != nil {
		t.Fatal(err)
	}
	if advisory.CapturedPath == "" {
		t.Fatal("login_shell=1 row must still store a non-empty captured_path, not clear it")
	}
	if !advisory.CapturedPathAdvisory() {
		t.Fatal("login_shell=1 row: CapturedPathAdvisory() = false; want true")
	}

	plain, err := reopened.GetSession(ctx, "00000000-0000-4000-8000-000000000013")
	if err != nil {
		t.Fatal(err)
	}
	if plain.CapturedPathAdvisory() {
		t.Fatal("login_shell=0 row: CapturedPathAdvisory() = true; want false")
	}
}

func TestUpdateSessionStatusIsTargetedRecordsEventAndListsStoppedRows(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, "state.db")
	firstStore, err := OpenPath(home, path)
	if err != nil {
		t.Fatal(err)
	}
	defer firstStore.Close()
	secondStore, err := OpenPath(home, path)
	if err != nil {
		t.Fatal(err)
	}
	defer secondStore.Close()
	ctx := context.Background()
	for _, input := range []CreateSessionInput{
		{ID: "00000000-0000-4000-8000-000000000011", Name: "alpha", CWD: "/a", Agent: "shell", CapturedPath: "/bin", Status: "running", StatusSource: "user", StatusAt: 10, CreatedAt: 10},
		{ID: "00000000-0000-4000-8000-000000000012", Name: "beta", CWD: "/b", Agent: "shell", CapturedPath: "/bin", Status: "running", StatusSource: "user", StatusAt: 11, CreatedAt: 11},
	} {
		if _, err := firstStore.CreateSession(ctx, input); err != nil {
			t.Fatal(err)
		}
	}
	if err := firstStore.UpdateSessionStatus(ctx, StatusUpdateInput{
		SessionID: "00000000-0000-4000-8000-000000000011", Status: "stopped",
		Reason: "killed", Source: "user", At: 20,
	}); err != nil {
		t.Fatal(err)
	}

	// A separate SQLite connection sees the committed targeted update.
	sessions, err := secondStore.ListSessions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 2 {
		t.Fatalf("listed sessions = %d, want 2 including stopped rows", len(sessions))
	}
	if got := sessions[0]; got.Status != "stopped" || got.StatusReason != "killed" || got.StatusAt != 20 {
		t.Fatalf("updated session = %#v", got)
	}
	if got := sessions[1]; got.Status != "running" || got.StatusAt != 11 {
		t.Fatalf("unaddressed session changed = %#v", got)
	}
	var eventCount int
	var kind, reason string
	if err := secondStore.DB().QueryRow(`SELECT count(*), max(kind), max(reason) FROM events WHERE session_id = ?`, sessions[0].ID).Scan(&eventCount, &kind, &reason); err != nil {
		t.Fatal(err)
	}
	if eventCount != 1 || kind != "stopped" || reason != "killed" {
		t.Fatalf("transition event = count %d, kind %q, reason %q", eventCount, kind, reason)
	}
	if err := firstStore.UpdateSessionStatus(ctx, StatusUpdateInput{SessionID: "missing", Status: "stopped", At: 21}); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("missing session error = %v", err)
	}
	if err := secondStore.DB().QueryRow(`SELECT count(*) FROM events`).Scan(&eventCount); err != nil || eventCount != 1 {
		t.Fatalf("missing update appended an event: count %d, err %v", eventCount, err)
	}
}

func TestSetConversationIDPermissionProfileAndResumeStateRecordEvents(t *testing.T) {
	home := t.TempDir()
	store, err := OpenPath(home, filepath.Join(home, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	const id = "00000000-0000-4000-8000-000000000020"
	if _, err := store.CreateSession(ctx, CreateSessionInput{
		ID: id, Name: "mutations", CWD: "/work/mut", Agent: "claude", CapturedPath: "/bin", StatusAt: 1, CreatedAt: 1,
	}); err != nil {
		t.Fatal(err)
	}

	eventCountFor := func(t *testing.T, kind string) int {
		t.Helper()
		var count int
		if err := store.DB().QueryRow(`SELECT count(*) FROM events WHERE session_id = ? AND kind = ?`, id, kind).Scan(&count); err != nil {
			t.Fatal(err)
		}
		return count
	}

	const convoA = "11111111-1111-4111-8111-111111111111"
	if err := store.SetConversationID(ctx, id, convoA, "user", 10); err != nil {
		t.Fatal(err)
	}
	got, err := store.GetSession(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if got.ConversationID != convoA {
		t.Fatalf("conversation_id = %q; want %q", got.ConversationID, convoA)
	}
	if eventCountFor(t, "set_conversation_id") != 1 {
		t.Fatalf("expected exactly one set_conversation_id event")
	}

	if err := store.SetPermissionProfile(ctx, id, "yolo", "user", 11); err != nil {
		t.Fatal(err)
	}
	got, err = store.GetSession(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if got.PermissionProfile != "yolo" {
		t.Fatalf("permission_profile = %q; want yolo", got.PermissionProfile)
	}
	if eventCountFor(t, "set_permission_profile") != 1 {
		t.Fatalf("expected exactly one set_permission_profile event")
	}

	const convoB = "22222222-2222-4222-8222-222222222222"
	if err := store.SetResumePin(ctx, id, convoB, "user", 12); err != nil {
		t.Fatal(err)
	}
	got, err = store.GetSession(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if got.ResumeState != "pinned" || got.ResumePin != convoB {
		t.Fatalf("resume state = %q, pin = %q; want pinned/%q", got.ResumeState, got.ResumePin, convoB)
	}
	if eventCountFor(t, "set_resume_pin") != 1 {
		t.Fatalf("expected exactly one set_resume_pin event")
	}

	// A pin survives a reopen ("sticky across a deck restart").
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenPath(home, filepath.Join(home, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	got, err = reopened.GetSession(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if got.ResumeState != "pinned" || got.ResumePin != convoB {
		t.Fatalf("pin did not survive reopen: state=%q pin=%q", got.ResumeState, got.ResumePin)
	}

	if err := reopened.SetResumeStateAuto(ctx, id, "user", 13); err != nil {
		t.Fatal(err)
	}
	got, err = reopened.GetSession(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if got.ResumeState != "auto" || got.ResumePin != "" {
		t.Fatalf("resume state = %q, pin = %q; want auto with cleared pin", got.ResumeState, got.ResumePin)
	}

	if err := reopened.SetConversationID(ctx, "missing", convoA, "user", 14); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("missing session error = %v", err)
	}
}

func TestFreshOnceIsOneShotAndRevertsToAutoNotPinned(t *testing.T) {
	home := t.TempDir()
	store, err := OpenPath(home, filepath.Join(home, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	const id = "00000000-0000-4000-8000-000000000021"
	const pin = "33333333-3333-4333-8333-333333333333"
	if _, err := store.CreateSession(ctx, CreateSessionInput{
		ID: id, Name: "fresh-once", CWD: "/work/fresh", Agent: "claude", CapturedPath: "/bin",
		ResumePin: pin, ResumeState: "pinned", StatusAt: 1, CreatedAt: 1,
	}); err != nil {
		t.Fatal(err)
	}

	if err := store.SetResumeStateFreshOnce(ctx, id, "user", 20); err != nil {
		t.Fatal(err)
	}
	got, err := store.GetSession(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if got.ResumeState != "fresh-once" {
		t.Fatalf("resume_state = %q; want fresh-once", got.ResumeState)
	}
	// Arming fresh-once must not silently discard the pin: it can be reused
	// once the one-shot fresh launch has consumed the fresh-once state.
	if got.ResumePin != pin {
		t.Fatalf("resume_pin = %q; want unchanged %q", got.ResumePin, pin)
	}

	// Simulate the fresh launch having happened, then consume the one-shot.
	if err := store.ConsumeFreshOnce(ctx, id, "service", 21); err != nil {
		t.Fatal(err)
	}
	got, err = store.GetSession(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if got.ResumeState != "auto" {
		t.Fatalf("resume_state after consume = %q; want auto (never left as fresh-once, never reverted to pinned)", got.ResumeState)
	}

	// Consuming again is a benign no-op, not an error, and does not flip a
	// fresh state that was never armed.
	if err := store.ConsumeFreshOnce(ctx, id, "service", 22); err != nil {
		t.Fatal(err)
	}
	got, err = store.GetSession(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if got.ResumeState != "auto" {
		t.Fatalf("resume_state after redundant consume = %q; want auto", got.ResumeState)
	}
}

func TestStatusTransitionAppliesPrecedenceAcknowledgementAndEpochAtomically(t *testing.T) {
	home := t.TempDir()
	store, err := OpenPath(home, filepath.Join(home, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	const id = "status-machine"
	if _, err := store.CreateSession(ctx, CreateSessionInput{
		ID: id, Name: "status machine", CWD: "/work", Agent: "claude", CapturedPath: "/bin",
		Status: "running", StatusSource: "hook", StatusAt: 100, CreatedAt: 1,
	}); err != nil {
		t.Fatal(err)
	}

	// A fresh hook outranks a probe. The losing probe is nevertheless durable
	// evidence that sampling ran rather than dead code accidentally passing.
	if err := store.UpdateSessionStatus(ctx, StatusUpdateInput{
		SessionID: id, Status: "waiting", Source: "probe", At: 120,
		StaleAfter: 50, EventKind: "probe.waiting", Payload: `{"fixture":"prompt"}`,
	}); err != nil {
		t.Fatal(err)
	}
	got, err := store.GetSession(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "running" || got.StatusSource != "hook" || got.StatusAt != 100 {
		t.Fatalf("fresh-hook verdict was overwritten: %#v", got)
	}
	var probeEvents int
	if err := store.DB().QueryRow(`SELECT count(*) FROM events WHERE session_id = ? AND kind = 'probe.waiting'`, id).Scan(&probeEvents); err != nil || probeEvents != 1 {
		t.Fatalf("losing probe events = %d, %v; want 1", probeEvents, err)
	}

	// Once stale, the same lower-quality source may correct the verdict.
	if err := store.UpdateSessionStatus(ctx, StatusUpdateInput{
		SessionID: id, Status: "waiting", Reason: "permission_prompt", Source: "probe", At: 150,
		StaleAfter: 50, EventKind: "probe.waiting",
	}); err != nil {
		t.Fatal(err)
	}
	got, err = store.GetSession(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "waiting" || got.StatusSource != "probe" || got.Acknowledged {
		t.Fatalf("stale correction/attention reset = %#v", got)
	}

	acknowledged := true
	if err := store.UpdateSessionStatus(ctx, StatusUpdateInput{
		SessionID: id, Status: "running", Source: "user", At: 160,
		EventKind: "attached", Acknowledged: &acknowledged,
	}); err != nil {
		t.Fatal(err)
	}
	got, err = store.GetSession(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "running" || !got.Acknowledged || got.NotifyEpoch != 1 {
		t.Fatalf("leaving attention state = %#v; want acknowledged and epoch 1", got)
	}

	// An event failure rolls the status mutation back with it.
	if _, err := store.DB().Exec(`CREATE TRIGGER reject_atomic_event BEFORE INSERT ON events
		WHEN NEW.kind = 'reject.me' BEGIN SELECT RAISE(ABORT, 'reject event'); END`); err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateSessionStatus(ctx, StatusUpdateInput{
		SessionID: id, Status: "idle", Source: "hook", At: 170, EventKind: "reject.me",
	}); err == nil {
		t.Fatal("transition unexpectedly succeeded when matching event was rejected")
	}
	got, err = store.GetSession(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "running" || got.StatusAt != 160 {
		t.Fatalf("event failure left a partial row update: %#v", got)
	}
}

func TestStatusTransitionEnforcesCallerPolicyInsideEventTransaction(t *testing.T) {
	home := t.TempDir()
	store, err := OpenPath(home, filepath.Join(home, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	const id = "policy-guard"
	if _, err := store.CreateSession(ctx, CreateSessionInput{
		ID: id, Name: id, CWD: "/work", Agent: "claude", CapturedPath: "/bin",
		Status: "running", StatusSource: "hook", StatusAt: 10, CreatedAt: 1,
	}); err != nil {
		t.Fatal(err)
	}

	input := StatusUpdateInput{
		SessionID: id, Status: "waiting", Reason: "question", Source: "hook", At: 20,
		EventKind: "notification", Payload: `{"late":true}`, LastMessage: "must not apply",
		AllowedCurrentStatuses: []string{"idle"},
	}
	if err := store.UpdateSessionStatus(ctx, input); err != nil {
		t.Fatal(err)
	}
	got, err := store.GetSession(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "running" || got.StatusReason != "" || got.StatusSource != "hook" || got.StatusAt != 10 || got.LastMessage != "" || !got.Acknowledged || got.NotifyEpoch != 0 {
		t.Fatalf("rejected policy transition changed row: %#v", got)
	}
	var events int
	if err := store.DB().QueryRow(`SELECT count(*) FROM events WHERE session_id = ? AND kind = 'notification' AND payload = ?`, id, input.Payload).Scan(&events); err != nil || events != 1 {
		t.Fatalf("rejected transition audit events = %d, %v; want 1", events, err)
	}

	input.At = 21
	input.AllowedCurrentStatuses = []string{"running"}
	if err := store.UpdateSessionStatus(ctx, input); err != nil {
		t.Fatal(err)
	}
	got, err = store.GetSession(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "waiting" || got.StatusReason != "question" || got.StatusAt != 21 || got.Acknowledged {
		t.Fatalf("allowed policy transition did not apply: %#v", got)
	}
}

func TestStatusTransitionDoesNotReviveProcessCrashWithHook(t *testing.T) {
	home := t.TempDir()
	store, err := OpenPath(home, filepath.Join(home, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	const id = "crash-policy-guard"
	exit := 137
	if _, err := store.CreateSession(ctx, CreateSessionInput{
		ID: id, Name: id, CWD: "/work", Agent: "claude", CapturedPath: "/bin",
		Status: "error", StatusSource: "tmux", StatusAt: 10, CreatedAt: 1,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.DB().Exec(`UPDATE sessions SET status_reason = 'pane exit', pane_exit_status = ?, crash_tail = 'fatal', acknowledged = 0 WHERE id = ?`, exit, id); err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateSessionStatus(ctx, StatusUpdateInput{
		SessionID: id, Status: "running", Source: "hook", At: 20,
		EventKind: "user_prompt_submitted", AllowedCurrentStatuses: []string{"error"},
	}); err != nil {
		t.Fatal(err)
	}
	got, err := store.GetSession(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "error" || got.StatusReason != "pane exit" || got.StatusSource != "tmux" || got.StatusAt != 10 || got.PaneExitStatus == nil || *got.PaneExitStatus != exit || got.CrashTail != "fatal" || got.Acknowledged {
		t.Fatalf("hook revived process crash: %#v", got)
	}
	var events int
	if err := store.DB().QueryRow(`SELECT count(*) FROM events WHERE session_id = ? AND kind = 'user_prompt_submitted'`, id).Scan(&events); err != nil || events != 1 {
		t.Fatalf("rejected crash hook events = %d, %v; want 1", events, err)
	}
}

func TestRecordAttachmentHandlesWaitingErrorAndRacedStatusAtomically(t *testing.T) {
	home := t.TempDir()
	store, err := OpenPath(home, filepath.Join(home, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()

	for _, id := range []string{"waiting-attach", "error-attach", "raced-attach"} {
		if _, err := store.CreateSession(ctx, CreateSessionInput{
			ID: id, Name: id, CWD: "/work", Agent: "claude", CapturedPath: "/bin",
			Status: "running", StatusSource: "hook", StatusAt: 100, CreatedAt: 1,
		}); err != nil {
			t.Fatal(err)
		}
	}

	if err := store.UpdateSessionStatus(ctx, StatusUpdateInput{
		SessionID: "waiting-attach", Status: "waiting", Reason: "permission_prompt",
		Source: "hook", At: 110, EventKind: "notification",
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.RecordAttachment(ctx, "waiting-attach", 120); err != nil {
		t.Fatal(err)
	}
	waiting, err := store.GetSession(ctx, "waiting-attach")
	if err != nil {
		t.Fatal(err)
	}
	if waiting.Status != "running" || waiting.StatusReason != "" || waiting.StatusSource != "user" || waiting.StatusAt != 120 || !waiting.Acknowledged || waiting.NotifyEpoch != 1 {
		t.Fatalf("waiting attachment = %#v", waiting)
	}

	exit := 137
	if err := store.UpdateSessionStatus(ctx, StatusUpdateInput{
		SessionID: "error-attach", Status: "error", Reason: "pane exited", Source: "tmux", At: 210,
		EventKind: "tmux.pane_crash", PaneExitStatus: &exit, CrashTail: "fatal output", LastMessage: "last hook message",
	}); err != nil {
		t.Fatal(err)
	}
	// Populate the remaining orthogonal fields to prove the targeted error
	// acknowledgement cannot accidentally reset them.
	if _, err := store.DB().Exec(`UPDATE sessions SET killed_by_user = 1, notify_epoch = 7 WHERE id = 'error-attach'`); err != nil {
		t.Fatal(err)
	}
	before, err := store.GetSession(ctx, "error-attach")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.RecordAttachment(ctx, "error-attach", 220); err != nil {
		t.Fatal(err)
	}
	after, err := store.GetSession(ctx, "error-attach")
	if err != nil {
		t.Fatal(err)
	}
	if !after.Acknowledged || after.Status != before.Status || after.StatusReason != before.StatusReason ||
		after.StatusSource != before.StatusSource || after.StatusAt != before.StatusAt ||
		after.KilledByUser != before.KilledByUser || after.NotifyEpoch != before.NotifyEpoch ||
		after.PaneExitStatus == nil || before.PaneExitStatus == nil || *after.PaneExitStatus != *before.PaneExitStatus ||
		after.CrashTail != before.CrashTail || after.LastMessage != before.LastMessage {
		t.Fatalf("error attachment changed verdict fields: before=%#v after=%#v", before, after)
	}

	if err := store.UpdateSessionStatus(ctx, StatusUpdateInput{
		SessionID: "raced-attach", Status: "error", Reason: "old error", Source: "hook", At: 310,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateSessionStatus(ctx, StatusUpdateInput{
		SessionID: "raced-attach", Status: "idle", Reason: "resolved", Source: "hook", At: 320,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.RecordAttachment(ctx, "raced-attach", 330); err != nil {
		t.Fatal(err)
	}
	raced, err := store.GetSession(ctx, "raced-attach")
	if err != nil {
		t.Fatal(err)
	}
	if raced.Status != "idle" || raced.StatusReason != "resolved" || raced.StatusSource != "hook" || raced.StatusAt != 320 || raced.Acknowledged || raced.NotifyEpoch != 1 {
		t.Fatalf("raced status was overwritten: %#v", raced)
	}
	var racedAttachEvents int
	if err := store.DB().QueryRow(`SELECT count(*) FROM events WHERE session_id = 'raced-attach' AND kind = 'attached'`).Scan(&racedAttachEvents); err != nil || racedAttachEvents != 0 {
		t.Fatalf("raced attachment events = %d, %v; want 0", racedAttachEvents, err)
	}
}

func TestStatusTransitionProtectsUserKillAndPersistsHookAndCrashFields(t *testing.T) {
	home := t.TempDir()
	store, err := OpenPath(home, filepath.Join(home, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	const id = "terminal-fields"
	if _, err := store.CreateSession(ctx, CreateSessionInput{
		ID: id, Name: "terminal fields", CWD: "/work", Agent: "claude", CapturedPath: "/bin",
		Status: "running", StatusSource: "hook", StatusAt: 1, CreatedAt: 1,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateSessionStatus(ctx, StatusUpdateInput{
		SessionID: id, Status: "stopped", Reason: "killed", Source: "user", At: 10,
		EventKind: "killed", KilledByUser: true,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateSessionStatus(ctx, StatusUpdateInput{
		SessionID: id, Status: "running", Source: "hook", At: 11, EventKind: "session.start",
	}); err != nil {
		t.Fatal(err)
	}
	got, err := store.GetSession(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "stopped" || !got.KilledByUser || got.StatusAt != 10 {
		t.Fatalf("hook undid terminal user kill: %#v", got)
	}
	var startEvents int
	if err := store.DB().QueryRow(`SELECT count(*) FROM events WHERE session_id = ? AND kind = 'session.start'`, id).Scan(&startEvents); err != nil || startEvents != 1 {
		t.Fatalf("protected hook event count = %d, %v; want 1", startEvents, err)
	}

	if err := store.UpdateSessionStatus(ctx, StatusUpdateInput{
		SessionID: id, Status: "starting", Source: "user", At: 12,
		EventKind: "resume", ClearKilledByUser: true,
	}); err != nil {
		t.Fatal(err)
	}
	message := strings.Repeat("x", 2047) + "€trailing"
	if err := store.UpdateSessionStatus(ctx, StatusUpdateInput{
		SessionID: id, Status: "idle", Source: "hook", At: 13,
		EventKind: "stop", LastMessage: message,
	}); err != nil {
		t.Fatal(err)
	}
	got, err = store.GetSession(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if got.KilledByUser || len(got.LastMessage) > 2*1024 || !strings.HasSuffix(got.LastMessage, "x") {
		t.Fatalf("resume/last message fields = killed:%v bytes:%d suffix:%q", got.KilledByUser, len(got.LastMessage), got.LastMessage[len(got.LastMessage)-1:])
	}

	exit := 137
	if err := store.UpdateSessionStatus(ctx, StatusUpdateInput{
		SessionID: id, Status: "error", Reason: "pane exit", Source: "tmux", At: 14,
		EventKind: "tmux.pane_crash", PaneExitStatus: &exit, CrashTail: "first tail",
	}); err != nil {
		t.Fatal(err)
	}
	otherExit := 1
	if err := store.UpdateSessionStatus(ctx, StatusUpdateInput{
		SessionID: id, Status: "error", Reason: "racing observer", Source: "tmux", At: 15,
		EventKind: "tmux.pane_crash", PaneExitStatus: &otherExit, CrashTail: "replacement",
	}); err != nil {
		t.Fatal(err)
	}
	got, err = store.GetSession(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if got.PaneExitStatus == nil || *got.PaneExitStatus != exit || got.CrashTail != "first tail" || got.StatusAt != 14 || got.Acknowledged {
		t.Fatalf("first-writer crash fields = %#v", got)
	}
}

func TestRecordOrphanEventUsesNullSessionAndRequiresTimestamp(t *testing.T) {
	home := t.TempDir()
	store, err := OpenPath(home, filepath.Join(home, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	if err := store.RecordOrphanEvent(ctx, EventInput{Kind: "notification"}); err == nil || !strings.Contains(err.Error(), "timestamp") {
		t.Fatalf("orphan without timestamp error = %v", err)
	}
	input := EventInput{At: 42, Kind: "notification", Reason: "permission_prompt", Payload: `{"unresolved":true}`}
	if err := store.RecordOrphanEvent(ctx, input); err != nil {
		t.Fatal(err)
	}
	var sessionID sql.NullString
	var at int64
	var kind, reason, payload string
	if err := store.DB().QueryRow(`SELECT session_id, at, kind, reason, payload FROM events`).Scan(&sessionID, &at, &kind, &reason, &payload); err != nil {
		t.Fatal(err)
	}
	if sessionID.Valid || at != input.At || kind != input.Kind || reason != input.Reason || payload != input.Payload {
		t.Fatalf("orphan event = session:%#v at:%d kind:%q reason:%q payload:%q", sessionID, at, kind, reason, payload)
	}
	var version int
	if err := store.DB().QueryRow(`SELECT version FROM meta WHERE key = 'schema_version'`).Scan(&version); err != nil || version != SchemaVersion {
		t.Fatalf("schema version changed: db=%d constant=%d err=%v", version, SchemaVersion, err)
	}
}

func TestUIStateAccessorsDegradeToDocumentedDefaults(t *testing.T) {
	home := t.TempDir()
	store, err := OpenPath(home, filepath.Join(home, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()

	mode, err := store.GetLayoutMode(ctx)
	if err != nil || mode != DefaultLayoutMode {
		t.Fatalf("GetLayoutMode before any write = %q, %v; want %q", mode, err, DefaultLayoutMode)
	}
	width, err := store.GetSidebarWidth(ctx)
	if err != nil || width != DefaultSidebarWidth {
		t.Fatalf("GetSidebarWidth before any write = %d, %v; want %d", width, err, DefaultSidebarWidth)
	}

	if err := store.SetLayoutMode(ctx, "stacked"); err != nil {
		t.Fatal(err)
	}
	if err := store.SetSidebarWidth(ctx, 50); err != nil {
		t.Fatal(err)
	}
	mode, err = store.GetLayoutMode(ctx)
	if err != nil || mode != "stacked" {
		t.Fatalf("GetLayoutMode after write = %q, %v; want %q", mode, err, "stacked")
	}
	width, err = store.GetSidebarWidth(ctx)
	if err != nil || width != 50 {
		t.Fatalf("GetSidebarWidth after write = %d, %v; want 50", width, err)
	}

	// Overwriting an existing key updates in place rather than duplicating a
	// row — ui_state.key is a PRIMARY KEY, exercised here via the accessor
	// rather than raw SQL.
	if err := store.SetSidebarWidth(ctx, 60); err != nil {
		t.Fatal(err)
	}
	var rows int
	if err := store.DB().QueryRow(`SELECT count(*) FROM ui_state WHERE key = 'sidebar_width'`).Scan(&rows); err != nil || rows != 1 {
		t.Fatalf("ui_state sidebar_width rows = %d, %v; want 1", rows, err)
	}
	width, err = store.GetSidebarWidth(ctx)
	if err != nil || width != 60 {
		t.Fatalf("GetSidebarWidth after overwrite = %d, %v; want 60", width, err)
	}
}

func TestOpenMigratesV1FixtureToUIStateWithoutRecreatingSessionRow(t *testing.T) {
	home := filepath.Join(t.TempDir(), "deck")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(home, "state.db")
	fixture, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	// Exactly schemaV1 (no ui_state) plus a real schema_version=1 marker and
	// one pre-existing session row, mirroring a database created by a
	// pre-task-010 binary.
	for _, statement := range schemaV1 {
		if _, err := fixture.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := fixture.Exec(`INSERT INTO meta (key, version) VALUES ('schema_version', 1)`); err != nil {
		t.Fatal(err)
	}
	const fixtureID = "v1-fixture-session"
	if _, err := fixture.Exec(`INSERT INTO sessions (
		id, name, slug, cwd, agent, captured_path, status, status_source, status_at, created_at
	) VALUES (?, 'kept', 'kept', '/tmp', 'shell', '/bin/sh', 'stopped', 'test', 1, 1)`, fixtureID); err != nil {
		t.Fatal(err)
	}
	if err := fixture.Close(); err != nil {
		t.Fatal(err)
	}

	store, err := OpenPath(home, path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	var version int
	if err := store.DB().QueryRow(`SELECT version FROM meta WHERE key = 'schema_version'`).Scan(&version); err != nil || version != SchemaVersion {
		t.Fatalf("migrated version = %d, %v; want %d", version, err, SchemaVersion)
	}
	var name string
	if err := store.DB().QueryRow(`SELECT name FROM sessions WHERE id = ?`, fixtureID).Scan(&name); err != nil || name != "kept" {
		t.Fatalf("session row after v1->v2 migration = %q, %v; want the original row still at id %s", name, err, fixtureID)
	}
	var sessionCount int
	if err := store.DB().QueryRow(`SELECT count(*) FROM sessions`).Scan(&sessionCount); err != nil || sessionCount != 1 {
		t.Fatalf("session count after migration = %d, %v; want 1 (no row recreated)", sessionCount, err)
	}
	var uiStateTables int
	if err := store.DB().QueryRow(`SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = 'ui_state'`).Scan(&uiStateTables); err != nil || uiStateTables != 1 {
		t.Fatalf("ui_state table after migration = %d, %v; want 1", uiStateTables, err)
	}

	// The accessors work against the freshly migrated database too.
	ctx := context.Background()
	mode, err := store.GetLayoutMode(ctx)
	if err != nil || mode != DefaultLayoutMode {
		t.Fatalf("GetLayoutMode after migration = %q, %v; want %q", mode, err, DefaultLayoutMode)
	}
}

func TestSlugContainsOnlyTmuxSafeASCII(t *testing.T) {
	if got := Slug("...:::"); got != "" {
		t.Fatalf("Slug punctuation = %q, want empty", got)
	}
	if got := Slug("München １２"); got != "m-nchen" {
		t.Fatalf("Slug = %q, want only ASCII [a-z0-9_-]", got)
	}
}

// TestListSessionsDefaultsWorkspaceToCWDBasename covers SPEC requirement 30's
// grouping key: sessions.workspace defaults to the basename of cwd when a
// row has never had one recorded (every CreateSession call today, since
// there is no create-time input for it yet), and an explicit column value
// — written directly, the only way a workspace ever gets set right now —
// is preserved verbatim rather than being overridden by the default. Never
// derived from anything "repo"-shaped.
func TestListSessionsDefaultsWorkspaceToCWDBasename(t *testing.T) {
	home := t.TempDir()
	st, err := OpenPath(home, filepath.Join(home, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	if _, err := st.CreateSession(ctx, CreateSessionInput{
		ID: "00000000-0000-4000-8000-0000000000a1", Name: "defaulted", CWD: "/work/svc-a",
		Agent: "shell", CapturedPath: "/bin", StatusAt: 100, CreatedAt: 100,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateSession(ctx, CreateSessionInput{
		ID: "00000000-0000-4000-8000-0000000000a2", Name: "explicit", CWD: "/work/svc-b",
		Agent: "shell", CapturedPath: "/bin", StatusAt: 101, CreatedAt: 101,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.DB().ExecContext(ctx, `UPDATE sessions SET workspace = ? WHERE id = ?`, "team-shared", "00000000-0000-4000-8000-0000000000a2"); err != nil {
		t.Fatal(err)
	}
	sessions, err := st.ListSessions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 2 {
		t.Fatalf("len(sessions) = %d, want 2", len(sessions))
	}
	if sessions[0].Workspace != "svc-a" {
		t.Fatalf("defaulted workspace = %q, want basename of cwd %q", sessions[0].Workspace, "svc-a")
	}
	if sessions[1].Workspace != "team-shared" {
		t.Fatalf("explicit workspace = %q, want the recorded value unchanged by the default", sessions[1].Workspace)
	}
	// GetSession goes through the same scanSession/sessionColumns path;
	// prove it independently rather than assuming ListSessions and GetSession
	// can never drift.
	got, err := st.GetSession(ctx, "00000000-0000-4000-8000-0000000000a1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Workspace != "svc-a" {
		t.Fatalf("GetSession workspace = %q, want %q", got.Workspace, "svc-a")
	}
}

// TestOpenMigratesV1V2V3FixturesToRecentCwdsWithoutRecreatingSessionRow covers
// task 006: schemaV4 (recent_cwds, SPEC §4:276-279) must be reachable by
// migration from schema versions 1, 2 and 3 alike, and none of those paths
// may recreate the pre-existing session row to gain the new table.
func TestOpenMigratesV1V2V3FixturesToRecentCwdsWithoutRecreatingSessionRow(t *testing.T) {
	for _, fromVersion := range []int{1, 2, 3} {
		fromVersion := fromVersion
		t.Run(fmt.Sprintf("fromV%d", fromVersion), func(t *testing.T) {
			home := filepath.Join(t.TempDir(), "deck")
			if err := os.MkdirAll(home, 0o755); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(home, "state.db")
			fixture, err := sql.Open("sqlite", path)
			if err != nil {
				t.Fatal(err)
			}
			for _, statement := range schemaV1 {
				if _, err := fixture.Exec(statement); err != nil {
					t.Fatal(err)
				}
			}
			if fromVersion >= 2 {
				for _, statement := range schemaV2 {
					if _, err := fixture.Exec(statement); err != nil {
						t.Fatal(err)
					}
				}
			}
			if fromVersion >= 3 {
				for _, statement := range schemaV3 {
					if _, err := fixture.Exec(statement); err != nil {
						t.Fatal(err)
					}
				}
			}
			if _, err := fixture.Exec(`INSERT INTO meta (key, version) VALUES ('schema_version', ?)`, fromVersion); err != nil {
				t.Fatal(err)
			}
			const fixtureID = "pre-existing-session"
			if _, err := fixture.Exec(`INSERT INTO sessions (
				id, name, slug, cwd, agent, captured_path, status, status_source, status_at, created_at
			) VALUES (?, 'kept', 'kept', '/tmp/kept', 'shell', '/bin/sh', 'stopped', 'test', 42, 42)`, fixtureID); err != nil {
				t.Fatal(err)
			}
			if err := fixture.Close(); err != nil {
				t.Fatal(err)
			}

			st, err := OpenPath(home, path)
			if err != nil {
				t.Fatalf("OpenPath migrating from v%d: %v", fromVersion, err)
			}
			defer st.Close()

			var version int
			if err := st.DB().QueryRow(`SELECT version FROM meta WHERE key = 'schema_version'`).Scan(&version); err != nil || version != SchemaVersion {
				t.Fatalf("migrated version = %d, %v; want %d", version, err, SchemaVersion)
			}
			var recentCwdsTables int
			if err := st.DB().QueryRow(`SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = 'recent_cwds'`).Scan(&recentCwdsTables); err != nil || recentCwdsTables != 1 {
				t.Fatalf("recent_cwds table after migration from v%d = %d, %v; want 1", fromVersion, recentCwdsTables, err)
			}

			ctx := context.Background()
			got, err := st.GetSession(ctx, fixtureID)
			if err != nil {
				t.Fatalf("GetSession after migration from v%d: %v", fromVersion, err)
			}
			if got.ID != fixtureID || got.Name != "kept" || got.Slug != "kept" || got.CWD != "/tmp/kept" ||
				got.Agent != "shell" || got.CapturedPath != "/bin/sh" || got.Status != "stopped" ||
				got.StatusSource != "test" || got.StatusAt != 42 || got.CreatedAt != 42 {
				t.Fatalf("session row after migration from v%d = %+v, want the byte-identical pre-existing row", fromVersion, got)
			}
			var sessionCount int
			if err := st.DB().QueryRow(`SELECT count(*) FROM sessions`).Scan(&sessionCount); err != nil || sessionCount != 1 {
				t.Fatalf("session count after migration from v%d = %d, %v; want 1 (no row recreated)", fromVersion, sessionCount, err)
			}
		})
	}
}

func assertMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Fatalf("mode for %s = %#o, want %#o", path, got, want)
	}
}

// openRecentCwdTestStore returns a fresh v4 store for the PromoteRecentCwd
// tests below. DECK_CLOCK is frozen so these tests can prove ordering comes
// from used_seq alone, never from wall time (task 007, SPEC §4/§13.1).
func openRecentCwdTestStore(t *testing.T) *Store {
	t.Helper()
	t.Setenv("DECK_CLOCK", "2025-06-01T00:00:00Z")
	home := filepath.Join(t.TempDir(), "deck")
	st, err := OpenPath(home, filepath.Join(home, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func TestPromoteRecentCwdOrdersMostRecentFirstByMonotonicSequence(t *testing.T) {
	st := openRecentCwdTestStore(t)
	ctx := context.Background()
	for _, path := range []string{"/a", "/b", "/c"} {
		if err := st.PromoteRecentCwd(ctx, path, 10); err != nil {
			t.Fatalf("promote %s: %v", path, err)
		}
	}
	got, err := st.RecentCwds(ctx)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"/c", "/b", "/a"}
	if len(got) != len(want) {
		t.Fatalf("recent cwds = %+v, want %d entries", got, len(want))
	}
	for i, path := range want {
		if got[i].Path != path {
			t.Fatalf("recent cwds[%d] = %q, want %q (full: %+v)", i, got[i].Path, path, got)
		}
	}
	// used_seq must strictly increase with promotion order (never a repeated
	// or clock-derived value) even though DECK_CLOCK never advances above.
	if !(got[0].UsedSeq > got[1].UsedSeq && got[1].UsedSeq > got[2].UsedSeq) {
		t.Fatalf("used_seq did not strictly increase in promotion order: %+v", got)
	}
}

func TestPromoteRecentCwdRepromotingExistingPathMovesToFrontWithoutDuplicating(t *testing.T) {
	st := openRecentCwdTestStore(t)
	ctx := context.Background()
	for _, path := range []string{"/a", "/b", "/c"} {
		if err := st.PromoteRecentCwd(ctx, path, 10); err != nil {
			t.Fatalf("promote %s: %v", path, err)
		}
	}
	if err := st.PromoteRecentCwd(ctx, "/a", 10); err != nil {
		t.Fatalf("re-promote /a: %v", err)
	}
	got, err := st.RecentCwds(ctx)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"/a", "/c", "/b"}
	if len(got) != len(want) {
		t.Fatalf("recent cwds = %+v, want %d entries (no duplicate /a)", got, len(want))
	}
	for i, path := range want {
		if got[i].Path != path {
			t.Fatalf("recent cwds[%d] = %q, want %q (full: %+v)", i, got[i].Path, path, got)
		}
	}
}

func TestPromoteRecentCwdLimitZeroKeepsNothing(t *testing.T) {
	st := openRecentCwdTestStore(t)
	ctx := context.Background()
	if err := st.PromoteRecentCwd(ctx, "/a", 0); err != nil {
		t.Fatalf("promote with limit 0: %v", err)
	}
	got, err := st.RecentCwds(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("recent cwds with limit 0 = %+v, want none", got)
	}
}

func TestPromoteRecentCwdEvictsBeyondCallerSuppliedLimit(t *testing.T) {
	st := openRecentCwdTestStore(t)
	ctx := context.Background()
	for _, path := range []string{"/a", "/b", "/c", "/d", "/e"} {
		if err := st.PromoteRecentCwd(ctx, path, 3); err != nil {
			t.Fatalf("promote %s: %v", path, err)
		}
	}
	got, err := st.RecentCwds(ctx)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"/e", "/d", "/c"}
	if len(got) != len(want) {
		t.Fatalf("recent cwds = %+v, want exactly the 3 most recent %v", got, want)
	}
	for i, path := range want {
		if got[i].Path != path {
			t.Fatalf("recent cwds[%d] = %q, want %q (full: %+v)", i, got[i].Path, path, got)
		}
	}
}

func TestPromoteRecentCwdRejectsRelativePath(t *testing.T) {
	st := openRecentCwdTestStore(t)
	ctx := context.Background()
	if err := st.PromoteRecentCwd(ctx, "relative/path", 5); err == nil {
		t.Fatal("promote of a relative path succeeded, want an error naming the requirement")
	}
	got, err := st.RecentCwds(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("recent cwds after rejected promotion = %+v, want none", got)
	}
}

// TestClearRecentCwdsRemovesEveryEntryButNothingElse covers task 013
// (requirement 17, §11.5): clearing the §11.7 directory history is a real,
// observable store mutation -- RecentCwds returns nothing afterward -- and
// it costs only that history, never a session: a session row created
// alongside the seeded recent_cwds entries survives ClearRecentCwds
// byte-for-byte, and a later PromoteRecentCwd (the create modal's own
// effect on a later create) still works, starting the table over from
// empty rather than erroring on some leftover state.
func TestClearRecentCwdsRemovesEveryEntryButNothingElse(t *testing.T) {
	st := openRecentCwdTestStore(t)
	ctx := context.Background()
	for _, path := range []string{"/a", "/b", "/c"} {
		if err := st.PromoteRecentCwd(ctx, path, 10); err != nil {
			t.Fatalf("promote %s: %v", path, err)
		}
	}
	session, err := st.CreateSession(ctx, CreateSessionInput{
		ID: "clear-recent-cwds-session", Name: "clear-recent-cwds-session", CWD: "/a",
		Agent: "shell", CapturedPath: "/bin", StatusAt: 100, CreatedAt: 100,
	})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if err := st.ClearRecentCwds(ctx); err != nil {
		t.Fatalf("clear recent cwds: %v", err)
	}
	got, err := st.RecentCwds(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("recent cwds after clear = %+v, want none", got)
	}
	rows, err := st.ListSessions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].ID != session.ID || rows[0].CWD != session.CWD {
		t.Fatalf("sessions after clearing recent cwds = %+v, want the untouched seeded session", rows)
	}
	if err := st.PromoteRecentCwd(ctx, "/z", 10); err != nil {
		t.Fatalf("promote after clear: %v", err)
	}
	got, err = st.RecentCwds(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Path != "/z" {
		t.Fatalf("recent cwds after post-clear promote = %+v, want exactly [/z]", got)
	}
}

// TestSetSessionEnvValueMergesMarksDirtyAndRecordsEventWithKeyOnly proves
// task 021's write path at the store layer: an edit merges into the
// session's existing env map (an untouched key survives), marks env_dirty,
// records a set_env event whose payload is the key name only (never the
// value -- the store's own never-log-a-value convention, task 019/024), and
// the edit survives a close/reopen exactly like every other durable field.
func TestSetSessionEnvValueMergesMarksDirtyAndRecordsEventWithKeyOnly(t *testing.T) {
	home := t.TempDir()
	st, err := OpenPath(home, filepath.Join(home, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	const id = "00000000-0000-4000-8000-000000000021"
	if _, err := st.CreateSession(ctx, CreateSessionInput{
		ID: id, Name: "env-edit", CWD: "/work/env-edit", Agent: "claude", CapturedPath: "/bin",
		StatusAt: 1, CreatedAt: 1, Env: map[string]string{"UNTOUCHED_KEY": "keep-me"},
	}); err != nil {
		t.Fatal(err)
	}

	if err := st.SetSessionEnvValue(ctx, id, "EDITED_KEY", "top-secret-value", "user", 10); err != nil {
		t.Fatal(err)
	}
	got, err := st.GetSession(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if got.Env["EDITED_KEY"] != "top-secret-value" {
		t.Fatalf("env[EDITED_KEY] = %q, want top-secret-value", got.Env["EDITED_KEY"])
	}
	if got.Env["UNTOUCHED_KEY"] != "keep-me" {
		t.Fatalf("env[UNTOUCHED_KEY] = %q, want it left alone by an edit to a different key", got.Env["UNTOUCHED_KEY"])
	}
	if !got.EnvDirty {
		t.Fatalf("env_dirty = false, want true after SetSessionEnvValue")
	}

	var payload string
	if err := st.DB().QueryRowContext(ctx, `SELECT payload FROM events WHERE session_id = ? AND kind = 'set_env'`, id).Scan(&payload); err != nil {
		t.Fatalf("read set_env event: %v", err)
	}
	if payload != "EDITED_KEY" {
		t.Fatalf("set_env event payload = %q, want exactly the key name %q (never the value)", payload, "EDITED_KEY")
	}
	if strings.Contains(payload, "top-secret-value") {
		t.Fatalf("set_env event payload leaks the value: %q", payload)
	}

	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenPath(home, filepath.Join(home, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	got, err = reopened.GetSession(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if got.Env["EDITED_KEY"] != "top-secret-value" || !got.EnvDirty {
		t.Fatalf("edit did not survive reopen: env=%+v envDirty=%v", got.Env, got.EnvDirty)
	}
}

// TestSetSessionEnvValueRejectsMissingSessionOrKey proves the same
// input-validation shape every other Set* mutator in this package uses.
func TestSetSessionEnvValueRejectsMissingSessionOrKey(t *testing.T) {
	home := t.TempDir()
	st, err := OpenPath(home, filepath.Join(home, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	if err := st.SetSessionEnvValue(ctx, "", "KEY", "value", "user", 1); err == nil {
		t.Fatalf("expected an error for a missing session id")
	}
	if err := st.SetSessionEnvValue(ctx, "some-id", "", "value", "user", 1); err == nil {
		t.Fatalf("expected an error for a missing key")
	}
	if err := st.SetSessionEnvValue(ctx, "does-not-exist", "KEY", "value", "user", 1); err == nil {
		t.Fatalf("expected an error for a session that does not exist")
	}
}

// TestClearEnvDirtyResetsFlagAndPersistsAcrossReopen proves task 022's `R`
// restart's own store-side half: ClearEnvDirty resets env_dirty back to
// false (the sidebar's `env↻` badge source) without touching the
// session's own env map, records an event naming the restart source, and
// the cleared flag survives a close/reopen exactly like every other
// durable column.
func TestClearEnvDirtyResetsFlagAndPersistsAcrossReopen(t *testing.T) {
	home := t.TempDir()
	st, err := OpenPath(home, filepath.Join(home, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	const id = "00000000-0000-4000-8000-000000000022"
	if _, err := st.CreateSession(ctx, CreateSessionInput{
		ID: id, Name: "env-dirty-clear", CWD: "/work/env-dirty-clear", Agent: "claude", CapturedPath: "/bin",
		StatusAt: 1, CreatedAt: 1, Env: map[string]string{"SOME_KEY": "some-value"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.SetSessionEnvValue(ctx, id, "SOME_KEY", "edited-value", "user", 10); err != nil {
		t.Fatal(err)
	}
	got, err := st.GetSession(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if !got.EnvDirty {
		t.Fatalf("env_dirty = false, want true after an edit, before ClearEnvDirty")
	}

	if err := st.ClearEnvDirty(ctx, id, 20); err != nil {
		t.Fatal(err)
	}
	got, err = st.GetSession(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if got.EnvDirty {
		t.Fatalf("env_dirty = true, want false after ClearEnvDirty")
	}
	if got.Env["SOME_KEY"] != "edited-value" {
		t.Fatalf("ClearEnvDirty changed env[SOME_KEY] = %q, want it untouched (edited-value)", got.Env["SOME_KEY"])
	}

	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenPath(home, filepath.Join(home, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	got, err = reopened.GetSession(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if got.EnvDirty {
		t.Fatalf("cleared env_dirty did not survive reopen")
	}
}

// TestClearEnvDirtyRejectsMissingSessionOrUnknownID mirrors
// TestSetSessionEnvValueRejectsMissingSessionOrKey's input-validation shape.
func TestClearEnvDirtyRejectsMissingSessionOrUnknownID(t *testing.T) {
	home := t.TempDir()
	st, err := OpenPath(home, filepath.Join(home, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	if err := st.ClearEnvDirty(ctx, "", 1); err == nil {
		t.Fatalf("expected an error for a missing session id")
	}
	if err := st.ClearEnvDirty(ctx, "does-not-exist", 1); err == nil {
		t.Fatalf("expected an error for a session that does not exist")
	}
	if err := st.ClearEnvDirty(ctx, "some-id", 0); err == nil {
		t.Fatalf("expected an error for a missing timestamp")
	}
}
