package features

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"time"

	"github.com/creack/pty"
	"github.com/cucumber/godog"
	"github.com/n-orlov/deck/internal/tmux"
)

// registerInteractiveRefusalsSteps wires features/interactive_refusals.feature
// (PRD Part II requirement 47/48): the setup steps each of the three named
// refusal cases needs, none of which any earlier feature file has a step
// for. The refusal itself, and the "press a to attach" offer, are asserted
// with the ordinary "screen contains" step already registered elsewhere.
func registerInteractiveRefusalsSteps(sc *godog.ScenarioContext) {
	sc.Step(`^a real tmux client attaches to deck session "([^"]+)" at (\d+)x(\d+)$`, aRealTmuxClientAttachesToDeckSessionAt)
	sc.Step(`^another live process holds ownership of deck session "([^"]+)"'s window$`, anotherLiveProcessHoldsOwnershipOfDeckSessionsWindow)
}

// aRealTmuxClientAttachesToDeckSessionAt resolves name's own tmux window
// target (its store-observed slug, through tmux.SessionName -- the same
// resolution enterInteractive itself does) and attaches a second, real tmux
// client to it through a real pty, storing it under h.rawAttachedTmuxClients
// keyed by the DECK session name so the existing "the real tmux client
// attached to session %q detaches" step (interactive_sigwinch_budget_test.go,
// registered for the same scenario package) can tear it down without this
// file needing a detach step of its own. #{session_attached} is polled
// after attaching, so the following enters-interactive-mode step never races
// tmux's own bookkeeping of the attach.
func aRealTmuxClientAttachesToDeckSessionAt(ctx context.Context, name string, cols, rows uint16) error {
	h, err := scenarioHarness(ctx)
	if err != nil {
		return err
	}
	slug, err := sessionSlugByName(h, name)
	if err != nil {
		return err
	}
	target, err := tmux.SessionName(slug)
	if err != nil {
		return fmt.Errorf("resolve tmux session name for deck session %q: %w", name, err)
	}
	// The same technique aRealTmuxClientAttachesToSessionAt
	// (interactive_sigwinch_budget_test.go) uses, reproduced rather than
	// called directly because that function stores the live client keyed
	// by ITS OWN "session" argument (a bare tmux target) -- here the key
	// needs to be the deck session's own display NAME instead, so the
	// pre-existing "the real tmux client attached to session %q detaches"
	// step (keyed by whatever name its own attach step was given) still
	// finds this client without this file needing a detach step of its
	// own.
	attachCtx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(attachCtx, "tmux", "-L", h.Socket, "attach-session", "-t", target)
	cmd.Env = append(os.Environ(), "TERM=xterm-256color")
	terminal, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: rows, Cols: cols})
	if err != nil {
		cancel()
		return fmt.Errorf("attach real tmux client to %q at %dx%d: %w", target, cols, rows, err)
	}
	go func() { _, _ = io.CopyBuffer(io.Discard, terminal, make([]byte, 4096)) }()
	if h.rawAttachedTmuxClients == nil {
		h.rawAttachedTmuxClients = make(map[string]*rawAttachedTmuxClient)
	}
	h.rawAttachedTmuxClients[name] = &rawAttachedTmuxClient{terminal: terminal, cmd: cmd, cancel: cancel}
	client := tmux.Client{Socket: h.Socket, Timeout: 5 * time.Second}
	return waitForFeatureSessionAttachedCount(ctx, client, target, 1)
}

// anotherLiveProcessHoldsOwnershipOfDeckSessionsWindow claims
// internal/tmux/ownership.go's OwnershipOption on name's own window with a
// `<tag>:<pid>` value naming THIS TEST BINARY's own pid -- alive for the
// rest of the scenario, and owned by the same user, so
// Client.ClaimWindowOwnership's kill(pid, 0) liveness probe answers "alive"
// exactly the way a second real deck process's claim would. This is a raw
// tmux invocation rather than a call into internal/tmux (whose claim-tag/
// pid-format helpers are unexported), which keeps this scenario's own claim
// from going through the exact code path enterInteractive is being tested
// against.
func anotherLiveProcessHoldsOwnershipOfDeckSessionsWindow(ctx context.Context, name string) error {
	h, err := scenarioHarness(ctx)
	if err != nil {
		return err
	}
	slug, err := sessionSlugByName(h, name)
	if err != nil {
		return err
	}
	target, err := tmux.SessionName(slug)
	if err != nil {
		return fmt.Errorf("resolve tmux session name for deck session %q: %w", name, err)
	}
	claim := fmt.Sprintf("other-owner-claim:%d", os.Getpid())
	cmd := exec.CommandContext(ctx, "tmux", "-L", h.Socket, "set-option", "-w", "-t", target, "@deck_isize_owner", claim)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("tmux -L %s set-option -w -t %s @deck_isize_owner %s: %w: %s", h.Socket, target, claim, err, output)
	}
	return nil
}
