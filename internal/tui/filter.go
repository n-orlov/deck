package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/n-orlov/deck/internal/store"
)

// This file is task 123's `/` list filter (SPEC.md:984/§11.3, requirement
// 33, I-10; re-aimed off the removed workspace model by task 015/R129
// part 4): filters the sidebar by name, group and cwd, incrementally,
// clearing on Esc. Unlike every full-screen dialog in this package
// (createView, renameView, eventLogView, ...), filtering never replaces
// View() -- mainView keeps rendering the (now filtered) session list, with
// filterStatusLine stating the filter is in force so a hidden row is never
// mistaken for a deleted one, exactly as SPEC.md:319-320 requires.
//
// Archived rows (archived_at != 0) are excluded from store.ListSessions'
// own default view entirely -- SPEC requirement 27 -- so the filter is the
// only way one is found again: while a query is in force, archivedSessions
// (fetched by loadArchivedSessions every time `/` opens) is searched by the
// same name/group/cwd match as every other row, so typing enough of an
// archived session's own name, group or cwd surfaces it exactly the
// way it would surface a live one -- there is no separate "show archived"
// mode or keyword. Finding it is not the whole way back: SPEC.md:323-332
// makes archived_at reversible, so `U` on a row surfaced here clears the
// flag (R71, issue #8) and returns it to the default list.

// filterMatches reports whether session matches query (SPEC requirement
// 33) against its name, group LABEL (via sessionGroupLabel -- finding B1's
// cure: sessionGroupKey alone returns "" for both a NULL group_id and a
// dangling one, so "/default" matched nothing even though the sidebar
// header both cases fall under literally reads "default"; sessionGroupLabel
// resolves that same empty key to the literal "default" string the header
// and the `i`/`g` dialogs already show, so the filter matches whatever
// label a session actually renders under) or cwd -- the three fields the
// requirement names, and the only three ever consulted -- as a plain,
// case-insensitive substring test. An empty query matches everything (the
// unfiltered state). Because a session's own group label is one of the
// fields checked, a query matching only a group's label (and no session's
// own name or cwd) makes every member of that group match individually --
// there is no separate "match the group" step: a non-matching group simply
// has none of its sessions survive the filter, so groupSessions()
// (internal/tui/group.go), which buckets m.filteredSessions()' own output,
// never emits a header for it, and the header it DOES emit for a matching
// group carries that group's matching member count (len(group.Sessions),
// read off the already-filtered set), never the group's unfiltered total.
func filterMatches(session store.Session, query string) bool {
	if query == "" {
		return true
	}
	q := strings.ToLower(query)
	if strings.Contains(strings.ToLower(session.Name), q) {
		return true
	}
	if strings.Contains(strings.ToLower(sessionGroupLabel(session)), q) {
		return true
	}
	if strings.Contains(strings.ToLower(session.CWD), q) {
		return true
	}
	return false
}

// filteredSessions is the DISPLAYED session list (SPEC requirement 33):
// m.baseSessions verbatim while no query is in force, or every
// baseSessions/archivedSessions row matching filterMatches while one is,
// archivedSessions appended after baseSessions so a newly-surfaced
// archived match never displaces a live row's existing position. This is
// the one place requirement 33's "how are archived rows reached" is
// answered: they are excluded from m.baseSessions entirely (store.
// ListSessions' own default view), so a filter that only ever narrowed
// baseSessions could never surface one -- this widens the search pool to
// archivedSessions too, exactly while, and only while, a query is in
// force. A session present in both IS reachable, not merely a defensive
// checked-anyway case: `dd` on an archived row (archived_at != 0) only
// ever calls store.SoftDeleteSession, which sets deleted_at without
// touching archived_at (and store.RestoreSession's `u` undo is the exact
// mirror, clearing deleted_at alone) -- so deleted_at != 0 AND archived_at
// != 0 is a real, reachable row state. It never actually collides here,
// though: baseSessions never held it (ListSessions' own WHERE excludes
// any deleted_at != 0 row), and archivedSessions' own ListArchivedSessions
// query excludes it too (deleted_at = 0 AND archived_at != 0 -- `dd`
// setting deleted_at drops the row out of THAT list the same moment it
// leaves ListSessions', without ever clearing archived_at itself). The
// de-duplication below is kept anyway, favouring the baseSessions copy on
// an id collision, in case a future accessor ever loosens either query.
func (m Model) filteredSessions() []store.Session {
	if m.filterQuery == "" {
		return m.baseSessions
	}
	out := make([]store.Session, 0, len(m.baseSessions))
	seen := make(map[string]bool, len(m.baseSessions))
	for _, s := range m.baseSessions {
		seen[s.ID] = true
		if filterMatches(s, m.filterQuery) {
			out = append(out, s)
		}
	}
	for _, s := range m.archivedSessions {
		if seen[s.ID] {
			continue
		}
		if filterMatches(s, m.filterQuery) {
			out = append(out, s)
		}
	}
	return out
}

