package tui

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/agent"
	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
)

// key is defined in tui_test.go and reused here to send synthetic key
// presses through Model.Update.

// guardAdapter is a throwaway agent.Adapter, distinct from shell/claude/pi,
// used to prove PRD requirement 1 ("adding an adapter never requires
// touching internal/tui") end-to-end: it must show up in the Agent field,
// offer exactly its own declared profiles, and produce a degradation
// reason for a requested profile it does not support, purely by virtue of
// being registered — with zero edits to internal/tui.
type guardAdapter struct{}

func (guardAdapter) Kind() string { return "zzz-guard-adapter" }
func (guardAdapter) Capabilities() agent.Caps {
	return agent.Caps{Profiles: []string{"safe", "edits"}}
}
func (guardAdapter) Launch(agent.LaunchInput) ([]string, error) { return nil, nil }
func (guardAdapter) Resume(agent.ResumeInput) ([]string, error) { return nil, nil }
func (guardAdapter) Instrument(agent.LaunchInput) ([]string, map[string]string) {
	return nil, nil
}
func (guardAdapter) Probe(string) (string, string)                        { return "", "" }
func (guardAdapter) TranscriptPaths(agent.TranscriptInput) (string, bool) { return "", false }

// TestBlackBoxRegistrySwapNeedsNoTUIEdit proves (PRD requirement 1) that a
// registry whose adapter membership differs from the stock shell/claude/pi
// set — an extra kind present, "pi" absent — is fully reflected by the TUI
// without any internal/tui source change: the extra kind appears in the
// create modal's Agent field, the absent stock kind appears nowhere in the
// modal, and the extra kind's own declared profiles (plus an explicit
// degradation reason for a profile it does not support) are exactly what
// gets offered/shown.
// TestDefaultCreateAgentPrefersShellRegardlessOfSortOrder pins the rule in
// defaultCreateAgent: "shell" must be the create modal's opening default
// whenever it is registered, even when the registry's alphabetical
// Kinds() order would put some other adapter first (e.g. an adapter kind
// that sorts before "shell"). A prior regression (task 002/003) silently
// switched the default to registry.Kinds()[0] == "claude", which broke
// the shell-only create-session flow the cmd/deck PTY tests depend on;
// registry_guard_test.go's own case never pinned this because it never
// exercised a registry where shell was not already first. This test also
// covers the fallback: when "shell" is absent entirely, the default must
// be Kinds()[0].
func TestDefaultCreateAgentPrefersShellRegardlessOfSortOrder(t *testing.T) {
	registry := agent.NewRegistry()
	registry.Register(agent.NewShell())
	registry.Register(agent.NewClaude())
	registry.Register(guardAdapter{}) // Kind() "zzz-guard-adapter" sorts after shell, so add an adapter that sorts before it too
	registry.Register(aardvarkAdapter{})

	kinds := registry.Kinds()
	if kinds[0] == "shell" {
		t.Fatalf("test setup bug: want a registry where alphabetical order does not put shell first, got %v", kinds)
	}
	if got := defaultCreateAgent(kinds); got != "shell" {
		t.Fatalf("defaultCreateAgent(%v) = %q, want \"shell\" preferred over alphabetical order", kinds, got)
	}

	m := NewWithShellCreatorAttacherKillerResumerProfileSwitcherResumeModerAgentCreatorAndRegistry(
		nil, config.Settings{}, "", nil, nil, nil, nil, nil, nil, nil, nil, registry,
	)
	updated, _ := m.Update(key("n"))
	nm := updated.(Model)
	if !nm.creating {
		t.Fatalf("pressing 'n' did not open the create modal")
	}
	if nm.createAgent != "shell" {
		t.Fatalf("create modal opened with default agent %q, want \"shell\"", nm.createAgent)
	}

	noShell := agent.NewRegistry()
	noShell.Register(agent.NewClaude())
	noShell.Register(agent.NewPi())
	noShellKinds := noShell.Kinds()
	if got := defaultCreateAgent(noShellKinds); got != noShellKinds[0] {
		t.Fatalf("defaultCreateAgent(%v) = %q, want fallback to Kinds()[0] = %q when shell is absent", noShellKinds, got, noShellKinds[0])
	}
}

