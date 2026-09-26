package tui

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/n-orlov/deck/internal/theme"
	"github.com/n-orlov/deck/internal/tmux"
)

// entryRefusalKind names why entering interactive mode was refused --
// SPEC §11.9/R143 (GH #38). The set is exactly the seven kinds task
// 007's own successCriteria enumerates: two are about someone ELSE's
// claim on the window (contention, both also offer `F`), one is the
// stopped-session guard `a` refuses too, one is deck's own 7-inner-row
// floor, one is the no-live-pane placeholder, one is the fall-out when
// the preview shrinks below the floor WHILE already interactive, and the
// last is every other error that stops entry (a failed tmux call, a
// store failure on the durable attachment transaction, ...).
type entryRefusalKind string

const (
	entryRefusalAttachedElsewhere entryRefusalKind = "attached-elsewhere"
	entryRefusalOwnedElsewhere    entryRefusalKind = "owned-elsewhere"
	entryRefusalStopped           entryRefusalKind = "stopped"
	entryRefusalRowFloor          entryRefusalKind = "row floor"
	entryRefusalNoLivePane        entryRefusalKind = "no live pane"
	entryRefusalShrank            entryRefusalKind = "shrank"
	entryRefusalOther             entryRefusalKind = "other"
)

// allEntryRefusalKinds is exactly SPEC §11.9's seven kinds, in the order
// task 007's successCriteria lists them. The table-driven test in
// entry_refusal_test.go iterates this slice, so a kind added later that
// is not also added here fails that test rather than shipping uncovered.
var allEntryRefusalKinds = []entryRefusalKind{
	entryRefusalAttachedElsewhere,
	entryRefusalOwnedElsewhere,
	entryRefusalStopped,
	entryRefusalRowFloor,
	entryRefusalNoLivePane,
	entryRefusalShrank,
	entryRefusalOther,
}

// entryRefusalState is SPEC §11.9's refusal, held on Model instead of
// folded into the plain-string m.attachError footer note (task 007/R143,
// GH #38): unlike a footer note it is drawn as a banner OVER the preview
// (entryRefusalBannerLines below) and it belongs to the refused SESSION,
// not to whichever render call first showed it -- sessionID is what lets
// a later render (or a later task's clearing logic) tell whether the
// selection has moved on. active is the zero-value guard, so a Model
// built by New() -- whose entryRefusal field is entirely zero -- carries
// no refusal until something sets one.
type entryRefusalState struct {
	active    bool
	sessionID string
	kind      entryRefusalKind
	reason    string
}

// setEntryRefusal is the ONE place every entry-refusal call site in the
// package constructs a refusal (enterInteractiveBody's own ladder in
// interactive.go, and tui.go's shrank fall-out) -- going through one
// function rather than a bare struct literal at each site is what keeps
// this task's own criterion true structurally: nothing that calls this
// can ALSO leave m.attachError set, because this clears it in the same
// call.
func (m *Model) setEntryRefusal(sessionID string, kind entryRefusalKind, reason string) {
	m.entryRefusal = entryRefusalState{active: true, sessionID: sessionID, kind: kind, reason: reason}
	m.attachError = ""
}

// clearEntryRefusal drops the refusal outright. Task 007 itself only ever
// needs it once (a successful entry, mirroring enterInteractiveBody's
// existing `m.attachError = ""` on success) -- task 008 (R143's own
// lifetime: selection move, a later tick finding the reason gone, Esc)
// reaches for this same helper rather than a second struct literal.
func (m *Model) clearEntryRefusal() {
	m.entryRefusal = entryRefusalState{}
}

// clearEntryRefusalIfSessionStarted is task 008/R143's (GH #38) own "a
// later tick finds the reason gone" clause for the entryRefusalStopped
// kind: called every time m.sessions is refreshed (sessionsLoaded), it
// looks up the refused session by ID in the JUST-refreshed list and
// clears the refusal the moment canReachPane says the session has a pane
// to reach again -- "the session started" in SPEC §11.9's own wording.
// Every other kind is left untouched here; the holder-left half of the
// same clause (the two contention kinds) is entryRefusalHolderCheck
// below, which needs a live tmux probe rather than m.sessions' own
// status. A session that has since left m.sessions entirely (deleted
// mid-refusal) is left alone too -- there is nothing to have "started".
func (m *Model) clearEntryRefusalIfSessionStarted() {
	if !m.entryRefusal.active || m.entryRefusal.kind != entryRefusalStopped {
		return
	}
	for _, s := range m.sessions {
		if s.ID == m.entryRefusal.sessionID {
			if canReachPane(s) {
				m.clearEntryRefusal()
			}
			return
		}
	}
}

