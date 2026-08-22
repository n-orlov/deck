package features

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/cucumber/godog"
	_ "modernc.org/sqlite"
)

// registerStoreFeatureSteps constructs fixtures and observes the released
// binary through SQLite and its stderr; it deliberately does not import deck's
// store package or schema constants.
func registerStoreFeatureSteps(sc *godog.ScenarioContext) {
	sc.Step(`^the scenario has an older supported database fixture$`, olderDatabaseFixture)
	sc.Step(`^the scenario has a newer unsupported database fixture$`, newerDatabaseFixture)
	sc.Step(`^the released deck binary opens the newer database$`, releasedBinaryOpensNewerDatabase)
	sc.Step(`^it clearly refuses the newer database$`, clearlyRefusesNewerDatabase)
	sc.Step(`^the newer database fixture remains unchanged$`, newerDatabaseRemainsUnchanged)
	sc.Step(`^the scenario has a v1 database fixture with an existing session "([^"]+)"$`, v1DatabaseFixtureWithSession)
	sc.Step(`^the state database session "([^"]+)" still has id "([^"]+)"$`, stateDatabaseSessionStillHasID)
}

func writeDatabaseFixture(h *ScenarioHarness, version int) error {
	path := filepath.Join(h.Home, "state.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return fmt.Errorf("open external schema fixture: %w", err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE meta (key TEXT PRIMARY KEY, version INTEGER NOT NULL); INSERT INTO meta VALUES ('schema_version', ?);`, version); err != nil {
		return fmt.Errorf("create external schema v%d fixture: %w", version, err)
	}
	return nil
}

func olderDatabaseFixture(ctx context.Context) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	return writeDatabaseFixture(h, 0)
}

func newerDatabaseFixture(ctx context.Context) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	if err := writeDatabaseFixture(h, 5); err != nil {
		return err
	}
	h.databaseFixture, err = os.ReadFile(filepath.Join(h.Home, "state.db"))
	if err != nil {
		return fmt.Errorf("snapshot newer external fixture: %w", err)
	}
	return nil
}

func releasedBinaryOpensNewerDatabase(ctx context.Context) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, h.Binary)
	cmd.Env = append(os.Environ(), h.Environment()...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("run released binary against newer fixture: %w: %s", err, strings.TrimSpace(string(output)))
	}
	h.newerRefusal = string(output)
	return nil
}

func clearlyRefusesNewerDatabase(ctx context.Context) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	if !strings.Contains(h.newerRefusal, "newer than supported") {
		return fmt.Errorf("released binary did not clearly refuse newer database: %q", h.newerRefusal)
	}
	return nil
}

func newerDatabaseRemainsUnchanged(ctx context.Context) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	got, err := os.ReadFile(filepath.Join(h.Home, "state.db"))
	if err != nil {
		return fmt.Errorf("read newer fixture after refusal: %w", err)
	}
	if string(got) != string(h.databaseFixture) {
		return fmt.Errorf("newer database fixture was modified by refusal")
	}
	return nil
}

// v1SchemaStatements mirrors deck's own v1 schema (internal/store.schemaV1)
// independently, without importing internal packages, so this fixture stays
// an external, black-box observer of the released binary's migration path
// (task 010, SPEC §4 invariant: "every column above is reachable by
// migration from schema version 1 -- the store is never rebuilt and a
// session row is never recreated to gain a field").
var v1SchemaStatements = []string{
	`CREATE TABLE meta (key TEXT PRIMARY KEY, version INTEGER NOT NULL)`,
	`CREATE TABLE sessions (
		id TEXT PRIMARY KEY, name TEXT NOT NULL UNIQUE, slug TEXT NOT NULL UNIQUE,
		cwd TEXT NOT NULL, agent TEXT NOT NULL, launch_args TEXT NOT NULL DEFAULT '[]',
		env TEXT NOT NULL DEFAULT '{}', env_dirty INTEGER NOT NULL DEFAULT 0,
		captured_path TEXT NOT NULL, pre_launch TEXT, login_shell INTEGER NOT NULL DEFAULT 0,
		permission_profile TEXT NOT NULL DEFAULT 'safe', permission_profile_reason TEXT, conversation_id TEXT, resume_pin TEXT,
		resume_state TEXT NOT NULL DEFAULT 'auto', status TEXT NOT NULL, status_reason TEXT,
		status_source TEXT NOT NULL, status_at INTEGER NOT NULL, killed_by_user INTEGER NOT NULL DEFAULT 0,
		pane_exit_status INTEGER, crash_tail TEXT, notify_epoch INTEGER NOT NULL DEFAULT 0,
		last_message TEXT, sensitive INTEGER NOT NULL DEFAULT 0, notify_rules TEXT,
		important INTEGER NOT NULL DEFAULT 0, workspace TEXT, snoozed_until INTEGER NOT NULL DEFAULT 0,
		acknowledged INTEGER NOT NULL DEFAULT 1, launch_lease_owner TEXT, launch_lease_until INTEGER NOT NULL DEFAULT 0,
		created_at INTEGER NOT NULL, last_attached_at INTEGER NOT NULL DEFAULT 0,
		archived_at INTEGER NOT NULL DEFAULT 0, deleted_at INTEGER NOT NULL DEFAULT 0
	)`,
	`CREATE TABLE events (
		seq INTEGER PRIMARY KEY AUTOINCREMENT, session_id TEXT REFERENCES sessions(id) ON DELETE CASCADE,
		at INTEGER NOT NULL, kind TEXT NOT NULL, reason TEXT, payload TEXT
	)`,
}

func v1DatabaseFixtureWithSession(ctx context.Context, name string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	path := filepath.Join(h.Home, "state.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return fmt.Errorf("open v1 database fixture: %w", err)
	}
	defer db.Close()
	for _, statement := range v1SchemaStatements {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("create v1 fixture schema: %w", err)
		}
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO meta (key, version) VALUES ('schema_version', 1)`); err != nil {
		return fmt.Errorf("record v1 fixture schema version: %w", err)
	}
	id := "v1-fixture-" + name
	if _, err := db.ExecContext(ctx, `INSERT INTO sessions (
		id, name, slug, cwd, agent, captured_path, status, status_source, status_at, created_at
	) VALUES (?, ?, ?, '/tmp', 'shell', '/bin/sh', 'stopped', 'test', 1, 1)`, id, name, name); err != nil {
		return fmt.Errorf("insert v1 fixture session: %w", err)
	}
	h.v1FixtureSessionID = id
	return nil
}

func stateDatabaseSessionStillHasID(ctx context.Context, name, id string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	if h.v1FixtureSessionID != id {
		return fmt.Errorf("scenario did not fixture session %q with id %q (fixtured id %q)", name, id, h.v1FixtureSessionID)
	}
	db, err := openObservedDatabase(h)
	if err != nil {
		return err
	}
	defer db.Close()
	var count int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM sessions WHERE id = ? AND name = ?`, id, name).Scan(&count); err != nil {
		return fmt.Errorf("observe fixtured session id: %w", err)
	}
	if count != 1 {
		return fmt.Errorf("session %q id %q count = %d, want 1 (row was recreated, not migrated in place)", name, id, count)
	}
	return nil
}