// aardvarkAdapter is a throwaway adapter whose Kind() sorts alphabetically
// before "shell", used to prove defaultCreateAgent does not just pick
// Kinds()[0].
type aardvarkAdapter struct{}

func (aardvarkAdapter) Kind() string { return "aardvark-guard-adapter" }
func (aardvarkAdapter) Capabilities() agent.Caps {
	return agent.Caps{Profiles: []string{"safe"}}
}
func (aardvarkAdapter) Launch(agent.LaunchInput) ([]string, error) { return nil, nil }
func (aardvarkAdapter) Resume(agent.ResumeInput) ([]string, error) { return nil, nil }
func (aardvarkAdapter) Instrument(agent.LaunchInput) ([]string, map[string]string) {
	return nil, nil
}
func (aardvarkAdapter) Probe(string) (string, string)                        { return "", "" }
func (aardvarkAdapter) TranscriptPaths(agent.TranscriptInput) (string, bool) { return "", false }

// transcriptGuardEnvKey is an invented env key name, chosen to look
// nothing like any adapter this repo ships ("CODEX_HOME") -- the whole
// point of transcriptGuardAdapter is to prove internal/tui resolves
// whatever key a *newly registered* adapter names, not one it happens to
// already recognise.
const transcriptGuardEnvKey = "ZZZ_GUARD_TRANSCRIPT_ROOT"

// transcriptGuardAdapter is a throwaway adapter, distinct from every
// shipped adapter, whose transcript convention is keyed on
// transcriptGuardEnvKey: TranscriptPaths joins that resolved env value with
// the conversation id and declines outright when the caller never resolved
// the key at all (nil Env, as every shipped adapter with no
// TranscriptEnvKeys need gets) or resolved it to the empty string.
type transcriptGuardAdapter struct{}

func (transcriptGuardAdapter) Kind() string { return "zzz-transcript-guard-adapter" }
func (transcriptGuardAdapter) Capabilities() agent.Caps {
	return agent.Caps{HasTranscript: true, TranscriptEnvKeys: []string{transcriptGuardEnvKey}}
}
func (transcriptGuardAdapter) Launch(agent.LaunchInput) ([]string, error) { return nil, nil }
func (transcriptGuardAdapter) Resume(agent.ResumeInput) ([]string, error) { return nil, nil }
func (transcriptGuardAdapter) Instrument(agent.LaunchInput) ([]string, map[string]string) {
	return nil, nil
}
func (transcriptGuardAdapter) Probe(string) (string, string) { return "", "" }
func (transcriptGuardAdapter) TranscriptPaths(in agent.TranscriptInput) (string, bool) {
	root, ok := in.Env[transcriptGuardEnvKey]
	if !ok || root == "" {
		return "", false
	}
	return filepath.Join(root, in.ConversationID+".transcript"), true
}

