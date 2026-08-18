// Command deck starts the deck terminal user interface.
package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/n-orlov/deck/internal/agent"
	"github.com/n-orlov/deck/internal/audit"
	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/service"
	"github.com/n-orlov/deck/internal/store"
	"github.com/n-orlov/deck/internal/tmux"
	"github.com/n-orlov/deck/internal/tui"
)

func main() {
	settings, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "deck configuration:", err)
		return
	}
	db, err := store.Open(settings.Paths)
	if err != nil {
		fmt.Fprintln(os.Stderr, "deck state:", err)
		return
	}
	defer db.Close()

	logger, err := audit.New(settings.Paths, settings.Clock)
	if err != nil {
		fmt.Fprintln(os.Stderr, "deck audit:", err)
		return
	}
	client := tmux.Client{Socket: settings.Socket}
	registry := agent.NewRegistry()
	registry.Register(agent.NewShell())
	registry.Register(agent.NewClaude())
	registry.Register(agent.NewPi())
	sessions := service.Service{
		Store: db, TMux: client, Audit: logger,
		Clock: settings.Clock, IDs: settings.IDs, Agents: registry,
	}
	// The TUI invokes this liveness pass immediately before each configured
	// list refresh, so a real client observes externally removed tmux sessions
	// and servers without ever bootstrapping a replacement server.
	model := tui.NewWithShellCreatorAttacherKillerResumerProfileSwitcherResumeModerAndAgentCreator(db, settings, tui.TmuxHealth(settings), sessions.CreateShell, client.AttachCommand, sessions.Kill, sessions.Reconcile, sessions.Resume, sessions.SetPermissionProfile, sessions.ResumeMode, sessions.CreateAgent)
	if _, err := tea.NewProgram(model, tea.WithAltScreen()).Run(); err != nil {
		fmt.Fprintln(os.Stderr, "deck:", err)
	}
}