// updateFilter handles every key while task 123's `/` filter input has
// keyboard focus (m.filtering == true). It is a single free-text field, so
// SPEC \u00a711.4's shared dialogContract has nothing to add here (Esc's own
// behaviour below differs from that contract's plain cancel-and-close
// anyway: it clears the query, not merely the focus) -- the same reason
// renameView's updateRenameDialog also bypasses it for its own
// backspace/typing cases while still being a single free-text field.
// Every edit recomputes m.sessions immediately (SPEC's "incrementally")
// and re-lands the selection on the nearest still-visible row so an
// already-narrowed list never leaves the cursor on a row the new query
// just hid.
func (m Model) updateFilter(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		// SPEC §11.10 "esc clears the query and returns to the unfiltered
		// list": unlike Enter below, Esc discards the query entirely and
		// returns to the unfiltered list, not merely closing the text
		// field with the query still applied.
		selectedGroupHadNoRows := m.selectedGroupHadNoRows()
		m.filtering = false
		m.filterQuery = ""
		m.sessions = m.filteredSessions()
		m.selectVisibleStopAfterReload(selectedGroupHadNoRows)
		m.followSelectionViewport()
		return m, nil
	case "enter":
		// Closes the text field only; the query and the narrowed list it
		// produced both stay in force, so the freed keymap (arrows, mark,
		// attach, ...) now acts on the filtered set until a later `/` or
		// Esc changes it again.
		m.filtering = false
		return m, nil
	case "backspace", "ctrl+h":
		if m.filterQuery != "" {
			runes := []rune(m.filterQuery)
			m.filterQuery = string(runes[:len(runes)-1])
		}
	default:
		if runes := msg.Runes; len(runes) > 0 {
			m.filterQuery += string(runes)
		}
	}
	// cure-01-05 follow-up (task 022 sweep): fail-before was
	// features/filter.feature's own "dd found through / tombstones an
	// archived row" scenario, red at 340b4d6 ("after scenario hook failed:
	// timed out waiting for frame \"again\"") -- archiving the sidebar's
	// only session left m.selected on the (now empty) default group's
	// header, exactly like sessionsLoaded's own analogous case above, and
	// this handler's unconditional nearestVisibleSelection(m.selected) call
	// used to just hand that same header cursor straight back (a header
	// that is still visible is always its own "nearest visible stop"),
	// leaving the filtered-in archived row unreachable by `d`/`dd` even
	// though it was the only row on screen. selectedGroupHadNoRows must be
	// read from m.selected/m.sessions BEFORE filteredSessions() below
	// overwrites the list this keystroke just narrowed or widened.
	selectedGroupHadNoRows := m.selectedGroupHadNoRows()
	m.sessions = m.filteredSessions()
	m.selectVisibleStopAfterReload(selectedGroupHadNoRows)
	// cure-01-03 (R142, SPEC §11): a query edit (this shared tail --
	// backspace or a typed rune, never esc/enter above, which each return
	// earlier in the switch) is an intentional selection-follow operation,
	// not a "global control opened" one -- SPEC's drift rule ends the drift
	// on the very key that moves or acts on the selection, and narrowing or
	// widening the visible set by typing is exactly that: the row the
	// cursor now names may have moved to a completely different screen
	// position. followSelectionViewport below already recomputes the
	// offset for THIS keystroke; without also clearing the flag, the very
	// next background reload/re-sort saw sidebarScrollDrifted still true and
	// froze the stale wheel offset in place instead of following the
	// selection with its usual context margin.
	m.sidebarScrollDrifted = false
	m.followSelectionViewport()
	return m, nil
}

// filterStatusLine is the one line (rendered outside the panels, budgeted
// by computeLayout exactly like attachErrorLines/undoNoteLines/etc. --
// requirement 30) that states the filter is in force, per SPEC.md:319-320:
// "the sidebar states the filter is in force so a hidden row is never
// mistaken for a deleted one". Empty whenever there is nothing to state
// (never filtering and no query held over from a closed one), so it costs
// nothing in the common case, matching every sibling *Lines helper's own
// convention.
func (m Model) filterStatusLine(width int) []string {
	if !m.filtering && m.filterQuery == "" {
		return nil
	}
	if m.filtering {
		return m.canvasWrapText("Filter: "+m.filterQuery+"_", width)
	}
	return m.canvasWrapText(fmt.Sprintf("Filter %q in force (%d matching) \u2014 / to change, Esc to clear", m.filterQuery, len(m.sessions)), width)
}
