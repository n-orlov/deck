package store

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// sessionColumnsPreV8 is every sessions column that exists once schemaV1-7
// have all landed but schemaV8's own pinned_at has not -- the exact shape
// buildSchemaV7PopulatedFixture below produces. It deliberately omits
// pinned_at (schemaV8, this task) and workspace (dropped by schemaV7), and
// is used to read the SAME set of columns both before and after migrating
// to schemaV8, so the "every other column value is byte-identical"
// assertion compares apples to apples rather than tripping over the new
// column's own absence pre-migration.
var sessionColumnsPreV8 = []string{
	"id", "name", "slug", "cwd", "agent", "launch_args", "env", "env_dirty",
	"captured_path", "pre_launch", "login_shell", "permission_profile",
	"permission_profile_reason", "conversation_id", "resume_pin", "resume_state",
	"status", "status_reason", "status_source", "status_at", "killed_by_user",
	"pane_exit_status", "crash_tail", "notify_epoch", "last_message", "sensitive",
	"notify_rules", "important", "snoozed_until", "acknowledged",
	"launch_lease_owner", "launch_lease_until", "created_at", "last_attached_at",
	"archived_at", "deleted_at", "last_probe_at", "post_destroy", "launch_dirty",
	"group_id",
}

// sessionColumnsKeptByV10 is sessionColumnsPreV8 without notify_rules and
// snoozed_until, the two columns schemaV10 (R228) drops. Every migration test
// that carries a populated database up to the current SchemaVersion compares
// this set before and after; the dropped pair is covered by
// TestSchemaV10MigratesPopulatedV9Database, which asserts they are gone.
var sessionColumnsKeptByV10 = func() []string {
	var kept []string
	for _, column := range sessionColumnsPreV8 {
		if column != "notify_rules" && column != "snoozed_until" {
			kept = append(kept, column)
		}
	}
	return kept
}()

// buildSchemaV7PopulatedFixture creates a fresh, standalone schemaV1-V7
// database file at path (schema_version = 7) and seeds it with a groups
// row plus four real-looking session rows that between them cover every
// state TestSchemaV8MigratesPopulatedV7Database is required to exercise:
// a group membership, a non-trivial env map, a resume_pin, an archived
// row (archived_at != 0) and a tombstoned row (deleted_at != 0). Returns
// the four session ids in insertion order.
func buildSchemaV7PopulatedFixture(t *testing.T, home, path string) [4]string {
	t.Helper()
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	fixture, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	for _, ladder := range [][]string{schemaV1, schemaV2, schemaV3, schemaV4, schemaV5, schemaV6, schemaV7} {
		for _, statement := range ladder {
			if _, err := fixture.Exec(statement); err != nil {
				t.Fatalf("build schemaV7 fixture: %v", err)
			}
		}
	}
	if _, err := fixture.Exec(`INSERT INTO meta (key, version) VALUES ('schema_version', 7)`); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.Exec(`INSERT INTO groups (id, name) VALUES (1, 'infra')`); err != nil {
		t.Fatal(err)
	}

	ids := [4]string{
		"v7-fixture-session-a", "v7-fixture-session-b", "v7-fixture-session-c", "v7-fixture-session-d",
	}
	rows := []struct {
		name, slug, env, resumePin string
		groupID                    sql.NullInt64
		archivedAt, deletedAt      int64
	}{
		// a: grouped, real env, a resume pin, live.
		{name: "alpha", slug: "alpha", env: `{"FOO":"bar","STAGE":"prod"}`, resumePin: "abc123",
			groupID: sql.NullInt64{Int64: 1, Valid: true}, archivedAt: 0, deletedAt: 0},
		// b: ungrouped (default), no env entries, archived.
		{name: "bravo", slug: "bravo", env: `{}`, resumePin: "",
			groupID: sql.NullInt64{}, archivedAt: 5000, deletedAt: 0},
		// c: grouped, tombstoned.
		{name: "charlie", slug: "charlie", env: `{"PATH":"/usr/local/bin"}`, resumePin: "def456",
			groupID: sql.NullInt64{Int64: 1, Valid: true}, archivedAt: 0, deletedAt: 9000},
		// d: ungrouped, live, exercises the other nullable/text columns.
		{name: "delta", slug: "delta", env: `{"KEY":"value"}`, resumePin: "",
			groupID: sql.NullInt64{}, archivedAt: 0, deletedAt: 0},
	}
	for i, id := range ids {
		row := rows[i]
		if _, err := fixture.Exec(`INSERT INTO sessions (
			id, name, slug, cwd, agent, launch_args, env, env_dirty, captured_path, pre_launch,
			login_shell, permission_profile, permission_profile_reason, conversation_id, resume_pin,
			resume_state, status, status_reason, status_source, status_at, killed_by_user,
			pane_exit_status, crash_tail, notify_epoch, last_message, sensitive, notify_rules,
			important, snoozed_until, acknowledged, launch_lease_owner, launch_lease_until,
			created_at, last_attached_at, archived_at, deleted_at, last_probe_at, post_destroy,
			launch_dirty, group_id
		) VALUES (
			?, ?, ?, ?, 'claude', '["--flag"]', ?, 1, ?, 'echo pre',
			0, 'safe', 'reasoned', ?, ?,
			'auto', 'running', 'because', 'probe', ?, 0,
			NULL, 'tail text', 3, 'hi there', 0, '{"rule":"x"}',
			1, 0, 1, 'owner@boot#1', 1234,
			?, ?, ?, ?, 42, 'echo post',
			0, ?
		)`,
			id, row.name, row.slug, "/work/"+row.name, row.env, "/tmp/"+row.name,
			"conv-"+row.name, row.resumePin, int64(i+1)*1000,
			int64(i+1)*1000, int64(i+1)*1000, row.archivedAt, row.deletedAt,
			row.groupID); err != nil {
			t.Fatalf("seed populated fixture session %d (%s): %v", i, row.name, err)
		}
	}
	if err := fixture.Close(); err != nil {
		t.Fatal(err)
	}
	return ids
}

