package tui

import (
	"sort"

	"github.com/n-orlov/deck/internal/store"
	"github.com/n-orlov/deck/internal/theme"
)

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

// sessionWorkspace is internal/tui's one group-key accessor (task 007,
// Tier 2 preparation; task 008, R128, rewired it to read the actual group
// model instead of borrowing it): every other file that needs to know
// which group a session belongs to calls this (or sessionGroupDisplayName
// below) rather than reading store.Session.GroupName directly, so there
// is exactly one place to change when a later task (009's group CRUD,
// 011-013's group-id sidebar) needs the key to carry more than a bare
// name. It returns store.Session.GroupName verbatim -- empty for the
// implicit default group, per SPEC §11, with no cwd-derived fallback of
// any kind (that concept left with the removed Workspace field and
// store.DefaultWorkspace; SPEC.md:203-204's OWN basename-of-cwd rule for
// the create modal's blank-name default is unrelated and now lives in
// tui.go's createNameCWDBasename, never through this seam).
func sessionWorkspace(session store.Session) string {
	return session.GroupName
}

// sessionGroupDisplayName is the seam's companion display-name accessor:
// the label a group's header should show for one of its member sessions.
// Identical to sessionWorkspace's group key at this commit -- there is
// still no separate group-name storage distinct from the key itself (a
// group's name IS its key, per SPEC §4's groups table) -- kept as its
// own function so a later task that ever needs the two to diverge only
// has to change this function's body, not every call site across
// internal/tui.
func sessionGroupDisplayName(session store.Session) string {
	return sessionWorkspace(session)
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

// reorderPreservingGrouping is task 305/309's (R53) answer to composing a
// non-attention sort_order with SPEC requirement 30's workspace grouping:
// which workspace group renders FIRST must keep following groupBasis's own
// first-appearance order (in practice always the attention-sorted session
// list -- "the group with the most urgent member leads", tui.go's
// sessionsLoaded comment) regardless of which order the user picked, while
// the ROWS within each workspace follow rowBasis (the chosen order) --
// task 309's success criteria states this explicitly: "group order itself
// unchanged from today's behavior". Both slices must hold exactly the same
// set of sessions, only reordered differently; the result feeds
// m.baseSessions so that groupSessions() itself (whose own
// first-appearance rule is left untouched, per this file's package doc
// comment) reproduces exactly this same bucket sequence and per-bucket row
// order without needing to know anything about sort_order at all -- every
// workspace's sessions land contiguously in the returned slice, in
// rowBasis's relative order, and the contiguous blocks themselves are
// ordered by groupBasis's first-appearance sequence.
func reorderPreservingGrouping(groupBasis, rowBasis []store.Session) []store.Session {
	workspacePriority := make(map[string]int)
	for _, s := range groupBasis {
		ws := sessionWorkspace(s)
		if _, ok := workspacePriority[ws]; !ok {
			workspacePriority[ws] = len(workspacePriority)
		}
	}
	buckets := make(map[string][]store.Session, len(workspacePriority))
	var order []string
	seen := make(map[string]bool, len(workspacePriority))
	for _, s := range rowBasis {
		ws := sessionWorkspace(s)
		if !seen[ws] {
			seen[ws] = true
			order = append(order, ws)
		}
		buckets[ws] = append(buckets[ws], s)
	}
	sort.SliceStable(order, func(i, j int) bool {
		return workspacePriority[order[i]] < workspacePriority[order[j]]
	})
	out := make([]store.Session, 0, len(rowBasis))
	for _, ws := range order {
		out = append(out, buckets[ws]...)
	}
	return out
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

// groupingEnabled reports SPEC requirement 30/35's `[ui]
// group_by_workspace` switch (default true in the real config, task 007's
// I-4 schema plumbing; a zero-value config.Settings{} as many tests build
// directly reads false here, exactly as it already does for
// m.settings.Mouse -- a test that needs grouped-mode behaviour opts in
// explicitly). This is the one place that decision is read; every
// grouping-aware primitive below (visualOrder, isSessionVisible,
// sidebarEntries, the `c` key) calls this rather than reading
// m.settings.GroupByWorkspace itself, so there is exactly one switch to
// flip if the setting is ever renamed or the default changes.
func (m Model) groupingEnabled() bool {
	return m.settings.GroupByWorkspace
}

// isGroupCollapsed reports whether m has collapsed the given workspace's
// group. A workspace never explicitly collapsed defaults to expanded, so a
// nil map (the zero Model) behaves exactly like an empty one. This is
// unconditional (it does not consult groupingEnabled) because it is a pure
// bookkeeping read -- the callers that matter for requirement 35 (isSession
// Visible, sidebarEntries, the `c` key handler) are themselves gated so
// collapse state is never populated or consulted while grouping is off.
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
// or when i is out of range. Requirement 35: collapse state is meaningless
// ("absent rather than inert") when grouping is off, so every session is
// visible regardless of m.collapsedGroups' contents.
func (m Model) isSessionVisible(i int) bool {
	if i < 0 || i >= len(m.sessions) {
		return false
	}
	if !m.groupingEnabled() {
		return true
	}
	return !m.isGroupCollapsed(sessionWorkspace(m.sessions[i]))
}

// visualOrder returns every m.sessions index in the exact order the
// sidebar paints them: groupSessions()'s workspace buckets, flattened,
// ignoring collapse state entirely. This is the one place index order is
// reconciled with paint order (operator-reported defect, 002-steering.md,
// found on 786dfde): groupSessions() appends a later session into an
// EARLIER group when its workspace was already seen, so painted order and
// m.sessions order only coincide when every workspace's sessions happen to
// be adjacent. Every navigation primitive below resolves through this
// list (or visibleSessionIndices, its collapse-filtered view) rather than
// stepping m.sessions by +1/-1 directly, so one press always moves exactly
// one visual row. This does not reorder m.sessions itself (that stays the
// attention sort's job, SPEC.md:876) and does not touch groupSessions'
// own bucketing (SPEC requirement 30).
func (m Model) visualOrder() []int {
	// Requirement 35: with grouping off there are no buckets to flatten --
	// the flat list paints m.sessions in its own existing (already
	// attention-sorted, SPEC §11) order, index for index, never reordered
	// by groupSessions' first-appearance bucketing. Task 009 (I-6) is the
	// dedicated proof that this and the grouped branch agree with every
	// navigation primitive built on top of this function.
	if !m.groupingEnabled() {
		order := make([]int, len(m.sessions))
		for i := range m.sessions {
			order[i] = i
		}
		return order
	}
	var order []int
	for _, group := range m.groupSessions() {
		for _, is := range group.Sessions {
			order = append(order, is.Index)
		}
	}
	return order
}

// visibleSessionIndices is visualOrder filtered to the sessions actually
// shown right now (i.e. not hidden by a collapsed workspace group).
// Paging (sidebarRowsPerPage) and any future "how many rows can I move"
// primitive should walk this list, since a collapsed group's hidden rows
// must not count as a step.
func (m Model) visibleSessionIndices() []int {
	var out []int
	for _, idx := range m.visualOrder() {
		if m.isSessionVisible(idx) {
			out = append(out, idx)
		}
	}
	return out
}

// nearestVisibleSelection returns the closest visible session index to
// from IN VISUAL ORDER, searching forward first (so expanding/collapsing
// near the top of the list keeps selection moving in the direction of
// travel) and then backward, or 0 when no session is visible (an empty
// list is handled by every caller already, since m.selected is
// meaningless there).
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
	order := m.visualOrder()
	pos := 0
	for i, idx := range order {
		if idx == from {
			pos = i
			break
		}
	}
	for i := pos; i < len(order); i++ {
		if m.isSessionVisible(order[i]) {
			return order[i]
		}
	}
	for i := pos - 1; i >= 0; i-- {
		if m.isSessionVisible(order[i]) {
			return order[i]
		}
	}
	return 0
}

// nextVisibleSelection and prevVisibleSelection are ↑/↓'s SPEC requirement
// 30 "remains navigable" behaviour: stepping past a collapsed group's
// hidden rows in one keypress rather than requiring one press per hidden
// row (which would silently do nothing on each of those presses). Both
// step through visualOrder (painted order), never m.sessions index order
// directly, so one press always moves exactly one visual row.
func (m Model) nextVisibleSelection(from int) (int, bool) {
	order := m.visualOrder()
	pos := -1
	for i, idx := range order {
		if idx == from {
			pos = i
			break
		}
	}
	if pos == -1 {
		return from, false
	}
	for i := pos + 1; i < len(order); i++ {
		if m.isSessionVisible(order[i]) {
			return order[i], true
		}
	}
	return from, false
}

func (m Model) prevVisibleSelection(from int) (int, bool) {
	order := m.visualOrder()
	pos := -1
	for i, idx := range order {
		if idx == from {
			pos = i
			break
		}
	}
	if pos == -1 {
		return from, false
	}
	for i := pos - 1; i >= 0; i-- {
		if m.isSessionVisible(order[i]) {
			return order[i], true
		}
	}
	return from, false
}

// pageSelection is PgUp/PgDn's own step (SPEC requirement 19): moves delta
// VISUAL rows (positive = down, negative = up) from m.selected's current
// visual position among the presently visible rows, clamping at either
// end rather than wrapping. Like nextVisibleSelection/prevVisibleSelection,
// this walks visibleSessionIndices (painted order) rather than doing
// index arithmetic against m.sessions, which is the same defect ↑/↓ had
// (002-steering.md). Returns 0 when nothing is visible.
func (m Model) pageSelection(delta int) int {
	visible := m.visibleSessionIndices()
	if len(visible) == 0 {
		return 0
	}
	pos := -1
	for i, idx := range visible {
		if idx == m.selected {
			pos = i
			break
		}
	}
	if pos == -1 {
		near := m.nearestVisibleSelection(m.selected)
		for i, idx := range visible {
			if idx == near {
				pos = i
				break
			}
		}
		if pos == -1 {
			pos = 0
		}
	}
	pos += delta
	if pos < 0 {
		pos = 0
	}
	if pos > len(visible)-1 {
		pos = len(visible) - 1
	}
	return visible[pos]
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
	return m.colorToken(theme.Group, text)
}
