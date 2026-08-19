// Package agent defines session-agent adapters and their registry.
//
// An adapter declares, up front, what it can honestly do: which permission
// profiles it supports, whether it accepts a caller-assigned conversation
// id, and whether it can be resumed at all (SPEC §8). internal/tui and
// internal/service consume adapters only through this interface and the
// Registry lookup below, so adding a new agent kind never requires a change
// under internal/tui.
package agent

import (
	"fmt"
	"sort"
)

// Caps declares what an adapter can honestly do. It is queried, never
// assumed: a caller must not ask an adapter for a profile or an
// assigned-id behaviour it did not declare here.
type Caps struct {
	// Profiles lists the permission profiles this adapter supports, using
	// the SPEC §5 names ("safe", "plan", "edits", "yolo"). An adapter that
	// has no notion of permission profiles at all (e.g. shell) reports a
	// nil/empty slice.
	Profiles []string
	// AssignsConversationID reports whether the adapter accepts a
	// caller-assigned conversation id (deck mints one and passes it in),
	// as opposed to minting its own or having none at all.
	AssignsConversationID bool
	// Resumable reports whether Resume is meaningful for this adapter at
	// all. A shell has no conversation to resume.
	Resumable bool
}

// SupportsProfile reports whether p is one of the profiles Caps declares.
func (c Caps) SupportsProfile(p string) bool {
	for _, have := range c.Profiles {
		if have == p {
			return true
		}
	}
	return false
}

// ResolveProfile resolves a requested permission profile against what this
// adapter honestly declares. If requested is supported, it is returned
// unchanged with degraded=false. If it is not supported, ResolveProfile
// falls back to "safe" (the most restrictive profile) and reports
// degraded=true along with a human-readable reason a caller can surface to
// the user (SPEC §5) rather than silently pretending the requested profile
// was honoured.
func (c Caps) ResolveProfile(kind, requested string) (resolved string, degraded bool, reason string) {
	if c.SupportsProfile(requested) {
		return requested, false, ""
	}
	return "safe", true, fmt.Sprintf("%s does not support permission profile %q; falling back to safe", kind, requested)
}

// LaunchInput carries what an adapter needs to build a launch argv. It is
// deliberately independent of internal/store so this package has no
// dependency on the persistence layer; internal/service adapts a stored
// session into this shape.
type LaunchInput struct {
	// CWD is the session's working directory.
	CWD string
	// ConversationID is the id deck assigned before launch, when the
	// adapter's Caps.AssignsConversationID is true. Empty otherwise.
	ConversationID string
	// Profile is the requested permission profile (SPEC §5 name), already
	// resolved/degraded by the caller if the adapter does not support it.
	Profile string
	// ExtraArgs are additional launch_args from the session row, appended
	// verbatim after the adapter's own argv.
	ExtraArgs []string
	// DeckExecutable is the absolute path to the running deck binary. Hook
	// commands embed this path rather than relying on the pane's PATH.
	DeckExecutable string
	// DeckSessionID is deck's row identity, which may differ from the agent's
	// conversation id. It is the fallback identity supplied to hook processes.
	DeckSessionID string
	// DeckHome is the resolved data root whose state database receives hooks.
	DeckHome string
}

// ResumeInput carries what an adapter needs to build a resume argv.
type ResumeInput struct {
	CWD            string
	ConversationID string
	Profile        string
	ExtraArgs      []string
}

// Adapter is implemented by each supported agent kind. It declares its
// capabilities and turns launch/resume requests into argv — it never runs
// anything itself; internal/service is responsible for the pane.
type Adapter interface {
	// Kind is the stable, lowercase identifier used in config and the
	// store (e.g. "claude", "pi", "shell").
	Kind() string
	// Capabilities declares what this adapter honestly supports.
	Capabilities() Caps
	// Launch returns the argv to start a fresh conversation.
	Launch(in LaunchInput) (argv []string, err error)
	// Resume returns the argv to resume an existing conversation. Callers
	// must not call this when Capabilities().Resumable is false.
	Resume(in ResumeInput) (argv []string, err error)
	// Instrument returns per-process argv and environment additions. It is a
	// pure function: adapters never write settings files or mutate user env.
	Instrument(in LaunchInput) (argv []string, env map[string]string)
}

// Registry looks adapters up by kind. The zero value is not usable; use
// NewRegistry.
type Registry struct {
	adapters map[string]Adapter
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{adapters: make(map[string]Adapter)}
}

// Register adds (or replaces) the adapter under its own Kind().
func (r *Registry) Register(a Adapter) {
	r.adapters[a.Kind()] = a
}

// Lookup returns the adapter registered for kind, and whether it was found.
func (r *Registry) Lookup(kind string) (Adapter, bool) {
	a, ok := r.adapters[kind]
	return a, ok
}

// Kinds returns a stable, sorted list of every registered kind.
func (r *Registry) Kinds() []string {
	kinds := make([]string, 0, len(r.adapters))
	for k := range r.adapters {
		kinds = append(kinds, k)
	}
	sort.Strings(kinds)
	return kinds
}
