package tui

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/n-orlov/deck/internal/store"
	"github.com/n-orlov/deck/internal/tmux"
)

// R321: a full attach evaluates the episode a trigger observed at the trigger,
// while the attach is still running, from the store -- not from the row deck
// showed when the attach began, and not at detach.

// liveStep happens between the row read before a sample and the sample (so it
// lands inside the interval that sample closes).
type liveStep struct {
	status string // a hook status written while attached, "" = none
	keys   int64  // keys forwarded so far
	sample tmux.AttachSample
	check  func() // runs while the attach is still going
}

type liveRig struct {
	t       *testing.T
	m       Model
	db      *store.Store
	id      string
	base    int64
	writes  int
	stamped int64
}

func newLiveRig(t *testing.T) *liveRig {
	t.Helper()
	m, db := inertFixture(t, SortOrderAttention, "row-running", false)
	r := &liveRig{t: t, db: db, id: "row-running"}
	inner := m.markSeen
	m.markSeen = func(ctx context.Context, id string, at store.StatusEpisode) (bool, error) {
		r.writes++
		return inner(ctx, id, at)
	}
	m.engagementCell()
	r.m = m
	r.base = r.row().StatusAt
	r.stamped = r.base
	return r
}

func (r *liveRig) row() store.Session { return snapshotDurable(r.t, r.db, r.id).Session }

// hook writes a status as a hook would, one millisecond after the last.
func (r *liveRig) hook(status string) {
	r.t.Helper()
	r.stamped++
	err := r.db.UpdateSessionStatus(context.Background(), store.StatusUpdateInput{SessionID: r.id, Status: status, Source: "hook", At: r.stamped})
	if err != nil {
		r.t.Fatal(err)
	}
}

// run plays steps through watchedAttach.watch with the Model's own engageRow
// as the seen write.
func (r *liveRig) run(steps []liveStep) {
	r.t.Helper()
	session, _ := r.m.selectedSession()
	var keys atomic.Int64
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	next := 0
	w := &watchedAttach{
		slug: "s", interval: time.Millisecond, settle: time.Second, session: session,
		now:    func() time.Time { return time.Unix(1000, 0).Add(time.Duration(next) * 200 * time.Millisecond) },
		read:   r.m.attachRowReader(session.ID),
		engage: r.m.engageRow,
	}
	w.sample = func(context.Context, string, int) (tmux.AttachSample, error) {
		if next >= len(steps) {
			cancel()
			return tmux.AttachSample{}, context.Canceled
		}
		step := steps[next]
		next++
		if step.check != nil {
			step.check()
		}
		if step.status != "" {
			r.hook(step.status)
		}
		keys.Store(step.keys)
		return step.sample, nil
	}
	bracket := &episodeBracket{read: w.readRow, reads: map[int]store.Session{}}
	bracket.before(ctx, 0)
	baseline := preAttachSample()
	w.watch(ctx, 1, &baseline, &keys, bracket)
}

var (
	liveQuiet = attachSampleAt(80, 24, "prompt")
	liveScrol = func() tmux.AttachSample {
		s := attachSampleAt(80, 24, "prompt")
		s.InMode, s.ScrollPosition = true, "3"
		return s
	}()
	liveOther = attachSampleAt(80, 24, "the agent asks")
)

func TestFullAttachEngagesPromptThatArrivesWhileAttachedAtTheTrigger(t *testing.T) {
	for _, tc := range []struct {
		name, status string
		steps        func(r *liveRig) []liveStep
	}{
		{"waiting then scroll", "waiting", func(r *liveRig) []liveStep {
			return []liveStep{{status: "waiting", sample: liveQuiet}, {sample: liveQuiet}, {sample: liveScrol}, {sample: liveScrol, check: r.mustBeSeen("waiting")}}
		}},
		{"waiting then key", "waiting", func(r *liveRig) []liveStep {
			return []liveStep{{status: "waiting", sample: liveQuiet}, {sample: liveQuiet}, {keys: 1, sample: liveQuiet}, {keys: 1, sample: liveQuiet}, {keys: 1, sample: liveQuiet, check: r.mustBeSeen("waiting")}}
		}},
		{"waiting then screen change", "waiting", func(r *liveRig) []liveStep {
			return []liveStep{{status: "waiting", sample: liveQuiet}, {sample: liveQuiet}, {sample: liveOther}, {sample: liveOther, check: r.mustBeSeen("waiting")}}
		}},
		{"error then key", "error", func(r *liveRig) []liveStep {
			return []liveStep{{status: "error", sample: liveQuiet}, {sample: liveQuiet}, {keys: 1, sample: liveQuiet}, {keys: 1, sample: liveQuiet}, {keys: 1, sample: liveQuiet, check: r.mustBeSeen("error")}}
		}},
		{"error then scroll", "error", func(r *liveRig) []liveStep {
			return []liveStep{{status: "error", sample: liveQuiet}, {sample: liveQuiet}, {sample: liveScrol}, {sample: liveScrol, check: r.mustBeSeen("error")}}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := newLiveRig(t)
			r.run(tc.steps(r))
			if r.writes != 1 {
				t.Fatalf("%d seen writes, want 1", r.writes)
			}
			row := r.row()
			if row.Status != tc.status || !row.StatusSeen() {
				t.Fatalf("after the attach: status=%s seen=%d status_at=%d", row.Status, row.SeenStatusAt, row.StatusAt)
			}
		})
	}
}

