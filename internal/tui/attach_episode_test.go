package tui

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/n-orlov/deck/internal/store"
	"github.com/n-orlov/deck/internal/tmux"
)

// R319/R321/R322 in a full attach: the watcher's trigger is delivered after
// the attach ends, as attachFinished, and by then the row may be in a newer
// status episode -- one sharing the observed millisecond, or dated by a
// stepped-back clock -- and a sessionsLoaded may already have shown it. The
// trigger engages only the episode the watcher observed it on.

// attachEpisodeMove writes one status update to the rig's target row.
func attachEpisodeMove(t *testing.T, db *store.Store, status string, at int64) {
	t.Helper()
	if err := db.UpdateSessionStatus(context.Background(), store.StatusUpdateInput{
		SessionID: "row-waiting", Status: status, Reason: "episode test", Source: "hook", At: at,
	}); err != nil {
		t.Fatal(err)
	}
}

// finishAttach delivers attachFinished for the target row to m and runs the
// engagement write it issued, if any, returning whether one was issued.
func finishAttach(t *testing.T, m Model, msg attachFinished) (Model, bool) {
	t.Helper()
	m.attachingSessionID = "row-waiting"
	updated, cmd := m.Update(msg)
	m = updated.(Model)
	var engaged *sessionEngaged
	var walk func(tea.Cmd)
	walk = func(c tea.Cmd) {
		if c == nil {
			return
		}
		switch got := c().(type) {
		case tea.BatchMsg:
			for _, inner := range got {
				walk(inner)
			}
		case sessionEngaged:
			engaged = &got
		}
	}
	walk(cmd)
	if engaged == nil {
		return m, false
	}
	if engaged.err != nil {
		t.Fatalf("engagement write failed: %v", engaged.err)
	}
	updated, _ = m.Update(*engaged)
	return reloadInert(t, updated.(Model)), true
}

// The delayed trigger was observed on the first prompt; the row then moved to
// a newer attention episode and the model reloaded it before attachFinished
// arrived. Neither the write nor its absence may touch the newer episode, and
// the cached decision must not suppress engaging it afterwards.
func TestDelayedFullAttachTriggerCannotSeeAReloadedEpisode(t *testing.T) {
	for _, tc := range []struct {
		name string
		move func(t *testing.T, db *store.Store, observedAt int64)
	}{
		// The finding's own two cases.
		{"error on the same stamp", func(t *testing.T, db *store.Store, at int64) {
			attachEpisodeMove(t, db, "error", at)
		}},
		{"error after a clock step back", func(t *testing.T, db *store.Store, at int64) {
			attachEpisodeMove(t, db, "error", at-800)
		}},
		// The same rule, two more ways the next prompt can collide.
		{"a new waiting prompt via running, all in one millisecond", func(t *testing.T, db *store.Store, at int64) {
			attachEpisodeMove(t, db, "running", at)
			attachEpisodeMove(t, db, "waiting", at)
		}},
		{"a new waiting prompt back on the observed stamp after the clock stepped back", func(t *testing.T, db *store.Store, at int64) {
			attachEpisodeMove(t, db, "running", at-300)
			attachEpisodeMove(t, db, "error", at-200)
			attachEpisodeMove(t, db, "waiting", at)
		}},
	} {
		for _, trigger := range []engagementTrigger{engageKey, engageScroll, engageScreen} {
			t.Run(tc.name+"/"+string(trigger), func(t *testing.T) {
				rig := engagementFixture(t, SortOrderAttention, "row-waiting", false)
				observed := snapshotDurable(t, rig.db, "row-waiting").Session
				if observed.StatusSeen() || observed.Status != "waiting" {
					t.Fatalf("fixture: want an unseen waiting row, got %+v", observed)
				}
				tc.move(t, rig.db, observed.StatusAt)
				m := reloadInert(t, rig.m) // sessionsLoaded lands before attachFinished
				newer := snapshotDurable(t, rig.db, "row-waiting").Session
				if newer.Episode() == observed.Episode() || newer.StatusSeen() {
					t.Fatalf("fixture: want a newer unseen episode, observed=%+v newer=%+v", observed.Episode(), newer)
				}
				m, _ = finishAttach(t, m, attachFinished{engaged: trigger, observed: observed})
				got := snapshotDurable(t, rig.db, "row-waiting").Session
				if got.StatusSeen() || got.SeenStatusAt != newer.SeenStatusAt {
					t.Fatalf("a trigger observed on episode %+v marked the newer episode %+v seen (seen_status_at %d -> %d)",
						observed.Episode(), newer.Episode(), newer.SeenStatusAt, got.SeenStatusAt)
				}
				if got.Status != newer.Status || got.StatusAt != newer.StatusAt || got.StatusSeq != newer.StatusSeq {
					t.Fatalf("the delayed trigger moved the row: %+v -> %+v", newer, got)
				}
				// Engagement observed on the newer episode still sets it seen:
				// the older episode's cached decision does not suppress it.
				m, issued := finishAttach(t, m, attachFinished{engaged: trigger, observed: newer})
				if !issued {
					t.Fatal("the older episode's engagement suppressed engaging the newer one")
				}
				if got := snapshotDurable(t, rig.db, "row-waiting").Session; !got.StatusSeen() || got.SeenStatusAt != got.StatusAt {
					t.Fatalf("engagement observed on the newer episode did not set seen: %+v", got)
				}
				if _, again := finishAttach(t, m, attachFinished{engaged: trigger, observed: newer}); again {
					t.Fatal("a second engagement of the same episode issued another write")
				}
			})
		}
	}
}

