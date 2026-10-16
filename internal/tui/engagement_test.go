package tui

import (
	"context"
	"os/exec"
	"reflect"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/n-orlov/deck/internal/store"
)

// R321: the first forwarded key, the first scroll (a passive-preview scroll
// included) and the first screen-content change on an unseen waiting/error
// row set seen_status_at = status_at through one function (Model.engage).
// Engagement sets seen only: no status change, no moved row, and a second
// engagement writes nothing.

type engagementRig struct {
	m      Model
	db     *store.Store
	writes *int // markSeen invocations that reached the store
}

// engagementFixture seeds the four inert rows, selects target and counts every
// markSeen call. interactiveMode enters the interactive preview on target.
func engagementFixture(t *testing.T, sortOrder, target string, interactiveMode bool) engagementRig {
	t.Helper()
	m, db := inertFixture(t, sortOrder, target, true) // mouse on: the wheel triggers need it
	m = inertPane(t, m, db, target)
	writes := 0
	inner := m.markSeen
	if inner == nil {
		t.Fatal("fixture: the model has no markSeen write")
	}
	m.markSeen = func(ctx context.Context, id string, at int64) (bool, error) {
		writes++
		return inner(ctx, id, at)
	}
	if interactiveMode {
		next, _ := m.enterInteractive()
		m = next.(Model)
		if !m.interactive {
			t.Fatalf("interactive entry refused: %q %+v", m.attachError, m.entryRefusal)
		}
		t.Cleanup(func() { m.exitInteractive() })
	}
	if got := snapshotDurable(t, db, target).Session; got.StatusSeen() {
		t.Fatalf("fixture: %s must start unseen", target)
	}
	return engagementRig{m: m, db: db, writes: &writes}
}

// settleEngagement runs the engagement write cmd and feeds its result, and the
// reload that follows, back through Update.
func settleEngagement(t *testing.T, m Model, cmd tea.Cmd) Model {
	t.Helper()
	if cmd == nil {
		return m
	}
	msg := cmd()
	engaged, ok := msg.(sessionEngaged)
	if !ok {
		t.Fatalf("the engagement cmd returned %T, want sessionEngaged", msg)
	}
	if engaged.err != nil {
		t.Fatalf("engagement write failed: %v", engaged.err)
	}
	updated, reload := m.Update(engaged)
	m = updated.(Model)
	if reload != nil {
		updated, _ = m.Update(reload())
		m = updated.(Model)
	}
	return m
}

func assertSeenOnly(t *testing.T, what string, db *store.Store, id string, before durableSnapshot) {
	t.Helper()
	after := snapshotDurable(t, db, id)
	if !after.Session.StatusSeen() {
		t.Fatalf("%s did not set seen: seen_status_at=%d status_at=%d", what, after.Session.SeenStatusAt, after.Session.StatusAt)
	}
	if after.Session.SeenStatusAt != before.Session.StatusAt {
		t.Fatalf("%s set seen_status_at=%d, want the row's status_at %d", what, after.Session.SeenStatusAt, before.Session.StatusAt)
	}
	want := before
	want.Session.SeenStatusAt = before.Session.StatusAt
	if !reflect.DeepEqual(after, want) {
		t.Fatalf("%s changed more than seen_status_at:\nbefore=%+v\nafter =%+v", what, before, after)
	}
}

var engagementRows = []string{"row-waiting", "row-error"}

func TestEngagementFirstForwardedKeySetsSeen(t *testing.T) {
	for _, id := range engagementRows {
		t.Run(id, func(t *testing.T) {
			rig := engagementFixture(t, SortOrderActivity, id, true)
			before := snapshotDurable(t, rig.db, id)
			updated, cmd := rig.m.Update(key("x"))
			m := settleEngagement(t, updated.(Model), cmd)
			assertSeenOnly(t, "the first forwarded key", rig.db, id, before)
			if *rig.writes != 1 {
				t.Fatalf("the first forwarded key wrote %d times, want 1", *rig.writes)
			}
			if !m.interactive {
				t.Fatal("the key left interactive mode")
			}
		})
	}
}

