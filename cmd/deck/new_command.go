package main

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/n-orlov/deck/internal/agent"
	"github.com/n-orlov/deck/internal/audit"
	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/service"
	"github.com/n-orlov/deck/internal/store"
	"github.com/n-orlov/deck/internal/tmux"
)

// newCreateArgs is the parsed creation form of `deck new` (R220, SPEC §3.4):
// `deck new --agent KIND [--name N] [--cwd DIR] [--permission P]`.
type newCreateArgs struct {
	agent      string
	name       string
	cwd        string
	permission string
}

// isNewCreateRequest reports whether args is the creation form of `deck new`:
// the word `new` followed by at least one further argument. Bare `deck new`
// stays what SPEC §3.4 always made it -- the profile named `new` -- so the
// profile is not shadowed; only a `new` that carries flags is the verb.
func isNewCreateRequest(args []string) bool {
	return len(args) >= 3 && args[1] == "new"
}

// parseNewCreateArgs parses the arguments after `deck new`. Each option takes
// its value as the next argument or as `--option=value`. An unknown flag is
// reported exactly as the profile parser reports one, and a positional
// argument is refused (the profile is chosen with DECK_PROFILE here).
func parseNewCreateArgs(rest []string, stderr io.Writer) (newCreateArgs, bool) {
	var out newCreateArgs
	targets := map[string]*string{"--agent": &out.agent, "--name": &out.name, "--cwd": &out.cwd, "--permission": &out.permission}
	for i := 0; i < len(rest); i++ {
		arg := rest[i]
		flag, value, hasValue := strings.Cut(arg, "=")
		target, known := targets[flag]
		switch {
		case !strings.HasPrefix(arg, "-"):
			sayf(stderr, "error: deck new takes options only, got %q\n", arg)
			return out, false
		case !known:
			sayf(stderr, "error: unknown flag %s\n", arg)
			return out, false
		case !hasValue && i+1 < len(rest):
			i++
			value = rest[i]
		case !hasValue:
			sayf(stderr, "error: %s needs a value\n", flag)
			return out, false
		}
		*target = value
	}
	return out, true
}

// resolveNewAgent picks the kind `deck new` creates: --agent, else config.toml's
// agent key. The kind is checked against the adapter registry with the create
// service's own `unknown agent kind` diagnostic, before anything is opened.
func resolveNewAgent(flagValue, configured string, registry *agent.Registry) (string, error) {
	kind := flagValue
	if kind == "" {
		kind = configured
	}
	if kind == "" {
		return "", errNoNewAgent
	}
	if _, err := registry.Require(kind); err != nil {
		return "", err
	}
	return kind, nil
}

var errNoNewAgent = &newUsageError{"deck new needs --agent KIND (or an agent key in config.toml)"}

type newUsageError struct{ msg string }

func (e *newUsageError) Error() string { return e.msg }

// newAgentRegistry is the adapter registry every deck entry point shares.
func newAgentRegistry() *agent.Registry {
	registry := agent.NewRegistry()
	registry.Register(agent.NewShell())
	registry.Register(agent.NewClaude())
	registry.Register(agent.NewPi())
	registry.Register(agent.NewCodex())
	registry.Register(agent.NewCopilot())
	return registry
}

// newCreateInput turns the parsed options and the chosen kind into the create
// service's input: cwd defaults to the working directory, the name to the
// cwd's basename.
func newCreateInput(opts newCreateArgs, kind string, getwd func() (string, error)) (service.AgentCreateInput, error) {
	cwd := opts.cwd
	if cwd == "" {
		wd, err := getwd()
		if err != nil {
			return service.AgentCreateInput{}, err
		}
		cwd = wd
	}
	cwd, err := filepath.Abs(cwd)
	if err != nil {
		return service.AgentCreateInput{}, err
	}
	name := opts.name
	if name == "" {
		name = filepath.Base(cwd)
	}
	profile := opts.permission
	if profile == "" {
		profile = "safe"
	}
	return service.AgentCreateInput{Name: name, CWD: cwd, Agent: kind, PermissionProfile: profile}, nil
}

// runNewCommand is `deck new --agent KIND ...`: it creates one detached agent
// session in the resolved deck profile and prints its name. Exit 2 is a usage
// or unknown-kind refusal (nothing opened), 1 a creation failure.
func runNewCommand(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	opts, ok := parseNewCreateArgs(args[2:], stderr)
	if !ok {
		return 2
	}
	_, code, proceed := resolveStartupProfile([]string{args[0]}, false, stdin, stderr)
	if !proceed {
		return code
	}
	settings, err := config.LoadFromProfile(os.Getenv, os.UserHomeDir, "")
	if err != nil {
		sayln(stderr, "deck configuration:", err)
		return 1
	}
	registry := newAgentRegistry()
	kind, err := resolveNewAgent(opts.agent, settings.Agent, registry)
	if err != nil {
		sayf(stderr, "error: %v\n", err)
		return 2
	}
	input, err := newCreateInput(opts, kind, os.Getwd)
	if err != nil {
		sayf(stderr, "error: %v\n", err)
		return 2
	}
	session, err := createDetachedSession(settings, registry, input)
	if err != nil {
		sayf(stderr, "deck new: %v\n", err)
		return 1
	}
	sayln(stdout, session.Name)
	return 0
}

// createDetachedSession opens the profile's store and creates input's session
// through the same service the TUI's create dialog uses.
func createDetachedSession(settings config.Settings, registry *agent.Registry, input service.AgentCreateInput) (store.Session, error) {
	db, err := store.Open(settings.Paths)
	if err != nil {
		return store.Session{}, err
	}
	defer func() { _ = db.Close() }()
	logger, err := audit.New(settings.Paths, settings.Clock)
	if err != nil {
		return store.Session{}, err
	}
	executable, err := absoluteExecutable()
	if err != nil {
		return store.Session{}, err
	}
	client := tmux.Client{Socket: settings.Socket, Mouse: settings.TmuxMouse}
	sessions := newSessionService(db, settings, logger, client, registry, executable)
	return sessions.CreateAgent(context.Background(), input)
}

// newSessionService wires the service both the TUI and `deck new` use.
func newSessionService(db *store.Store, settings config.Settings, logger *audit.Logger, client tmux.Client, registry *agent.Registry, executable string) service.Service {
	return service.Service{
		Store: db, TMux: client, Audit: logger,
		Clock: settings.Clock, IDs: settings.IDs, Agents: registry,
		ConfigEnv: settings.Env, DeckExecutable: executable, DeckHome: settings.Paths.Home,
		DataRoot:          settings.DataRoot,
		Profile:           settings.Profile,
		GlobalPreLaunch:   settings.PreLaunch,
		GlobalPostDestroy: settings.PostDestroy,
		RecentCwdLimit:    settings.RecentCwdLimit,
	}
}