// TestBlackBoxRegistrySwapTranscriptEnvKeyNeedsNoTUIEdit extends the
// registry-swap guard above to the transcript capability (task 006, cure
// for B2/005): a registry whose only transcript-having adapter is
// transcriptGuardAdapter -- an invented Kind() and an invented
// TranscriptEnvKeys name the shipped adapters (shell/claude/pi/codex) never
// use -- must still have its declared env key resolved from the session's
// own §6.1 env layering (config [env] loses to session env) and handed to
// TranscriptPaths, producing the adapter's own joined path, with zero edit
// under internal/tui. A caller that special-cased a known key name (the
// pre-task-005 literal-CODEX_HOME shape) would resolve nothing for this
// key and TranscriptPaths would decline; this guard fails against that
// shape and passes against the shipped resolveEnvKey-driven
// transcriptPathFor.
func TestBlackBoxRegistrySwapTranscriptEnvKeyNeedsNoTUIEdit(t *testing.T) {
	registry := agent.NewRegistry()
	registry.Register(agent.NewShell())
	registry.Register(agent.NewClaude())
	registry.Register(transcriptGuardAdapter{})

	settings := config.Settings{Env: map[string]string{transcriptGuardEnvKey: "/config-layer/should-lose"}}
	m := NewWithShellCreatorAttacherKillerResumerProfileSwitcherResumeModerAgentCreatorAndRegistry(
		nil, settings, "", nil, nil, nil, nil, nil, nil, nil, nil, registry,
	)

	wantRoot := "/session-layer/wins"
	session := store.Session{
		Name:           "transcript-guard-session",
		Agent:          "zzz-transcript-guard-adapter",
		ConversationID: "convo-123",
		Env:            map[string]string{transcriptGuardEnvKey: wantRoot},
	}

	gotPath, ok := m.transcriptPathFor(session)
	if !ok {
		t.Fatalf("transcriptPathFor(%+v) declined; want it to resolve %q from the session env layer", session, transcriptGuardEnvKey)
	}
	want := filepath.Join(wantRoot, session.ConversationID+".transcript")
	if gotPath != want {
		t.Fatalf("transcriptPathFor(%+v) = %q, want %q (session env layer for %q, not the config layer)", session, gotPath, want, transcriptGuardEnvKey)
	}

	// Absent from every layer: TranscriptPaths must decline, never guess.
	noEnvSession := session
	noEnvSession.Env = nil
	noEnvSettings := config.Settings{}
	mNoConfig := NewWithShellCreatorAttacherKillerResumerProfileSwitcherResumeModerAgentCreatorAndRegistry(
		nil, noEnvSettings, "", nil, nil, nil, nil, nil, nil, nil, nil, registry,
	)
	if path, ok := mNoConfig.transcriptPathFor(noEnvSession); ok || path != "" {
		t.Fatalf("transcriptPathFor with %q resolved nowhere = (%q, %v), want (\"\", false)", transcriptGuardEnvKey, path, ok)
	}
}

