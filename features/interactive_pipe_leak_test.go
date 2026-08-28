package features

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cucumber/godog"
	"github.com/n-orlov/deck/internal/tmux"
)

// registerInteractivePipeLeakSteps wires features/interactive_pipe_leak.feature
// (PRD R89/task 030): the "next start reclaims a leaked interactive pipe"
// guarantee, proven end to end against a real deck binary and a real tmux
// server -- SIGKILL the client while interactive, observe the leak
// (pipe-pane still armed, temp dir/FIFO still on disk -- both are this
// scan's own positive control, the same "prove the mechanism finds a real,
// present thing before trusting its negative" discipline
// features/no_leak_test.go's scan uses), then start a fresh client against
// the same DECK_HOME/socket and observe both gone.
func registerInteractivePipeLeakSteps(sc *godog.ScenarioContext) {
	sc.Step(`^tmux pane pipe is armed for session "([^"]+)"$`, tmuxPanePipeIsArmedForSession)
	sc.Step(`^tmux pane pipe is not armed for session "([^"]+)"$`, tmuxPanePipeIsNotArmedForSession)
	sc.Step(`^a deck interactive pipe temp directory for session "([^"]+)" exists on disk$`, aDeckInteractivePipeTempDirectoryForSessionExistsOnDisk)
	sc.Step(`^no deck interactive pipe temp directory for session "([^"]+)" exists on disk$`, noDeckInteractivePipeTempDirectoryForSessionExistsOnDisk)
	sc.Step(`^tmux window "([^"]+)" option "([^"]+)" is set in the window scope$`, tmuxWindowOptionIsSetInWindowScope)
}

// deckSessionWindowTarget resolves name's own tmux window target the same
// way enterInteractive itself does (store slug -> tmux.SessionName), so
// every step in this file addresses exactly the window deck itself claimed
// and fitted.
func deckSessionWindowTarget(h *ScenarioHarness, name string) (string, error) {
	slug, err := sessionSlugByName(h, name)
	if err != nil {
		return "", err
	}
	target, err := tmux.SessionName(slug)
	if err != nil {
		return "", fmt.Errorf("resolve tmux session name for deck session %q: %w", name, err)
	}
	return target, nil
}

func tmuxPanePipeIsArmedForSession(ctx context.Context, name string) error {
	h, err := scenarioHarness(ctx)
	if err != nil {
		return err
	}
	target, err := deckSessionWindowTarget(h, name)
	if err != nil {
		return err
	}
	client := tmux.Client{Socket: h.Socket, Timeout: 5 * time.Second}
	armed, err := client.PanePipe(ctx, target)
	if err != nil {
		return fmt.Errorf("read #{pane_pipe} for %q: %w", target, err)
	}
	if !armed {
		return fmt.Errorf("#{pane_pipe} reads 0 for %q, want 1 (armed)", target)
	}
	return nil
}

func tmuxPanePipeIsNotArmedForSession(ctx context.Context, name string) error {
	h, err := scenarioHarness(ctx)
	if err != nil {
		return err
	}
	target, err := deckSessionWindowTarget(h, name)
	if err != nil {
		return err
	}
	client := tmux.Client{Socket: h.Socket, Timeout: 5 * time.Second}
	armed, err := client.PanePipe(ctx, target)
	if err != nil {
		return fmt.Errorf("read #{pane_pipe} for %q: %w", target, err)
	}
	if armed {
		return fmt.Errorf("#{pane_pipe} still reads 1 for %q, want 0 (disarmed)", target)
	}
	return nil
}

// findDeckInteractivePipeTempDir scans the real OS temp directory (exactly
// where ArmPipePane's own os.MkdirTemp call -- production code never
// overrides tmux.interactivePipeTempRoot -- and ReclaimLeakedInteractivePipes'
// own scan both look) for a deck-interactive-pipe-* dir whose own persisted
// tmux.InteractiveClaimRecord names windowTarget, rather than assuming any
// one particular dir is "the" leaked one -- other scenarios in the same
// suite process may have their own, unrelated, live interactive sessions
// with their own temp dirs present at the same time.
func findDeckInteractivePipeTempDir(windowTarget string) (string, error) {
	root := os.TempDir()
	entries, err := os.ReadDir(root)
	if err != nil {
		return "", fmt.Errorf("scan %q for a deck interactive pipe temp dir: %w", root, err)
	}
	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), "deck-interactive-pipe-") {
			continue
		}
		dir := filepath.Join(root, entry.Name())
		data, err := os.ReadFile(filepath.Join(dir, "claim.json"))
		if err != nil {
			continue
		}
		var record tmux.InteractiveClaimRecord
		if err := json.Unmarshal(data, &record); err != nil {
			continue
		}
		if record.WindowTarget == windowTarget {
			return dir, nil
		}
	}
	return "", nil
}

func aDeckInteractivePipeTempDirectoryForSessionExistsOnDisk(ctx context.Context, name string) error {
	h, err := scenarioHarness(ctx)
	if err != nil {
		return err
	}
	target, err := deckSessionWindowTarget(h, name)
	if err != nil {
		return err
	}
	dir, err := findDeckInteractivePipeTempDir(target)
	if err != nil {
		return err
	}
	if dir == "" {
		return fmt.Errorf("no deck interactive pipe temp dir found on disk naming window target %q, want one to exist", target)
	}
	return nil
}

func noDeckInteractivePipeTempDirectoryForSessionExistsOnDisk(ctx context.Context, name string) error {
	h, err := scenarioHarness(ctx)
	if err != nil {
		return err
	}
	target, err := deckSessionWindowTarget(h, name)
	if err != nil {
		return err
	}
	dir, err := findDeckInteractivePipeTempDir(target)
	if err != nil {
		return err
	}
	if dir != "" {
		return fmt.Errorf("deck interactive pipe temp dir %q still on disk naming window target %q, want none", dir, target)
	}
	return nil
}

// tmuxWindowOptionIsSetInWindowScope is the "is set" complement
// tmux_option_scope_test.go's own registerTmuxOptionScopeSteps never
// needed until now (every existing scenario only ever asserted a KNOWN
// value or "unset") -- this scenario's own ownership claim carries a
// random tag it cannot predict up front, so it only needs to know the
// window-scoped option is PRESENT at all, not which value it holds.
func tmuxWindowOptionIsSetInWindowScope(ctx context.Context, target, name string) error {
	h, err := scenarioHarness(ctx)
	if err != nil {
		return err
	}
	got, err := readTmuxOptionInScope(ctx, h.Socket, target, tmuxOptionScopeWindow, name)
	if err != nil {
		return err
	}
	if !got.Set {
		return fmt.Errorf("tmux option %q in window scope of %q is unset, want set", name, target)
	}
	return nil
}
