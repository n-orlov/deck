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
)

// registerCodexHooksSteps backs SPEC §13.4's @codex scenario (R127, task
// 024): the window between a Codex row's creation and its first prompt,
// when it carries no conversation id at all, and the two hook-driven
// transitions that follow -- SessionStart's id adoption and
// PermissionRequest's tool-named "waiting". It drives the fake-codex
// fixture's own pane-command surface (cmd/fake-codex's "prompt"/
// "permission" commands, task 021) exactly the way registerClaudeHookStatusSteps
// drives fake-claude's "hook" command, and it never imports internal/agent:
// codexTranscriptPathForConversationID below duplicates cmd/fake-codex's own
// rollout-file convention, not internal/agent/codex.go's TranscriptPaths,
// keeping this file black-box like every other file in this package.
func registerCodexHooksSteps(sc *godog.ScenarioContext) {
	sc.Step(`^the deck config probes agent panes quickly$`, deckConfigProbesQuickly)
	sc.Step(`^the state database session "([^"]+)" has no conversation id$`, sessionHasNoConversationID)
	sc.Step(`^fake Codex session "([^"]+)" is prompted with "([^"]*)"$`, fakeCodexIsPromptedWith)
	sc.Step(`^fake Codex session "([^"]+)" requests approval to run tool "([^"]+)"$`, fakeCodexRequestsApprovalForTool)
	sc.Step(`^session "([^"]+)"'s codex transcript does not mention session "([^"]+)"'s conversation id$`, sessionCodexTranscriptDoesNotMentionOthersConversationID)
	sc.Step(`^within 3 seconds deck client "([^"]+)" row "([^"]+)" contains "([^"]+)"$`, clientRowContainsWithinThreeSeconds)
}

// deckConfigProbesQuickly writes stale_after = 1 (the schema's own declared
// minimum, internal/config/schema.go) to the scenario's config.toml, so a
// freshly created row (StatusAt = its own creation time) becomes probe-
// eligible (internal/service/reconcile.go's probeEligible) after 1 real
// second rather than the schema's 45s default -- this feature has no need
// for status_probe.feature's own frozen/steppable DECK_CLOCK machinery,
// since it never asserts precedence at the exact stale_after boundary,
// only that a sampled verdict eventually appears. Must run before "deck
// client ... is started": config.toml is read once, at client startup.
func deckConfigProbesQuickly(ctx context.Context) error {
	h, err := scenarioHarness(ctx)
	if err != nil {
		return err
	}
	path := filepath.Join(h.Home, "config.toml")
	if err := os.WriteFile(path, []byte("stale_after = 1\n"), 0o600); err != nil {
		return fmt.Errorf("write scenario config.toml: %w", err)
	}
	return nil
}