func TestBlackBoxRegistrySwapNeedsNoTUIEdit(t *testing.T) {
	registry := agent.NewRegistry()
	registry.Register(agent.NewShell())
	registry.Register(agent.NewClaude())
	registry.Register(guardAdapter{}) // extra kind; "pi" deliberately omitted

	m := NewWithShellCreatorAttacherKillerResumerProfileSwitcherResumeModerAgentCreatorAndRegistry(
		nil, config.Settings{}, "", nil, nil, nil, nil, nil, nil, nil, nil, registry,
	)
	// Task 030: framedDialog no longer grows to fit content, so a
	// wide-enough viewport keeps the Agent field's full cycle list off a
	// word-wrap boundary and on one physical line.
	m.width = 100
	// Task 005/PRD R112: the Agent field's cycle and row now read
	// m.createAvailableAgentKinds, populated only on the "n" open path
	// (WithAvailableAgentKindsProber -> computeAvailableAgentKinds). This
	// test drives the modal by setting fields directly rather than
	// pressing "n", so it injects an all-available prober (every
	// registered kind is "available" -- guardAdapter has no binary of its
	// own to probe) and populates the field the same way "n" would, so
	// the assertions below still exercise the real registry-swap
	// membership rather than this seam.
	m = m.WithAvailableAgentKindsProber(func() []string { return registry.Kinds() })
	m.createAvailableAgentKinds = m.computeAvailableAgentKinds()
	m.creating = true
	m.createName = "guard-session"
	m.createCWD = t.TempDir()
	m.createAgent = registry.Kinds()[0]
	m.createProfile = "safe"
	m.createField = 2

	kinds := registry.Kinds()
	for _, k := range kinds {
		if k == "pi" {
			t.Fatal("test setup bug: registry unexpectedly contains \"pi\"")
		}
	}
	view := m.createView()
	if !strings.Contains(view, "zzz-guard-adapter") {
		t.Fatalf("create modal does not list the extra registered kind:\n%s", view)
	}
	var agentLine string
	for _, line := range strings.Split(view, "\n") {
		if strings.Contains(line, "Agent:") {
			agentLine = line
			break
		}
	}
	if agentLine == "" {
		t.Fatalf("create modal has no Agent field line:\n%s", view)
	}
	wantCycles := "cycles: " + strings.Join(kinds, ", ")
	if !strings.Contains(agentLine, wantCycles) {
		t.Fatalf("Agent field line = %q, want it to contain %q", agentLine, wantCycles)
	}
	if strings.Contains(agentLine, "pi") {
		t.Fatalf("Agent field line mentions the absent stock kind %q: %q", "pi", agentLine)
	}

	// Cycle to the extra kind and confirm only its own declared profiles
	// (never claude's, never a hardcoded literal) are offered.
	found := false
	for i := 0; i < len(registry.Kinds())+1; i++ {
		if m.createAgent == "zzz-guard-adapter" {
			found = true
			break
		}
		m.cycleCreateField(1)
	}
	if !found {
		t.Fatalf("left/right cycling never reached the extra registered kind; ended on %q", m.createAgent)
	}
	options := m.createProfileOptionsFor(m.createAgent, true)
	if strings.Join(options, ",") != "safe,edits" {
		t.Fatalf("createProfileOptionsFor(extra kind) = %v, want [safe edits]", options)
	}

	// A session created against the extra kind with an unsupported
	// requested profile ("plan", which guardAdapter does not declare) must
	// carry an explicit degradation reason, and the detail pane must show
	// it — derived purely from the registered adapter's Caps, not from any
	// internal/tui special-case for a known kind name.
	caps, applicable := m.agentCapabilities("zzz-guard-adapter")
	if !applicable {
		t.Fatal("agentCapabilities reported the extra registered kind as not applicable")
	}
	resolved, degraded, reason := caps.ResolveProfile("zzz-guard-adapter", "plan")
	if !degraded || resolved != "safe" || reason == "" {
		t.Fatalf("ResolveProfile(extra kind, plan) = (%q,%v,%q), want degraded to safe with a reason", resolved, degraded, reason)
	}

	session := store.Session{
		Name: "guard-session", Agent: "zzz-guard-adapter", Status: "starting",
		PermissionProfile:       resolved,
		PermissionProfileReason: reason,
	}
	m.sessions = []store.Session{session}
	m.selected = rowCursor(0)
	m.creating = false
	m.detail = true
	detailView := m.View()
	// Task 030: framedDialog's box no longer grows to fit content (fixed at
	// 80% of the viewport, clamped to [26, 80]), so this 94-column
	// degradation sentence always wraps at a word boundary regardless of
	// viewport width; assert the phrases that word-wrap keeps intact
	// (never inside a word) rather than the whole dynamic `reason` string
	// as one contiguous run.
	if !strings.Contains(detailView, "degraded") ||
		!strings.Contains(detailView, "does not support permission profile") ||
		!strings.Contains(detailView, `"plan"`) ||
		!strings.Contains(detailView, "falling back to safe") {
		t.Fatalf("detail view missing the extra kind's degradation reason:\n%s", detailView)
	}
}

// forbiddenGroupModelIdentifierRe matches a `workspace`/`Workspace` Go
// identifier (or any longer identifier that carries it as a substring,
// e.g. the old sessionWorkspace/DefaultWorkspace/GroupByWorkspace names
// R128/R129 deleted) case-insensitively. It deliberately targets
// *ast.Ident nodes only -- never the raw file text -- so it never trips
// on the legitimate historical artifacts task 008/012 left behind on
// purpose: schemaV1's `workspace TEXT` column literal and schemaV7's
// `DROP COLUMN workspace` statement (both inside Go string literals,
// needed verbatim because that really was the historical column's name
// and the migration must name it exactly to drop it), and the handful of
// doc comments/user-facing help text across these three trees that still
// use "workspace" as an English word describing what the removed feature
// used to be. Neither a string literal nor a comment is an *ast.Ident, so
// this regexp only ever sees real identifiers: variable, field, function,
// type and (deliberately, for full closure) test-function names.
var forbiddenGroupModelIdentifierRe = regexp.MustCompile(`(?i)workspace`)