// sessionColumnName is the shape of a column name snapshotSessionColumns accepts.
var sessionColumnName = regexp.MustCompile(`^[a-z_][a-z0-9_]*$`)

// snapshotSessionColumns reads columns (id first) from every row in
// sessions, ordered by id, scanning each value into a sql.NullString --
// database/sql's standard int64/float64->string conversion makes this a
// faithful, type-agnostic byte-for-byte capture of what the column reads
// back as, regardless of whether the underlying SQLite storage class is
// INTEGER, TEXT or NULL. Returned keyed by id.
func snapshotSessionColumns(t *testing.T, db *sql.DB, columns []string) map[string][]sql.NullString {
	t.Helper()
	// Column names are identifiers, which a query parameter cannot carry: every
	// caller passes literal column names, and each is checked here to be a bare
	// identifier before it is spliced into the statement.
	for _, column := range columns {
		if !sessionColumnName.MatchString(column) {
			t.Fatalf("snapshot sessions columns: %q is not a bare column name", column)
		}
	}
	rows, err := db.Query(`SELECT ` + strings.Join(columns, ", ") + ` FROM sessions ORDER BY id`) //nolint:gosec // G202: identifiers validated above, they cannot be bound parameters
	if err != nil {
		t.Fatalf("snapshot sessions columns: %v", err)
	}
	defer rows.Close()
	result := map[string][]sql.NullString{}
	for rows.Next() {
		values := make([]sql.NullString, len(columns))
		ptrs := make([]any, len(columns))
		for i := range values {
			ptrs[i] = &values[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			t.Fatalf("scan snapshot row: %v", err)
		}
		result[values[0].String] = values
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate snapshot rows: %v", err)
	}
	return result
}

// TestSchemaV8MigratesPopulatedV7Database is task 002's required migration
// proof: a schemaV7 fixture holding four real-looking session rows --
// spanning a group membership, a non-trivial env map, a resume_pin, an
// archived row and a tombstoned row -- migrates, via schemaV8's single
// ALTER TABLE statement, to schema version 8 with every row's pinned_at
// reading back 0 and every other column value byte-identical to what it
// read before the migration ran.
func TestSchemaV8MigratesPopulatedV7Database(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, "state.db")
	ids := buildSchemaV7PopulatedFixture(t, home, path)

	before, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	beforeSnapshot := snapshotSessionColumns(t, before, sessionColumnsKeptByV10)
	if len(beforeSnapshot) != len(ids) {
		t.Fatalf("pre-migration snapshot has %d rows, want %d", len(beforeSnapshot), len(ids))
	}
	if err := before.Close(); err != nil {
		t.Fatal(err)
	}

	st, err := OpenPath(home, path)
	if err != nil {
		t.Fatalf("open v7 populated fixture for migration: %v", err)
	}
	defer st.Close()

	var version int
	if err := st.DB().QueryRow(`SELECT version FROM meta WHERE key = 'schema_version'`).Scan(&version); err != nil || version != SchemaVersion {
		t.Fatalf("migrated version = %d, %v; want %d", version, err, SchemaVersion)
	}
	if SchemaVersion < 8 {
		t.Fatalf("SchemaVersion = %d, want at least 8 (schemaV8 is one rung of the ladder)", SchemaVersion)
	}

	cols := tableColumnSet(t, st.DB(), "sessions")
	if !cols["pinned_at"] {
		t.Fatal("sessions.pinned_at missing after migrating to schemaV8")
	}

	afterSnapshot := snapshotSessionColumns(t, st.DB(), sessionColumnsKeptByV10)
	if len(afterSnapshot) != len(ids) {
		t.Fatalf("post-migration snapshot has %d rows, want %d", len(afterSnapshot), len(ids))
	}
	for _, id := range ids {
		beforeRow, ok := beforeSnapshot[id]
		if !ok {
			t.Fatalf("row %s missing from pre-migration snapshot", id)
		}
		afterRow, ok := afterSnapshot[id]
		if !ok {
			t.Fatalf("row %s missing from post-migration snapshot", id)
		}
		for i, column := range sessionColumnsKeptByV10 {
			if beforeRow[i] != afterRow[i] {
				t.Fatalf("row %s column %s changed across migration: before %+v, after %+v", id, column, beforeRow[i], afterRow[i])
			}
		}
		var pinnedAt int64
		if err := st.DB().QueryRow(`SELECT pinned_at FROM sessions WHERE id = ?`, id).Scan(&pinnedAt); err != nil {
			t.Fatalf("read migrated row %s pinned_at: %v", id, err)
		}
		if pinnedAt != 0 {
			t.Fatalf("row %s pinned_at = %d, want 0", id, pinnedAt)
		}
	}

	sessions, err := st.ListSessionsIncludingArchived(context.Background())
	if err != nil {
		t.Fatalf("list sessions after migration: %v", err)
	}
	// ListSessionsIncludingArchived still excludes tombstoned rows
	// (deleted_at != 0, "charlie"), so 3 of the 4 fixture rows come back.
	if len(sessions) != 3 {
		t.Fatalf("len(sessions) after migration = %d, want 3", len(sessions))
	}
	for _, s := range sessions {
		if s.PinnedAt != 0 {
			t.Fatalf("session %s PinnedAt = %d, want 0", s.Name, s.PinnedAt)
		}
	}
}

