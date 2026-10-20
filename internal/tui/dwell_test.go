package tui

import (
	"context"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/n-orlov/deck/internal/store"
	"github.com/n-orlov/deck/internal/tmux"
)

// R322: a dwell of 1.5 s with the pane in view sets seen only when the prompt
// was already on screen when the dwell started. The five cases: prompt present
// at the start; prompt appears mid-dwell; pane collapsed; deck unfocused;
// arrowing past a row. Each drives the Model with a stepped dwell clock and
// the passive preview's capture messages, and reads the durable row.

const (
	dwellPromptPane = "Claude wants to use Bash: go test ./...\n\nDo you want to proceed?\n❯ 1. Yes\n  2. Yes, and don't ask again\n  3. No\n"
	dwellQuietPane  = "╭─ Claude Code ─────────────────╮\n│ ❯ refactor the status parser │\n╰───────────────────────────────╯\n✳ Working…  (esc to interrupt)\n"
	dwellTarget     = "row-waiting"
)

type dwellRig struct {
	t      *testing.T
	m      Model
	db     *store.Store
	now    time.Time
	writes int
}

func newDwellRig(t *testing.T) *dwellRig {
	t.Helper()
	m, db := inertFixture(t, SortOrderAttention, dwellTarget, false)
	r := &dwellRig{t: t, db: db, now: time.Unix(1_700_000_000, 0)}
	inner := m.markSeen
	if inner == nil {
		t.Fatal("fixture: the model has no markSeen write")
	}
	m.markSeen = func(ctx context.Context, id string, at store.StatusEpisode) (bool, error) {
		r.writes++
		return inner(ctx, id, at)
	}
	m.engagementCell().now = func() time.Time { return r.now }
	r.m = m
	if snapshotDurable(t, db, dwellTarget).Session.StatusSeen() {
		t.Fatal("fixture: the waiting row must start unseen")
	}
	return r
}

// send feeds one message through Update.
func (r *dwellRig) send(msg tea.Msg) {
	r.t.Helper()
	updated, _ := r.m.Update(msg)
	r.m = updated.(Model)
}

// capture delivers a passive-preview capture of the target showing text.
func (r *dwellRig) capture(text string) {
	r.t.Helper()
	r.send(previewCaptured{sessionID: dwellTarget, capture: tmux.PreviewCapture{Live: true, Bytes: []byte(text), Width: 80, Height: 24}})
}

// advance moves the dwell clock.
func (r *dwellRig) advance(d time.Duration) { r.now = r.now.Add(d) }

// tick runs the dwell trigger as a preview tick does and settles the write it
// issues, if any.
func (r *dwellRig) tick() {
	r.t.Helper()
	cmd := r.m.engageOnDwell()
	r.m = settleEngagement(r.t, r.m, cmd)
}

func (r *dwellRig) seen() bool {
	r.t.Helper()
	return snapshotDurable(r.t, r.db, dwellTarget).Session.StatusSeen()
}

func TestDwellWithPromptOnScreenAtStartSetsSeen(t *testing.T) {
	r := newDwellRig(t)
	before := snapshotDurable(t, r.db, dwellTarget)
	r.capture(dwellPromptPane)

	r.advance(seenDwell - time.Millisecond)
	r.tick()
	if r.seen() || r.writes != 0 {
		t.Fatalf("seen=%v writes=%d just short of the dwell, want unseen and no write", r.seen(), r.writes)
	}

	r.advance(time.Millisecond)
	r.tick()
	if !r.seen() || r.writes != 1 {
		t.Fatalf("seen=%v writes=%d after %v with the prompt on screen at the start, want seen by one write", r.seen(), r.writes, seenDwell)
	}
	after := snapshotDurable(t, r.db, dwellTarget)
	if after.Session.Status != before.Session.Status || after.Session.StatusAt != before.Session.StatusAt || after.Events != before.Events {
		t.Fatalf("a dwell changed more than seen: before %+v after %+v", before, after)
	}
	r.advance(time.Minute)
	r.tick()
	if r.writes != 1 {
		t.Fatalf("a second dwell tick wrote again: %d writes", r.writes)
	}
}

func TestDwellPromptAppearingMidDwellStaysUnseenUntilEngagement(t *testing.T) {
	r := newDwellRig(t)
	r.capture(dwellQuietPane) // the dwell starts with no prompt on screen
	r.advance(500 * time.Millisecond)
	r.capture(dwellPromptPane) // the prompt appears mid-dwell
	for i := 0; i < 4; i++ {
		r.advance(seenDwell)
		r.tick()
	}
	if r.seen() || r.writes != 0 {
		t.Fatalf("seen=%v writes=%d: a prompt that appeared mid-dwell was set seen by the dwell", r.seen(), r.writes)
	}
	// An engagement still sets it.
	r.m = settleEngagement(t, r.m, r.m.engage(engageKey))
	if !r.seen() {
		t.Fatal("an engagement after the dwell did not set seen")
	}
}

