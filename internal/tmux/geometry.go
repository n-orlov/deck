package tmux

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

// WindowGeometry is what entering interactive mode reads before it changes
// anything (PRD phase3b II-7 / SPEC.md §11.9), so exit (task 035, II-9) has
// exactly what it needs to restore byte-exact. Width/Height are the
// window's own current #{window_width}x#{window_height}. WindowSizeSet and
// WindowSizeValue are the WINDOW-LOCAL `window-size` option exactly as
// `show-options -wv` reports it -- never the merged/effective value (task
// 028's scope distinction matters here specifically: what gets restored on
// exit is whatever the window-local table held before entry, which is
// "unset" on almost every window since deck's own Bootstrap only ever
// writes `window-size latest` at the server-global scope, not the window
// one).
//
// `window-size` is a BUILTIN option, not a "@"-prefixed user option like
// ownership.go's OwnershipOption: tmux's two different "unset" shapes
// (task 028) mean a builtin window option that has never been set
// window-locally prints an empty line and exits 0, rather than the
// "invalid option" error a user option produces when unset. WindowSizeSet
// distinguishes "read, and it happens to be empty" (impossible for this
// option, but the type does not assume that) from "never set", the same
// way ownership.go's own windowOwnershipState does for its own option.
type WindowGeometry struct {
	Width           int
	Height          int
	WindowSizeSet   bool
	WindowSizeValue string
}

// readBuiltinWindowOption reads a BUILTIN (non-"@"-prefixed) option in the
// window scope via `show-options -wv`. An unset builtin window option
// prints an empty line and exits 0 -- the shape task 028's
// tmuxOptionScope_test.go step already distinguishes from a "@"-prefixed
// user option's "invalid option" error (ownership.go's
// readWindowOwnership); this is that same read, kept separate from
// readWindowOwnership because the two option kinds fail differently when
// unset and a helper that conflated them would silently misread whichever
// kind it was never tested against.
func (c Client) readBuiltinWindowOption(ctx context.Context, target, name string) (value string, set bool, err error) {
	commandCtx, cancel := context.WithTimeout(ctx, c.timeout())
	defer cancel()
	output, err := c.command(commandCtx, "show-options", "-wv", "-t", target, name).CombinedOutput()
	trimmed := strings.TrimRight(string(output), "\n")
	if err != nil {
		return "", false, fmt.Errorf("tmux -L %s show-options -wv -t %s %s: %w: %s", c.Socket, target, name, err, trimmed)
	}
	if trimmed == "" {
		return "", false, nil
	}
	return trimmed, true, nil
}

// ServerEnvironment reads one variable from the tmux SERVER's own global
// environment table via `show-environment -g` -- the environment the
// server process itself inherited when it started (and any later
// `set-environment -g`), never this calling process's own ambient
// environment. That distinction is exactly R121's fix: a later TUI
// observing an already-running server can have a different ambient value
// for the same key than the server does, and only this query answers
// what the server (and therefore any pane it hosts) actually has. ok is
// false whenever the key is not present in the server's global table
// (tmux exits non-zero with "unknown variable: KEY", the same error for
// a key that was never set and one explicitly cleared with `-u`) or the
// query itself cannot be run at all (no server on this socket, tmux
// missing) -- callers that only care whether a value exists treat both
// identically, the same way this package's other option reads decline
// rather than error on "not set".
func (c Client) ServerEnvironment(ctx context.Context, key string) (value string, ok bool, err error) {
	commandCtx, cancel := context.WithTimeout(ctx, c.timeout())
	defer cancel()
	output, runErr := c.command(commandCtx, "show-environment", "-g", key).CombinedOutput()
	if runErr != nil {
		return "", false, nil
	}
	trimmed := strings.TrimRight(string(output), "\n")
	name, value, found := strings.Cut(trimmed, "=")
	if !found || name != key {
		// Either the marked-unset "-KEY" shape or something unrecognized;
		// either way there is no value to report.
		return "", false, nil
	}
	return value, true, nil
}

