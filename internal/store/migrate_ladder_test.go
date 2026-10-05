package store

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

// openRawLadderDB opens a fresh SQLite file with a single connection and
// foreign keys on (as OpenPath configures it), outside the Store's own open
// path, so migrate can be driven rung by rung.
func openRawLadderDB(t *testing.T) *Store {
	t.Helper()
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "ladder.db"))
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`PRAGMA foreign_keys=ON`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return &Store{db: db}
}

func ladderSchemaVersion(t *testing.T, s *Store) (int, error) {
	t.Helper()
	var v int
	err := s.db.QueryRow(`SELECT version FROM meta WHERE key = 'schema_version'`).Scan(&v)
	return v, err
}

func TestMigrateAtTheCurrentVersionTouchesNothing(t *testing.T) {
	s := openRawLadderDB(t)
	if err := s.migrate(SchemaVersion); err != nil {
		t.Fatal(err)
	}
	var tables int
	if err := s.db.QueryRow(`SELECT count(*) FROM sqlite_master`).Scan(&tables); err != nil || tables != 0 {
		t.Fatalf("migrate at the current version created %d schema objects (err %v), want none", tables, err)
	}
}

func TestMigrateRefusesAVersionWithNoPath(t *testing.T) {
	s := openRawLadderDB(t)
	err := s.migrate(SchemaVersion + 5)
	if err == nil || !strings.Contains(err.Error(), fmt.Sprintf("no migration path from schema version %d", SchemaVersion+5)) {
		t.Fatalf("err = %v, want a no-migration-path refusal", err)
	}
	if _, err := ladderSchemaVersion(t, s); err == nil {
		t.Fatal("a refused migration recorded a schema version")
	}
}

func TestMigrateFromScratchRecordsTheCurrentVersion(t *testing.T) {
	s := openRawLadderDB(t)
	if err := s.migrate(0); err != nil {
		t.Fatal(err)
	}
	if v, err := ladderSchemaVersion(t, s); err != nil || v != SchemaVersion {
		t.Fatalf("recorded schema version = %d, %v; want %d", v, err, SchemaVersion)
	}
}

// TestMigrateNamesTheFailingRungAndRollsEverythingBack breaks each rung of
// the ladder in turn: the error must name that rung, and nothing the earlier
// rungs of the same call created may survive (one transaction).
func TestMigrateNamesTheFailingRungAndRollsEverythingBack(t *testing.T) {
	ladder := []*[]string{&schemaV1, &schemaV2, &schemaV3, &schemaV4, &schemaV5, &schemaV6, &schemaV7, &schemaV8, &schemaV9}
	for rung, steps := range ladder {
		version := rung + 1
		t.Run(fmt.Sprintf("v%d", version), func(t *testing.T) {
			original := *steps
			*steps = append([]string{"THIS IS NOT VALID SQL"}, original...)
			t.Cleanup(func() { *steps = original })

			s := openRawLadderDB(t)
			err := s.migrate(0)
			want := fmt.Sprintf("create schema v%d", version)
			if err == nil || !strings.Contains(err.Error(), want) {
				t.Fatalf("err = %v, want it to name %q", err, want)
			}
			var objects int
			if err := s.db.QueryRow(`SELECT count(*) FROM sqlite_master`).Scan(&objects); err != nil || objects != 0 {
				t.Fatalf("%d schema objects survive a failed migration (err %v), want a full rollback", objects, err)
			}
		})
	}
}

func TestMigrateFromAMidVersionRunsOnlyTheLaterRungs(t *testing.T) {
	original := schemaV1
	schemaV1 = []string{"THIS IS NOT VALID SQL"} // would fail if rung 1 ever ran
	t.Cleanup(func() { schemaV1 = original })

	s := openRawLadderDB(t)
	// A real v1 database is needed for the later rungs to apply to.
	for _, stmt := range original {
		if _, err := s.db.Exec(stmt); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.migrate(1); err != nil {
		t.Fatalf("migrate from v1 must skip schemaV1: %v", err)
	}
	if v, err := ladderSchemaVersion(t, s); err != nil || v != SchemaVersion {
		t.Fatalf("recorded schema version = %d, %v; want %d", v, err, SchemaVersion)
	}
}

func TestMigrateReportsAFailedVersionRecord(t *testing.T) {
	original := schemaV9
	schemaV9 = append(append([]string{}, original...), `DROP TABLE meta`)
	t.Cleanup(func() { schemaV9 = original })

	s := openRawLadderDB(t)
	err := s.migrate(0)
	if err == nil || !strings.Contains(err.Error(), "record schema version") {
		t.Fatalf("err = %v, want a record-schema-version failure", err)
	}
	var objects int
	if err := s.db.QueryRow(`SELECT count(*) FROM sqlite_master`).Scan(&objects); err != nil || objects != 0 {
		t.Fatalf("%d schema objects survive, want a full rollback (err %v)", objects, err)
	}
}

func TestMigrateReportsAFailedCommit(t *testing.T) {
	original := schemaV9
	// A deferred foreign-key violation only surfaces at COMMIT.
	schemaV9 = append(append([]string{}, original...),
		`CREATE TABLE ladder_parent (id INTEGER PRIMARY KEY)`,
		`CREATE TABLE ladder_child (p INTEGER REFERENCES ladder_parent(id) DEFERRABLE INITIALLY DEFERRED)`,
		`INSERT INTO ladder_child (p) VALUES (99)`)
	t.Cleanup(func() { schemaV9 = original })

	s := openRawLadderDB(t)
	err := s.migrate(0)
	if err == nil || !strings.Contains(err.Error(), "commit state database migration") {
		t.Fatalf("err = %v, want a commit failure", err)
	}
	if _, err := ladderSchemaVersion(t, s); err == nil {
		t.Fatal("a version was recorded although the commit failed")
	}
}

func TestMigrateReportsAFailedBegin(t *testing.T) {
	s := openRawLadderDB(t)
	if err := s.db.Close(); err != nil {
		t.Fatal(err)
	}
	err := s.migrate(0)
	if err == nil || !strings.Contains(err.Error(), "begin state database migration") {
		t.Fatalf("err = %v, want a begin failure", err)
	}
}