func TestDwellWithPaneCollapsedDoesNotSetSeen(t *testing.T) {
	r := newDwellRig(t)
	// A stacked layout too short for the preview's floor collapses the preview
	// panel away (SPEC §11.2: no capture runs for it either).
	r.m.layoutMode = LayoutStacked
	r.m.height = 10
	if r.m.computeLayout().PreviewShown {
		t.Fatal("fixture: the preview is still shown")
	}
	r.capture(dwellPromptPane)
	for i := 0; i < 3; i++ {
		r.advance(seenDwell)
		r.tick()
	}
	if r.seen() || r.writes != 0 {
		t.Fatalf("seen=%v writes=%d with the pane collapsed, want unseen", r.seen(), r.writes)
	}
}

func TestDwellWithDeckUnfocusedDoesNotSetSeen(t *testing.T) {
	r := newDwellRig(t)
	r.send(tea.BlurMsg{})
	r.capture(dwellPromptPane)
	for i := 0; i < 3; i++ {
		r.advance(seenDwell)
		r.tick()
	}
	if r.seen() || r.writes != 0 {
		t.Fatalf("seen=%v writes=%d with the deck unfocused, want unseen", r.seen(), r.writes)
	}

	// Focus back: a new dwell starts then, and it is the focused 1.5 s that counts.
	r.send(tea.FocusMsg{})
	r.advance(seenDwell - time.Millisecond)
	r.tick()
	if r.seen() {
		t.Fatal("the unfocused time counted toward the dwell")
	}
	r.advance(time.Millisecond)
	r.tick()
	if !r.seen() {
		t.Fatal("a full focused dwell with the prompt on screen did not set seen")
	}
}

func TestDwellArrowingPastARowDoesNotSetSeen(t *testing.T) {
	r := newDwellRig(t)
	r.capture(dwellPromptPane)
	r.advance(seenDwell - 100*time.Millisecond)

	r.send(key("down")) // arrow past the row ...
	if got, _ := r.m.selectedSession(); got.ID == dwellTarget {
		t.Fatal("fixture: down did not leave the waiting row")
	}
	r.advance(10 * time.Millisecond)
	r.tick()
	r.send(key("up")) // ... and back, between two ticks
	if got, _ := r.m.selectedSession(); got.ID != dwellTarget {
		t.Fatalf("fixture: up landed on %s", got.ID)
	}
	r.advance(200 * time.Millisecond)
	r.tick()
	if r.seen() || r.writes != 0 {
		t.Fatalf("seen=%v writes=%d: time on the row before it was arrowed past counted", r.seen(), r.writes)
	}

	r.advance(seenDwell)
	r.tick()
	if !r.seen() {
		t.Fatal("a full dwell after returning to the row did not set seen")
	}
}

// TestPreviewTickRunsTheDwellTrigger: the dwell is wired to the preview tick,
// the one cadence the product has, not only reachable from the helper.
func TestPreviewTickRunsTheDwellTrigger(t *testing.T) {
	r := newDwellRig(t)
	r.capture(dwellPromptPane)
	r.advance(seenDwell)
	updated, cmd := r.m.Update(previewTick(r.now))
	r.m = updated.(Model)
	if cmd == nil {
		t.Fatal("the preview tick returned no command")
	}
	var engaged *sessionEngaged
	var walk func(tea.Msg)
	walk = func(msg tea.Msg) {
		switch v := msg.(type) {
		case tea.BatchMsg:
			for _, c := range v {
				if c != nil {
					walk(c())
				}
			}
		case sessionEngaged:
			engaged = &v
		}
	}
	walk(cmd())
	if engaged == nil || engaged.trigger != engageDwell || engaged.err != nil {
		t.Fatalf("the preview tick did not issue the dwell write: %+v", engaged)
	}
	if !r.seen() {
		t.Fatal("the dwell write did not set seen")
	}
}

// setStatus writes a hook status for the target and reloads the list, as a
// hook followed by a store reload does.
func (r *dwellRig) setStatus(status string, at int64) {
	r.t.Helper()
	if err := r.db.UpdateSessionStatus(context.Background(), store.StatusUpdateInput{
		SessionID: dwellTarget, Status: status, Reason: "dwell test " + status, Source: "hook", At: at,
	}); err != nil {
		r.t.Fatal(err)
	}
	r.send(r.m.loadSessions())
	if got, _ := r.m.selectedSession(); got.ID != dwellTarget {
		r.t.Fatalf("fixture: the reload moved the selection to %s", got.ID)
	}
}

// settle advances past the dwell several times, ticking each time.
func (r *dwellRig) settle() {
	for i := 0; i < 4; i++ {
		r.advance(seenDwell)
		r.tick()
	}
}

