package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/n-orlov/deck/internal/audit"
	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
	"github.com/n-orlov/deck/internal/tmux"
)

// RunReconciler runs "until ctx is cancelled" and a cancellation is how its
// caller stops it, so cancelling while a pass has a tmux call in flight (the
// child is killed: `signal: killed`) ends the loop cleanly instead of
// reporting that kill as a loop failure. The tmux stand-in hangs list-panes,
// so the cancellation lands inside the call on every run.
func TestRunReconcilerStoppedMidPassReturnsNilNotTheKilledCall(t *testing.T) {
	home := t.TempDir()
	clock, err := config.NewClock("2025-01-02T03:04:05Z", "")
	if err != nil {
		t.Fatal(err)
	}
	paths := config.Paths{Home: home, LogDir: filepath.Join(home, "log"), StateDB: filepath.Join(home, "state.db")}
	db, err := store.Open(paths)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	logger, err := audit.New(paths, clock)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	entered := filepath.Join(dir, "entered")
	binary := filepath.Join(dir, "tmux-hangs")
	script := strings.NewReplacer("@ENTERED@", entered).Replace("#!/bin/sh\n: > '@ENTERED@'\nexec sleep 60\n")
	if err := os.WriteFile(binary, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	service := Service{Store: db, TMux: tmux.Client{Binary: binary, Socket: "midpass"}, Audit: logger, Clock: clock}

	// A budget far above the test's own duration: the pass cannot run out of
	// its own bound here, only the cancellation can end it.
	stop := startReconciler(t, service, time.Minute)
	deadline := time.Now().Add(30 * time.Second)
	for {
		if _, err := os.Stat(entered); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the liveness pass never reached its tmux call")
		}
		time.Sleep(2 * time.Millisecond)
	}
	if err := stop(); err != nil {
		t.Fatalf("cancelling the loop mid-pass returned %v, want nil", err)
	}
}
