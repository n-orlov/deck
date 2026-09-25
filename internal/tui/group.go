package tui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/n-orlov/deck/internal/store"
	"github.com/n-orlov/deck/internal/theme"
	"github.com/n-orlov/deck/internal/tui/sidebarcursor"
)

// §11/§11.3 manual groups (SPEC requirement 30, R128/R129): the sidebar
// groups rows by sessions.group_id (via sessionGroupKey's group-key
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

// sessionGroupKey is internal/tui's one group-key accessor (task 007,
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
func sessionGroupKey(session store.Session) string {
	return session.GroupName
}

// sessionGroupDisplayName is the seam's companion display-name accessor:
// the label a group's header should show for one of its member sessions.
// Identical to sessionGroupKey's group key at this commit -- there is
// still no separate group-name storage distinct from the key itself (a
// group's name IS its key, per SPEC §4's groups table) -- kept as its
// own function so a later task that ever needs the two to diverge only
// has to change this function's body, not every call site across
// internal/tui.
func sessionGroupDisplayName(session store.Session) string {
	return sessionGroupKey(session)
}

// sessionGroupID resolves the durable group identity a session's collapse
// state and §11.8 header hit-test key off of (task 013/R129 part 3): the
// real groups.id when sessionGroupKey resolves to a real, named group, or
// 0 -- a sentinel no real group row can ever hold, since SQLite rowids
// start at 1 -- for the implicit default group, including a session whose
// GroupID no longer resolves to a live groups row (SPEC §11: "a group_id
// that no longer resolves ... renders under default rather than
// vanishing"). This is deliberately keyed off sessionGroupKey's resolved
// name, not off session.GroupID directly: a dangling GroupID is non-nil
// but must still bucket under the SAME default identity (0) every true
// default session already uses, never under a distinct, meaningless
// id-shaped value.
func sessionGroupID(session store.Session) int64 {
	if sessionGroupKey(session) == "" {
		return 0
	}
	if session.GroupID != nil {
		return *session.GroupID
	}
	return 0
}

// sessionGroupLabel is the seam's display LABEL accessor: the group name
// to print for one session on a surface that has no group list of its own
// to resolve against -- the `i` detail dialog's "Group:" row and the `g`
// picker's "Current group:" row (R130 part 2). It is deliberately derived
// from the session row itself (store.Session.GroupName, the LEFT JOIN's
// resolved §11 label) rather than from any dialog's own snapshot of the
// group list: moveGroupOptions/createGroups are populated only while their
// dialog is open, so resolving through them showed "default" for a
// correctly-grouped session whenever detail was opened without the picker.
// The structural default group (sessionGroupID == 0, i.e. no group_id or a
// group_id that no longer resolves -- SPEC §11's "renders under default
// rather than vanishing") prints groupHeaderText's same literal label,
// "default", so the detail row and the sidebar header agree.
func sessionGroupLabel(session store.Session) string {
	if name := sessionGroupDisplayName(session); name != "" {
		return name
	}
	return "default"
}

// sidebarCursor is the sidebar cursor's own type (Model.selected):
// deliberately NOT a bare int, and -- since task 012/D.1's cure-03 --
// deliberately not declared in THIS package either. It is an alias for
// sidebarcursor.Cursor (internal/tui/sidebarcursor), whose fields are all
// unexported, so the package boundary is what stops a caller from reading
// a session index off a cursor without resolving its kind first: from here
// `m.selected.index` does not compile ("m.selected.index undefined (cannot
// refer to unexported field index)"), and the only ways in are the
// two-result accessors SessionIndex() (int, bool) and
// GroupID() (int64, bool). A per-package-visibility field declared here
// would have been readable by all ~490 cursor reads across internal/tui,
// which is exactly the misread this type exists to make impossible --
// headers became navigable stops this task (up/down/PgUp/PgDn/g/G all step
// onto one, not just past it), so "the selected session" is no longer a
// safe assumption anywhere that reads the cursor.
//
// The alias (rather than a fresh named type wrapping it) keeps every
// existing internal/tui call site, test assertion and `[]sidebarCursor`
// literal reading exactly as it did, and keeps the value comparable.
// cursor_kind_guard_test.go pins the two properties that make the
// enforcement real: the type is declared outside package tui, and it
// exposes no field and no bare-int accessor.
//
// The zero value is rowCursor(0): kind defaults to a row and index to 0,
// so a freshly zero-valued Model (every test fixture that never sets
// Model.selected explicitly) keeps behaving exactly as it did when
// Model.selected was a bare `int` defaulting to 0.
type sidebarCursor = sidebarcursor.Cursor

