package features

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/cucumber/godog"
)

// registerRealAgentHookSteps is intentionally registered in every run so the
// @real-agents feature is still parsed strictly, while the default tag filter
// prevents it from starting an installed CLI. These assertions do not adapt or
// normalize upstream payloads: an incompatible Claude upgrade must fail with a
// contract diagnostic.
func registerRealAgentHookSteps(sc *godog.ScenarioContext) {
	sc.Step(`^session "([^"]+)"'s launch instrumentation routes "([^"]+)" to the released deck _hook$`, realClaudeInstrumentationRoutesHook)
	sc.Step(`^session "([^"]+)" receives a real Claude "([^"]+)" hook$`, realClaudeHookDelivered)
	sc.Step(`^session "([^"]+)" submits the prompt "([^"]+)" to real Claude$`, submitRealClaudePrompt)
	sc.Step(`^session "([^"]+)" receives a conforming real Claude "([^"]+)" hook$`, realClaudeHookConforms)
}

func realClaudeInstrumentationRoutesHook(ctx context.Context, name, event string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	argv, err := mostRecentLaunchArgvForSession(h, name)
	if err != nil {
		return err
	}
	var rawSettings string
	for i := 0; i < len(argv)-1; i++ {
		if argv[i] == "--settings" {
			rawSettings = argv[i+1]
			break
		}
	}
	if rawSettings == "" {
		return fmt.Errorf("real Claude instrumentation contract unsupported: launch argv has no --settings value: %q", argv)
	}
	var settings struct {
		Hooks map[string][]struct {
			Hooks []struct {
				Type    string `json:"type"`
				Command string `json:"command"`
			} `json:"hooks"`
		} `json:"hooks"`
	}
	if err := json.Unmarshal([]byte(rawSettings), &settings); err != nil {
		return fmt.Errorf("real Claude instrumentation contract unsupported: decode --settings: %w", err)
	}
	groups := settings.Hooks[event]
	if len(groups) != 1 || len(groups[0].Hooks) != 1 {
		return fmt.Errorf("real Claude instrumentation contract unsupported: %s has %d groups with hook counts %v, want one command hook", event, len(groups), hookCounts(groups))
	}
	hook := groups[0].Hooks[0]
	wantCommand := shellQuotedForFeature(h.Binary) + " _hook"
	if hook.Type != "command" || hook.Command != wantCommand {
		return fmt.Errorf("real Claude instrumentation contract unsupported: %s hook = type %q command %q, want command %q", event, hook.Type, hook.Command, wantCommand)
	}
	return nil
}

func hookCounts(groups []struct {
	Hooks []struct {
		Type    string `json:"type"`
		Command string `json:"command"`
	} `json:"hooks"`
}) []int {
	counts := make([]int, len(groups))
	for i := range groups {
		counts[i] = len(groups[i].Hooks)
	}
	return counts
}