// windowAndPaneSize reads #{window_width}, #{window_height},
// #{pane_width} and #{pane_height} for paneTarget in a single
// display-message invocation. window_width/height are window-scoped
// formats but resolve identically from a pane target, since every pane
// belongs to exactly one window -- the same target therefore serves both
// the window-level and pane-level halves of one measurement without a
// second round trip that tmux could process pane output in between (the
// atomicity concern task 043/II-20 addresses for the transport does not
// apply to a pure geometry read, but reading both in one invocation is
// still one fewer place for the two numbers to be measured a moment
// apart).
func (c Client) windowAndPaneSize(ctx context.Context, paneTarget string) (windowWidth, windowHeight, paneWidth, paneHeight int, err error) {
	commandCtx, cancel := context.WithTimeout(ctx, c.timeout())
	defer cancel()
	output, err := c.command(commandCtx, "display-message", "-p", "-t", paneTarget,
		"#{window_width} #{window_height} #{pane_width} #{pane_height}").CombinedOutput()
	if err != nil {
		return 0, 0, 0, 0, fmt.Errorf("tmux -L %s display-message -p -t %s window/pane size: %w: %s", c.Socket, paneTarget, err, strings.TrimSpace(string(output)))
	}
	fields := strings.Fields(string(output))
	if len(fields) != 4 {
		return 0, 0, 0, 0, fmt.Errorf("tmux -L %s display-message -p -t %s window/pane size: unexpected output %q", c.Socket, paneTarget, string(output))
	}
	values := make([]int, 4)
	for index, field := range fields {
		parsed, convErr := strconv.Atoi(field)
		if convErr != nil {
			return 0, 0, 0, 0, fmt.Errorf("tmux -L %s display-message -p -t %s window/pane size: parse %q: %w", c.Socket, paneTarget, field, convErr)
		}
		values[index] = parsed
	}
	return values[0], values[1], values[2], values[3], nil
}

// CaptureWindowGeometry implements PRD phase3b II-7: read
// #{window_width}x#{window_height} and the window-local `window-size`
// value BEFORE entering interactive mode does anything else to target's
// window. Callers pass a pane or window target; tmux resolves either to
// the owning window for every format and command this file uses.
func (c Client) CaptureWindowGeometry(ctx context.Context, target string) (WindowGeometry, error) {
	windowWidth, windowHeight, _, _, err := c.windowAndPaneSize(ctx, target)
	if err != nil {
		return WindowGeometry{}, err
	}
	value, set, err := c.readBuiltinWindowOption(ctx, target, "window-size")
	if err != nil {
		return WindowGeometry{}, err
	}
	return WindowGeometry{Width: windowWidth, Height: windowHeight, WindowSizeSet: set, WindowSizeValue: value}, nil
}

// SessionAttachedCount reads `#{session_attached}` for the session owning
// target: the number of clients currently attached to it. PRD phase3b II-9
// gates exit's restore on this being exactly zero -- resizing a window a
// live client is still looking at fights that client's own next
// resize/redraw, where unsetting `window-size` alone (below) lets tmux's
// own `window-size latest` machinery follow that client immediately, with
// no explicit resize-window call and therefore no extra SIGWINCH.
func (c Client) SessionAttachedCount(ctx context.Context, target string) (int, error) {
	commandCtx, cancel := context.WithTimeout(ctx, c.timeout())
	defer cancel()
	output, err := c.command(commandCtx, "display-message", "-p", "-t", target, "#{session_attached}").CombinedOutput()
	if err != nil {
		return 0, fmt.Errorf("tmux -L %s display-message -p -t %s session_attached: %w: %s", c.Socket, target, err, strings.TrimSpace(string(output)))
	}
	trimmed := strings.TrimSpace(string(output))
	count, convErr := strconv.Atoi(trimmed)
	if convErr != nil {
		return 0, fmt.Errorf("tmux -L %s display-message -p -t %s session_attached: parse %q: %w", c.Socket, target, trimmed, convErr)
	}
	return count, nil
}