// mustBeSeen is a check that runs mid-attach: the episode is already seen.
func (r *liveRig) mustBeSeen(status string) func() {
	return func() {
		row := r.row()
		if row.Status != status || !row.StatusSeen() {
			r.t.Errorf("mid-attach, before detach: status=%s seen_status_at=%d status_at=%d; want %s already seen", row.Status, row.SeenStatusAt, row.StatusAt, status)
		}
	}
}

func TestFullAttachLeavesNewerUnobservedEpisodesUnseen(t *testing.T) {
	r := newLiveRig(t)
	unseen := func() {
		if row := r.row(); row.StatusSeen() {
			r.t.Errorf("newer unobserved episode was marked seen: status=%s seen=%d status_at=%d", row.Status, row.SeenStatusAt, row.StatusAt)
		}
	}
	r.run([]liveStep{
		{status: "waiting", sample: liveQuiet}, {sample: liveQuiet}, {sample: liveScrol}, {sample: liveScrol},
		// A second prompt arrives and nothing engages it before detach.
		{status: "running", sample: liveScrol}, {status: "error", sample: liveScrol}, {sample: liveScrol, check: unseen},
	})
	if r.writes != 1 {
		t.Fatalf("%d seen writes, want 1 (the first prompt's)", r.writes)
	}
	if row := r.row(); row.Status != "error" || row.StatusSeen() {
		t.Fatalf("status=%s seen=%d status_at=%d, want the newer error episode unseen", row.Status, row.SeenStatusAt, row.StatusAt)
	}
}

func TestFullAttachSecondTriggerOnAnEngagedEpisodeWritesNothing(t *testing.T) {
	r := newLiveRig(t)
	r.run([]liveStep{
		{status: "waiting", sample: liveQuiet}, {sample: liveQuiet}, {sample: liveScrol}, {sample: liveScrol},
		{keys: 1, sample: liveScrol}, {keys: 1, sample: liveScrol}, {keys: 2, sample: liveScrol}, {keys: 2, sample: liveScrol},
	})
	if r.writes != 1 {
		t.Fatalf("%d seen writes for one episode, want 1", r.writes)
	}
}

func TestFullAttachWithoutTriggerOrPromptWritesNothing(t *testing.T) {
	r := newLiveRig(t)
	r.run([]liveStep{{sample: liveQuiet}, {status: "waiting", sample: liveQuiet}, {sample: liveQuiet}, {sample: liveQuiet}})
	if r.writes != 0 {
		t.Fatalf("an attach with no trigger wrote %d times", r.writes)
	}
	if got := r.row(); got.Status != "waiting" || got.StatusSeen() {
		t.Fatalf("a prompt nobody engaged: status=%s seen=%d status_at=%d, want waiting and unseen", got.Status, got.SeenStatusAt, got.StatusAt)
	}
}

func TestFullAttachReportsAFailedMidAttachSeenWrite(t *testing.T) {
	m, _ := inertFixture(t, SortOrderAttention, "row-running", false)
	m.attachingSessionID = "row-running"
	updated, _ := m.Update(attachFinished{writes: []sessionEngaged{{err: context.DeadlineExceeded, trigger: engageKey}}})
	if got := updated.(Model).attachError; got != "Cannot mark seen: "+context.DeadlineExceeded.Error() {
		t.Fatalf("attach error %q, want the failed seen write reported", got)
	}
}
