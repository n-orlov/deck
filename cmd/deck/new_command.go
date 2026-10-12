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
// service's input: cwd defaults to the working directory; a blank name is left blank for
// createDetachedSession to derive.
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
	// A blank name stays blank here: createDetachedSession derives it
	// (`<cwd-basename>-<MMDD-HHMM>`, first free suffix) once the store is open.
	name, nameSource := opts.name, "user"
	if strings.TrimSpace(name) == "" {
		name, nameSource = "", "derived"
	}
	profile := opts.permission
	if profile == "" {
		profile = "safe"
	}
	return service.AgentCreateInput{Name: name, NameSource: nameSource, CWD: cwd, Agent: kind, PermissionProfile: profile}, nil
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
// through the same service the TUI's create dialog uses. A session whose
// harness takes its name only through the keyboard once ready (Codex) gets a
// detached applier, because no TUI is ticking to type it (SPEC §8.2).
func createDetachedSession(settings config.Settings, registry *agent.Registry, input service.AgentCreateInput) (store.Session, error) {
	db, sessions, err := openDetachedService(settings, registry)
	if err != nil {
		return store.Session{}, err
	}
	if input.Name == "" {
		input.Name, input.NameSource, err = sessions.ResolveBlankName(context.Background(), "", input.CWD)
		if err != nil {
			_ = db.Close()
			return store.Session{}, err
		}
	}
	session, err := sessions.CreateAgent(context.Background(), input)
	_ = db.Close()
	if err == nil && sessions.NeedsLaunchNameApply(session) {
		// After the close: the applier opens the same database.
		if spawnErr := startNameApplier(session.ID); spawnErr != nil {
			logLaunchApplierFailure(settings, session.ID, spawnErr)
		}
	}
	return session, err
}

func logLaunchApplierFailure(settings config.Settings, id string, err error) {
	if logger, logErr := audit.New(settings.Paths, settings.Clock); logErr == nil {
		_ = logger.Event("launch name applier for " + id + " not started: " + err.Error())
	}
}

// openDetachedService is the store and service a command without a TUI uses.
func openDetachedService(settings config.Settings, registry *agent.Registry) (*store.Store, service.Service, error) {
	db, err := store.Open(settings.Paths)
	if err != nil {
		return nil, service.Service{}, err
	}
	logger, err := audit.New(settings.Paths, settings.Clock)
	if err != nil {
		_ = db.Close()
		return nil, service.Service{}, err
	}
	executable, err := absoluteExecutable()
	if err != nil {
		_ = db.Close()
		return nil, service.Service{}, err
	}
	client := tmux.Client{Socket: settings.Socket, Mouse: settings.TmuxMouse}
	return db, newSessionService(db, settings, logger, client, registry, executable), nil
}

// newSessionService wires the service both the TUI and `deck new` use.
func newSessionService(db *store.Store, settings config.Settings, logger *audit.Logger, client tmux.Client, registry *agent.Registry, executable string) service.Service {
	routeNameDegrades(logger)
	return service.Service{
		Store: db, TMux: client, Audit: logger,
		Clock: settings.Clock, IDs: settings.IDs, Agents: registry,
		ConfigEnv: settings.Env, DeckExecutable: executable, DeckHome: settings.Paths.Home,
		DataRoot:          settings.DataRoot,
		Profile:           settings.Profile,
		GlobalPreLaunch:   settings.PreLaunch,
		GlobalPostDestroy: settings.PostDestroy,
		RecentCwdLimit:    settings.RecentCwdLimit,
		AdoptNames:        func() bool { return settings.AdoptHarnessNames },
	}
}

// routeNameDegrades sends the one-time "a name source degraded" line of the
// agent adapters to the audit log, where a TUI on the same terminal is never
// written over.
func routeNameDegrades(logger *audit.Logger) {
	if logger == nil {
		return
	}
	agent.SetNameDegradeLog(func(msg string) { _ = logger.Event(msg) })
}