// entryRefusalHolderCheck issues one read-only tmux probe per previewTick
// while an attached-elsewhere/owned-elsewhere refusal is active, so the
// banner drops the moment the contending client/process actually lets go
// -- SPEC §11.9's "the holder left" -- without waiting for the selection
// to move, for entry to be retried, or for Esc. Every other kind returns
// nil: the row-floor and shrank kinds have nothing to probe (the panel's
// own size, read synchronously by the entry ladder itself on the next
// attempt, governs those, not a tmux round trip), the stopped kind is
// clearEntryRefusalIfSessionStarted's job above, and no-live-pane/other
// name no single condition SPEC calls "the reason gone" -- every kind
// still clears on selection move, a successful retry, or Esc regardless.
func (m Model) entryRefusalHolderCheck() tea.Cmd {
	r := m.entryRefusal
	if !r.active || m.tmuxClient.Socket == "" {
		return nil
	}
	if r.kind != entryRefusalAttachedElsewhere && r.kind != entryRefusalOwnedElsewhere {
		return nil
	}
	var slug string
	found := false
	for _, s := range m.sessions {
		if s.ID == r.sessionID {
			slug, found = s.Slug, true
			break
		}
	}
	if !found {
		return nil
	}
	client := m.tmuxClient
	sessionID := r.sessionID
	kind := r.kind
	return func() tea.Msg {
		ctx := context.Background()
		windowTarget, err := tmux.SessionName(slug)
		if err != nil {
			return entryRefusalHolderRecheckDone{sessionID: sessionID, kind: kind}
		}
		switch kind {
		case entryRefusalAttachedElsewhere:
			if attached, aerr := client.SessionAttachedCount(ctx, windowTarget); aerr == nil && attached == 0 {
				return entryRefusalHolderRecheckDone{sessionID: sessionID, kind: kind, reasonGone: true}
			}
		case entryRefusalOwnedElsewhere:
			if state, perr := client.ProbeWindowOwnership(ctx, windowTarget); perr == nil && state != tmux.ClaimForeignLive {
				return entryRefusalHolderRecheckDone{sessionID: sessionID, kind: kind, reasonGone: true}
			}
		}
		return entryRefusalHolderRecheckDone{sessionID: sessionID, kind: kind}
	}
}

// activeEntryRefusalForSelection reports the current refusal, but only
// when it still belongs to the CURRENTLY SELECTED session and interactive
// mode is not itself active (an entry refusal is by definition about not
// having entered; the moment m.interactive is true there is nothing
// refused to show a banner about, even if a stale entryRefusal field is
// still sitting there while task 008's own clearing lands elsewhere).
func (m Model) activeEntryRefusalForSelection() (entryRefusalState, bool) {
	if !m.entryRefusal.active || m.interactive {
		return entryRefusalState{}, false
	}
	session, ok := m.selectedSession()
	if !ok || session.ID != m.entryRefusal.sessionID {
		return entryRefusalState{}, false
	}
	return m.entryRefusal, true
}

// entryRefusalSessionName resolves id's display name from m.sessions for
// the banner headline (line 1, "NOT ATTACHED: <session>"). A session that
// has since left m.sessions (deleted mid-refusal) falls back to the bare
// id rather than an empty headline.
func (m Model) entryRefusalSessionName(id string) string {
	for _, s := range m.sessions {
		if s.ID == id {
			return s.Name
		}
	}
	return id
}

// entryRefusalWayOut is line 3's content for kind: SPEC §11.9's "the way
// out for that kind (F and a for contention, a for the floor, and so
// on), always ending with `keys go to the list`". EVERY kind names its
// own way out ahead of that tail -- none of the seven is the bare tail
// alone:
//
//   - contention (attached-elsewhere, owned-elsewhere): F forces entry
//     over the holder, a attaches instead (SPEC's "both also offer F");
//   - stopped: r starts the session (the keymap's `r` resume/start),
//     then ↵ enters it -- neither F nor a helps, there is no pane yet;
//   - row floor: a attaches instead, the floor is deck's own limit and
//     offers only a;
//   - shrank: grow the preview and ↵ re-enters, or a attaches
//     instead -- the same floor, reached from inside interactive mode;
//   - no live pane: R restarts the session so it has a pane again, then
//     ↵ enters it;
//   - other: ↵ retries (the failure was a tmux/store error, not a
//     standing condition), or a attaches instead.
//
// Every branch ends with exactly the tail, unpunctuated, so a caller (or
// a test) can assert the literal suffix regardless of what precedes it.
func entryRefusalWayOut(kind entryRefusalKind) string {
	const tail = "keys go to the list"
	switch kind {
	case entryRefusalAttachedElsewhere, entryRefusalOwnedElsewhere:
		return "F forces entry, a attaches instead \u2014 " + tail
	case entryRefusalStopped:
		return "r starts it, then \u21b5 enters \u2014 " + tail
	case entryRefusalRowFloor:
		return "a attaches instead \u2014 " + tail
	case entryRefusalShrank:
		return "grow it and \u21b5, or a attaches \u2014 " + tail
	case entryRefusalNoLivePane:
		return "R restarts it, then \u21b5 enters \u2014 " + tail
	default: // entryRefusalOther
		return "\u21b5 retries, a attaches instead \u2014 " + tail
	}
}

