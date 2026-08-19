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

func realClaudeHookConforms(ctx context.Context, name, event string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	sessionID, err := sessionIDByName(h, name)
	if err != nil {
		return err
	}
	conversationID, err := sessionConversationID(h, name)
	if err != nil {
		return err
	}
	if h.workingDir == "" {
		return fmt.Errorf("real Claude hook contract test has no scenario working directory")
	}

	db, err := openObservedDatabase(h)
	if err != nil {
		return err
	}
	defer db.Close()
	deadline := time.Now().Add(20 * time.Second)
	for {
		var raw string
		err := db.QueryRowContext(ctx, `SELECT payload FROM events WHERE session_id = ? AND kind = 'session_start' ORDER BY id DESC LIMIT 1`, sessionID).Scan(&raw)
		if err == nil {
			var payload map[string]any
			if decodeErr := json.Unmarshal([]byte(raw), &payload); decodeErr != nil {
				return fmt.Errorf("real Claude hook contract unsupported: SessionStart payload is not a JSON object: %w", decodeErr)
			}
			expected := map[string]string{
				"hook_event_name": event,
				"session_id":      conversationID,
				"cwd":             h.workingDir,
			}
			if contractErr := requireRealClaudeHookFields(payload, expected); contractErr != nil {
				return contractErr
			}
			return nil
		}
		if err != sql.ErrNoRows {
			return fmt.Errorf("observe real Claude SessionStart hook: %w", err)
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("real Claude hook contract unsupported: no SessionStart reached deck _hook within 20s (installed CLI may require authentication or no longer accept injected hooks)")
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