func TestEngagementFirstInteractiveScrollSetsSeen(t *testing.T) {
	for _, id := range engagementRows {
		t.Run(id, func(t *testing.T) {
			rig := engagementFixture(t, SortOrderActivity, id, true)
			before := snapshotDurable(t, rig.db, id)
			layout := rig.m.computeLayout()
			x, y := layout.Sidebar.Width+3, 3
			if rig.m.hitTest(x, y).panel != hitPanelPreview {
				t.Fatalf("test setup: (%d,%d) is not over the preview", x, y)
			}
			updated, cmd := rig.m.Update(wheelUp(x, y))
			settleEngagement(t, updated.(Model), cmd)
			assertSeenOnly(t, "the first interactive scroll", rig.db, id, before)
			if *rig.writes != 1 {
				t.Fatalf("the first scroll wrote %d times, want 1", *rig.writes)
			}
		})
	}
}

func TestEngagementFirstPassivePreviewScrollSetsSeen(t *testing.T) {
	for _, id := range engagementRows {
		t.Run(id, func(t *testing.T) {
			rig := engagementFixture(t, SortOrderActivity, id, false)
			before := snapshotDurable(t, rig.db, id)
			layout := rig.m.computeLayout()
			x, y := layout.Sidebar.Width+3, 3
			if rig.m.hitTest(x, y).panel != hitPanelPreview {
				t.Fatalf("test setup: (%d,%d) is not over the preview", x, y)
			}
			updated, cmd := rig.m.Update(wheelDown(x, y))
			m := settleEngagement(t, updated.(Model), cmd)
			if m.interactive {
				t.Fatal("a passive-preview scroll entered interactive mode")
			}
			assertSeenOnly(t, "the first passive-preview scroll", rig.db, id, before)
			if *rig.writes != 1 {
				t.Fatalf("the first passive scroll wrote %d times, want 1", *rig.writes)
			}
		})
	}
}

func TestEngagementFirstScreenContentChangeSetsSeen(t *testing.T) {
	for _, id := range engagementRows {
		t.Run(id, func(t *testing.T) {
			rig := engagementFixture(t, SortOrderActivity, id, true)
			m := rig.m
			before := snapshotDurable(t, rig.db, id)
			// An unchanged screen engages nothing.
			if cmd := m.engageOnScreenChange(); cmd != nil {
				t.Fatal("an unchanged screen engaged")
			}
			// Something deck did not send reaches the pane: the tty echoes
			// the typed text onto its screen.
			target := m.interactiveWindowTarget
			socket := m.tmuxClient.Socket
			if out, err := exec.Command("tmux", "-L", socket, "send-keys", "-t", target, "-l", "engaged-by-output").CombinedOutput(); err != nil {
				t.Fatalf("send-keys: %v: %s", err, out)
			}
			var cmd tea.Cmd
			deadline := time.Now().Add(5 * time.Second)
			for cmd == nil {
				if cmd = m.engageOnScreenChange(); cmd == nil {
					if time.Now().After(deadline) {
						t.Fatal("the grid never showed the new output")
					}
					time.Sleep(20 * time.Millisecond)
				}
			}
			settleEngagement(t, m, cmd)
			assertSeenOnly(t, "the first screen-content change", rig.db, id, before)
			if *rig.writes != 1 {
				t.Fatalf("the screen change wrote %d times, want 1", *rig.writes)
			}
		})
	}
}

// TestSecondEngagementWritesNothing: after the first trigger set seen, every
// further trigger of every kind reaches no write at all (the model's markSeen
// is not even called), and the row is byte-identical.
func TestSecondEngagementWritesNothing(t *testing.T) {
	for _, id := range engagementRows {
		t.Run(id, func(t *testing.T) {
			rig := engagementFixture(t, SortOrderActivity, id, true)
			before := snapshotDurable(t, rig.db, id)
			updated, cmd := rig.m.Update(key("x"))
			m := settleEngagement(t, updated.(Model), cmd)
			if *rig.writes != 1 {
				t.Fatalf("first engagement wrote %d times, want 1", *rig.writes)
			}
			seen := snapshotDurable(t, rig.db, id)
			assertSeenOnly(t, "the first engagement", rig.db, id, before)

			x, y := m.computeLayout().Sidebar.Width+3, 3
			for name, run := range map[string]func() tea.Cmd{
				"key":    func() tea.Cmd { u, c := m.Update(key("y")); m = u.(Model); return c },
				"scroll": func() tea.Cmd { u, c := m.Update(wheelUp(x, y)); m = u.(Model); return c },
				"screen": m.engageOnScreenChange,
			} {
				if cmd := run(); cmd != nil {
					t.Fatalf("a second %s engagement issued a write", name)
				}
			}
			if *rig.writes != 1 {
				t.Fatalf("second engagements reached the store: %d writes, want 1", *rig.writes)
			}
			if got := snapshotDurable(t, rig.db, id); !reflect.DeepEqual(got, seen) {
				t.Fatalf("a second engagement changed the row:\nbefore=%+v\nafter =%+v", seen, got)
			}

			// Even a stale model that still shows the row unseen cannot write
			// twice: the stamp is remembered, and the store write itself is a
			// no-op on a seen row.
			stale := rig.m
			if cmd := stale.engage(engageKey); cmd != nil {
				t.Fatal("a model that already issued this stamp's write issued another")
			}
			changed, err := rig.db.MarkStatusSeen(context.Background(), id, seen.Session.StatusAt)
			if err != nil || changed {
				t.Fatalf("MarkStatusSeen on a seen row = (%v, %v), want (false, nil)", changed, err)
			}
			if got := snapshotDurable(t, rig.db, id); !reflect.DeepEqual(got, seen) {
				t.Fatalf("MarkStatusSeen on a seen row wrote:\nbefore=%+v\nafter =%+v", seen, got)
			}
		})
	}
}