// A repeat of the observed status (R303) is the same episode: the delayed
// trigger still sets it seen, after the reload too.
func TestDelayedFullAttachTriggerSurvivesARepeatOfItsEpisode(t *testing.T) {
	rig := engagementFixture(t, SortOrderAttention, "row-waiting", false)
	observed := snapshotDurable(t, rig.db, "row-waiting").Session
	attachEpisodeMove(t, rig.db, "waiting", observed.StatusAt+5000)
	repeated := snapshotDurable(t, rig.db, "row-waiting").Session
	if repeated.Episode() != observed.Episode() {
		t.Fatalf("fixture: a repeat moved the episode %+v -> %+v", observed.Episode(), repeated.Episode())
	}
	m := reloadInert(t, rig.m)
	if _, issued := finishAttach(t, m, attachFinished{engaged: engageKey, observed: observed}); !issued {
		t.Fatal("a repeat of the observed status suppressed its engagement")
	}
	if got := snapshotDurable(t, rig.db, "row-waiting").Session; !got.StatusSeen() || got.StatusAt != observed.StatusAt {
		t.Fatalf("the repeat's episode was not set seen: %+v", got)
	}
}

// A trigger with no observed row, or one observed on another row, writes
// nothing: the trigger alone never names an episode.
func TestFullAttachTriggerWithoutItsObservedRowWritesNothing(t *testing.T) {
	rig := engagementFixture(t, SortOrderAttention, "row-waiting", false)
	other := snapshotDurable(t, rig.db, "row-error").Session
	for _, observed := range []store.Session{{}, other} {
		if _, issued := finishAttach(t, rig.m, attachFinished{engaged: engageKey, observed: observed}); issued {
			t.Fatalf("a trigger observed on %q issued a write for row-waiting", observed.ID)
		}
	}
	if *rig.writes != 0 {
		t.Fatalf("%d writes", *rig.writes)
	}
}

// --- the watcher: which episode a trigger observed ------------------------

// scriptedAttach drives watchedAttach.watch through a fixed list of steps.
// Each step happens between the row read that precedes a sample and the
// sample itself, so it lands inside the interval that sample closes: the
// row may change episode, keys may be forwarded, and the sample is returned.
type attachStep struct {
	row    *store.Session // the row's new state, nil = unchanged
	keys   int64          // keys forwarded so far
	sample tmux.AttachSample
}

func runScriptedWatch(t *testing.T, start store.Session, steps []attachStep) attachEngagement {
	t.Helper()
	row := start
	var keys atomic.Int64
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	next := 0
	w := &watchedAttach{
		slug: "s", interval: time.Millisecond, settle: time.Second,
		now:  func() time.Time { return time.Unix(1000, 0).Add(time.Duration(next) * 200 * time.Millisecond) },
		read: func(context.Context) (store.Session, error) { return row, nil },
		sample: func(context.Context, string, int) (tmux.AttachSample, error) {
			if next >= len(steps) {
				cancel()
				return tmux.AttachSample{}, errors.New("script done")
			}
			step := steps[next]
			next++
			if step.row != nil {
				row = *step.row
			}
			keys.Store(step.keys)
			return step.sample, nil
		},
	}
	bracket := &episodeBracket{read: w.readRow, reads: map[int]store.Session{}}
	bracket.before(ctx, 0)
	baseline := preAttachSample()
	return w.watch(ctx, 1, &baseline, &keys, bracket)
}

func attachRow(status string, at, seq int64) *store.Session {
	return &store.Session{ID: "s", Status: status, StatusAt: at, StatusSeq: seq}
}

func TestAttachWatcherBindsTheTriggerToTheEpisodeItObserved(t *testing.T) {
	quiet := attachSampleAt(80, 24, "prompt")
	first := *attachRow("waiting", 3010, 1)
	for _, tc := range []struct {
		name    string
		steps   []attachStep
		want    engagementTrigger
		wantEp  store.StatusEpisode
		comment string
	}{
		{name: "a key on a steady episode", steps: []attachStep{
			{keys: 1, sample: quiet}, {keys: 1, sample: quiet},
		}, want: engageKey, wantEp: first.Episode()},
		{name: "a key whose interval saw an error on the same stamp", steps: []attachStep{
			{row: attachRow("error", 3010, 2), keys: 1, sample: quiet}, {keys: 1, sample: quiet},
		}},
		{name: "a key whose confirmation saw a stepped-back prompt", steps: []attachStep{
			{keys: 1, sample: quiet}, {row: attachRow("waiting", 2210, 3), keys: 1, sample: quiet},
		}},
		{name: "a screen change whose interval saw a new prompt on the same stamp", steps: []attachStep{
			{row: attachRow("waiting", 3010, 3), sample: attachSampleAt(80, 24, "prompt 2")},
		}},
		{name: "a key observed, then the row moved on: the old episode is reported", steps: []attachStep{
			{keys: 1, sample: quiet}, {keys: 1, sample: quiet}, {row: attachRow("error", 3010, 2), keys: 1, sample: quiet}, {keys: 1, sample: quiet},
		}, want: engageKey, wantEp: first.Episode()},
		{name: "a key on a later episode of the same attach", steps: []attachStep{
			{row: attachRow("error", 3010, 2), sample: quiet}, {sample: quiet}, {keys: 1, sample: quiet}, {keys: 1, sample: quiet},
		}, want: engageKey, wantEp: store.StatusEpisode{At: 3010, Seq: 2}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := runScriptedWatch(t, first, tc.steps)
			if got.trigger != tc.want {
				t.Fatalf("trigger %q, want %q (observed %+v)", got.trigger, tc.want, got.observed.Episode())
			}
			if tc.want != "" && got.observed.Episode() != tc.wantEp {
				t.Fatalf("observed episode %+v, want %+v", got.observed.Episode(), tc.wantEp)
			}
		})
	}
}
