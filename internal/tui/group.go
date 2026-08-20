package tui

import "github.com/n-orlov/deck/internal/store"

// §11/§11.3 workspace grouping (SPEC requirement 30): the sidebar groups
// rows by sessions.workspace, defaulting to the basename of cwd, and never
// by any notion of repo. Each group has a collapsible header; collapsing a
// group hides its member rows but leaves the sidebar otherwise navigable —
// selection never lands on a hidden row.
//
// This file only groups and tracks collapse state; it deliberately does not
// re-order m.sessions (that is the attention sort, internal/tui/attention.go,
// task 023) — grouping here preserves each session's existing relative
// position, bucketed by workspace in order of each workspace's first
// appearance. Wiring the attention sort's order into (or across) these
// groups is left to whichever of tasks 025/026 first needs the sidebar's
// fully-attention-ordered, grouped render; this task's own success
// criteria only asks for the grouping and its collapse behaviour.

// sessionWorkspace returns SPEC requirement 30's grouping key for one
// session. internal/store's scanSession already applies the
// store.DefaultWorkspace fallback (basename of cwd) when reading a row
// with no explicit workspace, but a Session built directly (as most
// internal/tui tests do, bypassing the store) will have a zero-value
// Workspace, so this applies the identical fallback rather than grouping
// every such session under an empty-string header.
func sessionWorkspace(session store.Session) string {
	if session.Workspace != "" {
		return session.Workspace
	}
	return store.DefaultWorkspace(session.CWD)
}

// indexedSession pairs a session with its index into m.sessions, so a
// group can be rendered (and, once selected, resolved back to an index)
// without re-scanning m.sessions to find it.
type indexedSession struct {
	Index   int
	Session store.Session
}

// sidebarGroup is one workspace's header plus the sessions rendered under
// it, in m.sessions' own relative order.
type sidebarGroup struct {
	Workspace string
	Sessions  []indexedSession
}

// groupSessions splits m.sessions into workspace groups (SPEC requirement
// 30), ordered by each workspace's first appearance in m.sessions.
func (m Model) groupSessions() []sidebarGroup {
	var groups []sidebarGroup
	firstSeen := map[string]int{}
	for i, session := range m.sessions {
		ws := sessionWorkspace(session)
		if gi, ok := firstSeen[ws]; ok {
			groups[gi].Sessions = append(groups[gi].Sessions, indexedSession{Index: i, Session: session})
			continue
		}
		firstSeen[ws] = len(groups)
		groups = append(groups, sidebarGroup{Workspace: ws, Sessions: []indexedSession{{Index: i, Session: session}}})
	}
	return groups
}

// isGroupCollapsed reports whether m has collapsed the given workspace's
// group. A workspace never explicitly collapsed defaults to expanded, so a
// nil map (the zero Model) behaves exactly like an empty one.
func (m Model) isGroupCollapsed(workspace string) bool {
	return m.collapsedGroups[workspace]
}

// setGroupCollapsed collapses or expands one workspace's group. When
// collapsing hides the currently selected session, selection moves to the
// nearest still-visible session (forward first, then backward) so the
// sidebar is never left selecting an invisible row.
func (m *Model) setGroupCollapsed(workspace string, collapsed bool) {
	if collapsed {
		if m.collapsedGroups == nil {
			m.collapsedGroups = map[string]bool{}
		}
		m.collapsedGroups[workspace] = true
	} else if m.collapsedGroups != nil {
		delete(m.collapsedGroups, workspace)
	}
	m.selected = m.nearestVisibleSelection(m.selected)
}

// toggleGroupCollapse flips one workspace's collapse state. Task 028's
// mouse header click is the first caller this exists to serve; it is its
// own method (rather than inlined there) so a future keyboard duplicate
// (SPEC §11.8: "no capability is ever mouse-only") has the identical
// behaviour to bind.
func (m *Model) toggleGroupCollapse(workspace string) {
	m.setGroupCollapsed(workspace, !m.isGroupCollapsed(workspace))
}

// isSessionVisible reports whether the session at index i is presently
// shown in the sidebar: false only when its workspace group is collapsed,
// or when i is out of range.
func (m Model) isSessionVisible(i int) bool {
	if i < 0 || i >= len(m.sessions) {
		return false
	}
	return !m.isGroupCollapsed(sessionWorkspace(m.sessions[i]))
}

// nearestVisibleSelection returns the closest visible session index to
// from, searching forward first (so expanding/collapsing near the top of
// the list keeps selection moving in the direction of travel) and then
// backward, or 0 when no session is visible (an empty list is handled by
// every caller already, since m.selected is meaningless there).
func (m Model) nearestVisibleSelection(from int) int {
	if len(m.sessions) == 0 {
		return 0
	}
	if from < 0 {
		from = 0
	}
	if from > len(m.sessions)-1 {
		from = len(m.sessions) - 1
	}
	for i := from; i < len(m.sessions); i++ {
		if m.isSessionVisible(i) {
			return i
		}
	}
	for i := from - 1; i >= 0; i-- {
		if m.isSessionVisible(i) {
			return i
		}
	}
	return 0
}

// nextVisibleSelection and prevVisibleSelection are ↑/↓'s SPEC requirement
// 30 "remains navigable" behaviour: stepping past a collapsed group's
// hidden rows in one keypress rather than requiring one press per hidden
// row (which would silently do nothing on each of those presses).
func (m Model) nextVisibleSelection(from int) (int, bool) {
	for i := from + 1; i < len(m.sessions); i++ {
		if m.isSessionVisible(i) {
			return i, true
		}
	}
	return from, false
}

func (m Model) prevVisibleSelection(from int) (int, bool) {
	for i := from - 1; i >= 0; i-- {
		if m.isSessionVisible(i) {
			return i, true
		}
	}
	return from, false
}

// groupHeaderText renders one group's header line (SPEC requirement 30):
// a collapse-state marker, the workspace name, and its representative cwd
// (the first member session's, since every session in the default grouping
// shares it — an explicit sessions.workspace can group different cwds under
// one label, in which case this simply shows the first one seen). Like
// sidebarRowLines' rows, this returns raw (untruncated, unpadded) text; the
// sidebar's own render pipeline (sidebarContentLine's padTrunc) crops it to
// the panel's actual content width, so there is exactly one place doing
// that cell-aware cropping (task 019).
func (m Model) groupHeaderText(group sidebarGroup) string {
	marker := m.glyph("\u25be", "v") // expanded
	if m.isGroupCollapsed(group.Workspace) {
		marker = m.glyph("\u25b8", ">") // collapsed
	}
	cwd := ""
	if len(group.Sessions) > 0 {
		cwd = group.Sessions[0].Session.CWD
	}
	text := marker + " " + group.Workspace
	if cwd != "" {
		text += "  " + cwd
	}
	return text
}
