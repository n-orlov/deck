package tui

import (
	"context"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/n-orlov/deck/internal/store"
	"github.com/n-orlov/deck/internal/tmux"
)

// R325: what counts as viewed. Viewed = the passive preview visible on the row
// for >= 1.5 s, or interactive mode on it; last_seen_at keeps advancing while a
// row stays in view. Six cases: preview visible 1.5 s, interactive, a watched
// session does not go unseen, collapsed preview, unfocused deck, and arrowing
// down a list of unseen rows. Each reads the durable last_seen_at against
// last_output_at.

const viewTarget = "row-idle"

type viewRig struct {
	t      *testing.T
	m      Model
	db     *store.Store
	now    time.Time
	writes int
}

func newViewRig(t *testing.T) *viewRig {
	t.Helper()
	m, db := inertFixture(t, SortOrderAttention, viewTarget, false)
	r := &viewRig{t: t, db: db, now: time.Unix(1_700_000_000, 0)}
	r.m = r.instrument(m)
	for _, row := range inertRows {
		if r.viewed(row.id) {
			t.Fatalf("fixture: %s must start with unviewed output", row.id)
		}
	}
	return r
}

// instrument counts every viewing write and fixes the viewing clock.
func (r *viewRig) instrument(m Model) Model {
	inner := m.markViewed
	if inner == nil {
		r.t.Fatal("fixture: the model has no markViewed write")
	}
	m.markViewed = func(ctx context.Context, id string) (bool, error) {
		r.writes++
		return inner(ctx, id)
	}
	m.engagementCell().now = func() time.Time { return r.now }
	return m
}

func (r *viewRig) send(msg tea.Msg) {
	r.t.Helper()
	updated, _ := r.m.Update(msg)
	r.m = updated.(Model)
}

func (r *viewRig) capture() {
	r.t.Helper()
	r.send(previewCaptured{sessionID: viewTarget, capture: tmux.PreviewCapture{Live: true, Bytes: []byte(dwellQuietPane), Width: 80, Height: 24}})
}

func (r *viewRig) advance(d time.Duration) { r.now = r.now.Add(d) }

// tick runs the viewing trigger as a preview tick does and settles its write.
func (r *viewRig) tick() {
	r.t.Helper()
	r.m = settleView(r.t, r.m, r.m.engageOnView())
}

func settleView(t *testing.T, m Model, cmd tea.Cmd) Model {
	t.Helper()
	if cmd == nil {
		return m
	}
	msg, ok := cmd().(sessionViewed)
	if !ok {
		t.Fatalf("the viewing cmd returned %T, want sessionViewed", msg)
	}
	if msg.err != nil {
		t.Fatalf("viewing write failed: %v", msg.err)
	}
	updated, reload := m.Update(msg)
	m = updated.(Model)
	if reload != nil {
		updated, _ = m.Update(reload())
		m = updated.(Model)
	}
	return m
}

// viewed reports last_seen_at == last_output_at for the row.
func (r *viewRig) viewed(id string) bool {
	r.t.Helper()
	s := snapshotDurable(r.t, r.db, id).Session
	return s.LastSeenAt == s.LastOutputAt
}

func (r *viewRig) output(id string, at int64) {
	r.t.Helper()
	if err := r.db.RecordOutput(context.Background(), id, at); err != nil {
		r.t.Fatal(err)
	}
	r.m = reloadInert(r.t, r.m)
}

func TestPreviewVisibleForDwellMarksRowViewed(t *testing.T) {
	r := newViewRig(t)
	before := snapshotDurable(t, r.db, viewTarget)
	r.capture()

	r.advance(seenDwell - time.Millisecond)
	r.tick()
	if r.viewed(viewTarget) || r.writes != 0 {
		t.Fatalf("viewed=%v writes=%d just short of 1.5 s, want unviewed and no write", r.viewed(viewTarget), r.writes)
	}
	r.advance(time.Millisecond)
	r.tick()
	if !r.viewed(viewTarget) || r.writes != 1 {
		t.Fatalf("viewed=%v writes=%d after 1.5 s of visible preview, want viewed by one write", r.viewed(viewTarget), r.writes)
	}
	after := snapshotDurable(t, r.db, viewTarget)
	if after.Session.Status != before.Session.Status || after.Session.StatusAt != before.Session.StatusAt ||
		after.Session.SeenStatusAt != before.Session.SeenStatusAt || after.Events != before.Events {
		t.Fatalf("viewing changed more than last_seen_at: before %+v after %+v", before, after)
	}
}

func TestInteractiveModeOnARowMarksItViewed(t *testing.T) {
	const target = "row-waiting"
	rig := engagementFixture(t, SortOrderAttention, target, true)
	r := &viewRig{t: t, db: rig.db, now: time.Unix(1_700_000_000, 0)}
	r.m = r.instrument(rig.m)
	if r.viewed(target) {
		t.Fatal("fixture: the row must start with unviewed output")
	}
	// No dwell: interactive mode on the row is a view at the first tick.
	r.tick()
	if !r.viewed(target) || r.writes != 1 {
		t.Fatalf("viewed=%v writes=%d in interactive mode, want viewed by one write", r.viewed(target), r.writes)
	}
}

