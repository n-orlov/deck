package service

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/n-orlov/deck/internal/audit"
	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
	"github.com/n-orlov/deck/internal/tmux"
)

// TestReconcilerCadenceIsAnchoredAtLoopStartNotFirstPassEnd drives the exact
// ordering that made the disappearance regression racy. The first (immediate)
// snapshot is taken while the session is alive, and its result is delivered
// late -- after the session has vanished. The reconcile cadence is "every
// interval", so the second pass must start one interval after the first one
// STARTED; if the ticker were only created once the first pass returned, the
// second pass would start one interval after the first one ENDED and the
// disappearance would surface a whole snapshot-latency late.
//
// The ordering is forced by a recording tmux double behind tmux.Client.Binary:
// its first list-panes blocks until the test releases it, every later one
// reports the session gone. Nothing here depends on a race being won; the
// bound compares two recorded start times against the injected latency.
func TestReconcilerCadenceIsAnchoredAtLoopStartNotFirstPassEnd(t *testing.T) {
	// snapshotLatency is the injected delay between the first snapshot being
	// taken and its result being delivered; it stays below interval so the
	// first pass remains inside its own budget. The cases vary both numbers so
	// the rule is the cadence, not one tuned pair.
	for _, tc := range []struct{ interval, snapshotLatency time.Duration }{
		{600 * time.Millisecond, 300 * time.Millisecond},
		{600 * time.Millisecond, 450 * time.Millisecond},
		{400 * time.Millisecond, 240 * time.Millisecond},
	} {
		t.Run(tc.interval.String()+"/"+tc.snapshotLatency.String(), func(t *testing.T) {
			assertReconcilerCadenceAnchored(t, tc.interval, tc.snapshotLatency)
		})
	}
}

func assertReconcilerCadenceAnchored(t *testing.T, interval, snapshotLatency time.Duration) {
	t.Helper()
	home, cwd := t.TempDir(), t.TempDir()
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
	session := reconcileSession(t, db, "00000000-0000-4000-8000-000000000031", "gamma", cwd, "shell", "running", "user")

	state := t.TempDir()
	script := filepath.Join(state, "fake-tmux")
	body := strings.NewReplacer("@STATE@", state, "@SLUG@", session.Slug).Replace(`#!/bin/sh
for arg in "$@"; do
	case "$arg" in
	list-panes)
		date +%s%N >> '@STATE@/starts'
		if [ ! -e '@STATE@/first-taken' ]; then
			: > '@STATE@/first-taken'
			while [ ! -e '@STATE@/release' ]; do sleep 0.005; done
			printf '%s\n' 'deck_@SLUG@|%1|/tmp|4242|0|||sh|80|24'
		fi
		exit 0
		;;
	esac
done
exit 0
`)
	if err := os.WriteFile(script, []byte(body), 0o700); err != nil {
		t.Fatal(err)
	}
	service := Service{Store: db, TMux: tmux.Client{Binary: script, Socket: "priv-cadence-anchor"}, Audit: logger, Clock: clock}

	stop := startReconciler(t, service, interval)
	if !waitForFile(t, filepath.Join(state, "first-taken"), 5*time.Second) {
		t.Fatal("the first snapshot was never requested")
	}
	// The session vanishes now (every later snapshot reports it gone) while the
	// first snapshot, taken alive, is still in flight.
	time.Sleep(snapshotLatency)
	if err := os.WriteFile(filepath.Join(state, "release"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	waitForStatus(t, db, session.ID, "stopped", 3*interval)
	if err := stop(); err != nil {
		t.Fatalf("reconcile loop: %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(state, "starts"))
	if err != nil {
		t.Fatal(err)
	}
	fields := strings.Fields(string(raw))
	if len(fields) < 2 {
		t.Fatalf("expected at least two passes, got start stamps %q", fields)
	}
	var starts []int64
	for _, field := range fields {
		ns, err := strconv.ParseInt(field, 10, 64)
		if err != nil {
			t.Fatalf("bad start stamp %q: %v", field, err)
		}
		starts = append(starts, ns)
	}
	gap := time.Duration(starts[1] - starts[0])
	// Anchored at loop start: gap ~ interval. Anchored at first-pass end:
	// gap ~ interval + snapshotLatency. Half the latency is the margin on both
	// sides of that difference.
	if limit := interval + snapshotLatency/2; gap >= limit {
		t.Fatalf("second pass started %s after the first started; the cadence must be one interval (%s) from the loop start, but this reads as one interval after the first pass ended (limit %s)", gap, interval, limit)
	}
	var events int
	if err := db.DB().QueryRow(`SELECT count(*) FROM events WHERE session_id = ? AND kind = 'tmux.session_gone'`, session.ID).Scan(&events); err != nil || events != 1 {
		t.Fatalf("disappearance events = %d, %v", events, err)
	}
}