// PaneDead reads `#{pane_dead}` for target: whether tmux has observed the
// pane's own process exit. This is the ONLY liveness signal the live path
// (PRD phase3b II-23) may rely on -- unlike passive preview's capture-pane
// snapshot, `pipe-pane`'s own stream gives no liveness signal of its own:
// under `remain-on-exit failed` (deck's own server-wide default, set by
// Bootstrap above) a dead pane's pipe never closes, because tmux keeps
// pushing (nothing) into the still-open pty for as long as the pane
// object exists, which under remain-on-exit=failed is forever. Polling
// this format is therefore the only way the live path notices a crashed
// target at all; EOF on the pipe is not a substitute (task 046/II-24
// covers the one case EOF ever does carry a real signal: displacement by
// a second pipe-pane holder, which this format cannot distinguish from
// death by itself).
func (c Client) PaneDead(ctx context.Context, target string) (bool, error) {
	commandCtx, cancel := context.WithTimeout(ctx, c.timeout())
	defer cancel()
	output, err := c.command(commandCtx, "display-message", "-p", "-t", target, "#{pane_dead}").CombinedOutput()
	if err != nil {
		return false, fmt.Errorf("tmux -L %s display-message -p -t %s pane_dead: %w: %s", c.Socket, target, err, strings.TrimSpace(string(output)))
	}
	trimmed := strings.TrimSpace(string(output))
	switch trimmed {
	case "1":
		return true, nil
	case "0":
		return false, nil
	default:
		return false, fmt.Errorf("tmux -L %s display-message -p -t %s pane_dead: unexpected output %q", c.Socket, target, trimmed)
	}
}

// PanePipe reads `#{pane_pipe}` for target: whether ANY `pipe-pane` is
// currently armed on it, deck's own or someone else's. `pipe-pane` is
// single-holder per pane (PRD phase3b II-24): a second `pipe-pane -IO`
// arm on the same target silently displaces deck's own, in ~4 ms, with
// no error to either side and an identical clean rc=0 EOF delivered to
// the displaced reader -- indistinguishable from a deliberate disable by
// EOF alone. Reading this format immediately after observing that EOF is
// what tells the two apart: still 1 means something else has since armed
// its own pipe-pane and deck's reader was simply cut off from it
// (displacement); 0 means nothing is piping this pane at all right now
// (the pipe was disabled, by deck's own release-on-exit path (task
// 047/II-25) or otherwise).
func (c Client) PanePipe(ctx context.Context, target string) (bool, error) {
	commandCtx, cancel := context.WithTimeout(ctx, c.timeout())
	defer cancel()
	output, err := c.command(commandCtx, "display-message", "-p", "-t", target, "#{pane_pipe}").CombinedOutput()
	if err != nil {
		return false, fmt.Errorf("tmux -L %s display-message -p -t %s pane_pipe: %w: %s", c.Socket, target, err, strings.TrimSpace(string(output)))
	}
	trimmed := strings.TrimSpace(string(output))
	switch trimmed {
	case "1":
		return true, nil
	case "0":
		return false, nil
	default:
		return false, fmt.Errorf("tmux -L %s display-message -p -t %s pane_pipe: unexpected output %q", c.Socket, target, trimmed)
	}
}

// unsetWindowSize issues `set-option -w -u window-size`, removing whatever
// value `resize-window` left in the WINDOW scope (task 034's
// FitWindowToPane always leaves it "manual" there as a tmux side effect of
// calling resize-window at all) so the window falls back to the
// server-global value -- "latest" per Client.Bootstrap -- and, with a
// client attached, is immediately resized to follow it without deck ever
// issuing a second resize-window call.
func (c Client) unsetWindowSize(ctx context.Context, target string) error {
	if _, err := c.run(ctx, "set-option", "-w", "-u", "-t", target, "window-size"); err != nil {
		return fmt.Errorf("unset window-size on %q: %w", target, err)
	}
	return nil
}

// RestoreWindowGeometry implements PRD phase3b II-9's exit recipe, in the
// order the PRD states is load-bearing:
//
//  1. `resize-window` back to geometry's saved dimensions, but ONLY when
//     #{session_attached} == 0. With a client attached, an explicit resize
//     back to the pre-entry size is pointless (the very next step's unset
//     immediately hands control back to that client's own size via
//     tmux's `window-size latest`) and costs a real, wasted SIGWINCH the
//     PRD counts and rejects.
//  2. `set-option -w -u window-size`, unconditionally, LAST.
//
// Reversed, `unset` (step 2) then `resize-window` (step 1) would leave
// window-size back at "manual" -- resize-window always writes that as a
// side effect -- undoing the unset and pinning the window at whatever
// resize-window just set, ignoring every future attach. geometry_test.go's
// TestReversedRestoreOrderLeavesWindowPinned demonstrates exactly that,
// red, by calling the two steps in the wrong order directly; this function
// is the only place either order should ever be issued from shipped code.
//
// This does not attempt to restore geometry.WindowSizeValue if it was
// somehow already set window-locally before entry (WindowSizeSet==true) --
// deck's own Bootstrap never writes window-size window-locally (task
// 034's CaptureWindowGeometry doc comment), so that state is not expected
// to arise from deck's own entry path, and PRD II-9 names a plain unset,
// not a value-preserving restore, as the recipe.
func (c Client) RestoreWindowGeometry(ctx context.Context, target string, geometry WindowGeometry) error {
	attached, err := c.SessionAttachedCount(ctx, target)
	if err != nil {
		return err
	}
	if attached == 0 {
		if err := c.resizeWindow(ctx, target, geometry.Width, geometry.Height); err != nil {
			return err
		}
	}
	return c.unsetWindowSize(ctx, target)
}

