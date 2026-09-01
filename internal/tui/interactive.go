package tui

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/n-orlov/deck/internal/interactive"
	"github.com/n-orlov/deck/internal/tmux"
)

// interactiveMinInnerRows is PRD Part II requirement 48's measured floor:
// docs/reports/phase3b.md's real-tmux measurement (a Claude-shaped
// full-screen fixture at 41x22/36x22/41x7/41x6) shows 41x7 as the smallest
// USABLE box (one transcript row survives above the fixed header/box/hint
// chrome) and 41x6 as pure chrome with zero transcript -- so 7 inner rows
// is the refusal threshold requirement 47 names, not merely "greater than
// zero". It is still only a floor: the same measurement shows a real
// agent whose input box soft-wraps to two content rows consumes the
// entire 7-row budget on chrome alone, leaving zero transcript exactly
// like the 6-row case does for a single-line input.
const interactiveMinInnerRows = 7

// enterInteractive is `\u21b5`'s new job (SPEC \u00a711.9, PRD Part II task 061
// onward): claim the selected session's window, fit it to the preview
// panel's own content box and start the live transport, so every
// subsequent keystroke forwards to the pane instead of driving list
// navigation until Ctrl+Q leaves (updateInteractive/exitInteractive).
//
// A zero tmuxClient (every constructor before WithTmuxClient, and every
// pre-task-061 unit test) leaves interactive mode unavailable -- this
// degrades to a no-op exactly like attachSelected already does for a nil
// m.attach, never a real tmux invocation an unsuspecting caller did not
// ask for.
func (m Model) enterInteractive() (tea.Model, tea.Cmd) {
	if m.interactive || m.tmuxClient.Socket == "" || len(m.sessions) == 0 || m.selected < 0 || m.selected >= len(m.sessions) {
		return m, nil
	}
	session := m.sessions[m.selected]
	if !canReachPane(session) {
		m.attachError = "Cannot enter interactive mode: session is stopped; resume it first"
		return m, nil
	}
	ctx := context.Background()
	client := m.tmuxClient

	windowTarget, err := tmux.SessionName(session.Slug)
	if err != nil {
		m.attachError = "Cannot enter interactive mode: " + err.Error()
		return m, nil
	}

	// Refusal case 1 (PRD II-47/48): the preview box has fewer than the
	// measured 7-inner-row floor (interactiveMinInnerRows;
	// docs/reports/phase3b.md records the measurement). This is checked
	// FIRST, before the attached-client check below and before any tmux
	// call at all: previewContentSize is pure arithmetic over m's own
	// fields, it touches no tmux state and commits to nothing, so nothing
	// about ordering it first weakens the attached-client check's own
	// guarantee -- that check exists to stop deck from touching a window a
	// bystander is watching, and a refusal that never calls tmux at all
	// touches it even less. Ordering it first also makes the floor refusal
	// exercisable by a deterministic unit test with no tmux server present
	// at all (task 203/PRD F3 backstop): every check below this one calls
	// into client, which requires a live tmux to answer meaningfully.
	width, height := m.previewContentSize()
	if width <= 0 {
		m.attachError = "Cannot enter interactive mode: preview panel is too small; press a to attach instead"
		return m, nil
	}
	if height < interactiveMinInnerRows {
		m.attachError = fmt.Sprintf("Cannot enter interactive mode: preview panel has %d inner rows, fewer than the %d-row floor; press a to attach instead", height, interactiveMinInnerRows)
		return m, nil
	}

	// Refusal case 2 (PRD II-47): another client is attached to this
	// session. The squeeze is unavoidable -- one tmux window has one size
	// -- so this is checked, and refused, before anything else touches the
	// window; a bystander watching this session must never see it collapse
	// into the preview panel's own box. This must still precede every check
	// below it that actually touches the window (ClaimWindowOwnership,
	// FitWindowToPane, ...) -- only the floor check above, which touches
	// nothing, was allowed to move ahead of it.
	attached, err := client.SessionAttachedCount(ctx, windowTarget)
	if err != nil {
		m.attachError = "Cannot enter interactive mode: " + err.Error()
		return m, nil
	}
	if attached > 0 {
		m.attachError = "Cannot enter interactive mode: another client is attached to this session; press a to attach instead"
		return m, nil
	}

	pane, ok, err := client.PreviewPane(ctx, session.Slug)
	if err != nil {
		m.attachError = "Cannot enter interactive mode: " + err.Error()
		return m, nil
	}
	if !ok {
		m.attachError = "Cannot enter interactive mode: no live pane"
		return m, nil
	}

	geometry, err := client.CaptureWindowGeometry(ctx, windowTarget)
	if err != nil {
		m.attachError = "Cannot enter interactive mode: " + err.Error()
		return m, nil
	}
	ownership, acquired, err := client.ClaimWindowOwnership(ctx, windowTarget)
	if err != nil {
		m.attachError = "Cannot enter interactive mode: " + err.Error()
		return m, nil
	}
	if !acquired {
		// Refusal case 3 (PRD II-47): a LIVE process (another deck, or a
		// hand-crafted claim -- ClaimWindowOwnership's own liveness check via
		// kill(pid, 0) is what decides this, not merely "the option is set")
		// already holds ownership of this window.
		m.attachError = "Cannot enter interactive mode: a live process holds ownership of this window; press a to attach instead"
		return m, nil
	}
	if _, err := client.FitWindowToPane(ctx, windowTarget, pane.ID, width, height); err != nil {
		_ = ownership.Release(ctx)
		m.attachError = "Cannot enter interactive mode: " + err.Error()
		return m, nil
	}
	dispatcher, err := tmux.NewDispatcher(ctx, client, pane.ID)
	if err != nil {
		_ = client.RestoreWindowGeometry(ctx, windowTarget, geometry)
		_ = ownership.Release(ctx)
		m.attachError = "Cannot enter interactive mode: " + err.Error()
		return m, nil
	}
	// PRD II-5 (task 089): DECK_INTERACTIVE_TRANSPORT/interactive_transport
	// selects between the two implementations of the same §11.9 contract
	// (internal/config.interactiveTransportEnv already rejects any value
	// other than "pipe"/"capture" at load time, so "capture" is the only
	// other case that can reach here). "pipe" -- the config default -- is
	// interactive.TransportPipe, exactly the transport this call site used
	// before task 070/088/089 existed.
	transport := interactive.TransportPipe
	if m.settings.InteractiveTransport == "capture" {
		transport = interactive.TransportCapture
	}
	grid, err := interactive.StartWithTransport(ctx, client, pane.ID, width, height, func(ctx context.Context) ([]byte, error) {
		return interactive.CaptureSeed(ctx, client, pane.ID)
	}, transport)
	if err != nil {
		_ = client.RestoreWindowGeometry(ctx, windowTarget, geometry)
		_ = ownership.Release(ctx)
		m.attachError = "Cannot enter interactive mode: " + err.Error()
		return m, nil
	}
	// PRD R89/task 030: persist enough to reclaim this claim from a LATER
	// process's start if this one never gets to exitInteractive itself --
	// SIGKILL cannot be handled at all, so this is the only place the
	// guarantee can be made good. Best-effort: a write failure here must
	// never undo an otherwise-successful entry into interactive mode, and
	// there is nothing under TransportCapture to write against (no pipe is
	// ever armed, so PipeTempDir reports ok=false and this is skipped).
	if dir, ok := grid.PipeTempDir(); ok {
		_ = tmux.SaveInteractiveClaimRecord(dir, tmux.InteractiveClaimRecord{
			Socket:       client.Socket,
			PaneTarget:   pane.ID,
			WindowTarget: windowTarget,
			Geometry:     geometry,
		})
	}

	// SPEC §7: entering the interactive preview is a deck-mediated
	// attachment in exactly `a`'s sense -- the keyboard is about to reach
	// the pane -- so the same durable transaction (store.RecordAttachment,
	// via the same m.prepareAttach attachSelected consults) answers a
	// waiting row and acknowledges an error row, against the durable row
	// rather than the possibly-stale list frame. It runs only after every
	// refusal and every fallible tmux step above: a refused or failed
	// entry must not claim the user answered anything. A store failure
	// here refuses the entry like attachSelected refuses the attach, and
	// unwinds the claim already made -- grid first, then geometry, then
	// ownership, the same order exitInteractive's teardown uses.
	if m.prepareAttach != nil {
		if err := m.prepareAttach(ctx, session.ID); err != nil {
			_ = grid.Close()
			_ = client.RestoreWindowGeometry(ctx, windowTarget, geometry)
			_ = ownership.Release(ctx)
			m.attachError = "Cannot enter interactive mode: " + err.Error()
			return m, nil
		}
	}

	m.interactive = true
	m.interactiveWindowTarget = windowTarget
	m.interactiveGeometry = geometry
	m.interactiveOwnership = ownership
	m.interactiveGrid = grid
	m.interactiveDispatcher = dispatcher
	m.interactiveScrollOffset = 0
	m.attachError = ""
	return m, nil
}