// entryRefusalWayOutText is entryRefusalWayOut(kind) as the banner draws
// it in m's render mode: under `ascii` the two non-ASCII glyphs the way
// out uses (the " \u2014 " separator and the \u21b5 key name) become "; " and
// "Enter", so an ascii banner is plain ASCII throughout, not just its box.
func (m Model) entryRefusalWayOutText(kind entryRefusalKind) string {
	s := entryRefusalWayOut(kind)
	if m.settings.ASCII {
		s = strings.NewReplacer(" \u2014 ", "; ", "\u21b5", "Enter").Replace(s)
	}
	return s
}

// entryRefusalBannerRow paints one already-built, exactly-contentWidth
// banner row (a box edge or a centred text row) in whichever of SPEC
// §11.9's three render modes applies: a warning/error background token
// in colour, reverse video plus bold under NO_COLOR (unconditional of
// m.settings.Color, like theme_color.go's own reverseVideo -- NO_COLOR's
// whole point is that the cue survives with colour off), and ascii is
// orthogonal to both (m.box() already answers the `+-|` glyphs the row
// was built from; this only ever adds colour/attribute on top).
func (m Model) entryRefusalBannerRow(row string) string {
	if m.settings.Color {
		return m.bgColorToken(theme.Error, row)
	}
	// Built via fmt.Sprintf (theme_color.go's own "%d isn't [0-9;]" dodge
	// for TestNoColorLiterals -- SGR 1/7 are attributes, not colours, but
	// the guard only exempts the literal digits 0/7/27, not 1, so a
	// hardcoded escape naming both codes together would stay caught by
	// its regex; building it through Sprintf keeps the digits out of the
	// source text the same way sgrForToken's own dynamic build does)
	// rather than as a literal escape: bold (1) plus reverse video (7),
	// unconditional of m.settings.Color -- SPEC §11.9's NO_COLOR render
	// mode, matching reverseVideo's own "attribute, not colour" reasoning
	// in this same file.
	open := fmt.Sprintf("\x1b[%d;%dm", 1, 7)
	return open + row + "\x1b[0m"
}

// entryRefusalBannerLines builds SPEC §11.9's refusal banner: boxed,
// spanning exactly contentWidth (the pane's own width, per SPEC's
// "spanning the pane's width but not its height"), line 1 the headline,
// line 2 the reason, line 3 the way out. Returns nil below a 5-column
// width (nothing legible fits) so overlayEntryRefusalBanner's caller
// simply shows the background untouched rather than a garbled box.
//
// Line 3 is never truncated away: SPEC's own wording ("the way out for
// that reason ... and, always, that keys are going to the list") makes
// both halves mandatory, and at deck's 80x24 supported floor the box's
// own textWidth (37 columns there) is narrower than several kinds' way
// out text (up to 56 columns) -- a single centerTruncate row used to
// ellipsis away the "keys go to the list" tail entirely at that width
// (review finding: TestReviewRefusalWayOutSurvivesDefaultGeometry).
// wrapToWidth below breaks the way-out text across as many centred rows
// as it needs instead, so every kind's guidance AND the tail survive at
// every supported width; only a pane too narrow for entryRefusalBannerRow
// to draw legibly at all (contentWidth<5, above) drops content.
func (m Model) entryRefusalBannerLines(contentWidth int, sessionName string, r entryRefusalState) []string {
	if contentWidth < 5 {
		return nil
	}
	bc := m.box()
	edgeInner := contentWidth - 2
	top := bc.topLeft + strings.Repeat(bc.horizontal, edgeInner) + bc.topRight
	bottom := bc.bottomLeft + strings.Repeat(bc.horizontal, edgeInner) + bc.bottomRight

	textWidth := contentWidth - 4
	row := func(text string) string {
		return bc.vertical + " " + centerTruncate(m, text, textWidth) + " " + bc.vertical
	}

	headline := fmt.Sprintf("NOT ATTACHED: %s", sessionName)
	lines := []string{top, row(headline), row(r.reason)}
	for _, wl := range wrapToWidth(m.entryRefusalWayOutText(r.kind), textWidth) {
		lines = append(lines, row(wl))
	}
	lines = append(lines, bottom)
	for i, l := range lines {
		lines[i] = m.entryRefusalBannerRow(l)
	}
	return lines
}

