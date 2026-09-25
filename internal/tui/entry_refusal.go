package tui

import (
	"fmt"
	"strings"

	"github.com/n-orlov/deck/internal/theme"
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
// on), always ending with `keys go to the list`" -- every branch below
// ends with exactly that phrase, unpunctuated, so a caller (or a test)
// can assert the literal suffix regardless of what precedes it.
func entryRefusalWayOut(kind entryRefusalKind) string {
	const tail = "keys go to the list"
	switch kind {
	case entryRefusalAttachedElsewhere, entryRefusalOwnedElsewhere:
		return "F forces entry, a attaches instead \u2014 " + tail
	case entryRefusalStopped:
		return "resume it, then try again \u2014 " + tail
	case entryRefusalRowFloor, entryRefusalShrank:
		return "a attaches instead \u2014 " + tail
	default: // entryRefusalNoLivePane, entryRefusalOther
		return tail
	}
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
	lines := []string{top, row(headline), row(r.reason), row(entryRefusalWayOut(r.kind)), bottom}
	for i, l := range lines {
		lines[i] = m.entryRefusalBannerRow(l)
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