// teardownInteractive is exitInteractive's own disarm/restore/release
// sequence, factored out so it can run from two exit routes exitInteractive
// itself never sees: SIGTERM's QuitMsg (Bubble Tea's own signal handler
// hands that straight to Program.Run's eventLoop, which returns the model
// completely unchanged, never calling Update at all) and a panic anywhere
// in a single Update call's dynamic extent (Bubble Tea's own recover
// restores the terminal but discards the model outright once the panic
// unwinds past Run's own `model, err := p.eventLoop(...)` assignment).
// Both routes are handled from cmd/deck, which owns the process's own exit
// sequencing (see ShutdownInteractive below and cmd/deck's
// interactiveShutdownOnPanic wrapper) -- this only performs the tmux-facing
// half, never touching any of Model's own bookkeeping fields, because
// whoever calls it here is not about to hand back a "next" model for the
// rest of Update to keep using; the process is exiting.
func (m Model) teardownInteractive(ctx context.Context) {
	if !m.interactive {
		return
	}
	if m.interactiveGrid != nil {
		_ = m.interactiveGrid.Close()
	}
	if m.interactiveWindowTarget != "" {
		_ = m.tmuxClient.RestoreWindowGeometry(ctx, m.interactiveWindowTarget, m.interactiveGeometry)
	}
	if m.interactiveOwnership != nil {
		_ = m.interactiveOwnership.Release(ctx)
	}
}

