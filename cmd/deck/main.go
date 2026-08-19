// Command deck starts the deck terminal user interface.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/n-orlov/deck/internal/agent"
	"github.com/n-orlov/deck/internal/audit"
	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/hookrecv"
	"github.com/n-orlov/deck/internal/service"
	"github.com/n-orlov/deck/internal/store"
	"github.com/n-orlov/deck/internal/tmux"
	"github.com/n-orlov/deck/internal/tui"
)

func main() { os.Exit(run(os.Args, os.Stdin, os.Stderr)) }

func run(args []string, stdin io.Reader, stderr io.Writer) int {
	isHook := len(args) == 2 && args[1] == "_hook"
	settings, err := config.Load()
	if err != nil {
		fmt.Fprintln(stderr, "deck configuration:", err)
		if isHook {
			return 1
		}
		return 0
	}
	if isHook {
		if err := runHook(context.Background(), settings, stdin); err != nil {
			fmt.Fprintln(stderr, "deck hook:", err)
			return 1
		}
		return 0
	}

	db, err := store.Open(settings.Paths)
	if err != nil {
		fmt.Fprintln(stderr, "deck state:", err)
		return 0
	}
	defer db.Close()

	logger, err := audit.New(settings.Paths, settings.Clock)
	if err != nil {
		fmt.Fprintln(stderr, "deck audit:", err)
		return 0
	}
	executable, err := os.Executable()
	if err != nil {
		fmt.Fprintln(stderr, "deck executable:", err)
		return 0
	}
	executable, err = filepath.Abs(executable)
	if err != nil {
		fmt.Fprintln(stderr, "deck executable:", err)
		return 0
	}
	client := tmux.Client{Socket: settings.Socket}
	registry := agent.NewRegistry()
	registry.Register(agent.NewShell())
	registry.Register(agent.NewClaude())
	registry.Register(agent.NewPi())
	sessions := service.Service{
		Store: db, TMux: client, Audit: logger,
		Clock: settings.Clock, IDs: settings.IDs, Agents: registry,
		ConfigEnv: settings.Env, DeckExecutable: executable, DeckHome: settings.Paths.Home,
	}
	// The TUI invokes this liveness pass immediately before each configured
	// list refresh, so a real client observes externally removed tmux sessions
	// and servers without ever bootstrapping a replacement server.
	model := tui.NewWithShellCreatorAttacherKillerResumerProfileSwitcherResumeModerAgentCreatorAndRegistry(db, settings, tui.TmuxHealth(settings), sessions.CreateShell, client.AttachCommand, sessions.Kill, sessions.Reconcile, sessions.Resume, sessions.SetPermissionProfile, sessions.ResumeMode, sessions.CreateAgent, registry)
	if _, err := tea.NewProgram(model, tea.WithAltScreen()).Run(); err != nil {
		fmt.Fprintln(stderr, "deck:", err)
		return 0
	}
	return 0
}

// runHook is intentionally selected before opening the normal application
// store or constructing a tmux client. A late hook must not recreate deleted
// state or bootstrap a tmux server.
func runHook(ctx context.Context, settings config.Settings, stdin io.Reader) error {
	var raw json.RawMessage
	decoder := json.NewDecoder(stdin)
	if err := decoder.Decode(&raw); err != nil {
		return fmt.Errorf("read one JSON object: %w", err)
	}
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return errors.New("stdin must contain one JSON object")
	}
	var extra json.RawMessage
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return errors.New("stdin contains more than one JSON value")
		}
		return fmt.Errorf("reject trailing stdin: %w", err)
	}

	if info, err := os.Stat(settings.Paths.StateDB); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("state database does not exist: %s", settings.Paths.StateDB)
		}
		return fmt.Errorf("inspect state database: %w", err)
	} else if !info.Mode().IsRegular() {
		return fmt.Errorf("state database is not a regular file: %s", settings.Paths.StateDB)
	}
	db, err := store.Open(settings.Paths)
	if err != nil {
		return fmt.Errorf("open existing state database: %w", err)
	}
	defer db.Close()
	logger, err := audit.New(settings.Paths, settings.Clock)
	if err != nil {
		return fmt.Errorf("open audit log: %w", err)
	}
	timed := timedHookStore{Store: db, Audit: logger}
	result, err := hookrecv.Receive(ctx, timed, trimmed, os.Getenv("DECK_SESSION_ID"), settings.Clock.Now().UnixMilli())
	if err != nil {
		return err
	}
	// SessionEnd shares the agent's tight shutdown budget and is deliberately
	// one store write and exit. Every other hook pays for one bounded liveness
	// pass after (and therefore outside) the measured store callback. This is
	// the unattended path that notices and collects a different crashed pane;
	// ReconcileWithin is liveness-only and never runs pane-text probes.
	if result.Kind == "session_end" {
		return nil
	}
	liveness := service.Service{
		Store: db,
		TMux:  tmux.Client{Socket: settings.Socket},
		Audit: logger,
		Clock: settings.Clock,
	}
	if err := liveness.ReconcileWithin(ctx, settings.Reconcile); err != nil {
		return fmt.Errorf("post-hook liveness pass: %w", err)
	}
	return nil
}

// timedHookStore leaves resolution reads outside the measured span and wraps
// exactly the receiver's single durable mutation (resolved or orphan).
type timedHookStore struct {
	*store.Store
	Audit *audit.Logger
}

func (s timedHookStore) UpdateSessionStatus(ctx context.Context, input store.StatusUpdateInput) error {
	return s.Audit.HookStoreWrite(input.SessionID, func() error {
		return s.Store.UpdateSessionStatus(ctx, input)
	})
}

func (s timedHookStore) RecordOrphanEvent(ctx context.Context, input store.EventInput) error {
	return s.Audit.HookStoreWrite("", func() error {
		return s.Store.RecordOrphanEvent(ctx, input)
	})
}
