package service

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/n-orlov/deck/internal/agent"
	"github.com/n-orlov/deck/internal/audit"
	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
	"github.com/n-orlov/deck/internal/tmux"
)

// TestReconcileFeedsTheOutputClockFromWindowActivityAndThePane drives the
// clock through ReconcileWithProbes against a scripted tmux: list-panes
// reports the window_activity in ./activity and capture-pane prints ./pane.
// A transcript change in a new bucket sets last_output_at to that
// window_activity; a footer-only change in the next bucket does not; the
// hook-critical Reconcile never reads the pane for it; and no event row is
// appended by any of it.
func TestReconcileFeedsTheOutputClockFromWindowActivityAndThePane(t *testing.T) {
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
	t.Cleanup(func() { _ = db.Close() })
	logger, err := audit.New(paths, clock)
	if err != nil {
		t.Fatal(err)
	}
	session, err := db.CreateSession(context.Background(), store.CreateSessionInput{
		ID: "00000000-0000-4000-8000-0000000000b1", Name: "feed", CWD: t.TempDir(), Agent: "claude",
		CapturedPath: "/bin", Status: "idle", StatusSource: "hook", StatusAt: 1, CreatedAt: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	capturesLog := filepath.Join(home, "captures")
	script := filepath.Join(home, "fake-tmux")
	body := strings.NewReplacer("@DIR@", home, "@SLUG@", session.Slug).Replace(`#!/bin/sh
for arg in "$@"; do
	case "$arg" in
	list-panes) printf 'deck_@SLUG@|%%1|4242|0|||80|24|%s\n' "$(cat '@DIR@/activity')"; exit 0 ;;
	capture-pane) echo x >> '@DIR@/captures'; cat '@DIR@/pane'; exit 0 ;;
	esac
done
exit 0
`)
	if err := os.WriteFile(script, []byte(body), 0o700); err != nil {
		t.Fatal(err)
	}
	set := func(activity, transcript, footer string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(home, "activity"), []byte(activity), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(home, "pane"), []byte(pane(transcript, footer)), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	svc := Service{Store: db, TMux: tmux.Client{Binary: script, Socket: "feed"}, Audit: logger, Clock: clock,
		Agents: agent.NewRegistry(), Output: NewOutputClock(10 * time.Second)}
	reconcile := func() {
		t.Helper()
		if err := svc.ReconcileWithProbes(context.Background(), time.Hour); err != nil {
			t.Fatal(err)
		}
	}
	at := func() int64 {
		t.Helper()
		got, err := db.GetSession(context.Background(), session.ID)
		if err != nil {
			t.Fatal(err)
		}
		return got.LastOutputAt
	}
	var eventsBefore int
	if err := db.DB().QueryRow(`SELECT count(*) FROM events WHERE session_id = ?`, session.ID).Scan(&eventsBefore); err != nil {
		t.Fatal(err)
	}

	set("2000", "hello", "ctx:1%")
	reconcile() // baseline: counts nothing
	if got := at(); got != 0 {
		t.Fatalf("the baseline tick wrote last_output_at = %d", got)
	}
	set("2011", "hello\nmore", "ctx:1%")
	reconcile()
	if got := at(); got != 2011_000 {
		t.Fatalf("a transcript change in a new bucket: last_output_at = %d, want 2011000", got)
	}
	set("2025", "hello\nmore", "ctx:2%  other-branch")
	reconcile()
	if got := at(); got != 2011_000 {
		t.Fatalf("a footer-only change moved last_output_at to %d", got)
	}
	// The hook-critical pass never reads the pane for the clock.
	if err := os.Remove(capturesLog); err != nil {
		t.Fatal(err)
	}
	set("2040", "hello\nmore\neven more", "ctx:2%")
	if err := (Service{Store: db, TMux: svc.TMux, Audit: logger, Clock: clock, Output: svc.Output}).Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(capturesLog); err == nil {
		t.Fatal("Reconcile captured a pane for the output clock")
	}
	if got := at(); got != 2011_000 {
		t.Fatalf("Reconcile moved last_output_at to %d", got)
	}
	var eventsAfter int
	if err := db.DB().QueryRow(`SELECT count(*) FROM events WHERE session_id = ?`, session.ID).Scan(&eventsAfter); err != nil {
		t.Fatal(err)
	}
	if eventsAfter != eventsBefore {
		t.Fatalf("events for the session went %d -> %d; the output clock appends no event", eventsBefore, eventsAfter)
	}
}
