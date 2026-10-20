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

// movementRig is the output-movement fallback (R316) end to end: a real store,
// a scripted tmux that reports ./activity and prints ./pane, and an
// OutputClock whose wall clock the test sets with tick.
type movementRig struct {
	t     *testing.T
	home  string
	db    *store.Store
	svc   Service
	id    string
	clock *OutputClock
	now   time.Time
}

const movementStale = time.Hour

func newMovementRig(t *testing.T, agentKind string, hookAge time.Duration) *movementRig {
	t.Helper()
	home := t.TempDir()
	cfgClock, err := config.NewClock("2025-01-02T03:04:05Z", "")
	if err != nil {
		t.Fatal(err)
	}
	paths := config.Paths{Home: home, LogDir: filepath.Join(home, "log"), StateDB: filepath.Join(home, "state.db")}
	db, err := store.Open(paths)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	logger, err := audit.New(paths, cfgClock)
	if err != nil {
		t.Fatal(err)
	}
	session, err := db.CreateSession(context.Background(), store.CreateSessionInput{
		ID: "00000000-0000-4000-8000-0000000000c1", Name: "move", CWD: t.TempDir(), Agent: agentKind,
		CapturedPath: "/bin", Status: "idle", StatusSource: "hook",
		StatusAt: cfgClock.Now().Add(-hookAge).UnixMilli(), CreatedAt: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(home, "fake-tmux")
	body := strings.NewReplacer("@DIR@", home, "@SLUG@", session.Slug).Replace(`#!/bin/sh
for arg in "$@"; do
	case "$arg" in
	list-panes) printf 'deck_@SLUG@|%%1|4242|0|||80|24|%s\n' "$(cat '@DIR@/activity')"; exit 0 ;;
	capture-pane) cat '@DIR@/pane'; exit 0 ;;
	esac
done
exit 0
`)
	if err := os.WriteFile(script, []byte(body), 0o700); err != nil {
		t.Fatal(err)
	}
	r := &movementRig{t: t, home: home, db: db, id: session.ID, clock: NewOutputClock(10 * time.Second)}
	r.clock.Now = func() time.Time { return r.now }
	r.svc = Service{Store: db, TMux: tmux.Client{Binary: script, Socket: "move"}, Audit: logger, Clock: cfgClock,
		Agents: agent.NewRegistry(), Output: r.clock}
	return r
}

// tick runs one probing reconcile pass at wall-clock second nowSec, with the
// pane's window_activity at activity and the given transcript above a fixed
// composer and footer.
func (r *movementRig) tick(nowSec, activity int64, transcript, footer string) {
	r.t.Helper()
	r.now = time.Unix(nowSec, 0)
	for name, content := range map[string]string{
		"activity": itoa(activity),
		"pane":     pane(transcript, footer),
	} {
		if err := os.WriteFile(filepath.Join(r.home, name), []byte(content), 0o600); err != nil {
			r.t.Fatal(err)
		}
	}
	if err := r.svc.ReconcileWithProbes(context.Background(), movementStale); err != nil {
		r.t.Fatal(err)
	}
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	var digits []byte
	for ; n > 0; n /= 10 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
	}
	return string(digits)
}

func (r *movementRig) row() store.Session {
	r.t.Helper()
	got, err := r.db.GetSession(context.Background(), r.id)
	if err != nil {
		r.t.Fatal(err)
	}
	return got
}

func (r *movementRig) wantStatus(want, why string) {
	r.t.Helper()
	if got := r.row(); got.Status != want {
		r.t.Fatalf("%s: status = %q (source %q), want %q", why, got.Status, got.StatusSource, want)
	}
}

// outputInTwoBuckets drives a baseline and then a transcript change in each of
// two consecutive 10 s buckets (buckets 101 and 102).
func (r *movementRig) outputInTwoBuckets() {
	r.tick(1000, 1000, "start", "ctx:1%")
	r.tick(1010, 1010, "start\nmore", "ctx:1%")
	r.tick(1020, 1020, "start\nmore\nmost", "ctx:1%")
}

func TestPiEntersRunningOnOutputInTwoConsecutiveBuckets(t *testing.T) {
	r := newMovementRig(t, "pi", 0) // a hook verdict stamped just now: pi ignores it
	r.tick(1000, 1000, "start", "ctx:1%")
	r.tick(1010, 1010, "start\nmore", "ctx:1%")
	r.wantStatus("idle", "one counted bucket")
	r.tick(1020, 1020, "start\nmore\nmost", "ctx:1%")
	got := r.row()
	if got.Status != "running" || got.StatusSource != "movement" {
		t.Fatalf("two consecutive buckets: status %q source %q, want running from movement", got.Status, got.StatusSource)
	}
}

func TestPiDoesNotEnterRunningOnOneBucket(t *testing.T) {
	r := newMovementRig(t, "pi", 2*time.Hour)
	r.tick(1000, 1000, "start", "ctx:1%")
	r.tick(1010, 1010, "start\nmore", "ctx:1%")
	r.tick(1020, 1010, "start\nmore", "ctx:1%")
	r.wantStatus("idle", "output in one bucket")
	// A gap bucket between two counted buckets is not consecutive either.
	r.tick(1030, 1030, "start\nmore\nlater", "ctx:1%")
	r.tick(1040, 1040, "start\nmore\nlater\nlatest", "ctx:1%")
	r.wantStatus("running", "the buckets 103 and 104 are consecutive")
	r2 := newMovementRig(t, "pi", 2*time.Hour)
	r2.tick(1000, 1000, "start", "ctx:1%")
	r2.tick(1010, 1010, "start\nmore", "ctx:1%")
	r2.tick(1030, 1030, "start\nmore\nlater", "ctx:1%")
	r2.wantStatus("idle", "counted buckets 101 and 103 with a quiet bucket between")
}

func TestFooterOnlyChurnNeverEntersRunning(t *testing.T) {
	r := newMovementRig(t, "pi", 2*time.Hour)
	r.tick(1000, 1000, "start", "ctx:1%")
	for i, footer := range []string{"ctx:2%", "ctx:3% main", "ctx:4% other", "ctx:5% x"} {
		now := int64(1010 + 10*i)
		r.tick(now, now, "start", footer)
	}
	r.wantStatus("idle", "footer-only churn in four consecutive buckets")
}

func TestDeckCausedOutputNeverEntersRunning(t *testing.T) {
	r := newMovementRig(t, "pi", 2*time.Hour)
	r.tick(1000, 1000, "start", "ctx:1%")
	r.now = time.Unix(1010, 0)
	r.clock.Discount(r.id, DiscountKey, r.clock.at())
	r.tick(1010, 1010, "start\nechoed", "ctx:1%")
	r.now = time.Unix(1020, 0)
	r.clock.Discount(r.id, DiscountResize, r.clock.at())
	r.tick(1020, 1020, "start\nechoed\nredrawn", "ctx:1%")
	r.wantStatus("idle", "output inside two deck-caused windows")
	if got := r.row().LastOutputAt; got != 0 {
		t.Fatalf("deck-caused output moved last_output_at to %d", got)
	}
}

func TestHookedAgentWithFreshHookIgnoresMovement(t *testing.T) {
	for _, kind := range []string{"claude", "codex", "copilot"} {
		t.Run(kind, func(t *testing.T) {
			r := newMovementRig(t, kind, time.Second)
			r.outputInTwoBuckets()
			r.wantStatus("idle", "a hook verdict one second old")
			if got := r.row().StatusSource; got != "hook" {
				t.Fatalf("source = %q, the fresh hook verdict must stay", got)
			}
		})
	}
}

func TestHookedAgentWithStaleHookUsesMovement(t *testing.T) {
	for _, kind := range []string{"claude", "codex", "copilot"} {
		t.Run(kind, func(t *testing.T) {
			r := newMovementRig(t, kind, 2*movementStale)
			r.outputInTwoBuckets()
			got := r.row()
			if got.Status != "running" || got.StatusSource != "movement" {
				t.Fatalf("status %q source %q, want running from movement once the hook is older than stale_after", got.Status, got.StatusSource)
			}
		})
	}
}

func TestMovementRowLeavesRunningAfterOneQuietBucket(t *testing.T) {
	r := newMovementRig(t, "pi", 0)
	r.outputInTwoBuckets() // counted buckets 101 and 102
	r.wantStatus("running", "after two buckets")
	r.tick(1030, 1020, "start\nmore\nmost", "ctx:1%") // bucket 103: still in progress
	r.wantStatus("running", "bucket 103 has not finished quiet yet")
	r.tick(1040, 1020, "start\nmore\nmost", "ctx:1%") // bucket 103 passed without output
	got := r.row()
	if got.Status != "idle" || got.StatusSource != "movement" {
		t.Fatalf("after one quiet bucket: status %q source %q, want idle", got.Status, got.StatusSource)
	}
}

// A running row set by anything but movement is never taken back by it.
func TestMovementNeverEndsAHookRunningRow(t *testing.T) {
	r := newMovementRig(t, "pi", 0)
	if err := r.db.UpdateSessionStatus(context.Background(), store.StatusUpdateInput{
		SessionID: r.id, Status: "running", Source: "hook", At: r.svc.Clock.Now().UnixMilli() + 1, StaleAfter: movementStale.Milliseconds(),
	}); err != nil {
		t.Fatal(err)
	}
	r.tick(1000, 1000, "start", "ctx:1%")
	r.tick(1100, 1000, "start", "ctx:1%")
	r.wantStatus("running", "a hook-sourced running row in a quiet pane")
}
