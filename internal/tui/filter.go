package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/n-orlov/deck/internal/store"
)

// This file is task 123's `/` list filter (SPEC.md:984/§11.3, requirement
// 33, I-10): filters the sidebar by name, workspace and cwd, incrementally,
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
// same name/workspace/cwd match as every other row, so typing enough of an
// archived session's own name, workspace or cwd surfaces it exactly the
// way it would surface a live one -- there is no separate "show archived"
// mode or keyword. Finding it is not the whole way back: SPEC.md:323-332
// makes archived_at reversible, so `U` on a row surfaced here clears the
// flag (R71, issue #8) and returns it to the default list.

// filterMatches reports whether session matches query (SPEC requirement
// 33) against its name, workspace or cwd -- the three fields the
// requirement names, and the only three ever consulted -- as a plain,
// case-insensitive substring test. An empty query matches everything (the
// unfiltered state).
func filterMatches(session store.Session, query string) bool {
	if query == "" {
		return true
	}
	q := strings.ToLower(query)
	if strings.Contains(strings.ToLower(session.Name), q) {
		return true
	}
	if strings.Contains(strings.ToLower(sessionGroupKey(session)), q) {
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
		m.filtering = false
		m.filterQuery = ""
		m.sessions = m.filteredSessions()
		m.selected = m.nearestVisibleSelection(m.selected)
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
	m.sessions = m.filteredSessions()
	m.selected = m.nearestVisibleSelection(m.selected)
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