// R322: the dwell is anchored to the start of the uninterrupted focused
// visible view. A prompt that appears after that view began stays unseen
// until engagement whichever of the capture, the hook or the reload brings it
// first.
func TestDwellViewStartedBeforeThePromptStaysUnseenWhateverTheOrdering(t *testing.T) {
	for _, tc := range []struct {
		name string
		run  func(r *dwellRig)
	}{
		{"capture then hook", func(r *dwellRig) {
			r.capture(dwellPromptPane)
			r.advance(100 * time.Millisecond)
			r.setStatus("waiting", 50_000)
		}},
		{"hook then capture", func(r *dwellRig) {
			r.setStatus("waiting", 50_000)
			r.advance(100 * time.Millisecond)
			r.capture(dwellPromptPane)
		}},
		{"prompt already in the capture before any status", func(r *dwellRig) {
			r.setStatus("waiting", 50_000)
			r.capture(dwellPromptPane)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := newDwellRig(t)
			r.setStatus("running", 5_000) // the row is not waiting when the view begins
			r.capture(dwellQuietPane)
			r.advance(time.Minute) // a long uninterrupted view
			r.tick()
			tc.run(r)
			r.settle()
			if r.seen() || r.writes != 0 {
				t.Fatalf("seen=%v writes=%d: a prompt that appeared after the view began was set seen by the dwell", r.seen(), r.writes)
			}
			r.m = settleEngagement(t, r.m, r.m.engage(engageKey))
			if !r.seen() {
				t.Fatal("an engagement did not set the late prompt seen")
			}
		})
	}
}

// A new status_at on a row already waiting is a prompt that appeared after
// the view began too: it neither restarts the clock nor becomes seen by dwell.
func TestDwellNewStatusAtMidViewDoesNotRestartOrSetSeen(t *testing.T) {
	r := newDwellRig(t)
	r.capture(dwellPromptPane)
	r.advance(seenDwell - 300*time.Millisecond)
	r.setStatus("running", 55_000) // the agent moves on and asks again mid-view
	r.setStatus("waiting", 60_000)
	r.advance(300 * time.Millisecond)
	r.tick()
	if r.seen() || r.writes != 0 {
		t.Fatalf("seen=%v writes=%d: the new prompt was set seen by the old view's dwell", r.seen(), r.writes)
	}
	if !r.m.engagement.dwelling || r.m.engagement.dwell.startedAt != time.Unix(1_700_000_000, 0) {
		t.Fatalf("the new status_at restarted the view: %+v", r.m.engagement.dwell)
	}
}

// The prompt that was on screen at the start becomes seen at 1.5 s from the
// view's start even when the row's own status was stamped just before it.
func TestDwellAlreadyVisiblePromptBecomesSeenAfterTheDwellFromViewStart(t *testing.T) {
	r := newDwellRig(t)
	r.capture(dwellPromptPane)
	r.advance(seenDwell / 2)
	r.tick()
	r.send(tea.WindowSizeMsg{Width: 100, Height: 30}) // an unrelated message does not restart it
	r.advance(seenDwell / 2)
	r.tick()
	if !r.seen() || r.writes != 1 {
		t.Fatalf("seen=%v writes=%d at 1.5 s from the view start, want seen by one write", r.seen(), r.writes)
	}
}

// Collapsing ends the view: a prompt that arrived while collapsed is on
// screen at the start of the next view, which then dwells normally.
func TestDwellCollapseEndsTheViewAndTheNextOneReadsItsPromptAfresh(t *testing.T) {
	r := newDwellRig(t)
	r.setStatus("running", 5_000)
	r.capture(dwellQuietPane)
	r.advance(time.Minute)
	full := r.m.height
	r.m.layoutMode, r.m.height = LayoutStacked, 10
	r.send(tea.WindowSizeMsg{Width: 100, Height: 10})
	if r.m.computeLayout().PreviewShown {
		t.Fatal("fixture: the preview is still shown")
	}
	r.setStatus("waiting", 50_000)
	r.advance(seenDwell)
	r.tick()
	if r.seen() {
		t.Fatal("a collapsed pane dwelled to seen")
	}
	r.capture(dwellPromptPane) // the pane's capture already shows the prompt when it reopens
	r.send(tea.WindowSizeMsg{Width: 100, Height: full})
	if !r.m.computeLayout().PreviewShown {
		t.Skip("fixture could not restore the preview")
	}
	r.advance(seenDwell - time.Millisecond)
	r.tick()
	if r.seen() {
		t.Fatal("the collapsed time counted toward the new view")
	}
	r.advance(time.Millisecond)
	r.tick()
	if !r.seen() {
		t.Fatal("a full view with the prompt on screen at its start did not set seen")
	}
}
