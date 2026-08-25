package tui

import (
	"reflect"
	"testing"

	"github.com/n-orlov/deck/internal/store"
)

// assertStrictWeakOrdering exhaustively checks less over every ordered pair
// and triple of sessions: asymmetry (never both less(a,b) and less(b,a)),
// and transitivity (less(a,b) && less(b,c) => less(a,c)) -- the two
// properties sort.SliceStable silently assumes and does not itself verify.
// This is the "exhaustive less() checks over a 3-element tie set" the
// success criteria call for, in the shape attention.go's own doc comment
// warns a naive position/ID-mixing tie-break can violate.
func assertStrictWeakOrdering(t *testing.T, sessions []store.Session, less func(a, b store.Session) bool) {
	t.Helper()
	n := len(sessions)
	for i := 0; i < n; i++ {
		for j := 0; j < n; j++ {
			if i == j {
				continue
			}
			if less(sessions[i], sessions[j]) && less(sessions[j], sessions[i]) {
				t.Fatalf("asymmetry violated: less(%s,%s) and less(%s,%s) both true",
					sessions[i].ID, sessions[j].ID, sessions[j].ID, sessions[i].ID)
			}
		}
	}
	for i := 0; i < n; i++ {
		for j := 0; j < n; j++ {
			for k := 0; k < n; k++ {
				if i == j || j == k || i == k {
					continue
				}
				if less(sessions[i], sessions[j]) && less(sessions[j], sessions[k]) && !less(sessions[i], sessions[k]) {
					t.Fatalf("transitivity violated: less(%s,%s) and less(%s,%s) true but less(%s,%s) false",
						sessions[i].ID, sessions[j].ID, sessions[j].ID, sessions[k].ID, sessions[i].ID, sessions[k].ID)
				}
			}
		}
	}
}

// threeWayTieSet returns three sessions that tie on the given comparator's
// primary key but disagree on previous-frame membership: one has a
// previous position, one has a DIFFERENT previous position, and one has
// none at all (the exact mix attention.go's doc comment names as the
// concrete 3-cycle hazard: mixing "compare positions when both have one,
// else fall back to ID" is not a strict weak ordering once one row of a
// three-way tie is absent from previous). previous is built so id "b"
// preceded id "a" last frame (an order the ID tie-break alone would
// reverse), and "c" never appeared in previous at all.
func threeWayTieSet(withPrimary func(id string) store.Session) (previous []store.Session, tied []store.Session) {
	a := withPrimary("a")
	b := withPrimary("b")
	c := withPrimary("c")
	previous = []store.Session{b, a} // b at position 0, a at position 1; c absent
	tied = []store.Session{a, b, c}
	return previous, tied
}

// TestLessByCreatedStableOrdersNewestFirst proves the primary key: a later
// CreatedAt sorts before an earlier one.
func TestLessByCreatedStableOrdersNewestFirst(t *testing.T) {
	sessions := []store.Session{
		{ID: "old", CreatedAt: 100},
		{ID: "new", CreatedAt: 300},
		{ID: "mid", CreatedAt: 200},
	}
	got := sortSessionsStable(nil, sessions, lessByCreatedStable(nil))
	want := []string{"new", "mid", "old"}
	for i, id := range want {
		if got[i].ID != id {
			t.Fatalf("position %d: got %q, want %q (full: %v)", i, got[i].ID, id, idsOf(got))
		}
	}
}

// TestLessByActivityStableOrdersMostRecentFirst proves the primary key: a
// later StatusAt (more recent status CHANGE) sorts before an earlier one.
func TestLessByActivityStableOrdersMostRecentFirst(t *testing.T) {
	sessions := []store.Session{
		{ID: "old", StatusAt: 100},
		{ID: "new", StatusAt: 300},
		{ID: "mid", StatusAt: 200},
	}
	got := sortSessionsStable(nil, sessions, lessByActivityStable(nil))
	want := []string{"new", "mid", "old"}
	for i, id := range want {
		if got[i].ID != id {
			t.Fatalf("position %d: got %q, want %q (full: %v)", i, got[i].ID, id, idsOf(got))
		}
	}
}

// TestLessByNameStableOrdersCaseInsensitiveAscending proves the primary
// key: ascending, folding case, so "Bravo" and "alpha" compare by letter
// regardless of capitalisation.
func TestLessByNameStableOrdersCaseInsensitiveAscending(t *testing.T) {
	sessions := []store.Session{
		{ID: "z", Name: "Zulu"},
		{ID: "a", Name: "alpha"},
		{ID: "b", Name: "Bravo"},
	}
	got := sortSessionsStable(nil, sessions, lessByNameStable(nil))
	want := []string{"a", "b", "z"}
	for i, id := range want {
		if got[i].ID != id {
			t.Fatalf("position %d: got %q, want %q (full: %v)", i, got[i].ID, id, idsOf(got))
		}
	}
}

// TestSortComparatorsBreakTiesByIDWhenNoPreviousFrame proves the fallback
// half of the tie-break for all three new comparators: with no previous
// frame at all (both/all tied rows brand new), a primary-key tie resolves
// by ID ascending, exactly sortSessionsByAttention's own no-previous
// fallback.
func TestSortComparatorsBreakTiesByIDWhenNoPreviousFrame(t *testing.T) {
	cases := []struct {
		name string
		less func(previous []store.Session) func(a, b store.Session) bool
		make func(id string) store.Session
	}{
		{"created", lessByCreatedStable, func(id string) store.Session { return store.Session{ID: id, CreatedAt: 500} }},
		{"activity", lessByActivityStable, func(id string) store.Session { return store.Session{ID: id, StatusAt: 500} }},
		{"name", lessByNameStable, func(id string) store.Session { return store.Session{ID: id, Name: "same-name"} }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sessions := []store.Session{tc.make("zebra"), tc.make("alpha"), tc.make("mike")}
			got := sortSessionsStable(nil, sessions, tc.less(nil))
			want := []string{"alpha", "mike", "zebra"}
			for i, id := range want {
				if got[i].ID != id {
					t.Fatalf("%s: position %d: got %q, want %q (full: %v)", tc.name, i, got[i].ID, id, idsOf(got))
				}
			}
		})
	}
}