func TestWatchedSessionDoesNotGoUnseen(t *testing.T) {
	r := newViewRig(t)
	r.capture()
	r.advance(seenDwell)
	r.tick()
	if !r.viewed(viewTarget) {
		t.Fatal("the row was not viewed after 1.5 s")
	}
	// Output keeps landing in new buckets while the row stays selected.
	for i := int64(1); i <= 3; i++ {
		r.output(viewTarget, 2_000_000+i*10_000)
		if snapshotDurable(t, r.db, viewTarget).Session.LastSeenAt == 2_000_000+i*10_000 {
			t.Fatal("fixture: the output write moved last_seen_at")
		}
		r.advance(time.Second)
		r.tick()
		session := snapshotDurable(t, r.db, viewTarget).Session
		if session.LastSeenAt != 2_000_000+i*10_000 {
			t.Fatalf("round %d: last_seen_at = %d, want it to follow last_output_at %d while the row is watched", i, session.LastSeenAt, session.LastOutputAt)
		}
		if got, _ := r.m.selectedSession(); got.Unseen() {
			t.Fatalf("round %d: the watched row is unseen", i)
		}
	}
	if r.writes != 4 {
		t.Fatalf("%d viewing writes, want one per new output bucket (4)", r.writes)
	}
}

func TestCollapsedPreviewViewsNothing(t *testing.T) {
	r := newViewRig(t)
	r.m.layoutMode = LayoutStacked
	r.m.height = 10
	if r.m.computeLayout().PreviewShown {
		t.Fatal("fixture: the preview is still shown")
	}
	r.capture()
	for i := 0; i < 3; i++ {
		r.advance(seenDwell)
		r.tick()
	}
	if r.viewed(viewTarget) || r.writes != 0 {
		t.Fatalf("viewed=%v writes=%d with the preview collapsed, want unviewed", r.viewed(viewTarget), r.writes)
	}
}

func TestUnfocusedDeckViewsNothing(t *testing.T) {
	r := newViewRig(t)
	r.send(tea.BlurMsg{})
	r.capture()
	for i := 0; i < 3; i++ {
		r.advance(seenDwell)
		r.tick()
	}
	if r.viewed(viewTarget) || r.writes != 0 {
		t.Fatalf("viewed=%v writes=%d with the deck unfocused, want unviewed", r.viewed(viewTarget), r.writes)
	}
	// Focus back: the focused 1.5 s is what counts.
	r.send(tea.FocusMsg{})
	r.advance(seenDwell - time.Millisecond)
	r.tick()
	if r.viewed(viewTarget) {
		t.Fatal("the unfocused time counted toward the view")
	}
	r.advance(time.Millisecond)
	r.tick()
	if !r.viewed(viewTarget) {
		t.Fatal("a full focused 1.5 s did not mark the row viewed")
	}
}

func TestArrowingDownUnseenRowsLeavesPassedOverRowsUnseen(t *testing.T) {
	r := newViewRig(t)
	r.m = selectInert(t, r.m, r.m.sessions[0].ID)
	// show gives the selected row a live passive capture, as a preview tick would.
	show := func() string {
		current, _ := r.m.selectedSession()
		r.send(previewCaptured{sessionID: current.ID, capture: tmux.PreviewCapture{Live: true, Bytes: []byte(dwellQuietPane), Width: 80, Height: 24}})
		return current.ID
	}
	passed := []string{}
	for step := 0; step < len(r.m.sessions)-1; step++ {
		current := show()
		passed = append(passed, current)
		r.advance(seenDwell / 3) // each row is on screen for less than 1.5 s
		r.tick()
		r.send(key("down"))
		if got, _ := r.m.selectedSession(); got.ID == current {
			t.Fatalf("fixture: down did not leave %s", current)
		}
	}
	last := show()
	for _, id := range passed {
		if r.viewed(id) {
			t.Fatalf("row %s was passed over in under 1.5 s and was marked viewed", id)
		}
	}
	if r.writes != 0 {
		t.Fatalf("%d viewing writes while arrowing past rows, want none", r.writes)
	}
	// The row arrowing stopped on is viewed once it has been on screen 1.5 s.
	r.advance(seenDwell)
	r.tick()
	if !r.viewed(last) || r.writes != 1 {
		t.Fatalf("the row arrowing stopped on (%s) viewed=%v writes=%d after 1.5 s, want viewed by one write", last, r.viewed(last), r.writes)
	}
	for _, id := range passed {
		if r.viewed(id) {
			t.Fatalf("row %s became viewed when a different row was", id)
		}
	}
}

// TestPreviewTickRunsTheViewingTrigger: viewing is wired to the preview tick,
// the one cadence the product has, not only reachable from the helper.
func TestPreviewTickRunsTheViewingTrigger(t *testing.T) {
	r := newViewRig(t)
	r.capture()
	r.advance(seenDwell)
	updated, cmd := r.m.Update(previewTick(r.now))
	r.m = updated.(Model)
	if cmd == nil {
		t.Fatal("the preview tick returned no command")
	}
	var viewedMsg *sessionViewed
	var walk func(tea.Msg)
	walk = func(msg tea.Msg) {
		switch v := msg.(type) {
		case tea.BatchMsg:
			for _, c := range v {
				if c != nil {
					walk(c())
				}
			}
		case sessionViewed:
			viewedMsg = &v
		}
	}
	walk(cmd())
	if viewedMsg == nil || viewedMsg.err != nil || !viewedMsg.changed {
		t.Fatalf("the preview tick did not issue the viewing write: %+v", viewedMsg)
	}
}
