package features

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cucumber/godog"
	"gopkg.in/yaml.v3"
)

// registerHarnessNameCopilotSteps backs the Copilot @harness-names scenarios
// (R264). Copilot keeps a session's name in workspace.yaml (name: and
// user_named:) and puts "<name> - GitHub Copilot" in the pane title; the steps
// assert through those records and deck's store and screen, polling each with
// a deadline and never pausing for a fixed time.
func registerHarnessNameCopilotSteps(sc *godog.ScenarioContext) {
	sc.Step(`^deck client "([^"]+)" creates copilot session with a blank name expecting the derived name "([^"]+)"$`, clientCreatesCopilotSessionWithBlankName)
	sc.Step(`^fake Copilot session "([^"]+)" is renamed inside Copilot to "([^"]+)"$`, fakeCopilotIsRenamedInside)
	sc.Step(`^the fake Copilot workspace of session "([^"]+)" is named "([^"]+)" with user_named (true|false)$`, fakeCopilotWorkspaceIsNamed)
	sc.Step(`^the fake Copilot workspace of session "([^"]+)" has no name$`, fakeCopilotWorkspaceHasNoName)
	sc.Step(`^the fake Copilot pane of session "([^"]+)" is titled "([^"]+)"$`, fakeCopilotPaneIsTitled)
}

func clientCreatesCopilotSessionWithBlankName(ctx context.Context, clientName, derived string) error {
	return clientCreatesAgentSessionWithBlankName(ctx, clientName, "copilot", derived)
}

// copilotWorkspace is the fields of workspace.yaml the steps read.
type copilotWorkspace struct {
	Name      string `yaml:"name"`
	UserNamed bool   `yaml:"user_named"`
}

// readCopilotWorkspace reads the fake's workspace.yaml for the session; found
// is false while the file does not exist yet.
func readCopilotWorkspace(h *ScenarioHarness, id string) (ws copilotWorkspace, found bool, err error) {
	data, err := os.ReadFile(filepath.Join(h.agentHOMEDir, ".copilot", "session-state", id, "workspace.yaml"))
	if os.IsNotExist(err) {
		return ws, false, nil
	}
	if err != nil {
		return ws, false, err
	}
	if err := yaml.Unmarshal(data, &ws); err != nil {
		return ws, false, fmt.Errorf("parse workspace.yaml of conversation %q: %w", id, err)
	}
	return ws, true, nil
}

// pollCopilotWorkspace polls the session's workspace.yaml until ok holds. The
// conversation id is resolved once, up front: deck may adopt a new name for the
// row while the poll runs, and the file is pinned to the conversation anyway.
func pollCopilotWorkspace(ctx context.Context, session, what string, ok func(copilotWorkspace) bool) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	id, err := codexConversation(h, session)
	if err != nil {
		return err
	}
	return pollCopilotWorkspaceByID(ctx, h, id, what, ok)
}

func pollCopilotWorkspaceByID(ctx context.Context, h *ScenarioHarness, id, what string, ok func(copilotWorkspace) bool) error {
	ctx, cancel := withDefaultWaitDeadline(ctx)
	defer cancel()
	var last copilotWorkspace
	var lastErr error
	for {
		ws, found, err := readCopilotWorkspace(h, id)
		if err == nil && found && ok(ws) {
			return nil
		}
		last, lastErr = ws, err
		select {
		case <-ctx.Done():
			return fmt.Errorf("fake Copilot workspace.yaml of conversation %q never showed %s (last %+v, error: %w)", id, what, last, lastErr)
		case <-time.After(20 * time.Millisecond):
		}
	}
}

func fakeCopilotWorkspaceIsNamed(ctx context.Context, session, want, userNamed string) error {
	wantUser := userNamed == "true"
	return pollCopilotWorkspace(ctx, session, fmt.Sprintf("name %q with user_named %s", want, userNamed), func(ws copilotWorkspace) bool {
		return ws.Name == want && ws.UserNamed == wantUser
	})
}

// fakeCopilotWorkspaceHasNoName waits for the fake to have written
// workspace.yaml (it does so a moment after the pane starts), then checks it
// holds no name.
func fakeCopilotWorkspaceHasNoName(ctx context.Context, session string) error {
	var named string
	if err := pollCopilotWorkspace(ctx, session, "a workspace.yaml", func(ws copilotWorkspace) bool {
		named = ws.Name
		return true
	}); err != nil {
		return err
	}
	if named != "" {
		return fmt.Errorf("fake Copilot workspace.yaml of session %q is named %q, want no name", session, named)
	}
	return nil
}

// fakeCopilotIsRenamedInside types "/rename <title>" into the pane, the line a
// human types inside Copilot, and waits for workspace.yaml to carry it.
func fakeCopilotIsRenamedInside(ctx context.Context, session, title string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	id, err := codexConversation(h, session)
	if err != nil {
		return err
	}
	if err := sendFakeCodexPaneLine(ctx, session, "/rename "+title); err != nil {
		return err
	}
	return pollCopilotWorkspaceByID(ctx, h, id, fmt.Sprintf("name %q with user_named true", title), func(ws copilotWorkspace) bool {
		return ws.Name == title && ws.UserNamed
	})
}

// fakeCopilotPaneIsTitled polls tmux's pane title, which the fake sets with
// OSC 0 as "<name> - GitHub Copilot".
func fakeCopilotPaneIsTitled(ctx context.Context, session, title string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	slug, err := sessionSlugByName(h, session)
	if err != nil {
		return err
	}
	ctx, cancel := withDefaultWaitDeadline(ctx)
	defer cancel()
	want := title + " - GitHub Copilot"
	var got string
	for {
		out, _ := tmuxOutput(ctx, h, "display-message", "-p", "-t", "deck_"+slug, "#{pane_title}")
		got = strings.TrimSpace(string(out))
		if got == want {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("fake Copilot pane of session %q is titled %q, want %q", session, got, want)
		case <-time.After(20 * time.Millisecond):
		}
	}
}
