package service

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
)

// pane renders a composer-style pane: a transcript, then the composer framed
// by two separator lines, then a footer carrying the statusline chrome.
func pane(transcript, footer string) string {
	rule := strings.Repeat("─", 40)
	return transcript + "\n" + rule + "\n❯ \n" + rule + "\n" + footer + "\n"
}

// outputHarness is a real store holding one session, a clock with 10 s
// buckets, and a pane the test can change between ticks.
type outputHarness struct {
	t      *testing.T
	db     *store.Store
	id     string
	clock  *OutputClock
	pane   string
	reads  int
	writes int
}

func newOutputHarness(t *testing.T) *outputHarness {
	t.Helper()
	home := t.TempDir()
	db, err := store.Open(config.Paths{Home: home, LogDir: filepath.Join(home, "log"), StateDB: filepath.Join(home, "state.db")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	session, err := db.CreateSession(context.Background(), store.CreateSessionInput{
		ID: "00000000-0000-4000-8000-0000000000a1", Name: "clock", CWD: t.TempDir(), Agent: "claude",
		CapturedPath: "/bin", Status: "running", StatusSource: "hook", StatusAt: 1, CreatedAt: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	return &outputHarness{t: t, db: db, id: session.ID, clock: NewOutputClock(10 * time.Second)}
}

func (h *outputHarness) events() int {
	h.t.Helper()
	var n int
	if err := h.db.DB().QueryRow(`SELECT count(*) FROM events WHERE session_id = ?`, h.id).Scan(&n); err != nil {
		h.t.Fatal(err)
	}
	return n
}

func (h *outputHarness) lastOutputAt() int64 {
	h.t.Helper()
	got, err := h.db.GetSession(context.Background(), h.id)
	if err != nil {
		h.t.Fatal(err)
	}
	return got.LastOutputAt
}

// tick is one reconcile reading at tmux window_activity seconds.
func (h *outputHarness) tick(activity int64) bool {
	h.t.Helper()
	before := h.lastOutputAt()
	wrote, err := h.clock.Observe(context.Background(), h.db, h.id, activity, func(context.Context) (string, error) {
		h.reads++
		return h.pane, nil
	})
	if err != nil {
		h.t.Fatal(err)
	}
	if wrote != (h.lastOutputAt() != before) {
		h.t.Fatalf("Observe wrote=%v but last_output_at went %d -> %d", wrote, before, h.lastOutputAt())
	}
	if wrote {
		h.writes++
	}
	return wrote
}

// TestOutputClock is R304's five finite cases. Each step names the tmux
// window_activity (seconds, 10 s buckets) and the pane that tick sees, and
// what the clock must do. The first step of every case is the baseline read,
// which counts nothing. Every case also asserts that the column moved by
// UPDATE alone: the session's event count never changes.
func TestOutputClock(t *testing.T) {
	type step struct {
		activity   int64
		transcript string
		footer     string
		wantWrite  bool
		wantAt     int64 // last_output_at after the step
		wantReads  int   // cumulative captures after the step
	}
	const footerA = "~/proj  main  ctx:12%  opus"
	cases := []struct {
		name  string
		steps []step
	}{
		{"a transcript change counts", []step{
			{activity: 1000, transcript: "hello", footer: footerA, wantAt: 0, wantReads: 1},
			{activity: 1011, transcript: "hello\nworld", footer: footerA, wantWrite: true, wantAt: 1011_000, wantReads: 2},
		}},
		{"a footer-only change (ctx:%, branch, model) does not", []step{
			{activity: 1000, transcript: "hello", footer: footerA, wantAt: 0, wantReads: 1},
			{activity: 1011, transcript: "hello", footer: "~/proj  feature  ctx:13%  sonnet", wantAt: 0, wantReads: 2},
		}},
		{"no change does not", []step{
			{activity: 1000, transcript: "hello", footer: footerA, wantAt: 0, wantReads: 1},
			{activity: 1011, transcript: "hello", footer: footerA, wantAt: 0, wantReads: 2},
		}},
		{"two changes inside one bucket write once", []step{
			{activity: 1000, transcript: "hello", footer: footerA, wantAt: 0, wantReads: 1},
			{activity: 1011, transcript: "hello\none", footer: footerA, wantWrite: true, wantAt: 1011_000, wantReads: 2},
			{activity: 1013, transcript: "hello\none\ntwo", footer: footerA, wantAt: 1011_000, wantReads: 2},
			{activity: 1019, transcript: "hello\none\ntwo\nthree", footer: footerA, wantAt: 1011_000, wantReads: 2},
		}},
		{"a tick with no bucket change writes nothing", []step{
			{activity: 1000, transcript: "hello", footer: footerA, wantAt: 0, wantReads: 1},
			{activity: 1000, transcript: "hello\nmore", footer: footerA, wantAt: 0, wantReads: 1},
			{activity: 1009, transcript: "hello\nmore", footer: footerA, wantAt: 0, wantReads: 1},
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newOutputHarness(t)
			eventsBefore := h.events()
			for i, st := range tc.steps {
				h.pane = pane(st.transcript, st.footer)
				if got := h.tick(st.activity); got != st.wantWrite {
					t.Fatalf("step %d: wrote = %v, want %v", i, got, st.wantWrite)
				}
				if got := h.lastOutputAt(); got != st.wantAt {
					t.Fatalf("step %d: last_output_at = %d, want %d", i, got, st.wantAt)
				}
				if h.reads != st.wantReads {
					t.Fatalf("step %d: %d pane captures, want %d (a tick in a known bucket must not read the pane)", i, h.reads, st.wantReads)
				}
			}
			wantWrites := 0
			for _, st := range tc.steps {
				if st.wantWrite {
					wantWrites++
				}
			}
			if h.writes != wantWrites {
				t.Fatalf("%d writes, want %d", h.writes, wantWrites)
			}
			// The column is an UPDATE, never an event row.
			if got := h.events(); got != eventsBefore {
				t.Fatalf("events for the session went %d -> %d; last_output_at must not append an event", eventsBefore, got)
			}
		})
	}
}

// TestOutputClockLeavesStatusAlone: counting output touches no status field.
func TestOutputClockLeavesStatusAlone(t *testing.T) {
	h := newOutputHarness(t)
	h.pane = pane("a", "f")
	h.tick(1000)
	h.pane = pane("b", "f")
	if !h.tick(1011) {
		t.Fatal("a transcript change did not count")
	}
	got, err := h.db.GetSession(context.Background(), h.id)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "running" || got.StatusSource != "hook" || got.StatusAt != 1 {
		t.Fatalf("the output clock changed the status: %+v", got)
	}
}

// R306: SetBucket changes the width the clock buckets by while it runs; a
// non-positive width is ignored.
func TestOutputClockSetBucketAppliesLive(t *testing.T) {
	c := NewOutputClock(10 * time.Second)
	c.SetBucket(2 * time.Second)
	if got := c.bucketWidth(); got != 2*time.Second {
		t.Fatalf("bucket after SetBucket(2s) = %v", got)
	}
	if got := c.DiscountWindow(); got != 2*time.Second {
		t.Fatalf("discount window = %v, want it capped at the 2s bucket", got)
	}
	c.SetBucket(0)
	c.SetBucket(-time.Second)
	if got := c.bucketWidth(); got != 2*time.Second {
		t.Fatalf("a non-positive width changed the bucket to %v", got)
	}
}

// R324: statusline-only churn does not set the unseen marker. The row is idle
// and has been viewed; a footer-only change (ctx:%, branch, model) leaves
// last_output_at alone, so the row stays seen, while a transcript change then
// sets the marker and a view clears it again.
func TestStatuslineChurnDoesNotSetTheUnseenMarker(t *testing.T) {
	h := newOutputHarness(t)
	ctx := context.Background()
	if _, err := h.db.DB().Exec(`UPDATE sessions SET status = 'idle' WHERE id = ?`, h.id); err != nil {
		t.Fatal(err)
	}
	unseen := func() bool {
		t.Helper()
		s, err := h.db.GetSession(ctx, h.id)
		if err != nil {
			t.Fatal(err)
		}
		return s.Unseen()
	}
	h.pane = pane("hello", "~/proj  main  ctx:12%  opus")
	h.tick(1000)
	h.pane = pane("hello", "~/proj  feature  ctx:13%  sonnet")
	if h.tick(1011) || unseen() {
		t.Fatalf("a footer-only change set the unseen marker (unseen=%v)", unseen())
	}
	h.pane = pane("hello\nworld", "~/proj  feature  ctx:14%  sonnet")
	if !h.tick(1022) || !unseen() {
		t.Fatal("a transcript change did not set the unseen marker")
	}
	if _, err := h.db.MarkViewed(ctx, h.id); err != nil {
		t.Fatal(err)
	}
	if unseen() {
		t.Fatal("a view did not clear the marker")
	}
	h.pane = pane("hello\nworld", "~/proj  other  ctx:15%  haiku")
	if h.tick(1033) || unseen() {
		t.Fatal("statusline churn after a view set the marker")
	}
}

// R306: a fractional-second bucket stays exact in the clock. With 1.5 s
// buckets, window_activity 3 and 4 (3000 and 4000 ms) are one bucket, so two
// changes there write once; the first second of the next bucket writes again.
func TestOutputClockFractionalBucketNeverWritesTwiceInOneBucket(t *testing.T) {
	for _, tc := range []struct {
		name    string
		width   time.Duration
		samples []int64 // activities after the baseline, each a transcript change
		writes  []bool
	}{
		{"1.5s", 1500 * time.Millisecond, []int64{3, 4, 5}, []bool{true, false, true}},
		{"1500ms from 100", 1500 * time.Millisecond, []int64{102, 103, 104}, []bool{true, false, true}},
		{"2.5s", 2500 * time.Millisecond, []int64{5, 6, 7, 8}, []bool{true, false, false, true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newOutputHarness(t)
			h.clock = NewOutputClock(tc.width)
			h.pane = pane("base", "f")
			h.tick(1) // baseline
			for i, activity := range tc.samples {
				h.pane = pane("base"+strings.Repeat("+", i+1), "f")
				if got := h.tick(activity); got != tc.writes[i] {
					t.Fatalf("activity %d: wrote = %v, want %v", activity, got, tc.writes[i])
				}
			}
		})
	}
}

// SetBucket applies a fractional width live, exactly.
func TestOutputClockSetBucketKeepsAFractionalWidth(t *testing.T) {
	c := NewOutputClock(10 * time.Second)
	c.SetBucket(1500 * time.Millisecond)
	if got := c.bucketWidth(); got != 1500*time.Millisecond {
		t.Fatalf("bucket = %v, want 1.5s", got)
	}
	if b, _ := c.bucketOf(4); b != 2 {
		t.Fatalf("bucketOf(4) = %d at 1.5s, want 2", b)
	}
}