// TestSortComparatorsPreservePreviousRelativeOrderOnTie proves the middle
// tie-break level for all three new comparators, matching
// sortSessionsByAttentionStable's own behaviour: two rows tied on the
// primary key that already appeared together in the previous frame keep
// THEIR order from that frame, even when the ID order disagrees with it.
func TestSortComparatorsPreservePreviousRelativeOrderOnTie(t *testing.T) {
	cases := []struct {
		name string
		less func(previous []store.Session) func(a, b store.Session) bool
		make func(id string) store.Session
	}{
		{"created", lessByCreatedStable, func(id string) store.Session { return store.Session{ID: id, CreatedAt: 500} }},
		{"activity", lessByActivityStable, func(id string) store.Session { return store.Session{ID: id, StatusAt: 500} }},
		{"name", lessByNameStable, func(id string) store.Session { return store.Session{ID: id, Name: "same-name"} }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			zebra, alpha := tc.make("zebra"), tc.make("alpha")
			// zebra preceded alpha last frame -- the opposite of ID order.
			previous := []store.Session{zebra, alpha}
			incoming := []store.Session{alpha, zebra}
			got := sortSessionsStable(previous, incoming, tc.less(previous))
			if got[0].ID != "zebra" || got[1].ID != "alpha" {
				t.Fatalf("%s: previous relative order not preserved, got %v", tc.name, idsOf(got))
			}
		})
	}
}

// TestSortComparatorsThreeWayTieIsTransitive is the exhaustive check the
// success criteria demand: for each of the four orders, build a 3-element
// tie set on the primary key where the previous-frame membership is mixed
// (one row's previous position precedes another's, one row absent
// entirely -- attention.go's own doc comment names this exact mix as the
// hazard a naive "position when present, else ID" rule cannot survive) and
// prove less() is a strict weak ordering over it by exhaustive pairwise
// and triple-wise checks, not merely by observing one sorted output.
func TestSortComparatorsThreeWayTieIsTransitive(t *testing.T) {
	cases := []struct {
		name string
		less func(previous []store.Session) func(a, b store.Session) bool
		make func(id string) store.Session
	}{
		{"attention", attentionLessStable, func(id string) store.Session {
			return store.Session{ID: id, Status: "waiting", StatusAt: 500}
		}},
		{"created", lessByCreatedStable, func(id string) store.Session { return store.Session{ID: id, CreatedAt: 500} }},
		{"activity", lessByActivityStable, func(id string) store.Session { return store.Session{ID: id, StatusAt: 500} }},
		{"name", lessByNameStable, func(id string) store.Session { return store.Session{ID: id, Name: "same-name"} }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			previous, tied := threeWayTieSet(tc.make)
			assertStrictWeakOrdering(t, tied, tc.less(previous))
		})
	}
}

// TestSortSessionsByOrderDispatchesOnConfiguredOrder proves
// sortSessionsByOrder is the single entry point: each of the four names
// produces the order its own comparator would, and it never mutates
// incoming.
func TestSortSessionsByOrderDispatchesOnConfiguredOrder(t *testing.T) {
	sessions := []store.Session{
		{ID: "b", Name: "bravo", CreatedAt: 100, StatusAt: 100, Status: "idle"},
		{ID: "a", Name: "alpha", CreatedAt: 300, StatusAt: 300, Status: "waiting"},
		{ID: "c", Name: "charlie", CreatedAt: 200, StatusAt: 200, Status: "running"},
	}
	original := make([]store.Session, len(sessions))
	copy(original, sessions)

	cases := []struct {
		order string
		want  []string
	}{
		{SortOrderAttention, []string{"a", "c", "b"}},
		{SortOrderCreated, []string{"a", "c", "b"}},
		{SortOrderActivity, []string{"a", "c", "b"}},
		{SortOrderName, []string{"a", "b", "c"}},
	}
	for _, tc := range cases {
		t.Run(tc.order, func(t *testing.T) {
			got := sortSessionsByOrder(nil, sessions, tc.order)
			for i, id := range tc.want {
				if got[i].ID != id {
					t.Fatalf("order %q: position %d: got %q, want %q (full: %v)", tc.order, i, got[i].ID, id, idsOf(got))
				}
			}
		})
	}
	for i := range sessions {
		if !reflect.DeepEqual(sessions[i], original[i]) {
			t.Fatalf("sortSessionsByOrder mutated its input at index %d", i)
		}
	}
}

// TestSortSessionsByOrderUnknownValueFallsBackToAttention proves the
// dispatch never panics or silently misbehaves on a value outside the
// schema's four enum names -- it degrades to attention order (task 305
// covers surfacing this on the first painted frame; this is only the
// dispatch's own safety net).
func TestSortSessionsByOrderUnknownValueFallsBackToAttention(t *testing.T) {
	sessions := []store.Session{
		{ID: "w", Status: "waiting", StatusAt: 100},
		{ID: "s", Status: "stopped", StatusAt: 200},
	}
	got := sortSessionsByOrder(nil, sessions, "bogus-value")
	want := sortSessionsByAttentionStable(nil, sessions)
	for i := range want {
		if got[i].ID != want[i].ID {
			t.Fatalf("position %d: got %q, want %q (attention fallback)", i, got[i].ID, want[i].ID)
		}
	}
}
