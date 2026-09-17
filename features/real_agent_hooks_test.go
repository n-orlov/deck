package features

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os/exec"
	"reflect"
	"regexp"
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

	registerRealAgentCodexHookSteps(sc)
}

// registerRealAgentCodexHookSteps is real_agent_smoke.feature's codex
// conformance scenario (R127, task 026), following this file's own
// idiom -- opt-in, upstream fields asserted without alias or coercion --
// with exactly the divergences the real CLI forces:
//
//   - codex has no --settings JSON blob; its hook instrumentation is a run
//     of -c "hooks.<Event>=[{hooks=[{type=\"command\",command=\"...\"}]}]"
//     overrides (internal/agent/codex.go's own codexHookOverride, task 017),
//     so the routing check below decodes THAT shape, duplicating
//     cmd/fake-codex's own parseHookOverride black-box (task 020's own
//     comment already establishes this is the accepted idiom here: this
//     package never imports internal/agent or cmd/fake-codex).
//   - codex never mints a conversation id, or fires SessionStart, at launch
//     -- only on the first prompt (SPEC's real capture,
//     docs/reports/codex-cli-0.154.0-spike.md Q3d; task 021/024's own
//     fake-codex mirrors this) -- so the scenario checks the static argv
//     routing right after create, then prompts once before waiting on
//     either hook.
//
// The Given step is the one place this scenario differs on purpose from
// every other @real-agents step in this file: it returns godog.ErrSkip
// (never a failure) when `codex` does not resolve on PATH, after logging
// the stated reason via godog.Log, so an opted-in run on a codex-less host
// (this container, always) reports the scenario skipped, never failed, and
// the ordinary default-tag-filtered run never reaches it at all.
func registerRealAgentCodexHookSteps(sc *godog.ScenarioContext) {
	sc.Step(`^the installed Codex CLI resolves on PATH, or this scenario is skipped with a stated reason$`, installedCodexResolvesOnPATHOrSkipsScenario)
	sc.Step(`^session "([^"]+)"'s launch instrumentation routes "([^"]+)" to the released deck _hook via codex's -c overrides$`, realCodexInstrumentationRoutesHook)
	sc.Step(`^session "([^"]+)" receives a real Codex "([^"]+)" hook$`, realCodexHookDelivered)
	sc.Step(`^session "([^"]+)" submits the prompt "([^"]+)" to real Codex$`, submitRealCodexPrompt)
	sc.Step(`^session "([^"]+)" receives a conforming real Codex "([^"]+)" hook$`, realCodexHookConforms)
}

func installedCodexResolvesOnPATHOrSkipsScenario(ctx context.Context) error {
	if _, err := exec.LookPath("codex"); err != nil {
		godog.Log(ctx, fmt.Sprintf("skipping real Codex conformance scenario: no installed codex CLI on PATH: %v", err))
		return godog.ErrSkip
	}
	return nil
}

// codexHookOverrideValuePattern mirrors cmd/fake-codex's own
// hookOverrideValuePattern (task 020): the exact -c VALUE shape
// internal/agent/codex.go's codexHookOverride produces, a single hook group
// with exactly one command-type hook.
var codexHookOverrideValuePattern = regexp.MustCompile(`^\[\{hooks=\[\{type="command",command="((?:[^"\\]|\\.)*)"\}\]\}\]$`)

// codexHookCommandFromArgv scans launch argv for a `-c hooks.<event>=...`
// override and decodes its command, duplicating cmd/fake-codex's own
// parseHookOverride/codexTOMLUnescapeString black-box rather than importing
// them (that package is `main`, and this package stays import-free of
// internal/agent by its own established convention, task 024).
func codexHookCommandFromArgv(argv []string, event string) (string, bool) {
	for i := 0; i < len(argv)-1; i++ {
		if argv[i] != "-c" {
			continue
		}
		key, value, found := strings.Cut(argv[i+1], "=")
		if !found || key != "hooks."+event {
			continue
		}
		match := codexHookOverrideValuePattern.FindStringSubmatch(value)
		if match == nil {
			continue
		}
		return codexTOMLUnescapeStringForFeature(match[1]), true
	}
	return "", false
}

func codexTOMLUnescapeStringForFeature(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '\\' && i+1 < len(s) {
			i++
			b.WriteByte(s[i])
			continue
		}
		b.WriteByte(c)
	}
	return b.String()
}

func realCodexInstrumentationRoutesHook(ctx context.Context, name, event string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	argv, err := mostRecentLaunchArgvForSession(h, name)
	if err != nil {
		return err
	}
	command, found := codexHookCommandFromArgv(argv, event)
	if !found {
		return fmt.Errorf("real Codex instrumentation contract unsupported: launch argv has no -c hooks.%s override: %q", event, argv)
	}
	wantCommand := shellQuotedForFeature(h.Binary) + " _hook"
	if command != wantCommand {
		return fmt.Errorf("real Codex instrumentation contract unsupported: %s hook command = %q, want %q", event, command, wantCommand)
	}
	return nil
}

