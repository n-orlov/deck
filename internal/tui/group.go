package tui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/n-orlov/deck/internal/store"
	"github.com/n-orlov/deck/internal/theme"
)

// §11/§11.3 manual groups (SPEC requirement 30, R128/R129): the sidebar
// groups rows by sessions.group_id (via sessionWorkspace's group-key
// seam), never by cwd or repo. Each group has a collapsible header;
// collapsing a group hides its member rows but leaves the sidebar
// otherwise navigable — selection never lands on a hidden row.
//
// This file only groups and tracks collapse state; it deliberately does
// not re-order m.sessions itself (that stays whichever of the attention
// sort or [ui] sort_order is in force, internal/tui/attention.go and
// sort_order.go) — grouping here preserves each session's existing
// relative position, bucketed by group in order of each group's first
// appearance, and rows within a bucket keep that same relative order
// (SPEC §11: "Rows within a group follow the sort order"). What DOES
// change here (task 011, R129) is which BUCKET renders first: groups sort
// alphabetically, case-insensitively, with the implicit default group
// always last regardless of where its name would otherwise sort —
// deliberately not attention-ranked (the removed reorderPreservingGrouping
// used to compose group order with attention; SPEC §11 states this
// explicitly: "Group order is deliberately not attention-ranked... a list
// whose headers reshuffle when a session starts waiting is a list you
// cannot navigate from memory").

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

// groupSessions splits m.sessions into groups (SPEC requirement 30):
// sessions bucket by group (in order of each group's first appearance in
// m.sessions, so a bucket's own row order preserves m.sessions' existing
// relative order -- see this file's package doc comment), and the
// resulting buckets are then themselves reordered by groupSortsBefore
// (R129, task 011): alphabetical, case-insensitive, implicit default
// always last. Splitting the two steps like this keeps groupSortsBefore
// entirely ignorant of m.sessions/attention/sort_order -- it only ever
// compares two group names.
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
	sort.SliceStable(groups, func(i, j int) bool {
		return groupSortsBefore(groups[i].Workspace, groups[j].Workspace)
	})
	return groups
}

// groupSortsBefore is R129's group ORDER rule (task 011): alphabetical,
// case-insensitive, with the implicit default group (the empty
// sessionWorkspace key) always sorting last regardless of where its
// display name ("default") would otherwise land -- SPEC §11: "Order is
// alphabetical, case-insensitive, with default always last regardless of
// where its name would sort." This replaces the deleted
// reorderPreservingGrouping's attention-ranked group order (R53) entirely
// -- SPEC §11 states group order is "deliberately not attention-ranked":
// a manual group is a stable place the user learns the position of, and a
// list whose headers reshuffle when a session starts waiting is a list
// you cannot navigate from memory.
func groupSortsBefore(a, b string) bool {
	if a == "" {
		return false
	}
	if b == "" {
		return true
	}
	return strings.ToLower(a) < strings.ToLower(b)
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

// groupHeaderText renders one group's header line (SPEC requirement 30,
// rewritten for R129/task 011): a collapse-state marker, the group's name
// ("default" for the implicit group), and its member count --
// "<chevron> <name>  (<n>)", with an ASCII fallback for the chevron via
// m.glyph. R129 drops the old workspace-derived header's representative-
// cwd suffix entirely: a manual group's members can span any number of
// directories, so one member's cwd next to the group name is noise, not
// signal, the way it was when a group WAS a cwd-derived workspace.
//
// contentWidth is this header's own text budget -- the same contentWidth
// every other sidebarEntries line already receives. SPEC §11: "Every
// header carries its member count, including (0)" and "The name elides;
// the count and the chevron never do" -- so only the name shrinks (via
// elideToWidth) under a narrow sidebar; the chevron and "(n)" are budgeted
// for FIRST and always emitted in full, even at SidebarWidthFloor, even
// when that leaves no room at all for the name.
func (m Model) groupHeaderText(group sidebarGroup, contentWidth int) string {
	marker := m.glyph("\u25be", "v") // expanded
	if m.isGroupCollapsed(group.Workspace) {
		marker = m.glyph("\u25b8", ">") // collapsed
	}
	name := group.Workspace
	if name == "" {
		name = "default"
	}
	count := fmt.Sprintf("(%d)", len(group.Sessions))
	prefix := marker + " "
	suffix := "  " + count
	nameBudget := contentWidth - stringWidth(prefix) - stringWidth(suffix)
	if nameBudget < 0 {
		nameBudget = 0
	}
	name = m.elideToWidth(name, nameBudget)
	return m.colorToken(theme.Group, prefix+name+suffix)
}