// resizeWindow issues `resize-window`, the ONLY resize command this file
// ever calls (PRD II-8: "resize the window, never the pane" -- grepping
// this file (outside _test.go) for tmux's per-pane resizing subcommand
// name finds nothing; geometry_test.go's own self-check enforces this by
// reading this file's source rather than trusting a one-off manual grep).
func (c Client) resizeWindow(ctx context.Context, target string, width, height int) error {
	if _, err := c.run(ctx, "resize-window", "-t", target, "-x", strconv.Itoa(width), "-y", strconv.Itoa(height)); err != nil {
		return fmt.Errorf("resize window %q to %dx%d: %w", target, width, height, err)
	}
	return nil
}

// maxFitWindowAttempts bounds FitWindowToPane's chrome-compensated resize
// loop. It is not a retry-until-it-works loop against a moving target --
// on every split-window layout measured here (a two- and a three-pane
// vertical stack, both on a 42-row window, landing a 22-row main pane) the
// loop converges in 3-4 resize-window calls (measured, see
// docs/reports/phase3b.md; the PRD's own spike cites 5-6 for a layout this
// repository cannot re-run, since the raw spike evidence is unavailable
// here -- see tasks.json's discovered prdCorrections). The bound exists so
// a layout this package has not seen fails loudly with an error instead of
// looping forever, which is the treatment PRD II-8 reserves for the naive
// pane-targeting alternative this function deliberately is not (that one
// is demonstrated, not shipped, in geometry_test.go).
const maxFitWindowAttempts = 10

// FitWindowToPane implements PRD phase3b II-8: resize target's WINDOW --
// never the pane -- until paneTarget's own #{pane_width}x#{pane_height}
// equal wantWidth/wantHeight. target and paneTarget are usually the same
// tmux target (a pane inside a single-pane window resolves to its own
// window either way); they are separate parameters only so a caller
// fitting one particular pane of a multi-pane window can still name the
// window explicitly if it ever needs to.
//
// On a single-pane window, #{pane_height} == #{window_height} and the
// first resize-window call lands it exactly (chrome is zero). On a SPLIT
// window, tmux's layout engine redistributes space between sibling panes
// PROPORTIONALLY on every resize, so the "chrome" a sibling pane and its
// border consume is a function of the window's own current size, not a
// fixed offset measured once: computing chrome := window - pane at the OLD
// size and adding it to the wanted pane size gives the window size that
// would have produced the wanted pane size at the OLD proportions, which is
// close to but not exactly right once the window has actually changed size
// -- hence the loop, re-measuring chrome fresh every iteration rather than
// trusting the first reading, until the pane's own reported size matches
// exactly or the bound is exhausted.
//
// The return value is the number of resize-window calls actually issued
// (0 if the pane already matched on entry).
func (c Client) FitWindowToPane(ctx context.Context, target, paneTarget string, wantWidth, wantHeight int) (int, error) {
	resizes := 0
	for attempt := 0; attempt < maxFitWindowAttempts; attempt++ {
		windowWidth, windowHeight, paneWidth, paneHeight, err := c.windowAndPaneSize(ctx, paneTarget)
		if err != nil {
			return resizes, err
		}
		if paneWidth == wantWidth && paneHeight == wantHeight {
			return resizes, nil
		}
		newWidth := wantWidth + (windowWidth - paneWidth)
		newHeight := wantHeight + (windowHeight - paneHeight)
		if newWidth < 1 {
			newWidth = 1
		}
		if newHeight < 1 {
			newHeight = 1
		}
		if err := c.resizeWindow(ctx, target, newWidth, newHeight); err != nil {
			return resizes, err
		}
		resizes++
	}
	return resizes, fmt.Errorf("fit window %q to pane %q at %dx%d: did not converge in %d resizes", target, paneTarget, wantWidth, wantHeight, maxFitWindowAttempts)
}
