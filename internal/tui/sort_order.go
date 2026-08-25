package tui

import (
	"sort"
	"strings"

	"github.com/n-orlov/deck/internal/store"
)

// Sort-order names, matching internal/config/schema.go's ui.sort_order
// EnumValues exactly -- these four strings are the only values
// sortSessionsByOrder ever treats as a real order; anything else falls
// back to attention (task 305 is the one that surfaces that fallback on
// the first painted frame -- this file's job is only to never panic or
// silently misbehave on a bad value).
const (
	SortOrderAttention = "attention"
	SortOrderCreated   = "created"
	SortOrderActivity  = "activity"
	SortOrderName      = "name"
)

// sortSessionsByOrder is the sidebar's one sort entry point (requirement
// R53/task 304): every caller picks an order by one of the four names
// above and gets back a freshly sorted slice via this function alone,
// never by re-deriving a comparator inline. previous is the sidebar's own
// last frame (exactly sortSessionsByAttentionStable's own previous/incoming
// shape) so every order -- not just attention -- prefers an
// already-resolved tie's existing relative order over a coin flip on ID;
// see attentionLessStable's doc comment for why that matters (the same
// 3-cycle hazard, the same fix, in every one of the four comparators
// below).
func sortSessionsByOrder(previous, incoming []store.Session, order string) []store.Session {
	switch order {
	case SortOrderCreated:
		return sortSessionsStable(previous, incoming, lessByCreatedStable(previous))
	case SortOrderActivity:
		return sortSessionsStable(previous, incoming, lessByActivityStable(previous))
	case SortOrderName:
		return sortSessionsStable(previous, incoming, lessByNameStable(previous))
	case SortOrderAttention:
		return sortSessionsByAttentionStable(previous, incoming)
	default:
		return sortSessionsByAttentionStable(previous, incoming)
	}
}

// sortSessionsStable runs incoming through sort.SliceStable with less,
// leaving incoming itself untouched -- exactly sortSessionsByAttention's
// own no-mutation contract, shared here so every order's entry point gives
// the same guarantee.
func sortSessionsStable(previous, incoming []store.Session, less func(a, b store.Session) bool) []store.Session {
	sorted := make([]store.Session, len(incoming))
	copy(sorted, incoming)
	sort.SliceStable(sorted, func(i, j int) bool {
		return less(sorted[i], sorted[j])
	})
	return sorted
}

// previousPositionKey returns a closure mapping a session to its index in
// previous, or one shared sentinel (len(previous)) for a session absent
// from it -- exactly attentionLessStable's own posKey, factored out here so
// every order's tie-break can share it instead of re-deriving the sentinel
// rule (and risking the position/ID-mixing 3-cycle its doc comment warns
// about).
func previousPositionKey(previous []store.Session) func(store.Session) int {
	prevPos := make(map[string]int, len(previous))
	for i, s := range previous {
		prevPos[s.ID] = i
	}
	return func(s store.Session) int {
		if p, ok := prevPos[s.ID]; ok {
			return p
		}
		return len(previous) // new this load: one shared key, ID breaks the rest
	}
}

// lessByCreatedStable orders by CreatedAt descending (newest first, per the
// schema key's own doc comment), a tied pair keeping its previous relative
// order, and any remaining tie (both new this load, both the same
// timestamp) breaking by ID ascending -- the same three-level composite key
// shape as attentionLessStable (primary key, then previous position, then
// ID), so the same 3-cycle hazard cannot arise here either: primary-key
// equality is a genuine equivalence relation (integer equality), so the
// composite stays a strict weak ordering.
func lessByCreatedStable(previous []store.Session) func(a, b store.Session) bool {
	posKey := previousPositionKey(previous)
	return func(a, b store.Session) bool {
		if a.CreatedAt != b.CreatedAt {
			return a.CreatedAt > b.CreatedAt
		}
		if ka, kb := posKey(a), posKey(b); ka != kb {
			return ka < kb
		}
		return a.ID < b.ID
	}
}

// lessByActivityStable orders by StatusAt descending -- a status CHANGE
// (§7's own timestamp), never last pane output, which deck does not
// record; see the schema key's own doc comment. Same tie-break shape as
// lessByCreatedStable.
func lessByActivityStable(previous []store.Session) func(a, b store.Session) bool {
	posKey := previousPositionKey(previous)
	return func(a, b store.Session) bool {
		if a.StatusAt != b.StatusAt {
			return a.StatusAt > b.StatusAt
		}
		if ka, kb := posKey(a), posKey(b); ka != kb {
			return ka < kb
		}
		return a.ID < b.ID
	}
}

// lessByNameStable orders by Name case-insensitive ascending. Same
// tie-break shape as lessByCreatedStable; the case-fold happens once per
// comparison rather than pre-computing a lowercase slice, matching the
// pure/no-mutation contract the other three orders share. Two names that
// fold to the same lowercase string (e.g. differing only in case) are a
// genuine tie here, exactly like a CreatedAt/StatusAt tie above.
func lessByNameStable(previous []store.Session) func(a, b store.Session) bool {
	posKey := previousPositionKey(previous)
	return func(a, b store.Session) bool {
		na, nb := strings.ToLower(a.Name), strings.ToLower(b.Name)
		if na != nb {
			return na < nb
		}
		if ka, kb := posKey(a), posKey(b); ka != kb {
			return ka < kb
		}
		return a.ID < b.ID
	}
}
