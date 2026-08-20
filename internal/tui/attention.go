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