// ShutdownInteractive is deck's own last-chance cleanup for the SIGTERM and
// panic exit routes named above: a no-op, exactly like exitInteractive's
// own guard, unless interactive mode was still armed. It is exported so
// cmd/deck -- which owns both the post-Run() SIGTERM check and the panic
// recover -- can reach it without either wrapper needing to know anything
// about interactive mode's own fields.
func (m Model) ShutdownInteractive(ctx context.Context) {
	m.teardownInteractive(ctx)
}

// exitInteractive is Ctrl+Q's job (SPEC \u00a711.9, PRD Part II): tear down
// the transport, restore the window's own geometry byte-exact (PRD II-9/12)
// and release ownership, in that order, so a concurrent claimant never
// observes a window resized by an owner that has already let go of it.
func (m Model) exitInteractive() (tea.Model, tea.Cmd) {
	if !m.interactive {
		return m, nil
	}
	m.teardownInteractive(context.Background())
	m.interactive = false
	m.interactiveWindowTarget = ""
	m.interactiveGeometry = tmux.WindowGeometry{}
	m.interactiveOwnership = nil
	m.interactiveGrid = nil
	m.interactiveDispatcher = nil
	m.interactiveScrollOffset = 0
	// steer 018 item 4 / SPEC §11: RestoreWindowGeometry above just put the
	// window back at its PRE-entry size, which is not generally the preview
	// panel's own current content size -- previewFit's own coalescing
	// (previewFitSessionID) would otherwise treat this session as already
	// settled and skip re-fitting it, since its ID has not changed, leaving
	// it stuck at the restored size until some OTHER selection is visited
	// first. Clearing it here makes the very next previewTick re-evaluate
	// and fit it back to the panel, exactly as if the selection had just
	// settled on it.
	m.previewFitSessionID = ""
	// previewFitInFlight is deliberately NOT cleared here: if an attempt for
	// this session was already outstanding when interactive mode was
	// entered/left, it still owes its previewFitDone, and clearing the
	// marker early would let the next tick issue a second, overlapping fit
	// against the same window (task R63).
	return m, nil
}