// rowCursor builds a cursor resting on the session at m.sessions[index].
func rowCursor(index int) sidebarCursor {
	return sidebarcursor.Row(index)
}

// headerCursor builds a cursor resting on one group's header, keyed by
// its durable group id -- 0 is the implicit default group's sentinel,
// exactly like sessionGroupID's own sentinel (no real groups.id row can
// ever be 0, since SQLite rowids start at 1). No header cursor is ever
// built for a persisted-but-empty group while a filter is active:
// groupSessions itself already skips seeding one (cure-01-02's own
// comment above, "deliberately skipped while a filter query is in
// force"), and every header cursor this file builds comes from walking
// groupSessions' own output, never from m.allGroups directly.
func headerCursor(groupID int64) sidebarCursor {
	return sidebarcursor.Header(groupID)
}

// indexedSession pairs a session with its index into m.sessions, so a
// group can be rendered (and, once selected, resolved back to an index)
// without re-scanning m.sessions to find it.
type indexedSession struct {
	Index   int
	Session store.Session
}

// sidebarGroup is one workspace's header plus the sessions rendered under
// it, in m.sessions' own relative order. GroupID (task 013/R129 part 3) is
// the bucket's durable identity -- 0 for the implicit default group (a
// sentinel no real groups.id row can ever hold, since SQLite rowids start
// at 1) -- and is what collapse state and the §11.8 header hit-test key
// off of now, never Workspace: a group's NAME can change (a rename), its
// id cannot, so keying either one off Workspace would lose collapse state
// across a rename even though nothing else about the group changed.
type sidebarGroup struct {
	Name     string
	GroupID  int64
	Sessions []indexedSession
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
//
// cure-01-02 (R129/R131 review finding): while UNFILTERED, every group in
// m.allGroups (the persisted list read alongside sessions on the most
// recent reload, tui.go's sessionsLoaded) gets a bucket here too, even
// when it currently has zero members, and so does the structural default
// group -- SPEC §11: "Every header carries its member count, including
// (0)... A group the user defined but has not filled yet still renders:
// it is how they see the group exists and where to put something" and
// "default is not a row... it always exists". This is intentionally
// skipped while a filter query is in force: SPEC §11 states "Under an
// active filter only groups with a match render", and a defined-but-empty
// group by construction never has a filter match, so seeding it here
// would surface a header a filtered render must not show.
func (m Model) groupSessions() []sidebarGroup {
	var groups []sidebarGroup
	firstSeen := map[string]int{}
	for i, session := range m.sessions {
		ws := sessionGroupKey(session)
		if gi, ok := firstSeen[ws]; ok {
			groups[gi].Sessions = append(groups[gi].Sessions, indexedSession{Index: i, Session: session})
			continue
		}
		firstSeen[ws] = len(groups)
		groups = append(groups, sidebarGroup{Name: ws, GroupID: sessionGroupID(session), Sessions: []indexedSession{{Index: i, Session: session}}})
	}
	if m.filterQuery == "" {
		if _, ok := firstSeen[""]; !ok {
			firstSeen[""] = len(groups)
			groups = append(groups, sidebarGroup{Name: "", GroupID: 0})
		}
		for _, g := range m.allGroups {
			if _, ok := firstSeen[g.Name]; ok {
				continue
			}
			firstSeen[g.Name] = len(groups)
			groups = append(groups, sidebarGroup{Name: g.Name, GroupID: g.ID})
		}
	}
	sort.SliceStable(groups, func(i, j int) bool {
		return groupSortsBefore(groups[i].Name, groups[j].Name, m.settings.DefaultGroupFirst)
	})
	return groups
}

// groupSortsBefore is R129's group ORDER rule (task 011), plus task 002's
// `[ui] default_group_first` flag (task 001 plumbed the config field;
// task 003 wires it live): alphabetical, case-insensitive, with the
// implicit default group (the empty sessionGroupKey key, i.e. a == "" or
// b == "") sorting either always LAST (defaultFirst == false, R129's
// original rule, SPEC §11: "Order is alphabetical, case-insensitive, with
// default always last regardless of where its name would sort") or always
// FIRST (defaultFirst == true) -- in neither case as a consequence of
// comparing the literal string "default"/"" against a real name, and in
// neither case by consulting anything about a real group's NAME: a
// user-created group literally named "Default" is not the implicit
// default group -- its sessionGroupKey is the non-empty string "Default",
// not "" -- so it never hits either of the two `== ""` branches below and
// sorts purely alphabetically against every other real group, same as any
// other name. This still only ever compares the two group KEYS passed in
// (deliberately ignorant of m.sessions/attention/sort_order, so
// groupSessions above stays the one place that decides what those two
// keys ARE) -- see this function's docs for why: it replaces the deleted
// reorderPreservingGrouping's attention-ranked group order (R53) entirely
// -- SPEC §11 states group order is "deliberately not attention-ranked":
// a manual group is a stable place the user learns the position of, and a
// list whose headers reshuffle when a session starts waiting is a list
// you cannot navigate from memory.
func groupSortsBefore(a, b string, defaultFirst bool) bool {
	if a == "" && b == "" {
		return false
	}
	if a == "" {
		return defaultFirst
	}
	if b == "" {
		return !defaultFirst
	}
	return strings.ToLower(a) < strings.ToLower(b)
}

// isGroupCollapsed reports whether m has collapsed the group named by
// groupID (task 013/R129 part 3: keyed by the group's durable id, never
// its display name -- see sessionGroupID). A group never explicitly
// collapsed defaults to expanded, so a nil map (the zero Model) behaves
// exactly like an empty one.
func (m Model) isGroupCollapsed(groupID int64) bool {
	return m.collapsedGroups[groupID]
}

// setGroupCollapsed collapses or expands one group, by id. Task 014/D.3
// removed the eviction this used to do on every collapse
// (m.nearestVisibleSelection(m.selected), which could walk the cursor onto
// a DIFFERENT group's header entirely): folding must leave the cursor
// naming the SAME group it just folded, never a neighbour's, so `c`/left
// repeated on one header always re-targets that same group rather than
// sliding onto whichever one happens to sort next. A header cursor is
// never itself hidden by its own group's collapse (isStopVisible), so a
// header cursor never needs to move at all -- folding from a header
// leaves it in place. A row cursor whose session belongs to the group
// being folded loses its own row to the fold, so it lands on that same
// group's own header, the one still-visible stop that still names it. A
// row cursor belonging to some OTHER group is left exactly where it is:
// collapsing group X can never hide a row that already belongs to a
// different group. Expanding never hides anything the cursor could have
// been resting on, so it never touches m.selected either.
func (m *Model) setGroupCollapsed(groupID int64, collapsed bool) {
	if collapsed {
		if m.collapsedGroups == nil {
			m.collapsedGroups = map[int64]bool{}
		}
		m.collapsedGroups[groupID] = true
		if idx, ok := m.selected.SessionIndex(); ok && idx >= 0 && idx < len(m.sessions) && sessionGroupID(m.sessions[idx]) == groupID {
			m.selected = headerCursor(groupID)
		}
	} else if m.collapsedGroups != nil {
		delete(m.collapsedGroups, groupID)
	}
}

// toggleGroupCollapse flips one group's collapse state, by id. The mouse
// header click (internal/tui/mouse.go) is the other caller this exists to
// serve; it is its own method (rather than inlined at either call site) so
// the key binding and the mouse binding (SPEC §11.8: "no capability is
// ever mouse-only") have identical behaviour.
func (m *Model) toggleGroupCollapse(groupID int64) {
	m.setGroupCollapsed(groupID, !m.isGroupCollapsed(groupID))
}

// isSessionVisible reports whether the session at index i is presently
// shown in the sidebar: false only when its group (by id, sessionGroupID)
// is collapsed, or when i is out of range.
func (m Model) isSessionVisible(i int) bool {
	if i < 0 || i >= len(m.sessions) {
		return false
	}
	return !m.isGroupCollapsed(sessionGroupID(m.sessions[i]))
}

// isStopVisible is isSessionVisible's counterpart for a whole visual stop
// (task 012/D.1): a header stop is never hidden by its OWN collapse state
// (collapsing a group hides its ROWS, not its header -- there would be no
// way to reach `c`/click to expand it again otherwise), so this is true
// for every header cursor unconditionally, and defers to isSessionVisible
// for a row cursor.
func (m Model) isStopVisible(c sidebarCursor) bool {
	if c.IsHeader() {
		return true
	}
	idx, ok := c.SessionIndex()
	return ok && m.isSessionVisible(idx)
}

// cursorNamesVisibleStop reports whether c currently names a real,
// visible sidebar entry (cure-01-05, R137: "selection never names a
// hidden row or a filtered-out header"). Unlike isStopVisible, which
// trusts any header cursor unconditionally (a group's own collapse never
// hides its own header line), this also requires the header's BUCKET to
// still exist in m.visualOrder() at all -- the case isStopVisible cannot
// see: a header cursor whose group has lost its bucket entirely, e.g. a
// filter query that leaves that group with zero matches (groupSessions'
// own doc comment: "Under an active filter only groups with a match
// render").
func (m Model) cursorNamesVisibleStop(c sidebarCursor) bool {
	for _, o := range m.visibleSessionIndices() {
		if o == c {
			return true
		}
	}
	return false
}

// cursorGroupID resolves the durable group id the CURRENT cursor names --
// the group a row cursor's session belongs to (sessionGroupID), or a
// header cursor's own id directly -- for `c`'s collapse toggle (task
// 012/D.1: `c` now also works from a header cursor, not only from one of
// its member rows). ok is false only when the cursor is a row whose
// session index has drifted out of m.sessions' current bounds.
func (m Model) cursorGroupID() (int64, bool) {
	if idx, ok := m.selected.SessionIndex(); ok {
		if idx < 0 || idx >= len(m.sessions) {
			return 0, false
		}
		return sessionGroupID(m.sessions[idx]), true
	}
	if gid, ok := m.selected.GroupID(); ok {
		return gid, true
	}
	return 0, false
}

// visualOrder returns every visual STOP -- one header cursor per
// groupSessions() bucket plus one row cursor per session in it -- in the
// exact order the sidebar paints them, ignoring collapse state entirely
// (task 012/D.1: headers and rows are both stops now, so this walks
// groupSessions()'s buckets themselves rather than flattening straight to
// session indices the way it did before headers were navigable). This is
// the one place stop order is reconciled with paint order (operator-
// reported defect, 002-steering.md, found on 786dfde): groupSessions()
// appends a later session into an EARLIER group when its workspace was
// already seen, so painted order and m.sessions order only coincide when
// every workspace's sessions happen to be adjacent. Every navigation
// primitive below resolves through this list (or visibleSessionIndices,
// its visibility-filtered view) rather than stepping m.sessions by +1/-1
// directly, so one press always moves exactly one visual stop. This does
// not reorder m.sessions itself (that stays the attention sort's job,
// SPEC.md:876) and does not touch groupSessions' own bucketing (SPEC
// requirement 30). A caller that only ever wants session rows (attention
// jump, the mark set) filters this list for IsRow()/SessionIndex() itself
// rather than this function growing a second, row-only sibling.
func (m Model) visualOrder() []sidebarCursor {
	var order []sidebarCursor
	for _, group := range m.groupSessions() {
		order = append(order, headerCursor(group.GroupID))
		for _, is := range group.Sessions {
			order = append(order, rowCursor(is.Index))
		}
	}
	return order
}

// visibleSessionIndices is visualOrder filtered to the stops actually
// shown right now (i.e. a row not hidden by a collapsed group; every
// header, since a header is never itself hidden by its own collapse).
// Paging (sidebarRowsPerPage) and g/G walk this list, since a collapsed
// group's hidden rows must not count as a step -- its still-visible
// header does.
func (m Model) visibleSessionIndices() []sidebarCursor {
	var out []sidebarCursor
	for _, c := range m.visualOrder() {
		if m.isStopVisible(c) {
			out = append(out, c)
		}
	}
	return out
}

// firstVisibleRowInGroup returns the first visible row cursor in
// groupID's own bucket, in visualOrder (the group's own
// first-appearance-within-a-bucket row) -- cure-01-05 follow-up (task
// 022 sweep): the seam that lets a header cursor step onto a row that
// just appeared in its own, still-uncollapsed group, the one transition
// cursorNamesVisibleStop cannot see because a header is always visible
// regardless of what (if anything) is under it. ok is false when the
// group has no visible row at all (empty, or every member hidden by its
// own collapse) -- the caller then leaves the header selected, exactly
// as cursorNamesVisibleStop's own fallback already does for that case.
func (m Model) firstVisibleRowInGroup(groupID int64) (sidebarCursor, bool) {
	inGroup := false
	for _, c := range m.visualOrder() {
		if gid, ok := c.GroupID(); ok {
			inGroup = gid == groupID
			continue
		}
		if !inGroup {
			continue
		}
		if idx, ok := c.SessionIndex(); ok && m.isSessionVisible(idx) {
			return c, true
		}
	}
	return rowCursor(0), false
}

// selectedGroupHadNoRows reports, from the CURRENT (pre-reload) m.sessions,
// whether the currently selected header's own bucket has zero members --
// false unconditionally for a row cursor. Every caller that is about to
// recompute m.sessions (sessionsLoaded, updateFilter's live-narrowing
// edits) calls this BEFORE that recompute, then passes the result into
// selectVisibleStopAfterReload once the new m.sessions is in place, so
// the empty-bucket-gains-a-row transition can be told apart from a
// header whose bucket already had rows (and must stay exactly where
// cursorNamesVisibleStop already puts it).
func (m Model) selectedGroupHadNoRows() bool {
	gid, ok := m.selected.GroupID()
	if !ok {
		return false
	}
	for _, s := range m.sessions {
		if sessionGroupID(s) == gid {
			return false
		}
	}
	return true
}

// selectVisibleStopAfterReload normalizes m.selected once m.sessions has
// just been recomputed by a reload or a live filter/sort edit (cure-01-05
// plus its task-022-sweep follow-up). An invalid cursor -- one
// cursorNamesVisibleStop no longer recognizes as a real, visible stop --
// walks onto nearestVisibleSelection exactly as cure-01-05 already did. A
// cursor cursorNamesVisibleStop still accepts (every header always
// qualifies, whatever is or is not under it) additionally walks onto its
// own bucket's first row when selectedGroupHadNoRows (captured by the
// caller from the PRE-reload state) says that bucket just gained one --
// the transition cursorNamesVisibleStop cannot see on its own, since it
// never asked "visible stop" to mean "the BEST visible stop", only "A
// visible stop". Every other header selection is left exactly where
// cursorNamesVisibleStop already decided.
func (m *Model) selectVisibleStopAfterReload(selectedGroupHadNoRows bool) {
	if !m.cursorNamesVisibleStop(m.selected) {
		m.selected = m.nearestVisibleSelection(m.selected)
		return
	}
	if !selectedGroupHadNoRows {
		return
	}
	if gid, ok := m.selected.GroupID(); ok {
		if first, ok := m.firstVisibleRowInGroup(gid); ok {
			m.selected = first
		}
	}
}

// nearestVisibleSelection returns the closest visible stop to from IN
// VISUAL ORDER, searching forward first (so expanding/collapsing near the
// top of the list keeps selection moving in the direction of travel) and
// then backward, or rowCursor(0) when nothing is visible at all. from's
// own session index (if it is a row cursor) is clamped into m.sessions'
// current bounds first, exactly as the bare int version of this function
// used to clamp from itself -- a header cursor never needs clamping,
// since a group id does not go stale the way a session index does when
// m.sessions shrinks.
//
// cure-01-05 (R137/F6): this used to short-circuit to rowCursor(0)
// whenever len(m.sessions) == 0, on the premise that an empty session
// list means "nothing is visible" -- true before task 012/D.1 made
// headers navigable stops of their own, false the moment a header-only
// sidebar (every group has zero members, or every session is filtered
// out while at least one group still has a match) is reachable. A
// from-cursor that happens to be a row is simply left unable to match
// anything in visualOrder when there are no rows at all -- the walk
// below then falls through to whatever header IS visible, exactly as it
// already does for a row cursor stranded by a collapsed group.
func (m Model) nearestVisibleSelection(from sidebarCursor) sidebarCursor {
	if idx, ok := from.SessionIndex(); ok && len(m.sessions) > 0 {
		if idx < 0 {
			idx = 0
		}
		if idx > len(m.sessions)-1 {
			idx = len(m.sessions) - 1
		}
		from = rowCursor(idx)
	}
	order := m.visualOrder()
	pos := 0
	for i, c := range order {
		if c == from {
			pos = i
			break
		}
	}
	for i := pos; i < len(order); i++ {
		if m.isStopVisible(order[i]) {
			return order[i]
		}
	}
	for i := pos - 1; i >= 0; i-- {
		if m.isStopVisible(order[i]) {
			return order[i]
		}
	}
	return rowCursor(0)
}

// nextVisibleSelection and prevVisibleSelection are up/down's SPEC
// requirement 30 "remains navigable" behaviour: stepping past a collapsed
// group's hidden rows in one keypress rather than requiring one press per
// hidden row (which would silently do nothing on each of those presses),
// and (task 012/D.1) landing on a header exactly like any other stop.
// Both step through visualOrder (painted order), never m.sessions index
// order directly, so one press always moves exactly one visual stop.
func (m Model) nextVisibleSelection(from sidebarCursor) (sidebarCursor, bool) {
	order := m.visualOrder()
	pos := -1
	for i, c := range order {
		if c == from {
			pos = i
			break
		}
	}
	if pos == -1 {
		return from, false
	}
	for i := pos + 1; i < len(order); i++ {
		if m.isStopVisible(order[i]) {
			return order[i], true
		}
	}
	return from, false
}

func (m Model) prevVisibleSelection(from sidebarCursor) (sidebarCursor, bool) {
	order := m.visualOrder()
	pos := -1
	for i, c := range order {
		if c == from {
			pos = i
			break
		}
	}
	if pos == -1 {
		return from, false
	}
	for i := pos - 1; i >= 0; i-- {
		if m.isStopVisible(order[i]) {
			return order[i], true
		}
	}
	return from, false
}

// pageSelection is PgUp/PgDn's own step (SPEC requirement 19): moves delta
// VISUAL stops (positive = down, negative = up) from m.selected's current
// visual position among the presently visible stops, clamping at either
// end rather than wrapping. Like nextVisibleSelection/prevVisibleSelection,
// this walks visibleSessionIndices (painted order) rather than doing
// index arithmetic against m.sessions, which is the same defect up/down
// had (002-steering.md). Returns rowCursor(0) when nothing is visible.
func (m Model) pageSelection(delta int) sidebarCursor {
	visible := m.visibleSessionIndices()
	if len(visible) == 0 {
		return rowCursor(0)
	}
	pos := -1
	for i, c := range visible {
		if c == m.selected {
			pos = i
			break
		}
	}
	if pos == -1 {
		near := m.nearestVisibleSelection(m.selected)
		for i, c := range visible {
			if c == near {
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

// headerSelectionCue answers the background token a group header's own
// sidebarEntry wants painted, and whether the header is the one under the
// cursor (task 003/R141, GH #39, amended SPEC §11: "a header starts at the
// panel's first content column ... it reserves no gutter"). Cure-01-02's
// original cue (reused below in the doc history) painted a literal "> "
// glyph into a 2-column gutter every header reserved whether or not it was
// selected -- that indented every header for nothing, since only rows draw
// >/✓ there. The new cue drops the gutter entirely (a header's
// sidebarEntry.gutter is always "", so its chevron lands in the exact same
// column session rows' text does, level with "socket:") and moves the
// cursor cue onto two ATTRIBUTES instead of a glyph: the `selection`/
// `selection_idle` background (focus-aware, exactly sidebarSelectionToken's
// existing row cue) across the header's own sidebarEntry, plus reverse
// video over the header's rendered text (groupHeaderText below), which is
// what survives NO_COLOR/ascii now that no literal glyph remains -- an
// SGR attribute, never a colour.
func (m Model) headerSelectionCue(groupID int64) (theme.Token, bool) {
	selected := false
	if gid, ok := m.selected.GroupID(); ok && gid == groupID {
		selected = true
	}
	if !selected {
		return theme.Token(""), false
	}
	return m.sidebarSelectionToken(), true
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
// every other sidebarEntries line already receives, now that the header
// reserves no gutter (task 003/R141) and so gets the FULL content width,
// same as a row's own text minus its gutter used to leave less. SPEC §11:
// "Every header carries its member count, including (0)" and "The name
// elides; the count and the chevron never do" -- so only the name shrinks
// (via elideToWidth) under a narrow sidebar; the chevron and "(n)" are
// budgeted for FIRST and always emitted in full, even at SidebarWidthFloor,
// even when that leaves no room at all for the name.
//
// selected (task 003/R141) wraps the whole composed text in reverse video
// (m.reverseVideo, theme_color.go) -- unconditionally of m.settings.Color,
// because SPEC §11 requires this cue survive NO_COLOR/ascii precisely
// because it is an attribute, not a colour. It wraps OUTSIDE colorToken's
// own SGR-colour-plus-reset so a coloured header still shows its group
// colour reversed rather than losing it to reverse's own reset.
func (m Model) groupHeaderText(group sidebarGroup, contentWidth int, selected bool) string {
	marker := m.glyph("\u25be", "v") // expanded
	if m.isGroupCollapsed(group.GroupID) {
		marker = m.glyph("\u25b8", ">") // collapsed
	}
	name := group.Name
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
	text := m.colorToken(theme.Group, prefix+name+suffix)
	if selected {
		text = m.reverseVideo(text)
	}
	return text
}