// TestNoLegacyGroupModelIdentifierSurvivesInStoreTUIOrService is the
// R128/R129 closure guard (task 014): it parses every .go file (product
// and test) in internal/store, internal/tui and internal/service and
// fails if any Go identifier -- a var, field, function, type, or test
// name -- contains "workspace"/"Workspace", case-insensitively. R128
// (task 008) deleted store.DefaultWorkspace/Session.Workspace/
// Session.WorkspaceColumn and the sessions.workspace column (replaced by
// groups/group_id); R129 (tasks 011-013, task 012 specifically) deleted
// [ui] group_by_workspace, DECK_GROUP_BY_WORKSPACE and flat sidebar mode.
// This guard is the one place that keeps that removal closed:
// internal/tui's own group-key seam (sessionWorkspace, sidebarGroup.
// Workspace, sidebarEntry.workspace, and every test identifier that named
// them) was renamed to sessionGroupKey / sidebarGroup.Name /
// sidebarEntry.groupName in the SAME commit that added this guard,
// specifically so this test does not merely describe a rule -- it
// enforces one. The guard's scope is exactly these three trees:
// internal/agent/codex.go's `workspace-write` flag, cmd/fake-codex and
// internal/tmux/key.go's doc comment (none of which have anything to do
// with SPEC §11's manual groups) lie outside it by construction, because
// they live in neither internal/store, internal/tui nor internal/service.
//
// Observed failing (by construction, not merely by inspection): reverting
// the sessionGroupKey rename locally across internal/tui -- `sed -i
// s/sessionGroupKey/sessionWorkspace/g internal/tui/*.go` -- and
// rerunning this test reintroduces the exact identifier this guard exists
// to catch, and it fails with eight t.Errorf lines naming filter.go:43,
// group.go:47/60/76/120, group_test.go:28 and
// navigation_parity_test.go:43 (twice) before the revert is undone; the
// captured run is artifacts/task014-guard-observed-failing.log.
func TestNoLegacyGroupModelIdentifierSurvivesInStoreTUIOrService(t *testing.T) {
	dirs := []string{"../store", ".", "../service"}
	fset := token.NewFileSet()
	filesScanned := 0
	for _, dir := range dirs {
		files, err := filepath.Glob(filepath.Join(dir, "*.go"))
		if err != nil {
			t.Fatalf("glob %s: %v", dir, err)
		}
		if len(files) == 0 {
			t.Fatalf("no .go files found in %s -- guard test may be broken", dir)
		}
		for _, f := range files {
			node, err := parser.ParseFile(fset, f, nil, 0)
			if err != nil {
				t.Fatalf("parse %s: %v", f, err)
			}
			filesScanned++
			ast.Inspect(node, func(n ast.Node) bool {
				ident, ok := n.(*ast.Ident)
				if !ok {
					return true
				}
				if forbiddenGroupModelIdentifierRe.MatchString(ident.Name) {
					pos := fset.Position(ident.Pos())
					t.Errorf("%s:%d: identifier %q contains \"workspace\"/\"Workspace\" -- "+
						"R128/R129 removed the workspace grouping model from "+
						"internal/store, internal/tui and internal/service entirely; "+
						"no identifier in those three trees may name it, even in a test",
						pos.Filename, pos.Line, ident.Name)
				}
				return true
			})
		}
	}
	if filesScanned == 0 {
		t.Fatal("scanned zero .go files across internal/store, internal/tui and internal/service -- guard test may be broken")
	}
}
