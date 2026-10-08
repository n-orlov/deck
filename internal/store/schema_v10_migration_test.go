package store

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"reflect"
	"testing"
)

// buildSchemaV9PopulatedFixture is buildSchemaV8PopulatedFixture advanced one
// rung: the same four rows, migrated by schemaV9 and stamped schema version 9.
func buildSchemaV9PopulatedFixture(t *testing.T, home, path string) [4]string {
	t.Helper()
	ids := buildSchemaV8PopulatedFixture(t, home, path)
	fixture, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer fixture.Close()
	for _, statement := range schemaV9 {
		if _, err := fixture.Exec(statement); err != nil {
			t.Fatalf("build schemaV9 fixture: %v", err)
		}
	}
	if _, err := fixture.Exec(`UPDATE meta SET version = 9 WHERE key = 'schema_version'`); err != nil {
		t.Fatal(err)
	}
	return ids
}

// schemaShape reads every schema object (table, index, trigger, view) as
// type|name|table|sql, sorted by name, plus sessions' column list in order.
func schemaShape(t *testing.T, db *sql.DB) (objects []string, columns [][]string) {
	t.Helper()
	rows, err := db.Query(`SELECT type, name, tbl_name, COALESCE(sql, '') FROM sqlite_master
		WHERE name NOT LIKE 'sqlite_%' ORDER BY type, name`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var kind, name, table, ddl string
		if err := rows.Scan(&kind, &name, &table, &ddl); err != nil {
			t.Fatal(err)
		}
		objects = append(objects, kind+"|"+name+"|"+table+"|"+ddl)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if err := rows.Close(); err != nil {
		t.Fatal(err)
	}
	info, err := db.Query(`SELECT cid, name, type, "notnull", COALESCE(dflt_value, '<none>'), pk
		FROM pragma_table_info('sessions') ORDER BY cid`)
	if err != nil {
		t.Fatal(err)
	}
	defer info.Close()
	for info.Next() {
		var cid, notNull, pk int
		var name, colType, dflt string
		if err := info.Scan(&cid, &name, &colType, &notNull, &dflt, &pk); err != nil {
			t.Fatal(err)
		}
		columns = append(columns, []string{name, colType, dflt, fmt.Sprint(notNull), fmt.Sprint(pk)})
	}
	return objects, columns
}

// sessionsColumnSet is the set of column names on the sessions table.
func sessionsColumnSet(t *testing.T, db *sql.DB) map[string]bool {
	t.Helper()
	_, columns := schemaShape(t, db)
	set := map[string]bool{}
	for _, column := range columns {
		set[column[0]] = true
	}
	return set
}

// TestSchemaV10MigratesPopulatedV9Database is R228's migration proof: a
// schema 9 database with four real-looking rows migrates to schema 10 with
// notify_rules and snoozed_until gone, no outbox table, the three event-hook
// columns present and NULL on every row, and every other column byte-identical.
func TestSchemaV10MigratesPopulatedV9Database(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, "state.db")
	ids := buildSchemaV9PopulatedFixture(t, home, path)
	kept := append(append([]string{}, sessionColumnsKeptByV10...), "pinned_at", "hook_executable")

	before, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	// The v9 database really carries the columns the migration drops.
	if cols := sessionsColumnSet(t, before); !cols["notify_rules"] || !cols["snoozed_until"] {
		t.Fatalf("v9 fixture lacks notify_rules/snoozed_until: %v", cols)
	}
	if _, err := before.Exec(`UPDATE sessions SET hook_executable = '/opt/deck/deck' WHERE id = ?`, ids[0]); err != nil {
		t.Fatal(err)
	}
	beforeSnapshot := snapshotSessionColumns(t, before, kept)
	if err := before.Close(); err != nil {
		t.Fatal(err)
	}

	st, err := OpenPath(home, path)
	if err != nil {
		t.Fatalf("open v9 populated fixture for migration: %v", err)
	}
	defer st.Close()

	var version int
	if err := st.DB().QueryRow(`SELECT version FROM meta WHERE key = 'schema_version'`).Scan(&version); err != nil || version != SchemaVersion {
		t.Fatalf("migrated version = %d, %v; want %d", version, err, SchemaVersion)
	}
	if SchemaVersion < 10 {
		t.Fatalf("SchemaVersion = %d, want at least 10 (schemaV10 is one rung of the ladder)", SchemaVersion)
	}
	cols := sessionsColumnSet(t, st.DB())
	for _, gone := range []string{"notify_rules", "snoozed_until"} {
		if cols[gone] {
			t.Fatalf("sessions.%s survives the schemaV10 migration", gone)
		}
	}
	for _, added := range []string{"event_hook_enabled", "event_hook_events", "hook_fired"} {
		if !cols[added] {
			t.Fatalf("sessions.%s missing after the schemaV10 migration", added)
		}
	}
	var outbox int
	if err := st.DB().QueryRow(`SELECT count(*) FROM sqlite_master WHERE name = 'outbox'`).Scan(&outbox); err != nil || outbox != 0 {
		t.Fatalf("outbox objects = %d, %v; want 0", outbox, err)
	}

	after := snapshotSessionColumns(t, st.DB(), kept)
	if len(after) != len(ids) {
		t.Fatalf("post-migration snapshot has %d rows, want %d", len(after), len(ids))
	}
	for _, id := range ids {
		for i, column := range kept {
			if beforeSnapshot[id][i] != after[id][i] {
				t.Fatalf("row %s column %s changed across migration: before %+v, after %+v", id, column, beforeSnapshot[id][i], after[id][i])
			}
		}
		var enabled sql.NullInt64
		var events, fired sql.NullString
		if err := st.DB().QueryRow(`SELECT event_hook_enabled, event_hook_events, hook_fired FROM sessions WHERE id = ?`, id).Scan(&enabled, &events, &fired); err != nil {
			t.Fatal(err)
		}
		if enabled.Valid || events.Valid || fired.Valid {
			t.Fatalf("row %s event-hook columns = %+v %+v %+v; want all NULL", id, enabled, events, fired)
		}
	}

	sessions, err := st.ListSessionsIncludingArchived(context.Background())
	if err != nil || len(sessions) != 3 {
		t.Fatalf("list after migration: %d sessions, %v; want 3", len(sessions), err)
	}
}