// wayOutTailAtomic is entryRefusalWayOut's own mandatory tail, protected
// during wrapToWidth's word-split so it can never itself be broken
// across two wrapped lines: a wrap that split "keys go to the list"
// into "...keys" on one line and "go to the list" on the next would
// still fail TestEntryRefusalWayOutSurvivesDefaultGeometry's literal
// strings.Contains(view, "keys go to the list") check, because the two
// halves are joined by a newline in the rendered preview, not a space.
const wayOutTailAtomic = "keys go to the list"

// wrapToWidth breaks text into as many lines as needed so that every
// returned line's display width (stringWidth) is at most width, breaking
// only at the ASCII space between words -- never inside a word, and
// never inside wayOutTailAtomic wherever it appears in text -- so
// entryRefusalWayOut's own vocabulary (single-letter keys like `F`/`a`/
// `r`/`R`, the `↵` glyph, and short clauses) wraps the way a reader
// expects rather than mid-token, and the mandatory tail phrase always
// lands on one line, never split across two. A "word" wider than width
// by itself (the whole tail counts as one word here) is still placed
// alone on its own line; centerTruncate (this file's row() closure) is
// what would ellipsis it further if it somehow did not fit at all. A
// width<=0 or single-line-fits input returns text as its own
// one-element slice, matching the pre-wrap behaviour exactly.
func wrapToWidth(text string, width int) []string {
	if width <= 0 || stringWidth(text) <= width {
		return []string{text}
	}
	// U+00A0 (NBSP) has the same display width as an ordinary space but
	// is not the ASCII 0x20 byte strings.Split below breaks words on, so
	// swapping the tail's internal spaces for NBSP keeps it as a single
	// unsplittable token through the word loop; the final replace-back
	// restores an ordinary space in whatever line the tail ends up on.
	protectedTail := strings.ReplaceAll(wayOutTailAtomic, " ", "\u00a0")
	protected := strings.ReplaceAll(text, wayOutTailAtomic, protectedTail)
	words := strings.Split(protected, " ")
	var lines []string
	var cur strings.Builder
	curWidth := 0
	for _, w := range words {
		ww := stringWidth(w)
		switch {
		case curWidth == 0:
			cur.WriteString(w)
			curWidth = ww
		case curWidth+1+ww <= width:
			cur.WriteString(" ")
			cur.WriteString(w)
			curWidth += 1 + ww
		default:
			lines = append(lines, cur.String())
			cur.Reset()
			cur.WriteString(w)
			curWidth = ww
		}
	}
	if curWidth > 0 || len(lines) == 0 {
		lines = append(lines, cur.String())
	}
	for i, l := range lines {
		lines[i] = strings.ReplaceAll(l, "\u00a0", " ")
	}
	return lines
}

// centerTruncate centres s within width display columns, truncating with
// m's own ellipsis() when s is too long to fit -- the banner's own text
// rows never overflow the box regardless of session-name length or
// reason-text length.
func centerTruncate(m Model, s string, width int) string {
	if width <= 0 {
		return ""
	}
	text := s
	if stringWidth(text) > width {
		ellW := stringWidth(m.ellipsis())
		if width <= ellW {
			text = truncateToWidth(text, width)
		} else {
			text = truncateToWidth(text, width-ellW) + m.ellipsis()
		}
	}
	w := stringWidth(text)
	left := (width - w) / 2
	if left < 0 {
		left = 0
	}
	right := width - w - left
	if right < 0 {
		right = 0
	}
	return strings.Repeat(" ", left) + text + strings.Repeat(" ", right)
}

// overlayEntryRefusalBanner splices entryRefusalBannerLines' rows into
// background's own contentHeight rows, vertically centred -- SPEC
// §11.9's "spanning the pane's width but not its height, so the passive
// capture stays visible around it": every row above and below the
// banner's own span is left exactly as previewBodyLines' background
// branch (the passive capture, a placeholder, or the crash tail) built
// it, own provenance included. Every banner row is deck's own composed
// content, never a byte of a captured pane, so each is marked
// previewLineDeckOwned regardless of what it replaces.
func (m Model) overlayEntryRefusalBanner(background []string, owners []previewLineOwner, contentWidth int, r entryRefusalState) ([]string, []previewLineOwner) {
	height := len(background)
	if height == 0 {
		return background, owners
	}
	banner := m.entryRefusalBannerLines(contentWidth, m.entryRefusalSessionName(r.sessionID), r)
	n := len(banner)
	if n == 0 {
		return background, owners
	}
	if n > height {
		banner = banner[:height]
		n = height
	}
	start := (height - n) / 2
	out := append([]string(nil), background...)
	outOwners := append([]previewLineOwner(nil), owners...)
	for i := 0; i < n; i++ {
		out[start+i] = banner[i]
		outOwners[start+i] = previewLineDeckOwned
	}
	return out, outOwners
}
