package tui

import (
	"sort"

	"github.com/n-orlov/deck/internal/store"
)

// §11 attention sort (SPEC requirements 28, 29): one pure, unit-tested
// function turning an unordered slice of sessions into the sidebar's order.
//
// Sort order is exactly: waiting (oldest first) -> error -> running ->
// starting -> idle -> stopped (requirement 28). That group order is total
// and deterministic (requirement 29): within a status, ties are broken by
// ascending StatusAt (the timestamp of the session's *current* status —
// i.e. "oldest first" for waiting is simply "ascending StatusAt", and every
// other status orders its members the same way for one consistent rule),
// and any StatusAt tie is broken by ascending session ID, which is unique
// and stable for the session's lifetime. Both keys are total orders over
// their domain, so the combined key is total: a frozen clock (identical
// StatusAt values) still yields exactly one frame, never two, because the
// ID always breaks the remaining tie.
//
// A status outside the six SPEC enumerates (there is currently no reachable
// path to any such status - "archived" included, see the phase2b1 findings
// report) sorts after "stopped", so an unexpected value degrades to "least
// urgent" rather than silently vanishing or jumping to the front.
const (
	attentionRankWaiting = iota
	attentionRankError
	attentionRankRunning
	attentionRankStarting
	attentionRankIdle
	attentionRankStopped
	attentionRankOther
)

// attentionRank maps a session status to its position in the requirement-28
// group order. It is the single source of truth for that order; the sort
// below and any future caller (e.g. a group-internal sort, task 024) must
// go through this function rather than re-deriving the mapping.
func attentionRank(status string) int {
	switch status {
	case "waiting":
		return attentionRankWaiting
	case "error":
		return attentionRankError
	case "running":
		return attentionRankRunning
	case "starting":
		return attentionRankStarting
	case "idle":
		return attentionRankIdle
	case "stopped":
		return attentionRankStopped
	default:
		return attentionRankOther
	}
}

// sortSessionsByAttention returns a new slice holding sessions ordered by
// the requirement-28/29 rule documented above. The input slice is never
// mutated, so callers holding onto the original order (e.g. to preserve a
// selection by index across a resort) can still compare against it.
func sortSessionsByAttention(sessions []store.Session) []store.Session {
	sorted := make([]store.Session, len(sessions))
	copy(sorted, sessions)
	sort.SliceStable(sorted, func(i, j int) bool {
		return lessByAttention(sorted[i], sorted[j])
	})
	return sorted
}

// lessByAttention is the total order itself: rank, then StatusAt ascending,
// then ID ascending. Exported as its own function (rather than inlined into
// the sort call) so task 025's "one shared attention source" and any future
// grouping code (task 024) can reuse the exact same tie-break without
// re-deriving it.
func lessByAttention(a, b store.Session) bool {
	ra, rb := attentionRank(a.Status), attentionRank(b.Status)
	if ra != rb {
		return ra < rb
	}
	if a.StatusAt != b.StatusAt {
		return a.StatusAt < b.StatusAt
	}
	return a.ID < b.ID
}