// TestSchemaV10FreshDatabaseEqualsMigratedDatabase: a database created fresh
// and a v9 database migrated up have the identical schema -- every object's
// DDL and every sessions column's name, type, default, nullability and order.
func TestSchemaV10FreshDatabaseEqualsMigratedDatabase(t *testing.T) {
	freshHome := t.TempDir()
	fresh, err := OpenPath(freshHome, filepath.Join(freshHome, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer fresh.Close()

	migratedHome := t.TempDir()
	migratedPath := filepath.Join(migratedHome, "state.db")
	buildSchemaV9PopulatedFixture(t, migratedHome, migratedPath)
	migrated, err := OpenPath(migratedHome, migratedPath)
	if err != nil {
		t.Fatal(err)
	}
	defer migrated.Close()

	freshObjects, freshColumns := schemaShape(t, fresh.DB())
	migratedObjects, migratedColumns := schemaShape(t, migrated.DB())
	if len(freshColumns) == 0 {
		t.Fatal("fresh database reports no sessions columns")
	}
	if !reflect.DeepEqual(freshObjects, migratedObjects) {
		t.Fatalf("schema objects differ:\nfresh    %q\nmigrated %q", freshObjects, migratedObjects)
	}
	if !reflect.DeepEqual(freshColumns, migratedColumns) {
		t.Fatalf("sessions columns differ:\nfresh    %q\nmigrated %q", freshColumns, migratedColumns)
	}
}

// TestSchemaV10ReopenIsNoOp: a database already carrying the new columns (and
// values in them) reopens as a no-op -- version unchanged, nothing reset.
func TestSchemaV10ReopenIsNoOp(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, "state.db")
	ids := buildSchemaV9PopulatedFixture(t, home, path)

	first, err := OpenPath(home, path)
	if err != nil {
		t.Fatalf("first open (v9 -> v10 migration): %v", err)
	}
	if _, err := first.DB().Exec(`UPDATE sessions SET event_hook_enabled = 1,
		event_hook_events = '["waiting","error"]', hook_fired = '[["waiting","permission_prompt"]]' WHERE id = ?`, ids[0]); err != nil {
		t.Fatal(err)
	}
	all := append(append([]string{}, sessionColumnsKeptByV10...), "pinned_at", "hook_executable",
		"event_hook_enabled", "event_hook_events", "hook_fired")
	beforeSnapshot := snapshotSessionColumns(t, first.DB(), all)
	beforeObjects, beforeColumns := schemaShape(t, first.DB())
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}

	second, err := OpenPath(home, path)
	if err != nil {
		t.Fatalf("second open (already v10): %v", err)
	}
	defer second.Close()
	var version int
	if err := second.DB().QueryRow(`SELECT version FROM meta WHERE key = 'schema_version'`).Scan(&version); err != nil || version != SchemaVersion {
		t.Fatalf("version after second open = %d, %v; want %d", version, err, SchemaVersion)
	}
	afterSnapshot := snapshotSessionColumns(t, second.DB(), all)
	afterObjects, afterColumns := schemaShape(t, second.DB())
	if !reflect.DeepEqual(beforeObjects, afterObjects) || !reflect.DeepEqual(beforeColumns, afterColumns) {
		t.Fatal("schema changed across a no-op reopen")
	}
	for _, id := range ids {
		for i, column := range all {
			if beforeSnapshot[id][i] != afterSnapshot[id][i] {
				t.Fatalf("row %s column %s changed across reopen: before %+v, after %+v", id, column, beforeSnapshot[id][i], afterSnapshot[id][i])
			}
		}
	}
	var fired string
	if err := second.DB().QueryRow(`SELECT hook_fired FROM sessions WHERE id = ?`, ids[0]).Scan(&fired); err != nil || fired != `[["waiting","permission_prompt"]]` {
		t.Fatalf("hook_fired after reopen = %q, %v; want the recorded value", fired, err)
	}
}