// updateInteractive is reached for every key while m.interactive is true
// (the tea.KeyMsg case in Update returns through here before any
// dialog/list-mode switch even looks at the message): Ctrl+Q is the one,
// deliberate exception (SPEC \u00a711.9 names it the only way out -- Bubble
// Tea's own raw mode clears IXON, so Ctrl+Q survives the terminal driver
// and reaches deck rather than being consumed as a software flow-control
// resume, see docs/reports/phase3b-findings.md), and every other key
// forwards to the target pane instead of being interpreted here at all --
// in particular Ctrl+C, which would otherwise quit deck, is forwarded
// (interrupting the target's own process is exactly what a real attached
// terminal would do with it).
func (m Model) updateInteractive(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.String() == "ctrl+q" {
		return m.exitInteractive()
	}
	// Any keystroke forwarded to the target snaps the view back to the
	// live bottom first (PRD II-51): scrolled-back history is read-only by
	// construction (there is nowhere on screen to place a cursor a
	// scrollback line's own text came from), so typing "blindly" into a
	// pane the user cannot currently see would be far more surprising than
	// the ordinary terminal-emulator convention this matches -- scrolling
	// back, then resuming input, jumps back to the bottom.
	m.interactiveScrollOffset = 0
	if m.interactiveDispatcher == nil {
		return m, nil
	}
	ctx := context.Background()
	if named, ok := interactiveNamedKey(msg); ok {
		_ = m.interactiveDispatcher.SendNamedKey(ctx, named)
		return m, nil
	}
	if payload, ok := interactiveLiteralPayload(msg); ok {
		_ = interactive.SendKeyRun(ctx, m.interactiveDispatcher, payload)
	}
	return m, nil
}

// previewContentSize is the exact (contentWidth, contentHeight) box
// renderSideBySideFrame/renderStackedFrame hand previewBodyLines this
// frame -- computed the same way those two already do (`pw-4` regardless
// of layout; `height-2` off the SHARED sidebar/preview height in
// side-by-side/collapsed, or the preview's OWN height when stacked, since
// only stacked gives the two panels different heights) -- so
// enterInteractive fits the tmux window to precisely what the preview
// panel is about to render into, never a stale or mismatched size.
func (m Model) previewContentSize() (width, height int) {
	layout := m.computeLayout()
	width = max(layout.Preview.Width-4, 0)
	if layout.Effective == LayoutStacked {
		height = max(layout.Preview.Height-2, 0)
	} else {
		height = max(layout.Sidebar.Height-2, 0)
	}
	return width, height
}

// interactiveBodyLines renders the live grid into the preview panel while
// m.interactive is true, in place of the passive capture-pane preview
// previewBodyLines otherwise shows. previewContentLine (the caller's own
// per-line renderer) pads/crops each line to the panel's exact content
// width already, so this only needs to pad/truncate the row COUNT to
// contentHeight, exactly like every other previewBodyLines branch.
func (m Model) interactiveBodyLines(contentWidth, contentHeight int) []string {
	lines, _ := m.interactiveGrid.RenderRows(m.interactiveScrollOffset, contentHeight)
	// R93/task 206: mark an in-progress drag-to-copy selection, if any,
	// before the not-repainted check below -- highlightInProgressSelection
	// only ever adds self-closing SGR spans around existing content, so
	// interactiveGridIsBlank's own ANSI-stripping still sees the same
	// blank-or-not verdict either way, and applying it here (rather than
	// after fitLines) keeps viewRow == this slice's own index, the exact
	// row space interactiveGrid.AbsoluteRow/SelectionHighlightRange use.
	lines = m.highlightInProgressSelection(lines, contentHeight)
	// The not-repainted announcement (PRD II-49) only ever applies to the
	// LIVE view: scrolled-back history, if any exists at all, is by
	// definition real content that once appeared on screen, so it is never
	// blank in the sense this check means, and showing the announcement
	// over genuine history would misreport "nothing has happened yet" while
	// looking straight at something that did.
	if m.interactiveScrollOffset == 0 && interactiveGridIsBlank(lines) {
		lines = append([]string{interactiveNotRepaintedNotice}, lines...)
	}
	return fitLines(lines, contentHeight)
}

