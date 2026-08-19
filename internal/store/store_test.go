package store

import (
	"context"
	"database/sql"
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
	for _, table := range []string{"sessions", "events", "meta"} {
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
	if _, err := fixture.Exec(`CREATE TABLE meta (key TEXT PRIMARY KEY, version INTEGER NOT NULL); INSERT INTO meta VALUES ('schema_version', 2)`); err != nil {
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
	if err := store.DB().QueryRow(`SELECT version FROM meta WHERE key = 'schema_version'`).Scan(&version); err != nil || version != 1 || SchemaVersion != 1 {
		t.Fatalf("schema version changed: db=%d constant=%d err=%v", version, SchemaVersion, err)
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
