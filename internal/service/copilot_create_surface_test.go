package service

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/n-orlov/deck/internal/agent"
)

// R220.3: this product has no `deck new` subcommand and no `agent` config key;
// the single place an agent kind from any caller (the create dialog, a script
// driving the service) is accepted or refused is Service.CreateAgent, so that is
// where the criterion is asserted. `copilot` is accepted and persisted as the
// row's kind; an unknown kind -- including a near miss of copilot -- is still
// refused with the existing `unknown agent kind` error and leaves no row.
func TestCreateAgentAcceptsCopilotAndStillRefusesUnknownKinds(t *testing.T) {
	svc, _, _, _ := newCopilotTestService(t, "copilot-surface-kind")

	created, err := svc.CreateAgent(context.Background(), AgentCreateInput{Name: "Copilot kind", CWD: t.TempDir(), Agent: "copilot", PermissionProfile: "edits"})
	if err != nil {
		t.Fatalf("CreateAgent copilot: %v", err)
	}
	if created.Agent != "copilot" || created.PermissionProfile != "edits" {
		t.Fatalf("created row agent/profile = %q/%q, want copilot/edits", created.Agent, created.PermissionProfile)
	}

	for _, kind := range []string{"ghost", "Copilot", "copilots", "gh-copilot"} {
		_, err := svc.CreateAgent(context.Background(), AgentCreateInput{Name: "bad " + kind, CWD: t.TempDir(), Agent: kind})
		wantErr(t, "CreateAgent "+kind, err, `unknown agent kind "`+kind+`"`)
	}
}

// R220.4 (service half): AvailableKinds reports `copilot` the way it reports
// `codex` -- only when the executable resolves on the launch PATH -- and the
// kinds that were already registered keep their verdicts regardless of it.
func TestAvailableKindsReportsMissingCopilotLikeMissingCodex(t *testing.T) {
	registry := agent.NewRegistry()
	registry.Register(agent.NewShell())
	registry.Register(agent.NewClaude())
	registry.Register(agent.NewPi())
	registry.Register(agent.NewCodex())
	registry.Register(agent.NewCopilot())

	install := func(dir string, names ...string) {
		for _, name := range names {
			if err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
				t.Fatalf("write fake %s: %v", name, err)
			}
		}
	}

	// codex and copilot both absent: both unavailable, the rest as before.
	dir := t.TempDir()
	install(dir, "claude", "pi")
	t.Setenv("PATH", dir)
	got := Service{Agents: registry}.AvailableKinds()
	if want := []string{"claude", "pi", "shell"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("AvailableKinds with no codex and no copilot = %v, want %v", got, want)
	}

	// copilot alone installed: it appears, codex still does not.
	dir = t.TempDir()
	install(dir, "claude", "pi", "copilot")
	t.Setenv("PATH", dir)
	got = Service{Agents: registry}.AvailableKinds()
	if want := []string{"claude", "copilot", "pi", "shell"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("AvailableKinds with only copilot added = %v, want %v", got, want)
	}

	// codex alone installed (copilot missing): the mirror image.
	dir = t.TempDir()
	install(dir, "claude", "pi", "codex")
	t.Setenv("PATH", dir)
	got = Service{Agents: registry}.AvailableKinds()
	if want := []string{"claude", "codex", "pi", "shell"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("AvailableKinds with only codex added = %v, want %v", got, want)
	}
}