// interactiveRepaintAnsiEscapeRe strips the CSI (SGR) and OSC (hyperlink)
// escape sequences vt.SafeEmulator.Render() interleaves between styled
// runs, the same pattern internal/interactive's own gridContains test
// helper uses (task 085) -- without it, a coloured cursor cell or a
// background-only style code would read as "content" and defeat the
// blank check below on every render, announcement included.
var interactiveRepaintAnsiEscapeRe = regexp.MustCompile(`\x1b\[[0-9:;]*[A-Za-z]|\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)`)

// interactiveNotRepaintedNotice is PRD II-49's required text. Entering
// interactive mode always resizes the target window first
// (enterInteractive's FitWindowToPane, above); a target that produces no
// output at all in response -- requirement 49's named worst case, a
// target waiting on a network round-trip, is exactly what task 026's
// FAKE_CLAUDE_REPAINT_MODE=never fixture stands in for -- would otherwise
// leave the panel showing an empty bordered frame with no way to tell
// "the target has not repainted yet" apart from "deck itself is broken".
// Named once here so the render site and every test asserting it cannot
// drift apart.
const interactiveNotRepaintedNotice = "deck: the agent has not repainted since the resize"

// interactiveGridIsBlank reports whether every rendered row, once its own
// SGR/OSC escapes are stripped, is empty or all-whitespace. It is checked
// fresh on every render rather than cached on the Session: whatever put
// real content into the grid -- the initial seed capture racing ahead of
// the live pipe's own arm (PRD II-16), or a byte the live drain path
// delivered afterward -- clears the announcement the moment it happens,
// with no extra bookkeeping needed to tell the two apart, and a target
// that never writes anything at all (PRD II-49's fixture) leaves it
// showing for as long as interactive mode lasts.
func interactiveGridIsBlank(lines []string) bool {
	for _, line := range lines {
		stripped := interactiveRepaintAnsiEscapeRe.ReplaceAllString(line, "")
		if strings.TrimSpace(stripped) != "" {
			return false
		}
	}
	return true
}