// sortSessionsByAttentionStable is sortSessionsByAttention's exact
// requirement-28/29 total order (rank, then StatusAt ascending), plus one
// refinement Model's own sessionsLoaded handler needs and the pure
// function above deliberately does not: when two sessions land on a
// genuine tie on BOTH keys (same rank, same StatusAt down to the
// millisecond -- unremarkable when a reconcile pass promotes two
// co-created sessions from "starting" to "running" in the same loop, see
// docs/reports/phase3d-i1-rootcause.md), sortSessionsByAttention's own
// ID tie-break is a coin flip against the session's random UUID, uncorrelated
// with which was created, marked, or selected first. A single k/m/j marked-set
// idiom (SPEC requirement 29's own fingerprint scenarios) can span exactly
// one such promotion: the row the user just marked can silently swap places
// with its neighbor between two keystrokes, leaving the very next "down"
// with nothing below it even though nothing the user did was wrong.
//
// previous is the sidebar's own last frame (m.sessions before this load).
// For a pair that already appeared together in it, this prefers THEIR
// existing relative order over the coin flip -- a tie that was already
// resolved one way stays resolved that way until something the sort
// itself cares about (rank or a distinguishable StatusAt) actually
// changes. A pair with no shared previous frame (both brand new this very
// load) still falls back to sortSessionsByAttention's own ID order,
// exactly matching it -- there is no earlier order to prefer, and this is
// the only case TestSortSessionsByAttentionTiesBrokenByID (which calls
// sortSessionsByAttention directly, never this function) needs to keep
// covering.
//
// The tie-break below is a single lexicographic composite key
// (position, ID) rather than "compare positions when both rows have one,
// else fall back to ID" -- the latter is NOT a strict weak ordering once
// three or more rows tie on rank+StatusAt and at least one of them is
// absent from previous (steering note
// docs/reports/phase3d-i1-rootcause.md's comparator addendum has the
// concrete 3-cycle: mixing the two rules lets less(B,A), less(A,C) and
// less(C,B) all report true for a suitable position/ID disagreement,
// which sort.SliceStable does not detect but silently mis-orders). Every
// row absent from previous maps to ONE shared sentinel position
// (len(previous)) so among themselves -- and against nothing else -- ID
// still breaks the tie; two rows that share a real previous position never
// reach the ID compare at all.
func sortSessionsByAttentionStable(previous, incoming []store.Session) []store.Session {
	return sortSessionsStable(previous, incoming, attentionLessStable(previous))
}

// attentionLessStable is sortSessionsByAttentionStable's own less function,
// factored out (task 304) so the sort-order dispatch's other three
// comparators can be tested against the exact same 3-element-tie-set shape
// this one is -- see sort_order.go's lessByCreatedStable and friends, which
// share this function's tie-break structure (primary key, then previous
// position, then ID) precisely to avoid the 3-cycle hazard this function's
// own doc comment above warns about.
func attentionLessStable(previous []store.Session) func(a, b store.Session) bool {
	posKey := previousPositionKey(previous)
	return func(a, b store.Session) bool {
		ra, rb := attentionRank(a.Status), attentionRank(b.Status)
		if ra != rb {
			return ra < rb
		}
		if a.StatusAt != b.StatusAt {
			return a.StatusAt < b.StatusAt
		}
		if ka, kb := posKey(a), posKey(b); ka != kb {
			return ka < kb
		}
		return a.ID < b.ID
	}
}

// indexOfSessionID returns the index of the session with the given ID in
// sessions, or -1 when id is empty (no prior selection to preserve) or no
// longer present (the session was removed since the last load). Model's
// sessionsLoaded handler (internal/tui/tui.go) uses this to keep the same
// session selected across a resort triggered by task 038's attention-order
// wiring, rather than a plain index carrying over into whatever session
// happens to now sit at that position.
func indexOfSessionID(sessions []store.Session, id string) int {
	if id == "" {
		return -1
	}
	for i, s := range sessions {
		if s.ID == id {
			return i
		}
	}
	return -1
}

// NeedsAttention is the one shared "needs me" answer requirements 31 and 32
// call for: true for a session in a status the sort itself treats as more
// urgent than "running" (currently waiting or error, matching
// internal/store's own isAttentionStatus). The sort
// (sortSessionsByAttention/lessByAttention above), the collapsed strip's
// count (Model.attentionCount) and `space` (Model.nextAttentionSelection,
// internal/tui/tui.go) all call this instead of re-deriving the status set,
// so the three can never silently disagree about what counts.
func NeedsAttention(session store.Session) bool {
	return attentionRank(session.Status) < attentionRankRunning
}

// replaceSessionByID swaps the in-memory copy of session (matched by id) in
// both the displayed list and baseSessions for the fresh row an action
// result carries, without re-sorting or moving the selection -- the reload
// that follows the action still re-derives order. A row not present (or an
// empty id) is left alone.
func (m *Model) replaceSessionByID(session store.Session) {
	if session.ID == "" {
		return
	}
	if idx := indexOfSessionID(m.sessions, session.ID); idx >= 0 {
		m.sessions[idx] = session
	}
	if idx := indexOfSessionID(m.baseSessions, session.ID); idx >= 0 {
		m.baseSessions[idx] = session
	}
}
