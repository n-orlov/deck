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

// stoppedSessionRefusalTail is the one place in the package's non-test
// sources that spells out the stopped-session refusal, shared verbatim by
// enterInteractiveBody below ("Cannot enter interactive mode: " + tail) and
// by attachSelected in tui.go ("Cannot attach: " + tail). Both messages stay
// byte-identical to what they were before the split; keeping the phrase in a
// single constant is what keeps the refusal ladder's four literals to one
// occurrence each even though `a` refuses a stopped session too.
const stoppedSessionRefusalTail = "session is stopped; resume it first"

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
	return m.enterInteractiveBody(false)
}

// enterInteractiveBody is enterInteractive's own refusal ladder and claim
// sequence, factored out so a second entry path (task 105's `F`, force
// enabled) can share every refusal and every fallible tmux step with `↵`
// (force disabled) rather than maintaining a second copy that could drift
// -- the four refusal message literals below each still appear exactly
// once in the package's non-test sources because there is only one body
// producing them, whichever caller reaches it. force (wired to tui.go's
// list-mode `F` by task 105) is consulted in exactly two places below:
// it skips the attached-client refusal outright, and it takes the claim
// via ForceClaimWindowOwnership instead of ClaimWindowOwnership -- every
// other refusal (the floor, the stopped-session check, no live pane, a
// LIVE claim holder surviving the force claim itself) still applies.
func (m Model) enterInteractiveBody(force bool) (tea.Model, tea.Cmd) {
	if m.interactive || m.tmuxClient.Socket == "" || len(m.sessions) == 0 || m.selected < 0 || m.selected >= len(m.sessions) {
		return m, nil
	}
	session := m.sessions[m.selected]
	if !canReachPane(session) {
		m.attachError = "Cannot enter interactive mode: " + stoppedSessionRefusalTail
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
		// This wording spells the floor out literally rather than %d'ing
		// interactiveMinInnerRows into it, so the phrase below stays the
		// one place in the package's non-test sources naming this floor --
		// keep it in sync with the constant above if it ever moves off 7.
		m.attachError = fmt.Sprintf("Cannot enter interactive mode: preview panel has %d inner rows, fewer than the 7-row floor; press a to attach instead", height)
		return m, nil
	}

	// Refusal case 2 (PRD II-47): some other client is already attached to
	// this session. The squeeze is unavoidable -- one tmux window has one size
	// -- so this is checked, and refused, before anything else touches the
	// window; a bystander watching this session must never see it collapse
	// into the preview panel's own box. This must still precede every check
	// below it that actually touches the window (ClaimWindowOwnership,
	// FitWindowToPane, ...) -- only the floor check above, which touches
	// nothing, was allowed to move ahead of it.
	// force (task 105's `F`) skips only this one refusal: another client
	// attached is exactly the condition force exists to steal past, so it is
	// the sole check force bypasses. Every other refusal in this ladder
	// (floor, stopped session, no live pane, a LIVE claim holder below)
	// still applies unchanged under force.
	if !force {
		attached, err := client.SessionAttachedCount(ctx, windowTarget)
		if err != nil {
			m.attachError = "Cannot enter interactive mode: " + err.Error()
			return m, nil
		}
		if attached > 0 {
			m.attachError = "Cannot enter interactive mode: another client is attached to this session; press a to attach instead, or F to force it"
			return m, nil
		}
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
	var ownership *tmux.WindowOwnership
	var acquired bool
	if force {
		ownership, acquired, err = client.ForceClaimWindowOwnership(ctx, windowTarget)
	} else {
		ownership, acquired, err = client.ClaimWindowOwnership(ctx, windowTarget)
	}
	if err != nil {
		m.attachError = "Cannot enter interactive mode: " + err.Error()
		return m, nil
	}
	if !acquired {
		// Refusal case 3 (PRD II-47): a LIVE process (another deck, or a
		// hand-crafted claim -- ClaimWindowOwnership's own liveness check via
		// kill(pid, 0) is what decides this, not merely "the option is set")
		// already holds ownership of this window.
		m.attachError = "Cannot enter interactive mode: a live process holds ownership of this window; press a to attach instead, or F to force it"
		return m, nil
	}
	// R100 (SPEC section 11.9): the geometry to restore is recorded beside
	// the claim, written by whoever finds none there and read -- never
	// rewritten -- by whoever takes the claim afterwards. This runs only
	// once the claim above was actually acquired (both the first
	// claimant and a `F` steal reach here identically): geometry, already
	// captured above from this process's own CaptureWindowGeometry, is
	// what gets written when @deck_isize_geometry is absent; when it is
	// already present (this entry stole an existing holder's claim),
	// geometry is replaced with the value already recorded there, so the
	// ORIGINAL pre-entry size -- not this stealer's own capture of the
	// previous holder's already-fitted size -- is what every later step
	// below (the fit, SaveInteractiveClaimRecord, and eventually the
	// restore) uses.
	geometry, err = client.ResolveIsizeGeometry(ctx, windowTarget, geometry)
	if err != nil {
		// This bail is release-only, and deliberately so: nothing has
		// resized the window yet (the fit is below), so there is no
		// geometry to restore -- and @deck_isize_geometry must NOT be
		// cleared here, because the failure that lands here is either a
		// failed READ of an option that may well already hold an earlier
		// holder's original pre-entry size (clearing it would destroy the
		// one record R100 exists to preserve) or a failed WRITE that by
		// definition set nothing. err also leaves geometry zero-valued,
		// so restoring it would resize the window to 0x0.
		releaseIfStillMine(ctx, ownership)
		m.attachError = "Cannot enter interactive mode: " + err.Error()
		return m, nil
	}
	if _, err := client.FitWindowToPane(ctx, windowTarget, pane.ID, width, height); err != nil {
		// The first bail with real state behind it, so it unwinds through
		// the full still-mine-gated teardown rather than merely releasing
		// (task 112): FitWindowToPane reports its error AFTER however many
		// resize-window calls it already made (it returns that count), and
		// ResolveIsizeGeometry above has already written or adopted
		// @deck_isize_geometry -- so a release-only unwind here would leave
		// the window partially fitted and the geometry record stale behind
		// an entry that refused, and the NEXT entry would then adopt this
		// failed entry's own halfway size as the window's original
		// geometry. There is no transport yet (nil grid).
		teardownInteractiveClaim(ctx, client, ownership, windowTarget, geometry, nil)
		m.attachError = "Cannot enter interactive mode: " + err.Error()
		return m, nil
	}
	dispatcher, err := tmux.NewDispatcher(ctx, client, pane.ID)
	if err != nil {
		teardownInteractiveClaim(ctx, client, ownership, windowTarget, geometry, nil)
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
		teardownInteractiveClaim(ctx, client, ownership, windowTarget, geometry, nil)
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
	// ownership, the same order exitInteractive's teardown uses (and
	// through the same still-mine-gated helper, so an unwind that races a
	// steal disarms nothing of the winner's; task 112).
	if m.prepareAttach != nil {
		if err := m.prepareAttach(ctx, session.ID); err != nil {
			teardownInteractiveClaim(ctx, client, ownership, windowTarget, geometry, grid)
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
	teardownInteractiveClaim(ctx, m.tmuxClient, m.interactiveOwnership, m.interactiveWindowTarget, m.interactiveGeometry, m.interactiveGrid)
}

// claimStillMine is task 103's probe as every teardown path in this file
// asks it: true only when OwnershipOption on this ownership's own target
// still reads exactly the claim this process confirmed (ClaimStillMine).
// A nil ownership (nothing was ever claimed) and a probe transport error
// both answer false -- acting on an unconfirmed claim is never safer than
// standing down.
func claimStillMine(ctx context.Context, ownership *tmux.WindowOwnership) bool {
	if ownership == nil {
		return false
	}
	state, err := ownership.Probe(ctx)
	return err == nil && state == tmux.ClaimStillMine
}

// releaseIfStillMine releases ownership only once claimStillMine above
// confirms the claim; WindowOwnership.Release already self-gates its own
// unset the same way internally, but every teardown call site in this
// file consults the probe explicitly first so a stolen-from holder is
// uniformly observable (in tests and in this file's own control flow) as
// touching nothing rather than merely happening to no-op. It is the
// pre-fit unwind's own teardown: at the one bail that still uses it (a
// ResolveIsizeGeometry failure) nothing has resized the window, no
// transport exists yet and no geometry record was left behind, so
// releasing the claim is the whole of the unwind. Every bail from the
// fit onward goes through teardownInteractiveClaim below instead.
func releaseIfStillMine(ctx context.Context, ownership *tmux.WindowOwnership) {
	if !claimStillMine(ctx, ownership) {
		return
	}
	_ = ownership.Release(ctx)
}

// teardownInteractiveClaim is the ONE teardown sequence every post-fit
// exit route in this file goes through -- exitInteractive and
// ShutdownInteractive (both via teardownInteractive above) and
// enterInteractive's own failed-entry unwinds -- so SPEC §11.9's R100
// gating cannot be present on one route and missing on another (task
// 112).
//
// It consults task 103's probe FIRST, before touching anything at all,
// and every step below is decided by that single answer:
//
//   - the transport is always closed, but a stolen-from holder closes
//     only its OWN end of it (Session.CloseLocal, which issues no tmux
//     command): tmux's `pipe-pane -t target` disarm is target-scoped,
//     not holder-scoped, so once the claim has been stolen the pipe armed
//     on that pane is the WINNER's, and Session.Close would take the
//     winner's live transport down as a side effect of this holder
//     tidying up (internal/tmux.PanePipe.Close's own doc names this).
//     This is why the probe has to precede the transport close rather
//     than following it, as an earlier version of this teardown did.
//   - the geometry restore, the @deck_isize_geometry unset and the
//     ownership release run only for a claim that is still ours, in that
//     order (restore before release, so a concurrent claimant never
//     observes a window resized by an owner that has already let go of
//     it). A stolen claim (ClaimForeignLive) skips all three: the window
//     belongs to whoever stole it now, and restoring geometry or clearing
//     the geometry record out from under that new holder is exactly the
//     bug R100 exists to prevent.
//
// A probe transport error is treated the same as "not still mine". A nil
// grid (a bail before the transport was ever started) and an empty target
// (nothing was fitted) each simply skip their own step.
func teardownInteractiveClaim(ctx context.Context, client tmux.Client, ownership *tmux.WindowOwnership, target string, geometry tmux.WindowGeometry, grid *interactive.Session) {
	stillMine := claimStillMine(ctx, ownership)
	if grid != nil {
		if stillMine {
			_ = grid.Close()
		} else {
			_ = grid.CloseLocal()
		}
	}
	if !stillMine {
		return
	}
	if target != "" {
		_ = client.RestoreWindowGeometry(ctx, target, geometry)
		_ = client.ClearIsizeGeometry(ctx, target)
	}
	_ = ownership.Release(ctx)
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
//
// The second return is this slice's own per-row provenance (task 002/B1,
// the same class review's B1 finding named for cropPreviewBottomLeft):
// every row RenderRows/highlightInProgressSelection produced is the live
// grid's own foreign screen content and stays previewLineForeign, while
// interactiveNotRepaintedNotice (prepended below) and any pad row
// fitInteractiveBodyLines adds to reach contentHeight are deck's own
// composed copy and are marked previewLineDeckOwned -- see
// fitInteractiveBodyLines' own doc for why previewBodyLines used to mark
// this whole slice foreign instead.
func (m Model) interactiveBodyLines(contentWidth, contentHeight int) ([]string, []previewLineOwner) {
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
	notice := m.interactiveScrollOffset == 0 && interactiveGridIsBlank(lines)
	return fitInteractiveBodyLines(lines, contentHeight, notice)
}

// fitInteractiveBodyLines is interactiveBodyLines' own
// notice-then-pad/truncate sequence, factored out so a test can drive it
// directly on a caller-supplied lines slice without needing a live
// *interactive.Session (interactiveBodyLines itself calls
// m.interactiveGrid.RenderRows, which requires a real tmux pane to
// construct at all -- exactly the constraint interactive_test.go's older
// fitLinesWithOptionalNotice test helper already worked around for the
// notice-only claim; this is that same idea extended to per-row
// provenance).
//
// lines is RenderRows/highlightInProgressSelection's own output -- the
// live grid's foreign screen content, always previewLineForeign. When
// notice is true, interactiveNotRepaintedNotice is prepended as line 0,
// deck's own composed copy (previewLineDeckOwned), exactly like
// cropPreviewBottomLeft's geometry line. Whatever fitLines' own
// pad-or-truncate rule then does to reach contentHeight, any row IT adds
// (lines started shorter than contentHeight, mirroring cropPreviewBottomLeft's
// vertical blank-fill) is also deck's own synthesized copy, never a byte
// of the grid's own content, so it is marked previewLineDeckOwned too --
// never fitLines' own generic zero-value pad, which would default a
// padded []previewLineOwner entry to previewLineForeign instead.
// (m.interactiveGrid.RenderRows always returns exactly contentHeight rows
// today, so in production this pad branch never actually fires -- only
// notice's own truncation does, dropping RenderRows' own last, blank row
// when the notice pushes the slice one over contentHeight -- but the
// provenance is still correct symmetrically, and exercised directly by
// TestFitInteractiveBodyLinesOwnership below, in case that invariant ever
// changes.)
func fitInteractiveBodyLines(lines []string, contentHeight int, notice bool) ([]string, []previewLineOwner) {
	owners := foreignPreviewLines(len(lines))
	if notice {
		lines = append([]string{interactiveNotRepaintedNotice}, lines...)
		owners = append([]previewLineOwner{previewLineDeckOwned}, owners...)
	}
	for len(lines) < contentHeight {
		lines = append(lines, "")
		owners = append(owners, previewLineDeckOwned)
	}
	if len(lines) > contentHeight {
		lines = lines[:contentHeight]
		owners = owners[:contentHeight]
	}
	return lines, owners
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
// never KeyLeft with a modifier flag set), so leaving them out of
// interactiveBareNamedKey's switch below is not a mistranslation, it is
// the key vanishing into that switch's fallthrough with no bytes written
// and no record of the drop.
//
// Alt (issue #28) resolves through interactiveAltNamedKeys below instead
// of being refused outright, which is what this function did until that
// issue: SPEC.md §11.9 names `Alt` in the same breath as `Ctrl` and
// `Shift` ("Modified navigation keys forward, like the unmodified ones,
// by tmux key name"), so the blanket `if msg.Alt { return "", false }`
// that used to open this function turned every Alt-modified special key
// into precisely the failure that paragraph forbids -- "a keystroke that
// does nothing and reports nothing". The drop was silent because it was
// double-ended: interactiveLiteralPayload cannot catch an Alt-modified
// special key either, since EVERY special KeyType is NEGATIVE
// (charmbracelet/bubbletea@v1.3.10/key.go:205 spells
// `KeyRunes KeyType = -(iota + 1)`, and KeyUp/KeyDown/... count down from
// there), so tea.KeyUp fails that function's own `msg.Type >= 0 &&
// msg.Type <= 31` guard and lands in its default branch too, leaving
// updateInteractive above to write nothing at all. Alt+<rune>, Alt+Enter,
// Alt+Tab, Alt+Escape and Alt+Backspace were never affected and are not
// touched here: KeyRunes carries its bytes in msg.Runes, and
// Enter/Tab/Escape/Backspace are POSITIVE C0/DEL values (13, 9, 27, 127),
// so all of them still take interactiveLiteralPayload's own ESC-prefix
// path exactly as before.
func interactiveNamedKey(msg tea.KeyMsg) (string, bool) {
	name, ok := interactiveBareNamedKey(msg.Type)
	if !ok {
		return "", false
	}
	if !msg.Alt {
		return name, true
	}
	// A bare name whose Alt-modified form deck cannot forward maps to ""
	// in interactiveAltNamedKeys (one of the two listed gaps that table's
	// own comment enumerates), and a bare name absent from that table
	// altogether reads the same zero value -- so this refusal is the
	// enumerated gap §11.9 asks for rather than a default branch, and
	// interactive_test.go's
	// TestEveryBareNamedKeyHasAnExplicitAltVerdict fails if a name is ever
	// added to interactiveBareNamedKey without a verdict here, so the gap
	// cannot grow silently the way the blanket Alt refusal let it.
	altName := interactiveAltNamedKeys[name]
	if altName == "" {
		return "", false
	}
	return altName, true
}

// interactiveAltNamedKeys is the Alt half of SPEC.md §11.9's "enumerated
// and tested key by key" forwardable set (issue #28), keyed by the BARE
// name interactiveBareNamedKey below already resolved -- keying off the
// name rather than re-switching on tea.KeyType is what keeps the two
// tables from ever disagreeing about which KeyType a name belongs to.
// The value is the tmux key name carrying that same key WITH Alt, or ""
// for a listed gap (the two at the bottom of this map).
//
// tmux spells Alt `M-`, and its key-string parser accepts the modifier
// prefixes in any order, but every name below is written in the same
// C-/M-/S- order the survey used rather than relying on that. The survey
// is the one internal/tmux/key.go's namedKeyAllowlist comment requires,
// re-run for these names against a real tmux 3.6b: one fresh server per
// key on a private socket, `send-keys <name>` into a pane running `cat`
// (which echoes whatever bytes it receives straight back, unedited), the
// result read out of `capture-pane -p`'s own caret notation. Each
// captured sequence was then looked up in
// charmbracelet/bubbletea@v1.3.10/key.go's own `sequences` table and
// found to decode back to exactly the {KeyType, Alt: true} this map is
// keyed from, so tmux's translation and bubbletea's decoding are proven
// to agree for all forty-one names, not assumed to.
// internal/tmux/key_test.go's
// TestSendNamedKeyDeliversAltModifiedNavigationKeysByTmuxsOwnTranslation
// re-runs that survey from the suite and holds the exact bytes; the
// caret-notation summary is:
//
//	M-Up ^[[1;3A      M-Down ^[[1;3B      M-Left ^[[1;3D      M-Right ^[[1;3C
//	M-Home ^[[1;3H    M-End ^[[1;3F       M-PageUp ^[[5;3~    M-PageDown ^[[6;3~
//	M-Delete ^[[3;3~
//	C-M-Up ^[[1;7A    C-M-Down ^[[1;7B    C-M-Left ^[[1;7D    C-M-Right ^[[1;7C
//	C-M-Home ^[[1;7H  C-M-End ^[[1;7F     C-M-PgUp ^[[5;7~    C-M-PgDn ^[[6;7~
//	S-M-Up ^[[1;4A    S-M-Down ^[[1;4B    S-M-Left ^[[1;4D    S-M-Right ^[[1;4C
//	S-M-Home ^[[1;4H  S-M-End ^[[1;4F
//	C-M-S-Up ^[[1;8A  C-M-S-Down ^[[1;8B  C-M-S-Left ^[[1;8D  C-M-S-Right ^[[1;8C
//	C-M-S-Home ^[[1;8H  C-M-S-End ^[[1;8F
//	M-F1 ^[[1;3P      M-F2 ^[[1;3Q        M-F3 ^[[1;3R        M-F4 ^[[1;3S
//	M-F5 ^[[15;3~     M-F6 ^[[17;3~       M-F7 ^[[18;3~       M-F8 ^[[19;3~
//	M-F9 ^[[20;3~     M-F10 ^[[21;3~      M-F11 ^[[23;3~      M-F12 ^[[24;3~
//
// The 3.6b-not-3.5a caveat is the mirror image of the one already
// recorded at internal/tmux/key.go's allowlist comment: ci/Dockerfile
// pins golang:1.25-trixie, whose apt tmux is 3.5a, and ci/Dockerfile is a
// protected path this change cannot edit, so the in-suite survey runs
// against whatever tmux the host provides (3.6b where this table was
// produced). tmux's `M-` prefix and the xterm modifier parameter 3
// (Alt), 4 (Shift+Alt), 7 (Ctrl+Alt), 8 (Ctrl+Shift+Alt) it maps onto are
// the same in both, but only the version CI actually runs is verified by
// CI itself.
var interactiveAltNamedKeys = map[string]string{
	"Up":    "M-Up",
	"Down":  "M-Down",
	"Left":  "M-Left",
	"Right": "M-Right",

	"Home": "M-Home",
	"End":  "M-End",

	"PageUp":   "M-PageUp",
	"PageDown": "M-PageDown",

	"Delete": "M-Delete",

	"C-Up": "C-M-Up", "C-Down": "C-M-Down", "C-Left": "C-M-Left", "C-Right": "C-M-Right",
	"C-Home": "C-M-Home", "C-End": "C-M-End", "C-PgUp": "C-M-PgUp", "C-PgDn": "C-M-PgDn",
	"S-Up": "S-M-Up", "S-Down": "S-M-Down", "S-Left": "S-M-Left", "S-Right": "S-M-Right",
	"S-Home": "S-M-Home", "S-End": "S-M-End",
	"C-S-Up": "C-M-S-Up", "C-S-Down": "C-M-S-Down", "C-S-Left": "C-M-S-Left", "C-S-Right": "C-M-S-Right",
	"C-S-Home": "C-M-S-Home", "C-S-End": "C-M-S-End",

	"F1": "M-F1", "F2": "M-F2", "F3": "M-F3", "F4": "M-F4",
	"F5": "M-F5", "F6": "M-F6", "F7": "M-F7", "F8": "M-F8",
	"F9": "M-F9", "F10": "M-F10", "F11": "M-F11", "F12": "M-F12",

	// The two listed gaps SPEC.md §11.9 asks to be named rather than
	// silently omitted ("a key deck cannot encode must be a known, listed
	// gap"). Both are spelled out here with an empty value, not left out
	// of the map, so the "every bare name has an explicit Alt verdict"
	// test can tell a deliberate gap from a forgotten entry:
	//
	//   - Insert: tmux DOES translate `M-Insert` correctly (surveyed:
	//     ^[[2;3~, xterm's own Alt+Insert), but deck can never RECEIVE the
	//     keystroke to forward. bubbletea v1.3.10's `sequences` table has
	//     no entry for "\x1b[2;3~" at all; it instead carries
	//     "\x1b[3;2~" -> {KeyInsert, Alt: true} (key.go:407), which is
	//     xterm's Shift+Delete -- evidently an upstream transposition of
	//     the two parameters. So a terminal emitting the standard
	//     Alt+Insert bytes produces an unknown-CSI message that is never a
	//     tea.KeyMsg and never reaches updateInteractive, and the one
	//     KeyMsg that DOES arrive as {KeyInsert, Alt: true} was physically
	//     Shift+Delete. Forwarding "M-Insert" for that would type
	//     Alt+Insert into the agent for a key the user did not press, so
	//     this stays a gap until upstream fixes the table; deck is not
	//     contorted around it.
	//   - BTab (Shift+Tab): tmux cannot express the Alt-modified form at
	//     all. `send-keys M-BTab` was surveyed on 3.6b and delivers
	//     ^[[Z -- byte for byte identical to plain `BTab`, with the Alt
	//     silently discarded by tmux itself (od -c of both captures:
	//     `^ [ [ Z`). Naming it would forward Shift+Tab while the user
	//     pressed Alt+Shift+Tab, which is worse than the listed gap: §11.9
	//     forbids forwarding a modified key with the modifier dropped just
	//     as much as it forbids dropping the keystroke.
	"Insert": "",
	"BTab":   "",
}

// interactiveBareNamedKey is interactiveNamedKey's UNMODIFIED-by-Alt half:
// the tmux key name for a KeyType on its own, including the Ctrl/Shift/
// Ctrl+Shift-modified navigation KeyTypes bubbletea delivers as their own
// distinct constants. interactiveNamedKey layers Alt on top of whatever
// this returns via interactiveAltNamedKeys above -- which is keyed by the
// names below, so adding a KeyType here without also giving it an Alt
// verdict there fails a test rather than quietly re-creating issue #28's
// silent drop for that one key.
func interactiveBareNamedKey(keyType tea.KeyType) (string, bool) {
	switch keyType {
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
// The `msg.Type >= 0` half of that range guard is load-bearing and is the
// reason this function is NOT a fallback for interactiveNamedKey: every
// special key's KeyType is NEGATIVE (charmbracelet/bubbletea@v1.3.10/
// key.go:205, `KeyRunes KeyType = -(iota + 1)`), so tea.KeyUp and friends
// fall to the default branch here by construction rather than by
// oversight -- an ESC prefix in front of a key whose bytes deck does not
// know is not an encoding of anything. Issue #28 was exactly what happens
// when interactiveNamedKey ALSO refuses them: both halves say no and the
// keystroke disappears. The Alt keys this function does own are the ones
// whose base byte value is fixed regardless of the pane's terminal mode:
// runes/Space (KeyRunes carries them in msg.Runes) and the positive
// C0/DEL types, i.e. Alt+Enter (13), Alt+Tab (9), Alt+Escape (27),
// Alt+Backspace (127) and Alt+Ctrl+<letter>.
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