func shellQuotedForFeature(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func realClaudeHookDelivered(ctx context.Context, name, event string) error {
	payload, expected, err := waitForRealClaudeHook(ctx, name, event)
	if err != nil {
		return err
	}
	// SessionStart in current genuine Claude versions omits permission_mode.
	// Keep independent proof that the injected SessionStart hook reached deck,
	// while the full common-payload contract is checked on UserPromptSubmit.
	for _, field := range []string{"session_id", "cwd", "transcript_path"} {
		value, exists := payload[field]
		text, stringTyped := value.(string)
		if !exists || !stringTyped || text == "" {
			return fmt.Errorf("real Claude hook delivery unsupported: required field %q must be a non-empty string (got %s); payload keys/types: %s", field, describeJSONValue(value, exists), payloadShape(payload))
		}
	}
	for field, want := range expected {
		value, exists := payload[field]
		got, stringTyped := value.(string)
		if !exists || !stringTyped || got != want {
			return fmt.Errorf("real Claude hook delivery unsupported: field %q = %s, want string %q; payload keys/types: %s", field, describeJSONValue(value, exists), want, payloadShape(payload))
		}
	}
	return nil
}

func submitRealClaudePrompt(ctx context.Context, name, prompt string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	slug, err := sessionSlugByName(h, name)
	if err != nil {
		return err
	}
	pane := "deck_" + slug
	if _, err := tmuxOutput(ctx, h, "send-keys", "-t", pane, "-l", prompt); err != nil {
		return fmt.Errorf("send prompt to real Claude pane %q: %w", pane, err)
	}
	if _, err := tmuxOutput(ctx, h, "send-keys", "-t", pane, "Enter"); err != nil {
		return fmt.Errorf("submit prompt to real Claude pane %q: %w", pane, err)
	}
	return nil
}

func realClaudeHookConforms(ctx context.Context, name, event string) error {
	payload, expected, err := waitForRealClaudeHook(ctx, name, event)
	if err != nil {
		return err
	}
	return requireRealClaudeHookFields(payload, expected)
}

func waitForRealClaudeHook(ctx context.Context, name, event string) (map[string]any, map[string]string, error) {
	h, err := assertionHarness(ctx)
	if err != nil {
		return nil, nil, err
	}
	sessionID, err := sessionIDByName(h, name)
	if err != nil {
		return nil, nil, err
	}
	conversationID, err := sessionConversationID(h, name)
	if err != nil {
		return nil, nil, err
	}
	if h.workingDir == "" {
		return nil, nil, fmt.Errorf("real Claude hook contract test has no scenario working directory")
	}
	expected := map[string]string{
		"hook_event_name": event,
		"session_id":      conversationID,
		"cwd":             h.workingDir,
	}

	db, err := openObservedDatabase(h)
	if err != nil {
		return nil, nil, err
	}
	defer db.Close()
	kinds := map[string]string{
		"SessionStart":     "session_start",
		"UserPromptSubmit": "user_prompt_submitted",
	}
	kind, supported := kinds[event]
	if !supported {
		return nil, nil, fmt.Errorf("real Claude hook contract test has no stored kind for %s", event)
	}
	deadline := time.Now().Add(20 * time.Second)
	for {
		var raw string
		err := db.QueryRowContext(ctx, `SELECT payload FROM events WHERE session_id = ? AND kind = ? ORDER BY rowid DESC LIMIT 1`, sessionID, kind).Scan(&raw)
		if err == nil {
			// Retain the exact upstream JSON in the opt-in run log as auditable
			// conformance evidence; do not normalize or reconstruct it.
			fmt.Printf("genuine upstream %s payload: %s\n", event, raw)
			var payload map[string]any
			if decodeErr := json.Unmarshal([]byte(raw), &payload); decodeErr != nil {
				return nil, nil, fmt.Errorf("real Claude hook contract unsupported: %s payload is not a JSON object: %w", event, decodeErr)
			}
			return payload, expected, nil
		}
		if err != sql.ErrNoRows {
			return nil, nil, fmt.Errorf("observe real Claude %s hook: %w", event, err)
		}
		if time.Now().After(deadline) {
			return nil, nil, fmt.Errorf("real Claude hook contract unsupported: no %s reached deck _hook within 20s (installed CLI may require authentication or no longer accept injected hooks)", event)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func requireRealClaudeHookFields(payload map[string]any, expected map[string]string) error {
	for _, field := range []string{"session_id", "cwd", "transcript_path", "permission_mode"} {
		value, exists := payload[field]
		text, stringTyped := value.(string)
		if !exists || !stringTyped || text == "" {
			return fmt.Errorf("real Claude hook contract unsupported: required field %q must be a non-empty string (got %s); payload keys/types: %s", field, describeJSONValue(value, exists), payloadShape(payload))
		}
	}
	for field, want := range expected {
		value, exists := payload[field]
		got, stringTyped := value.(string)
		if !exists || !stringTyped || got != want {
			return fmt.Errorf("real Claude hook contract unsupported: field %q = %s, want string %q; payload keys/types: %s", field, describeJSONValue(value, exists), want, payloadShape(payload))
		}
	}
	return nil
}

func describeJSONValue(value any, exists bool) string {
	if !exists {
		return "<missing>"
	}
	return fmt.Sprintf("%s(%v)", reflect.TypeOf(value), value)
}

func payloadShape(payload map[string]any) string {
	parts := make([]string, 0, len(payload))
	for key, value := range payload {
		parts = append(parts, fmt.Sprintf("%s:%T", key, value))
	}
	sort.Strings(parts)
	return strings.Join(parts, ", ")
}

func TestRequireRealClaudeHookFieldsIsStrictAboutUpstreamContract(t *testing.T) {
	valid := map[string]any{
		"hook_event_name": "SessionStart",
		"session_id":      "conversation-1",
		"cwd":             "/work",
		"transcript_path": "/home/user/.claude/projects/work/conversation-1.jsonl",
		"permission_mode": "manual",
	}
	expected := map[string]string{
		"hook_event_name": "SessionStart",
		"session_id":      "conversation-1",
		"cwd":             "/work",
	}
	if err := requireRealClaudeHookFields(valid, expected); err != nil {
		t.Fatalf("valid contract rejected: %v", err)
	}

	wrongType := mapsClone(valid)
	wrongType["permission_mode"] = 1.0
	if err := requireRealClaudeHookFields(wrongType, expected); err == nil || !strings.Contains(err.Error(), `required field "permission_mode" must be a non-empty string`) {
		t.Fatalf("wrong field type error = %v, want explicit unsupported-contract diagnostic", err)
	}

	aliased := mapsClone(valid)
	delete(aliased, "transcript_path")
	aliased["transcriptPath"] = valid["transcript_path"]
	if err := requireRealClaudeHookFields(aliased, expected); err == nil || !strings.Contains(err.Error(), `required field "transcript_path"`) {
		t.Fatalf("aliased field error = %v, want exact upstream name to be required", err)
	}
}

func mapsClone(source map[string]any) map[string]any {
	clone := make(map[string]any, len(source))
	for key, value := range source {
		clone[key] = value
	}
	return clone
}
