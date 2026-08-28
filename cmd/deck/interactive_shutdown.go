// Command deck: last-chance interactive cleanup for the two exit routes
// exitInteractive (internal/tui/interactive.go, Ctrl+Q's own teardown)
// never runs on: SIGTERM and a panic (PRD R89/task 031).
//
// Bubble Tea's own signal handler turns SIGTERM into a QuitMsg that
// Program.Run's eventLoop returns straight through without ever calling
// Update again -- so a SIGTERM mid-interactive leaves the pipe-pane armed,
// the FIFO/temp dir on disk and window ownership/geometry claimed exactly
// like a SIGKILL would, unless something outside Update notices and tears
// it down. Unlike SIGKILL (task 030's ReclaimLeakedInteractivePipes, which
// can only run on a LATER process's start, because SIGKILL cannot be
// handled at all), SIGTERM and a panic both still run Go code in this same
// process before it exits, so the guarantee here is "the exit cleans it",
// not "the next start reclaims it".
//
// This file is unconditional, always-compiled production code (no build
// tag): every build, including a release, gets both routes.
package main

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"
)

// interactiveShutdownGuard implements tea.Model, wrapping whatever it is
// given (main.go applies it OUTERMOST, after every other wrap -- see
// main.go's own tea.NewProgram call) so its own recover sees a panic
// thrown by anything else deck composes onto the real tui.Model too, not
// only a panic from inside the real Update method itself. testpanic_hook.go
// (decktestpanic-tagged builds only) panics BEFORE ever delegating to the
// model it wraps, precisely the case that would slip past a recover placed
// any deeper than this.
type interactiveShutdownGuard struct {
	inner tea.Model
}

// wrapForInteractiveShutdownOnPanic is always active; there is no no-op
// variant and no build tag, unlike wrapForDeliberateTestPanic/
// wrapForInputCounting -- R89's panic exit route is a real product
// guarantee, not scaffolding for a test proof.
func wrapForInteractiveShutdownOnPanic(model tea.Model) tea.Model {
	return interactiveShutdownGuard{inner: model}
}

func (g interactiveShutdownGuard) Init() tea.Cmd { return g.inner.Init() }
func (g interactiveShutdownGuard) View() string  { return g.inner.View() }

// Unwrap lets shutdownArmedInteractiveClaim see through this wrapper too:
// main.go's own post-Run() SIGTERM check calls shutdownArmedInteractiveClaim
// directly on the model tea.Program.Run() hands back, which is always at
// least this guard itself (it is the outermost wrap -- see main.go).
func (g interactiveShutdownGuard) Unwrap() tea.Model { return g.inner }

func (g interactiveShutdownGuard) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	defer func() {
		if r := recover(); r != nil {
			// g.inner is exactly the model this Update call started with --
			// whatever the panic interrupted, nothing wrote back through
			// g's own return values, so this is still the freshest state
			// deck has for cleanup purposes.
			shutdownArmedInteractiveClaim(g.inner)
			panic(r)
		}
	}()
	next, cmd := g.inner.Update(msg)
	return interactiveShutdownGuard{inner: next}, cmd
}

// interactiveShutdowner is implemented by internal/tui.Model itself
// (ShutdownInteractive). Declared locally, not imported, so this file
// never needs to know the concrete tui.Model type -- only that whatever it
// eventually unwraps to might have this one method.
type interactiveShutdowner interface {
	ShutdownInteractive(ctx context.Context)
}

// modelUnwrapper is implemented by every wrapper this package composes
// onto the real tui.Model that is NOT itself an interactiveShutdowner
// (testpanic_hook.go's panicOnKeyModel, inputcount_hook.go's
// inputCountingModel) so shutdownArmedInteractiveClaim can see through any
// of them, in any order, without either wrapper needing to know anything
// about interactive mode.
type modelUnwrapper interface {
	Unwrap() tea.Model
}

// shutdownArmedInteractiveClaim walks model, and whatever it unwraps to,
// looking for the one thing that actually knows how to tear an armed
// interactive claim down (disarm the pipe-pane, restore geometry
// byte-exact, release ownership) -- called both from the panic recover
// above and from main.go's own post-Run() SIGTERM check, so a claim
// interrupted by either route is torn down exactly once, the same way,
// from exactly one place. A model with nothing left to unwrap and no
// ShutdownInteractive method (a nil interface value, most notably) is
// silently left alone: there is nothing safe to do with it.
func shutdownArmedInteractiveClaim(model tea.Model) {
	for model != nil {
		if shutdown, ok := model.(interactiveShutdowner); ok {
			shutdown.ShutdownInteractive(context.Background())
			return
		}
		unwrapper, ok := model.(modelUnwrapper)
		if !ok {
			return
		}
		model = unwrapper.Unwrap()
	}
}