func realCodexHookDelivered(ctx context.Context, name, event string) error {
	payload, expected, err := waitForRealCodexHook(ctx, name, event)
	if err != nil {
		return err
	}
	// Mirrors realClaudeHookDelivered's own independent-proof-of-delivery
	// shape: only the fields every codex hook is expected to carry
	// (docs/reports/codex-cli-0.154.0-spike.md Q3c) are required here; the
	// full contract, including codex's own model/prompt fields, is checked
	// on UserPromptSubmit by realCodexHookConforms below.
	for _, field := range []string{"session_id", "cwd", "transcript_path"} {
		value, exists := payload[field]
		text, stringTyped := value.(string)
		if !exists || !stringTyped || text == "" {
			return fmt.Errorf("real Codex hook delivery unsupported: required field %q must be a non-empty string (got %s); payload keys/types: %s", field, describeJSONValue(value, exists), payloadShape(payload))
		}
	}
	for field, want := range expected {
		value, exists := payload[field]
		got, stringTyped := value.(string)
		if !exists || !stringTyped || got != want {
			return fmt.Errorf("real Codex hook delivery unsupported: field %q = %s, want string %q; payload keys/types: %s", field, describeJSONValue(value, exists), want, payloadShape(payload))
		}
	}
	return nil
}

func submitRealCodexPrompt(ctx context.Context, name, prompt string) error {
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
		return fmt.Errorf("send prompt to real Codex pane %q: %w", pane, err)
	}
	if _, err := tmuxOutput(ctx, h, "send-keys", "-t", pane, "Enter"); err != nil {
		return fmt.Errorf("submit prompt to real Codex pane %q: %w", pane, err)
	}
	return nil
}

func realCodexHookConforms(ctx context.Context, name, event string) error {
	payload, expected, err := waitForRealCodexHook(ctx, name, event)
	if err != nil {
		return err
	}
	return requireRealCodexHookFields(payload, expected)
}

