package store

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
)

// onDiskSecretFixture opens a store, creates a session whose env holds the
// sentinel under key, overwrites that key via SetSessionEnvValue with each of
// overwrites in turn, closes the store and returns the raw bytes of state.db
// and state.db-wal.
func onDiskSecretFixture(t *testing.T, sentinel string, overwrites ...string) (db, wal []byte) {
	t.Helper()
	home := t.TempDir()
	path := filepath.Join(home, "state.db")
	s, err := OpenPath(home, path)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, err := s.CreateSession(ctx, CreateSessionInput{
		ID: "secret", Name: "secret", CWD: "/work", Agent: "claude", CapturedPath: "/bin",
		Status: "running", StatusSource: "hook", StatusAt: 10, CreatedAt: 1,
		Env: map[string]string{"API_TOKEN": sentinel},
	}); err != nil {
		t.Fatal(err)
	}
	for i, v := range overwrites {
		if err := s.SetSessionEnvValue(ctx, "secret", "API_TOKEN", v, "user", int64(20+i)); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	wal, err = os.ReadFile(path + "-wal")
	if err != nil {
		t.Fatalf("state.db-wal after Close: %v", err)
	}
	return db, wal
}

// TestOverwrittenEnvValueIsZeroedInStateDBButStaysInTheWAL pins the measured
// on-disk guarantee of SPEC section 6.4: with secure_delete=ON an overwritten
// env value is not kept in state.db's pages, while the stale WAL frame that
// carried it is not rewritten and persists in state.db-wal (the WAL stays on
// disk across closes), where the 0600 mode is the protection. A change to
// either half (a checkpoint-and-reset on close, or secure_delete turned off)
// must show up here as a deliberate edit of this test and of SPEC section 6.4.
func TestOverwrittenEnvValueIsZeroedInStateDBButStaysInTheWAL(t *testing.T) {
	const sentinel = "sentinel-secret-7f3a91c2e5d84b06"
	db, wal := onDiskSecretFixture(t, sentinel, "replacement-value")
	if bytes.Contains(db, []byte(sentinel)) {
		t.Fatal("the overwritten env value is readable in state.db; secure_delete=ON must zero freed pages (SPEC section 6.4)")
	}
	if !bytes.Contains(wal, []byte(sentinel)) {
		t.Fatal("the overwritten env value is absent from state.db-wal; SPEC section 6.4 says stale WAL frames stay until a later checkpoint cycle overwrites them (did the WAL get reset on close?)")
	}
}

// TestRepeatedEnvOverwritesLeaveNoSentinelInStateDB: same rule, a value
// overwritten twice (two freed copies) and a different sentinel; every
// superseded value is absent from state.db and present in the WAL.
func TestRepeatedEnvOverwritesLeaveNoSentinelInStateDB(t *testing.T) {
	first, second := "first-sentinel-0a1b2c3d4e5f", "second-sentinel-9f8e7d6c5b4a"
	db, wal := onDiskSecretFixture(t, first, second, "final-value")
	for _, v := range []string{first, second} {
		if bytes.Contains(db, []byte(v)) {
			t.Fatalf("superseded env value %q is readable in state.db", v)
		}
		if !bytes.Contains(wal, []byte(v)) {
			t.Fatalf("superseded env value %q is absent from state.db-wal", v)
		}
	}
}

// TestAnEnvValueNeverOverwrittenIsInTheWALAfterClose documents where a live
// value sits after a Close: in the persisted WAL, so state.db-wal (0600) is
// where env values live until a checkpoint moves them.
func TestAnEnvValueNeverOverwrittenIsInTheWALAfterClose(t *testing.T) {
	const sentinel = "live-sentinel-3c4d5e6f7a8b"
	_, wal := onDiskSecretFixture(t, sentinel)
	if !bytes.Contains(wal, []byte(sentinel)) {
		t.Fatal("the live env value is absent from state.db-wal after Close")
	}
}