// TestEngagementOfAnOlderPromptDoesNotHideANewerOne: the write names the
// stamp it saw, so a status that moved on in between stays unseen.
func TestEngagementOfAnOlderPromptDoesNotHideANewerOne(t *testing.T) {
	rig := engagementFixture(t, SortOrderActivity, "row-waiting", true)
	before := snapshotDurable(t, rig.db, "row-waiting")
	if err := rig.db.UpdateSessionStatus(context.Background(), store.StatusUpdateInput{
		SessionID: "row-waiting", Status: "error", Reason: "newer", Source: "hook", At: before.Session.StatusAt + 500,
	}); err != nil {
		t.Fatal(err)
	}
	changed, err := rig.db.MarkStatusSeen(context.Background(), "row-waiting", before.Session.StatusAt)
	if err != nil || changed {
		t.Fatalf("an engagement with the older stamp wrote: (%v, %v)", changed, err)
	}
	if rig.db == nil || snapshotDurable(t, rig.db, "row-waiting").Session.StatusSeen() {
		t.Fatal("the newer prompt was marked seen by an engagement with the older stamp")
	}
}

// TestEngagementChangesNoStatusAndMovesNoRow: each trigger, on each unseen
// attention row, in both sort orders, leaves every status, reason, source and
// status_at alone and keeps the list order. A waiting row sits in the same
// tier seen or unseen; an unseen error row drops below idle only through the
// seen predicate itself (R319), so its order is compared against the order Y
// would give, never against anything the engagement adds.
func TestEngagementChangesNoStatusAndMovesNoRow(t *testing.T) {
	triggers := map[string]func(t *testing.T, rig engagementRig) tea.Cmd{
		"key": func(_ *testing.T, rig engagementRig) tea.Cmd {
			_, cmd := rig.m.Update(key("x"))
			return cmd
		},
		"scroll": func(_ *testing.T, rig engagementRig) tea.Cmd {
			_, cmd := rig.m.Update(wheelUp(rig.m.computeLayout().Sidebar.Width+3, 3))
			return cmd
		},
	}
	for _, order := range []string{SortOrderAttention, SortOrderActivity} {
		for _, id := range engagementRows {
			for name, trigger := range triggers {
				t.Run(order+"/"+id+"/"+name, func(t *testing.T) {
					rig := engagementFixture(t, order, id, true)
					before := snapshotAll(t, rig.db)
					orderBefore := inertOrder(rig.m)
					cmd := trigger(t, rig)
					m := settleEngagement(t, rig.m, cmd)
					after := snapshotAll(t, rig.db)
					for rowID, was := range before {
						now := after[rowID]
						if rowID == id {
							was.Session.SeenStatusAt = was.Session.StatusAt
						}
						if !reflect.DeepEqual(now, was) {
							t.Fatalf("engagement changed row %s beyond seen:\nbefore=%+v\nafter =%+v", rowID, was, now)
						}
					}
					m = reloadInert(t, m)
					got := inertOrder(m)
					if order == SortOrderAttention && id == "row-error" {
						// R319: a seen error sorts just above stopped; that is
						// the predicate's tier, the same one Y produces.
						if got[len(got)-1] != "row-error" {
							t.Fatalf("seen error row is not in its seen-error tier: %v", got)
						}
						return
					}
					if !reflect.DeepEqual(got, orderBefore) {
						t.Fatalf("engagement moved a row: before=%v after=%v", orderBefore, got)
					}
				})
			}
		}
	}
}