// TestSchemaV8ReopenIsNoOp is task 002's required idempotency proof: a
// database already at schema version 8 (produced by migrating the v7
// fixture once) reopens a second time as a pure no-op migration -- schema
// version stays 8, and every row (including one whose pinned_at was set
// to a non-zero value between the two opens, proving a reopen never
// resets it) reads back byte-identical.
func TestSchemaV8ReopenIsNoOp(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, "state.db")
	ids := buildSchemaV7PopulatedFixture(t, home, path)

	first, err := OpenPath(home, path)
	if err != nil {
		t.Fatalf("first open (v7 -> v8 migration): %v", err)
	}
	var versionAfterFirstOpen int
	if err := first.DB().QueryRow(`SELECT version FROM meta WHERE key = 'schema_version'`).Scan(&versionAfterFirstOpen); err != nil || versionAfterFirstOpen != SchemaVersion {
		t.Fatalf("version after first open = %d, %v; want %d", versionAfterFirstOpen, err, SchemaVersion)
	}
	// Set a non-zero pinned_at on one row directly, outside any pin
	// feature code (task 002 lands the column, not the pin action), so
	// this test can prove the second open's no-op migration leaves an
	// already-set value alone rather than resetting it back to a default.
	if _, err := first.DB().Exec(`UPDATE sessions SET pinned_at = 7777 WHERE id = ?`, ids[0]); err != nil {
		t.Fatalf("set pinned_at on %s: %v", ids[0], err)
	}
	allColumnsPostV8 := append(append([]string{}, sessionColumnsKeptByV10...), "pinned_at")
	beforeSecondOpen := snapshotSessionColumns(t, first.DB(), allColumnsPostV8)
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}

	second, err := OpenPath(home, path)
	if err != nil {
		t.Fatalf("second open (already v8): %v", err)
	}
	defer second.Close()

	var versionAfterSecondOpen int
	if err := second.DB().QueryRow(`SELECT version FROM meta WHERE key = 'schema_version'`).Scan(&versionAfterSecondOpen); err != nil || versionAfterSecondOpen != SchemaVersion {
		t.Fatalf("version after second open = %d, %v; want %d", versionAfterSecondOpen, err, SchemaVersion)
	}

	afterSecondOpen := snapshotSessionColumns(t, second.DB(), allColumnsPostV8)
	if len(afterSecondOpen) != len(ids) {
		t.Fatalf("post-reopen snapshot has %d rows, want %d", len(afterSecondOpen), len(ids))
	}
	for _, id := range ids {
		beforeRow, ok := beforeSecondOpen[id]
		if !ok {
			t.Fatalf("row %s missing before the second open", id)
		}
		afterRow, ok := afterSecondOpen[id]
		if !ok {
			t.Fatalf("row %s missing after the second open", id)
		}
		for i, column := range allColumnsPostV8 {
			if beforeRow[i] != afterRow[i] {
				t.Fatalf("row %s column %s changed across a no-op reopen: before %+v, after %+v", id, column, beforeRow[i], afterRow[i])
			}
		}
	}
	var reopenedPinnedAt int64
	if err := second.DB().QueryRow(`SELECT pinned_at FROM sessions WHERE id = ?`, ids[0]).Scan(&reopenedPinnedAt); err != nil {
		t.Fatalf("read reopened pinned_at: %v", err)
	}
	if reopenedPinnedAt != 7777 {
		t.Fatalf("reopened pinned_at = %d, want 7777 (unchanged no-op reopen)", reopenedPinnedAt)
	}
}