// interactiveNamedKey maps a Bubble Tea KeyMsg to one of
// internal/tmux/key.go's namedKeyAllowlist names (PRD II-34/35): keys
// whose byte encoding depends on the pane's own terminal mode
// (application cursor keys, vt220 function-key escapes) go by NAME, so
// tmux -- which knows that mode and deck does not -- produces the bytes,
// never deck itself. Every key with a fixed, mode-independent byte value
// (runes, Space, every C0 control byte including Enter/Tab/Escape/
// Backspace) is handled by interactiveLiteralPayload instead; there is no
// overlap between the two.
//
// Ctrl/Shift/Ctrl+Shift-modified arrows, Home, End and the page keys go
// by tmux name here too (steer 017 item 1 / SPEC.md §11.9's "forwardable
// set is enumerated and tested key by key"): bubbletea decodes each of
// these to its OWN distinct KeyType (KeyCtrlLeft, KeyShiftHome, ... --
// never KeyLeft with a modifier flag set), so leaving them out of this
// switch is not a mistranslation, it is the key vanishing into the
// default branch below with no bytes written and no record of the drop.
// Alt-modified keys are refused above before reaching this switch, so an
// Alt-modified Ctrl/Shift combination (e.g. Ctrl+Alt+Left) is a known,
// deliberate gap, not silently handled here: internal/tmux/key.go's
// allowlist comment records why (no caller can reach that name yet).
func interactiveNamedKey(msg tea.KeyMsg) (string, bool) {
	if msg.Alt {
		return "", false
	}
	switch msg.Type {
	case tea.KeyUp:
		return "Up", true
	case tea.KeyDown:
		return "Down", true
	case tea.KeyLeft:
		return "Left", true
	case tea.KeyRight:
		return "Right", true
	case tea.KeyHome:
		return "Home", true
	case tea.KeyEnd:
		return "End", true
	case tea.KeyPgUp:
		return "PageUp", true
	case tea.KeyPgDown:
		return "PageDown", true
	case tea.KeyCtrlUp:
		return "C-Up", true
	case tea.KeyCtrlDown:
		return "C-Down", true
	case tea.KeyCtrlLeft:
		return "C-Left", true
	case tea.KeyCtrlRight:
		return "C-Right", true
	case tea.KeyCtrlHome:
		return "C-Home", true
	case tea.KeyCtrlEnd:
		return "C-End", true
	case tea.KeyCtrlPgUp:
		return "C-PgUp", true
	case tea.KeyCtrlPgDown:
		return "C-PgDn", true
	case tea.KeyShiftUp:
		return "S-Up", true
	case tea.KeyShiftDown:
		return "S-Down", true
	case tea.KeyShiftLeft:
		return "S-Left", true
	case tea.KeyShiftRight:
		return "S-Right", true
	case tea.KeyShiftHome:
		return "S-Home", true
	case tea.KeyShiftEnd:
		return "S-End", true
	case tea.KeyCtrlShiftUp:
		return "C-S-Up", true
	case tea.KeyCtrlShiftDown:
		return "C-S-Down", true
	case tea.KeyCtrlShiftLeft:
		return "C-S-Left", true
	case tea.KeyCtrlShiftRight:
		return "C-S-Right", true
	case tea.KeyCtrlShiftHome:
		return "C-S-Home", true
	case tea.KeyCtrlShiftEnd:
		return "C-S-End", true
	case tea.KeyDelete:
		return "Delete", true
	case tea.KeyInsert:
		return "Insert", true
	case tea.KeyShiftTab:
		return "BTab", true
	case tea.KeyF1:
		return "F1", true
	case tea.KeyF2:
		return "F2", true
	case tea.KeyF3:
		return "F3", true
	case tea.KeyF4:
		return "F4", true
	case tea.KeyF5:
		return "F5", true
	case tea.KeyF6:
		return "F6", true
	case tea.KeyF7:
		return "F7", true
	case tea.KeyF8:
		return "F8", true
	case tea.KeyF9:
		return "F9", true
	case tea.KeyF10:
		return "F10", true
	case tea.KeyF11:
		return "F11", true
	case tea.KeyF12:
		return "F12", true
	}
	return "", false
}

// interactiveLiteralPayload converts every other KeyMsg into the raw bytes
// to forward via a single Dispatcher.SendLiteral call (task 059/II-39's
// SendKeyRun wrapper -- one Update call is always exactly one decoded
// keystroke by the time it reaches here, so one call here is always one
// write, never a split payload). Runes/Space forward verbatim; every C0
// control byte (0-31, plus DEL/127) forwards as that literal byte -- the
// value is fixed regardless of the pane's own terminal mode, so it never
// needs tmux's own name translation the way arrows/Home/End/function keys
// do. Alt is a plain ESC prefix, matching a real terminal's own encoding
// for an Alt-modified key when 8-bit meta is off.
//
// Ctrl+Q never reaches here: updateInteractive intercepts it first.
func interactiveLiteralPayload(msg tea.KeyMsg) (string, bool) {
	var body string
	switch {
	case msg.Type == tea.KeyRunes || msg.Type == tea.KeySpace:
		if len(msg.Runes) == 0 {
			return "", false
		}
		body = string(msg.Runes)
	case msg.Type >= 0 && msg.Type <= 31:
		body = string([]byte{byte(msg.Type)})
	case msg.Type == 127:
		body = string([]byte{127})
	default:
		return "", false
	}
	if msg.Alt {
		body = "\x1b" + body
	}
	return body, true
}
