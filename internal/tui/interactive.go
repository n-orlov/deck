package tui

import (
	"context"
	"fmt"
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
	if session.Status == "stopped" {
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

	// Refusal case 1 (PRD II-47): another client is attached to this
	// session. The squeeze is unavoidable -- one tmux window has one size
	// -- so this is checked, and refused, before anything else touches the
	// window; a bystander watching this session must never see it collapse
	// into the preview panel's own box.
	attached, err := client.SessionAttachedCount(ctx, windowTarget)
	if err != nil {
		m.attachError = "Cannot enter interactive mode: " + err.Error()
		return m, nil
	}
	if attached > 0 {
		m.attachError = "Cannot enter interactive mode: another client is attached to this session; press a to attach instead"
		return m, nil
	}

	// Refusal case 2 (PRD II-47/48): the preview box has fewer than the
	// measured 7-inner-row floor (interactiveMinInnerRows;
	// docs/reports/phase3b.md records the measurement). This is checked
	// before any tmux call that would otherwise commit to a fit deck
	// already knows is too small to be usable.
	width, height := m.previewContentSize()
	if width <= 0 {
		m.attachError = "Cannot enter interactive mode: preview panel is too small; press a to attach instead"
		return m, nil
	}
	if height < interactiveMinInnerRows {
		m.attachError = fmt.Sprintf("Cannot enter interactive mode: preview panel has %d inner rows, fewer than the %d-row floor; press a to attach instead", height, interactiveMinInnerRows)
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
	grid, err := interactive.Start(ctx, client, pane.ID, width, height, func(ctx context.Context) ([]byte, error) {
		return interactive.CaptureSeed(ctx, client, pane.ID)
	})
	if err != nil {
		_ = client.RestoreWindowGeometry(ctx, windowTarget, geometry)
		_ = ownership.Release(ctx)
		m.attachError = "Cannot enter interactive mode: " + err.Error()
		return m, nil
	}

	m.interactive = true
	m.interactiveWindowTarget = windowTarget
	m.interactiveGeometry = geometry
	m.interactiveOwnership = ownership
	m.interactiveGrid = grid
	m.interactiveDispatcher = dispatcher
	m.attachError = ""
	return m, nil
}

// exitInteractive is Ctrl+Q's job (SPEC \u00a711.9, PRD Part II): tear down
// the transport, restore the window's own geometry byte-exact (PRD II-9/12)
// and release ownership, in that order, so a concurrent claimant never
// observes a window resized by an owner that has already let go of it.
func (m Model) exitInteractive() (tea.Model, tea.Cmd) {
	if !m.interactive {
		return m, nil
	}
	ctx := context.Background()
	if m.interactiveGrid != nil {
		_ = m.interactiveGrid.Close()
	}
	if m.interactiveWindowTarget != "" {
		_ = m.tmuxClient.RestoreWindowGeometry(ctx, m.interactiveWindowTarget, m.interactiveGeometry)
	}
	if m.interactiveOwnership != nil {
		_ = m.interactiveOwnership.Release(ctx)
	}
	m.interactive = false
	m.interactiveWindowTarget = ""
	m.interactiveGeometry = tmux.WindowGeometry{}
	m.interactiveOwnership = nil
	m.interactiveGrid = nil
	m.interactiveDispatcher = nil
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
	rendered := m.interactiveGrid.Grid().Render()
	lines := strings.Split(rendered, "\n")
	return fitLines(lines, contentHeight)
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
