package store

import (
	"errors"
	"path/filepath"
	"testing"
)

func stubExecutable(t *testing.T, path string) {
	t.Helper()
	prev := executablePath
	executablePath = func() (string, error) { return path, nil }
	t.Cleanup(func() { executablePath = prev })
}

// R204.2 (#56): opening the database records the running binary's absolute
// path in a meta row, with no SchemaVersion change, and a later open by a
// different binary replaces it.
func TestOpenRecordsWriterBinaryInMetaRow(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, "state.db")
	stubExecutable(t, "/opt/deck-a/deck")
	first, err := OpenPath(home, path)
	if err != nil {
		t.Fatal(err)
	}
	var raw string
	if err := first.DB().QueryRow(`SELECT CAST(version AS TEXT) FROM meta WHERE key = 'writer_binary'`).Scan(&raw); err != nil || raw != "/opt/deck-a/deck" {
		t.Fatalf("meta writer_binary = %q, %v; want /opt/deck-a/deck", raw, err)
	}
	var schema int
	if err := first.DB().QueryRow(`SELECT version FROM meta WHERE key = 'schema_version'`).Scan(&schema); err != nil || schema != SchemaVersion {
		t.Fatalf("schema_version = %d, %v; want unchanged %d", schema, err, SchemaVersion)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}

	stubExecutable(t, "/opt/deck-b/deck")
	second, err := OpenPath(home, path)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	if got, err := second.WriterBinary(); err != nil || got != "/opt/deck-b/deck" {
		t.Fatalf("WriterBinary = %q, %v; want the latest opener", got, err)
	}
}

func TestOpenKeepsRowWhenAlreadyCurrentAndSurvivesUnresolvableExecutable(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, "state.db")
	stubExecutable(t, "/opt/deck-a/deck")
	s, err := OpenPath(home, path)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	executablePath = func() (string, error) { return "", errors.New("no /proc") }
	s, err = OpenPath(home, path)
	if err != nil {
		t.Fatalf("open must not fail when the executable is unresolvable: %v", err)
	}
	defer s.Close()
	if got, _ := s.WriterBinary(); got != "/opt/deck-a/deck" {
		t.Fatalf("WriterBinary = %q, want the previous row kept", got)
	}
}

// The newer-schema refusal carries the recorded writer, read without writing.
func TestNewerSchemaErrorCarriesRecordedWriter(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, "state.db")
	stubExecutable(t, "/opt/deck-a/deck")
	s, err := OpenPath(home, path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB().Exec(`UPDATE meta SET version = ? WHERE key = 'schema_version'`, SchemaVersion+1); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	stubExecutable(t, "/opt/other/deck")
	_, err = OpenPath(home, path)
	var newer *NewerSchemaError
	if !errors.As(err, &newer) || newer.WriterBinary != "/opt/deck-a/deck" || newer.DB != SchemaVersion+1 {
		t.Fatalf("err = %v (%+v), want NewerSchemaError naming /opt/deck-a/deck", err, newer)
	}
}