// clientRowContainsWithinThreeSeconds is status_probe_test.go's
// clientRowContainsWithinReconcile widened to a fixed 3s deadline -- long
// enough to cross deckConfigProbesQuickly's 1s stale_after floor plus at
// least one reconcile tick, which the reconcile-interval-only wait
// (scenarioReconcileInterval + 250ms = 500ms) cannot reach.
func clientRowContainsWithinThreeSeconds(ctx context.Context, clientName, rowName, want string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	client, err := h.Client(clientName)
	if err != nil {
		return err
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		for _, line := range strings.Split(client.Frame(false), "\n") {
			if strings.Contains(line, rowName) && strings.Contains(line, want) {
				return nil
			}
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("client %q row %q did not contain %q within 3 seconds\nframe:\n%s", clientName, rowName, want, client.Frame(false))
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// sessionHasNoConversationID is sessionHasNonEmptyConversationID's negative
// counterpart: proves a just-created codex row mints nothing at launch
// (SPEC §8.2) -- unlike claude/pi, which sessionHasNonEmptyConversationID
// already covers.
func sessionHasNoConversationID(ctx context.Context, name string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	id, err := sessionConversationID(h, name)
	if err != nil {
		return err
	}
	if id != "" {
		return fmt.Errorf("session %q has conversation id %q, want none yet", name, id)
	}
	return nil
}

// fakeCodexIsPromptedWith sends cmd/fake-codex's "prompt" pane command
// (task 021) into the named session's private tmux pane, which mints (or
// reuses, on a `resume <id>` invocation) that invocation's one conversation
// id, fires SessionStart on the very first call and UserPromptSubmit on
// every call, and writes the rollout transcript's session_meta line. It
// polls the pane's own echoed "fake-codex session-id: <uuid>" banner line
// (cmd/fake-codex's submitPrompt) rather than returning as soon as
// send-keys completes, so a caller relying on the id having been minted
// (e.g. a later transcript-path lookup) never races the fixture's own
// asynchronous pane output.
func fakeCodexIsPromptedWith(ctx context.Context, name, text string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	slug, err := sessionSlugByName(h, name)
	if err != nil {
		return err
	}
	target := "deck_" + slug
	request, err := json.Marshal(map[string]any{"command": "prompt", "text": text})
	if err != nil {
		return err
	}
	if _, err := tmuxOutput(ctx, h, "send-keys", "-t", target, "-l", string(request)); err != nil {
		return fmt.Errorf("send prompt command to fake Codex pane %q: %w", target, err)
	}
	if _, err := tmuxOutput(ctx, h, "send-keys", "-t", target, "Enter"); err != nil {
		return fmt.Errorf("submit prompt command to fake Codex pane %q: %w", target, err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		output, err := tmuxOutput(ctx, h, "capture-pane", "-p", "-S", "-", "-t", target)
		if err == nil && strings.Contains(string(output), "fake-codex session-id:") {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("fake Codex pane %q never announced a minted session id; last capture (err=%v):\n%s", target, err, string(output))
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// fakeCodexRequestsApprovalForTool sends cmd/fake-codex's "permission" pane
// command, firing PermissionRequest with tool_name (task 018's own receiver
// mapping: PermissionRequest -> "waiting", ReasonField "tool_name") and
// printing the real approval-prompt text codex's own "waiting" probe rule
// keys on (task 019). name must already have been prompted at least once
// (cmd/fake-codex's requestPermission refuses otherwise).
func fakeCodexRequestsApprovalForTool(ctx context.Context, name, toolName string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	slug, err := sessionSlugByName(h, name)
	if err != nil {
		return err
	}
	target := "deck_" + slug
	sessionID, err := sessionIDByName(h, name)
	if err != nil {
		return err
	}
	db, err := openObservedDatabase(h)
	if err != nil {
		return err
	}
	defer db.Close()
	var before int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM events WHERE session_id = ?`, sessionID).Scan(&before); err != nil {
		return err
	}
	request, err := json.Marshal(map[string]any{"command": "permission", "tool_name": toolName})
	if err != nil {
		return err
	}
	if _, err := tmuxOutput(ctx, h, "send-keys", "-t", target, "-l", string(request)); err != nil {
		return fmt.Errorf("send permission command to fake Codex pane %q: %w", target, err)
	}
	if _, err := tmuxOutput(ctx, h, "send-keys", "-t", target, "Enter"); err != nil {
		return fmt.Errorf("submit permission command to fake Codex pane %q: %w", target, err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		var count int
		if err := db.QueryRowContext(ctx, `SELECT count(*) FROM events WHERE session_id = ?`, sessionID).Scan(&count); err != nil {
			return err
		}
		if count > before {
			return nil
		}
		if time.Now().After(deadline) {
			output, _ := tmuxOutput(ctx, h, "capture-pane", "-p", "-S", "-", "-t", target)
			return fmt.Errorf("fake Codex pane %q did not persist PermissionRequest; pane:\n%s", target, output)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// codexTranscriptPathForConversationID mirrors cmd/fake-codex's own
// writeRolloutSessionMeta convention (also internal/agent/codex.go's
// TranscriptPaths, task 015's provenance) against the scenario's own
// fixture HOME (h.agentHOMEDir, which lifecycle_test.go passes as every
// subsequently started named client's HOME=): <HOME>/.codex/sessions/
// <yyyy>/<mm>/<dd>/rollout-<ISO>-<conversationID>.jsonl. It globs on
// everything but the id-bearing filename suffix, since the date directories
// and timestamp prefix are wall-clock-derived and not reproduced here.
func codexTranscriptPathForConversationID(h *ScenarioHarness, conversationID string) (string, error) {
	if h.agentHOMEDir == "" {
		return "", fmt.Errorf("fixture HOME directory was not configured (call the fake codex PATH step first)")
	}
	if conversationID == "" {
		return "", fmt.Errorf("empty conversation id")
	}
	pattern := filepath.Join(h.agentHOMEDir, ".codex", "sessions", "*", "*", "*", "rollout-*-"+conversationID+".jsonl")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return "", fmt.Errorf("glob codex rollout transcript for id %q: %w", conversationID, err)
	}
	if len(matches) != 1 {
		return "", fmt.Errorf("codex rollout transcript glob %q matched %d files, want 1: %v", pattern, len(matches), matches)
	}
	return matches[0], nil
}

// sessionCodexTranscriptDoesNotMentionOthersConversationID resolves name's
// own rollout transcript file (by name's own conversation id, exactly the
// filename cmd/fake-codex minted it under) and asserts other's conversation
// id string never appears anywhere in its bytes -- proving the two
// sessions created in one working directory (this feature's own scenario)
// never cross-wrote or cross-referenced each other's transcript (SPEC
// §13.4's "neither row's id is the other's transcript").
func sessionCodexTranscriptDoesNotMentionOthersConversationID(ctx context.Context, name, other string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	selfID, err := sessionConversationID(h, name)
	if err != nil {
		return err
	}
	if selfID == "" {
		return fmt.Errorf("session %q has no conversation id", name)
	}
	otherID, err := sessionConversationID(h, other)
	if err != nil {
		return err
	}
	if otherID == "" {
		return fmt.Errorf("session %q has no conversation id", other)
	}
	path, err := codexTranscriptPathForConversationID(h, selfID)
	if err != nil {
		return err
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read codex transcript %q for session %q: %w", path, name, err)
	}
	if strings.Contains(string(content), otherID) {
		return fmt.Errorf("session %q's codex transcript %q unexpectedly mentions session %q's conversation id %q", name, path, other, otherID)
	}
	return nil
}
