// Package service coordinates durable deck state with tmux operations.
package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/n-orlov/deck/internal/agent"
	"github.com/n-orlov/deck/internal/audit"
	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
	"github.com/n-orlov/deck/internal/tmux"
)

// ShellCreateInput contains the user supplied fields for a plain shell session.
type ShellCreateInput struct {
	Name string
	CWD  string
	Env  map[string]string
}

// Service performs operations which must keep the SQLite store and private
// tmux server coherent. All dependencies are explicit so callers can give each
// client its own store connection and audit writer.
type Service struct {
	Store *store.Store
	TMux  tmux.Client
	Audit *audit.Logger
	Clock *config.Clock
	IDs   *config.IDGenerator

	// Agents looks up an adapter by its declared kind (e.g. "claude", "pi")
	// for CreateAgent and, later, Resume. Only CreateShell tolerates it
	// being nil; agent creation requires it.
	Agents *agent.Registry
	// ConfigEnv mirrors config.toml's [env] table (SPEC §6.3): the layer
	// between captured_path and the session's own env in PATH resolution
	// order. A nil map is the common, valid case of no configured overrides.
	ConfigEnv map[string]string

	// Shell overrides the user's $SHELL. It is primarily useful to embedded
	// callers; an empty value selects $SHELL, falling back to /bin/sh.
	Shell string
}

// CreateShell creates the durable row before starting its one-pane private
// tmux session. A failed tmux launch is represented as an error row plus a
// transition event, rather than leaving a misleading "starting" row behind.
func (s Service) CreateShell(ctx context.Context, input ShellCreateInput) (store.Session, error) {
	if s.Store == nil || s.Audit == nil || s.Clock == nil || s.IDs == nil {
		return store.Session{}, errors.New("shell creation requires store, audit logger, clock, and id generator")
	}
	if input.Name == "" || input.CWD == "" {
		return store.Session{}, errors.New("shell session name and working directory are required")
	}
	id, err := s.IDs.UUID()
	if err != nil {
		return store.Session{}, fmt.Errorf("generate shell session id: %w", err)
	}
	shell := s.Shell
	if shell == "" {
		shell = os.Getenv("SHELL")
	}
	if shell == "" {
		shell = "/bin/sh"
	}
	if !filepath.IsAbs(shell) {
		return store.Session{}, fmt.Errorf("user shell %q must be an absolute path", shell)
	}
	capturedPath := os.Getenv("PATH")
	if capturedPath == "" {
		return store.Session{}, errors.New("PATH is required to create a shell session")
	}
	now := s.Clock.Now().UnixMilli()
	session, err := s.Store.CreateSession(ctx, store.CreateSessionInput{
		ID: id, Name: input.Name, CWD: input.CWD, Agent: "shell", CapturedPath: capturedPath,
		Status: "starting", StatusSource: "user", StatusAt: now, CreatedAt: now,
	})
	if err != nil {
		return store.Session{}, fmt.Errorf("create durable shell session: %w", err)
	}
	if err := s.Audit.Transition(session.ID, "starting"); err != nil {
		return session, fmt.Errorf("audit starting shell session %q: %w", session.Name, err)
	}
	if _, err := s.TMux.Create(ctx, tmux.Launch{Slug: session.Slug, CWD: session.CWD, Command: []string{shell}, Env: input.Env}); err != nil {
		return s.launchFailed(ctx, session, fmt.Errorf("launch shell session %q: %w", session.Name, err))
	}
	if err := s.Audit.Launch(session.ID, []string{shell}, input.Env); err != nil {
		// The pane is not a successful deck launch if its required audit record
		// cannot be written, so remove it and leave an observable durable error.
		_ = s.TMux.Kill(ctx, session.Slug)
		return s.launchFailed(ctx, session, fmt.Errorf("audit shell launch %q: %w", session.Name, err))
	}
	// A user-sourced starting row is still being launched and must not be
	// mistaken for a disappeared pane by another live deck client. Once tmux
	// and its launch audit are both complete, mark that same visible state as
	// tmux-observed so reconciliation may safely own its liveness.
	if err := s.Store.UpdateSessionStatus(ctx, store.StatusUpdateInput{
		SessionID: session.ID, Status: "starting", Reason: "", Source: "tmux",
		At: s.Clock.Now().UnixMilli(), EventKind: "launch.ready",
	}); err != nil {
		return s.launchFailed(ctx, session, fmt.Errorf("record ready shell session %q: %w", session.Name, err))
	}
	if err := s.Audit.Transition(session.ID, "launch.ready"); err != nil {
		return s.launchFailed(ctx, session, fmt.Errorf("audit ready shell session %q: %w", session.Name, err))
	}
	session.StatusSource = "tmux"
	return session, nil
}

func (s Service) launchFailed(ctx context.Context, session store.Session, cause error) (store.Session, error) {
	if s.Clock == nil {
		return session, fmt.Errorf("%w (cannot record launch failure without clock)", cause)
	}
	if err := s.Store.UpdateSessionStatus(ctx, store.StatusUpdateInput{
		SessionID: session.ID, Status: "error", Reason: cause.Error(), Source: "tmux", At: s.Clock.Now().UnixMilli(), EventKind: "launch.failed",
	}); err != nil {
		return session, fmt.Errorf("%w (also record launch failure: %v)", cause, err)
	}
	if err := s.Audit.Transition(session.ID, "launch.failed"); err != nil {
		return session, fmt.Errorf("%w (also audit launch failure: %v)", cause, err)
	}
	return session, cause
}