func hookFiredOf(t *testing.T, st *Store, id string) sql.NullString {
	t.Helper()
	var fired sql.NullString
	if err := st.DB().QueryRow(`SELECT hook_fired FROM sessions WHERE id = ?`, id).Scan(&fired); err != nil {
		t.Fatal(err)
	}
	return fired
}

// TestHookFiredClearsWhenNotifyEpochAdvances: hook_fired belongs to one
// notify_epoch. It survives a write that leaves the epoch alone and is cleared
// by both paths that advance it -- an attention state resolving through a
// status verdict, and an attachment answering a waiting session.
func TestHookFiredClearsWhenNotifyEpochAdvances(t *testing.T) {
	home := t.TempDir()
	st, err := OpenPath(home, filepath.Join(home, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	const fired = `[["waiting","permission_prompt"]]`

	for _, id := range []string{"via-verdict", "via-attach"} {
		if _, err := st.CreateSession(ctx, CreateSessionInput{
			ID: id, Name: id, CWD: "/work", Agent: "claude", CapturedPath: "/bin",
			Status: "running", StatusSource: "hook", StatusAt: 100, CreatedAt: 1,
		}); err != nil {
			t.Fatal(err)
		}
		if err := st.UpdateSessionStatus(ctx, StatusUpdateInput{
			SessionID: id, Status: "waiting", Reason: "permission_prompt", Source: "hook", At: 110, EventKind: "notification",
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := st.DB().Exec(`UPDATE sessions SET hook_fired = ? WHERE id = ?`, fired, id); err != nil {
			t.Fatal(err)
		}
	}

	// Same attention episode: waiting -> error keeps the epoch, keeps hook_fired.
	if err := st.UpdateSessionStatus(ctx, StatusUpdateInput{
		SessionID: "via-verdict", Status: "error", Reason: "boom", Source: "hook", At: 120, EventKind: "stop_failure",
	}); err != nil {
		t.Fatal(err)
	}
	got, err := st.GetSession(ctx, "via-verdict")
	if err != nil || got.NotifyEpoch != 0 {
		t.Fatalf("epoch after attention->attention = %d, %v; want 0", got.NotifyEpoch, err)
	}
	if f := hookFiredOf(t, st, "via-verdict"); f.String != fired {
		t.Fatalf("hook_fired within one epoch = %+v; want it kept", f)
	}

	// The attention state resolves: the epoch advances and hook_fired clears.
	if err := st.UpdateSessionStatus(ctx, StatusUpdateInput{
		SessionID: "via-verdict", Status: "running", Source: "hook", At: 130, EventKind: "prompt",
	}); err != nil {
		t.Fatal(err)
	}
	got, err = st.GetSession(ctx, "via-verdict")
	if err != nil || got.NotifyEpoch != 1 {
		t.Fatalf("epoch after resolving = %d, %v; want 1", got.NotifyEpoch, err)
	}
	if f := hookFiredOf(t, st, "via-verdict"); f.Valid {
		t.Fatalf("hook_fired after the epoch advanced by a verdict = %+v; want NULL", f)
	}

	if err := st.RecordAttachment(ctx, "via-attach", 140); err != nil {
		t.Fatal(err)
	}
	got, err = st.GetSession(ctx, "via-attach")
	if err != nil || got.NotifyEpoch != 1 {
		t.Fatalf("epoch after attaching to a waiting session = %d, %v; want 1", got.NotifyEpoch, err)
	}
	if f := hookFiredOf(t, st, "via-attach"); f.Valid {
		t.Fatalf("hook_fired after the epoch advanced by an attachment = %+v; want NULL", f)
	}
}