func waitForRealCodexHook(ctx context.Context, name, event string) (map[string]any, map[string]string, error) {
	h, err := assertionHarness(ctx)
	if err != nil {
		return nil, nil, err
	}
	sessionID, err := sessionIDByName(h, name)
	if err != nil {
		return nil, nil, err
	}
	if h.workingDir == "" {
		return nil, nil, fmt.Errorf("real Codex hook contract test has no scenario working directory")
	}

	db, err := openObservedDatabase(h)
	if err != nil {
		return nil, nil, err
	}
	defer db.Close()
	// hookrecv's Mappings table (internal/hookrecv/receiver.go) keys the
	// stored event kind on the hook EVENT NAME alone, shared by every
	// agent kind -- codex's SessionStart/UserPromptSubmit land under the
	// identical "session_start"/"user_prompt_submitted" kinds Claude's do.
	kinds := map[string]string{
		"SessionStart":     "session_start",
		"UserPromptSubmit": "user_prompt_submitted",
	}
	kind, supported := kinds[event]
	if !supported {
		return nil, nil, fmt.Errorf("real Codex hook contract test has no stored kind for %s", event)
	}
	deadline := time.Now().Add(20 * time.Second)
	// Codex mints its conversation id, and fires SessionStart, only on the
	// first prompt (SPEC §8.2/R125) -- asynchronously with respect to this
	// helper's own start, since submitRealCodexPrompt only sends keystrokes
	// and does not itself wait on adoption. An initially empty id here is
	// the documented, normal interval before that first hook arrives, not a
	// contract violation: wait for authoritative adoption within the same
	// bounded deadline the hook poll below uses, and derive the expected
	// session_id only once adoption has actually happened.
	var conversationID string
	for {
		conversationID, err = sessionConversationID(h, name)
		if err != nil {
			return nil, nil, err
		}
		if conversationID != "" {
			break
		}
		if time.Now().After(deadline) {
			return nil, nil, fmt.Errorf("real Codex hook contract unsupported: session %q adopted no conversation id within 20s (codex only mints one on first prompt; installed CLI may require authentication, directory trust, or no longer accept injected hooks)", name)
		}
		time.Sleep(50 * time.Millisecond)
	}
	expected := map[string]string{
		"hook_event_name": event,
		"session_id":      conversationID,
		"cwd":             h.workingDir,
	}
	for {
		var raw string
		err := db.QueryRowContext(ctx, `SELECT payload FROM events WHERE session_id = ? AND kind = ? ORDER BY rowid DESC LIMIT 1`, sessionID, kind).Scan(&raw)
		if err == nil {
			// Retain the exact upstream JSON in the opt-in run log as auditable
			// conformance evidence; do not normalize or reconstruct it.
			fmt.Printf("genuine upstream codex %s payload: %s\n", event, raw)
			var payload map[string]any
			if decodeErr := json.Unmarshal([]byte(raw), &payload); decodeErr != nil {
				return nil, nil, fmt.Errorf("real Codex hook contract unsupported: %s payload is not a JSON object: %w", event, decodeErr)
			}
			return payload, expected, nil
		}
		if err != sql.ErrNoRows {
			return nil, nil, fmt.Errorf("observe real Codex %s hook: %w", event, err)
		}
		if time.Now().After(deadline) {
			return nil, nil, fmt.Errorf("real Codex hook contract unsupported: no %s reached deck _hook within 20s (installed CLI may require authentication, directory trust, or no longer accept injected hooks)", event)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// requireRealCodexHookFields checks the full UserPromptSubmit contract
// docs/reports/codex-cli-0.154.0-spike.md Q3c actually captured from a real
// session: session_id/cwd/transcript_path/prompt as non-empty strings, plus
// the exact-match fields waitForRealCodexHook computed. It does not require
// model/permission_mode/turn_id -- those are present in the spike capture
// but unconfirmed as a stable upstream guarantee across codex-cli releases,
// and this scenario's job is to catch a genuinely broken contract, not to
// pin fields no other requirement in this run depends on.
func requireRealCodexHookFields(payload map[string]any, expected map[string]string) error {
	for _, field := range []string{"session_id", "cwd", "transcript_path", "prompt"} {
		value, exists := payload[field]
		text, stringTyped := value.(string)
		if !exists || !stringTyped || text == "" {
			return fmt.Errorf("real Codex hook contract unsupported: required field %q must be a non-empty string (got %s); payload keys/types: %s", field, describeJSONValue(value, exists), payloadShape(payload))
		}
	}
	for field, want := range expected {
		value, exists := payload[field]
		got, stringTyped := value.(string)
		if !exists || !stringTyped || got != want {
			return fmt.Errorf("real Codex hook contract unsupported: field %q = %s, want string %q; payload keys/types: %s", field, describeJSONValue(value, exists), want, payloadShape(payload))
		}
	}
	return nil
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

// TestWaitForRealCodexHookAllowsDelayedFirstHookAdoption drives the actual
// waitForRealCodexHook helper (never a fixture stand-in) against a row that
// starts with no conversation id and only adopts one, alongside its first
// SessionStart hook, after a short nonzero delay -- the documented, normal
// asynchronous interval between codex's first prompt and its first hook
// (SPEC §8.2/R125), not a contract violation. Before this cure the helper
// rejected the initially empty id immediately, before its own bounded
// polling loop ever ran; this regression proves it now waits within that
// same deadline instead, and derives its expected session_id only once
// adoption has actually happened. The already-adopted control call proves
// the ordinary (already-adopted-by-the-time-of-observation) path still
// works unchanged.
func TestWaitForRealCodexHookAllowsDelayedFirstHookAdoption(t *testing.T) {
	h := &ScenarioHarness{Home: t.TempDir(), workingDir: "/real-codex-wait-cwd"}
	db, err := openObservedDatabase(h)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, stmt := range []string{
		`CREATE TABLE sessions (id TEXT, name TEXT, conversation_id TEXT)`,
		`CREATE TABLE events (session_id TEXT, kind TEXT, payload TEXT)`,
		`INSERT INTO sessions VALUES ('deck-row', 'one', '')`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatal(err)
		}
	}
	ctx := context.WithValue(context.Background(), scenarioHarnessKey{}, h)
	rawPayload, err := json.Marshal(map[string]any{
		"hook_event_name": "SessionStart",
		"session_id":      "codex-id",
		"cwd":             h.workingDir,
		"transcript_path": "/real-codex-wait-cwd/rollout-codex-id.jsonl",
	})
	if err != nil {
		t.Fatal(err)
	}

	published := make(chan error, 1)
	go func() {
		time.Sleep(200 * time.Millisecond)
		if _, err := db.Exec(`UPDATE sessions SET conversation_id='codex-id' WHERE id='deck-row'`); err != nil {
			published <- err
			return
		}
		_, err := db.Exec(`INSERT INTO events VALUES ('deck-row','session_start',?)`, string(rawPayload))
		published <- err
	}()

	start := time.Now()
	gotPayload, expected, waitErr := waitForRealCodexHook(ctx, "one", "SessionStart")
	elapsed := time.Since(start)
	if err := <-published; err != nil {
		t.Fatalf("fixture setup: %v", err)
	}
	if waitErr != nil {
		t.Fatalf("wait rejected before an on-time first hook: elapsed=%s error=%v; hook was published after 200ms", elapsed, waitErr)
	}
	if elapsed < 150*time.Millisecond {
		t.Fatalf("wait returned before the delayed hook could plausibly have arrived: elapsed=%s", elapsed)
	}
	if want := "codex-id"; expected["session_id"] != want {
		t.Fatalf("expected session_id = %q, want %q (must be derived after adoption)", expected["session_id"], want)
	}
	if got := gotPayload["session_id"]; got != "codex-id" {
		t.Fatalf("payload session_id = %v, want %q", got, "codex-id")
	}

	// Already-adopted control: once conversation id and hook are already
	// durable, the same helper must still return them immediately.
	if _, _, err := waitForRealCodexHook(ctx, "one", "SessionStart"); err != nil {
		t.Fatalf("already-adopted control rejected: %v", err)
	}
}
