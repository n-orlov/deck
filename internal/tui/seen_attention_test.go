package tui

import (
	"context"
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
)

// R319: needs-attention is waiting || (error && unseen), where seen means
// seen_status_at == status_at. The sort tier, the collapsed strip's count and
// `space` all call that one predicate; these tests drive the four rows of the
// requirement through all three and Y.

func seenRow(id, status string, statusAt int64, seen bool) store.Session {
	s := store.Session{ID: id, Name: id, Slug: id, Agent: "claude", Status: status, StatusAt: statusAt}
	if seen {
		s.SeenStatusAt = statusAt
	}
	return s
}

func fourAttentionRows() []store.Session {
	return []store.Session{
		seenRow("unseen-waiting", "waiting", 300, false),
		seenRow("seen-waiting", "waiting", 200, true),
		seenRow("unseen-error", "error", 100, false),
		seenRow("seen-error", "error", 50, true),
	}
}

func TestNeedsAttentionIsWaitingOrUnseenError(t *testing.T) {
	want := map[string]bool{
		"unseen-waiting": true,
		"seen-waiting":   true, // still counted and a space target
		"unseen-error":   true,
		"seen-error":     false,
	}
	for _, s := range fourAttentionRows() {
		if got := NeedsAttention(s); got != want[s.ID] {
			t.Errorf("NeedsAttention(%s) = %v, want %v", s.ID, got, want[s.ID])
		}
	}
}

// The sort tier, the strip count and space agree on every one of the four
// rows, each tested alone and then together.
func TestSortCountAndSpaceAgreeOnTheFourRows(t *testing.T) {
	for _, s := range fourAttentionRows() {
		t.Run(s.ID, func(t *testing.T) {
			need := NeedsAttention(s)
			inTopTiers := sessionAttentionRank(s) < attentionRankRunning
			m := Model{sessions: []store.Session{s}}
			counted := m.attentionCount() == 1
			_, spaceTarget := m.nextAttentionSelection(rowCursor(0))
			if inTopTiers != need || counted != need || spaceTarget != need {
				t.Fatalf("%s: NeedsAttention=%v sort-tier=%v strip-count=%v space=%v, want all equal", s.ID, need, inTopTiers, counted, spaceTarget)
			}
		})
	}

	m := Model{sessions: fourAttentionRows()}
	if got := m.attentionCount(); got != 3 {
		t.Errorf("attentionCount() = %d, want 3 (seen error dropped)", got)
	}
	// space from the seen error walks unseen-waiting, seen-waiting, unseen-error
	// and never lands on the seen error.
	var visited []string
	cur := rowCursor(3)
	for i := 0; i < 3; i++ {
		next, ok := m.nextAttentionSelection(cur)
		if !ok {
			t.Fatalf("space step %d found nothing", i)
		}
		idx, _ := next.SessionIndex()
		visited = append(visited, m.sessions[idx].ID)
		cur = next
	}
	if got, want := strings.Join(visited, ","), "unseen-waiting,seen-waiting,unseen-error"; got != want {
		t.Errorf("space visited %s, want %s", got, want)
	}
}

func TestSeenErrorSortsJustAboveStopped(t *testing.T) {
	in := append(fourAttentionRows(),
		seenRow("stopped", "stopped", 10, false),
		seenRow("idle", "idle", 20, true),
		seenRow("running", "running", 30, true),
		seenRow("starting", "starting", 40, true),
	)
	ids := func(ss []store.Session) string {
		out := make([]string, len(ss))
		for i, s := range ss {
			out[i] = s.ID
		}
		return strings.Join(out, ",")
	}
	// waiting oldest first (seen or not), then unseen error, then running,
	// starting, idle, then the seen error just above stopped.
	want := "seen-waiting,unseen-waiting,unseen-error,running,starting,idle,seen-error,stopped"
	if got := ids(sortSessionsByAttention(in)); got != want {
		t.Errorf("sortSessionsByAttention = %s, want %s", got, want)
	}
	if got := ids(sortSessionsByAttentionStable(nil, in)); got != want {
		t.Errorf("sortSessionsByAttentionStable = %s, want %s", got, want)
	}
}

// Y marks the row seen without attaching, is offered only where something is
// unseen and does nothing otherwise.
func TestYMarksSeenWithoutAttaching(t *testing.T) {
	m, db := ackOfferStoreModel(t)
	if !canAcknowledge(m.sessions[0]) {
		t.Fatalf("unseen waiting row must be acknowledgeable: %#v", m.sessions[0])
	}
	before := m.sessions[0]
	updated, cmd := m.Update(key("Y"))
	if cmd == nil {
		t.Fatal("Y on an unseen row scheduled nothing")
	}
	m = updated.(Model)
	updated, _ = m.Update(cmd())
	m = updated.(Model)
	got, err := db.GetSession(context.Background(), "waiting")
	if err != nil {
		t.Fatal(err)
	}
	if got.SeenStatusAt != got.StatusAt || !got.StatusSeen() {
		t.Errorf("Y did not mark seen: seen_status_at=%d status_at=%d", got.SeenStatusAt, got.StatusAt)
	}
	if got.Status != "waiting" || got.StatusAt != before.StatusAt || got.StatusSource != before.StatusSource {
		t.Errorf("Y changed the status verdict: before %q/%d/%s after %q/%d/%s",
			before.Status, before.StatusAt, before.StatusSource, got.Status, got.StatusAt, got.StatusSource)
	}
	var attached int
	if err := db.DB().QueryRow(`SELECT count(*) FROM events WHERE kind LIKE '%attach%'`).Scan(&attached); err != nil {
		t.Fatal(err)
	}
	if attached != 0 {
		t.Errorf("Y recorded %d attach events, want 0", attached)
	}
}

func TestYIsOfferedOnlyWhereSomethingIsUnseen(t *testing.T) {
	cases := []struct {
		row  store.Session
		want bool
	}{
		{seenRow("a", "waiting", 100, false), true},
		{seenRow("a", "error", 100, false), true},
		{seenRow("a", "waiting", 100, true), false},
		{seenRow("a", "error", 100, true), false},
		{seenRow("a", "running", 100, true), false},
		{seenRow("a", "idle", 100, true), false},
	}
	for _, c := range cases {
		name := c.row.Status + map[bool]string{true: "/seen", false: "/unseen"}[c.row.StatusSeen()]
		t.Run(name, func(t *testing.T) {
			if got := canAcknowledge(c.row); got != c.want {
				t.Errorf("canAcknowledge = %v, want %v", got, c.want)
			}
			m := New(nil, config.Settings{}, "")
			m.sessions = []store.Session{c.row}
			m.selected = rowCursor(0)
			if offered := strings.Contains(m.footerKeyLegend(), "Y acknowledge"); offered != c.want {
				t.Errorf("footer offers Y = %v, want %v", offered, c.want)
			}
			called := 0
			m.acknowledge = func(context.Context, string) error { called++; return nil }
			_, cmd := m.Update(key("Y"))
			if c.want != (cmd != nil) {
				t.Errorf("Y scheduled a command = %v, want %v", cmd != nil, c.want)
			}
			if !c.want && called != 0 {
				t.Errorf("acknowledge called %d times on a no-op Y", called)
			}
		})
	}
}
