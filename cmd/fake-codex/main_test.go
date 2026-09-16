package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/n-orlov/deck/internal/agent"
)

// TestRejectsSessionIDAndFullAuto proves both Claude-shaped flags codex-cli
// 0.154.0 does not have are rejected outright (SPEC §5): codex mints its own
// conversation id, and --full-auto was removed upstream.
func TestRejectsSessionIDAndFullAuto(t *testing.T) {
	for name, arguments := range map[string][]string{
		"--session-id": {"--session-id", "123e4567-e89b-12d3-a456-426614174000"},
		"--full-auto":  {"--full-auto"},
	} {
		t.Run(name, func(t *testing.T) {
			var output bytes.Buffer
			if _, err := run(arguments, &output, func(string) string { return "" }); err == nil {
				t.Fatalf("run(%q) unexpectedly succeeded", arguments)
			}
		})
	}
}

// TestAcceptsResumeAsPositionalSubcommand proves `resume <id>` works as a
// positional subcommand -- codex has no --resume flag at all -- and that it
// is only recognised as the very first argument.
func TestAcceptsResumeAsPositionalSubcommand(t *testing.T) {
	var output bytes.Buffer
	code, err := run([]string{"resume", "some-conversation-id", "-a", "never"}, &output, func(string) string { return "" })
	if err != nil || code != 0 {
		t.Fatalf("run = (%d, %v)", code, err)
	}
	if !strings.Contains(output.String(), "fake-codex resume: some-conversation-id") {
		t.Fatalf("output %q does not record the resumed id", output.String())
	}

	// "resume" is not treated as a subcommand once other tokens precede it;
	// it lands as ordinary trailing prompt text instead.
	var later bytes.Buffer
	if _, err := run([]string{"-a", "never", "resume", "not-a-subcommand-here"}, &later, func(string) string { return "" }); err != nil {
		t.Fatalf("run: %v", err)
	}
	if strings.Contains(later.String(), "fake-codex resume:") {
		t.Fatalf("output %q treated a non-leading \"resume\" as the subcommand", later.String())
	}

	if _, err := run([]string{"resume"}, &output, func(string) string { return "" }); err == nil {
		t.Fatal(`"resume" with no id unexpectedly succeeded`)
	}
}

// TestAcceptsOnlyDocumentedApprovalValues proves -a accepts exactly
// on-request and never, and rejects anything else -- including the older
// untrusted/on-failure values that codex-cli 0.154.0 removed.
func TestAcceptsOnlyDocumentedApprovalValues(t *testing.T) {
	for _, value := range []string{"on-request", "never"} {
		t.Run("accepts "+value, func(t *testing.T) {
			var output bytes.Buffer
			if code, err := run([]string{"-a", value}, &output, func(string) string { return "" }); err != nil || code != 0 {
				t.Fatalf("run = (%d, %v)", code, err)
			}
			if !strings.Contains(output.String(), "fake-codex ask-for-approval: "+value) {
				t.Fatalf("output %q does not record -a %s", output.String(), value)
			}
		})
	}
	for _, value := range []string{"untrusted", "on-failure", "always"} {
		t.Run("rejects "+value, func(t *testing.T) {
			var output bytes.Buffer
			if _, err := run([]string{"-a", value}, &output, func(string) string { return "" }); err == nil {
				t.Fatalf("run(-a %s) unexpectedly succeeded", value)
			}
		})
	}
}

// TestAcceptsOnlyDocumentedSandboxValues proves -s accepts exactly the three
// codex-cli 0.154.0 sandbox modes and rejects anything else.
func TestAcceptsOnlyDocumentedSandboxValues(t *testing.T) {
	for _, value := range []string{"read-only", "workspace-write", "danger-full-access"} {
		t.Run("accepts "+value, func(t *testing.T) {
			var output bytes.Buffer
			if code, err := run([]string{"-s", value}, &output, func(string) string { return "" }); err != nil || code != 0 {
				t.Fatalf("run = (%d, %v)", code, err)
			}
			if !strings.Contains(output.String(), "fake-codex sandbox: "+value) {
				t.Fatalf("output %q does not record -s %s", output.String(), value)
			}
		})
	}
	for _, value := range []string{"full-access", "none", "danger-full-access-plus"} {
		t.Run("rejects "+value, func(t *testing.T) {
			var output bytes.Buffer
			if _, err := run([]string{"-s", value}, &output, func(string) string { return "" }); err == nil {
				t.Fatalf("run(-s %s) unexpectedly succeeded", value)
			}
		})
	}
}

// TestAcceptsRepeatedConfigOverrides proves -c is accepted repeatedly, never
// deduplicated or rejected on a repeated key.
func TestAcceptsRepeatedConfigOverrides(t *testing.T) {
	arguments := []string{
		"-c", "model=gpt-5",
		"-c", "model=gpt-5-again",
		"-c", `hooks.SessionStart=[{hooks=[{type="command",command="echo hi"}]}]`,
	}
	parsed, err := parse(arguments)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(parsed.configOverrides) != 3 {
		t.Fatalf("configOverrides = %v, want 3 entries", parsed.configOverrides)
	}
	if parsed.configOverrides[0] != "model=gpt-5" || parsed.configOverrides[1] != "model=gpt-5-again" {
		t.Fatalf("configOverrides = %v, want both repeated model= values preserved in order", parsed.configOverrides)
	}
}

// TestParseHookOverrideRoundTripsAgainstTheRealEncoder proves this fixture's
// decoder correctly reverses internal/agent/codex.go's own -c value shape,
// including a command carrying a backslash and a quote.
func TestParseHookOverrideRoundTripsAgainstTheRealEncoder(t *testing.T) {
	for name, command := range map[string]string{
		"plain path":          `/usr/local/bin/deck _hook`,
		"space in path":       `/opt/My Deck/deck _hook`,
		"quote and backslash": `/opt/"weird"\deck _hook`,
	} {
		t.Run(name, func(t *testing.T) {
			escaped := strings.ReplaceAll(command, `\`, `\\`)
			escaped = strings.ReplaceAll(escaped, `"`, `\"`)
			raw := `hooks.SessionStart=[{hooks=[{type="command",command="` + escaped + `"}]}]`
			event, decoded, ok := parseHookOverride(raw)
			if !ok {
				t.Fatalf("parseHookOverride(%q) declined to match", raw)
			}
			if event != "SessionStart" {
				t.Fatalf("event = %q, want SessionStart", event)
			}
			if decoded != command {
				t.Fatalf("decoded command = %q, want %q", decoded, command)
			}
		})
	}

	if _, _, ok := parseHookOverride("model=gpt-5"); ok {
		t.Fatal("a non-hooks -c override was unexpectedly decoded as a hook")
	}
}

// TestHookTrustGateFiresOnlyWhenBypassFlagIsPresent is the trust gate's own
// contract test, in both directions (SPEC §8.2): without
// --dangerously-bypass-hook-trust, a pane "hook" command is a complete,
// silent no-op -- no subprocess runs, no output, no error, exit 0. With the
// flag present, the same command actually runs the injected command.
func TestHookTrustGateFiresOnlyWhenBypassFlagIsPresent(t *testing.T) {
	directory := t.TempDir()
	record := filepath.Join(directory, "hook-record.txt")
	hook := filepath.Join(directory, "injected-hook")
	script := fmt.Sprintf("#!/bin/sh\ncat >> %q\n", record)
	if err := os.WriteFile(hook, []byte(script), 0o755); err != nil {
		t.Fatalf("write hook recorder: %v", err)
	}

	override := fmt.Sprintf(`hooks.SessionStart=[{hooks=[{type="command",command="%s"}]}]`, strings.ReplaceAll(hook, `\`, `\\`))
	input := `{"command":"hook","event":"SessionStart","payload":{"session_id":"abc"}}` + "\n"
	getenv := func(key string) string {
		if key == commandsEnvironment {
			return "1"
		}
		return ""
	}

	t.Run("untrusted: silent no-op", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code, err := runWithIO([]string{"-c", override}, strings.NewReader(input), &stdout, &stderr, getenv)
		if err != nil || code != 0 {
			t.Fatalf("runWithIO = (%d, %v), stderr %q", code, err, stderr.String())
		}
		if strings.Contains(stdout.String(), "hook fired") {
			t.Fatalf("untrusted invocation reported a fired hook: %q", stdout.String())
		}
		if stderr.String() != "" {
			t.Fatalf("untrusted invocation warned on stderr: %q", stderr.String())
		}
		if _, err := os.Stat(record); !os.IsNotExist(err) {
			t.Fatalf("untrusted invocation ran the injected hook (record exists, err=%v)", err)
		}
	})

	t.Run("trusted: fires", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code, err := runWithIO([]string{"-c", override, dangerouslyBypassHookTrustFlag}, strings.NewReader(input), &stdout, &stderr, getenv)
		if err != nil || code != 0 {
			t.Fatalf("runWithIO = (%d, %v), stderr %q", code, err, stderr.String())
		}
		if !strings.Contains(stdout.String(), "fake-codex hook fired: SessionStart") {
			t.Fatalf("trusted invocation did not acknowledge the fired hook: %q", stdout.String())
		}
		recorded, err := os.ReadFile(record)
		if err != nil {
			t.Fatalf("read hook record: %v", err)
		}
		if !strings.Contains(string(recorded), `"session_id":"abc"`) {
			t.Fatalf("hook record %q does not carry the payload", recorded)
		}
		if !strings.Contains(string(recorded), `"hook_event_name":"SessionStart"`) {
			t.Fatalf("hook record %q does not carry hook_event_name", recorded)
		}
	})
}

// TestPaneHookCommandRequiresInjectedOverrideEvenWhenTrusted proves that
// trust alone is not enough: an event that was never injected via -c still
// errors, even with --dangerously-bypass-hook-trust present.
func TestPaneHookCommandRequiresInjectedOverrideEvenWhenTrusted(t *testing.T) {
	input := `{"command":"hook","event":"Stop","payload":{}}` + "\n"
	getenv := func(key string) string {
		if key == commandsEnvironment {
			return "1"
		}
		return ""
	}
	var stdout, stderr bytes.Buffer
	_, err := runWithIO([]string{dangerouslyBypassHookTrustFlag}, strings.NewReader(input), &stdout, &stderr, getenv)
	if err == nil || !strings.Contains(err.Error(), `was not injected via -c`) {
		t.Fatalf("runWithIO error = %v, want missing injected override", err)
	}
}

// TestExitCodeIsControlledOnlyByFixtureEnvironment mirrors fake-claude's own
// exit-code contract test.
func TestExitCodeIsControlledOnlyByFixtureEnvironment(t *testing.T) {
	var output bytes.Buffer
	getenv := func(key string) string {
		if key == exitCodeEnvironment {
			return "17"
		}
		return ""
	}
	if code, err := run(nil, &output, getenv); err != nil || code != 17 {
		t.Fatalf("run = (%d, %v), want (17, nil)", code, err)
	}
	if _, err := configuredExitCode("126"); err == nil {
		t.Fatal("out-of-range fixture exit code was accepted")
	}
}

// TestUnknownOptionIsRejected proves an arbitrary unrecognised flag is
// rejected the same way --session-id/--full-auto are, rather than silently
// accepted or treated as a positional prompt.
func TestUnknownOptionIsRejected(t *testing.T) {
	var output bytes.Buffer
	if _, err := run([]string{"--not-a-real-flag", "value"}, &output, func(string) string { return "" }); err == nil {
		t.Fatal("run with an unknown flag unexpectedly succeeded")
	}
}

// --- Task 021: session lifecycle, rollout transcript and pane text ---

// recorderOverride returns a -c hooks.<event>=... override, in the exact
// shape internal/agent/codex.go's own codexHookOverride produces, pointing
// at a script that appends every payload it receives (one JSON object per
// line) to recordPath.
func recorderOverride(t *testing.T, event, recordPath string) string {
	t.Helper()
	script := "cat >> " + recordPath
	escaped := strings.ReplaceAll(script, `\`, `\\`)
	escaped = strings.ReplaceAll(escaped, `"`, `\"`)
	return fmt.Sprintf(`hooks.%s=[{hooks=[{type="command",command="%s"}]}]`, event, escaped)
}

// allFiveEventOverrides returns one recorderOverride per codexHookEvents
// entry, all appending to the same recordPath -- so the record file ends
// up with one line per fired event, in fire order.
func allFiveEventOverrides(t *testing.T, recordPath string) []string {
	t.Helper()
	events := []string{"SessionStart", "UserPromptSubmit", "PermissionRequest", "Stop", "SessionEnd"}
	overrides := make([]string, 0, len(events))
	for _, event := range events {
		overrides = append(overrides, recorderOverride(t, event, recordPath))
	}
	return overrides
}

// recordedEvents reads recordPath (if it exists at all) and decodes each
// line's hook_event_name, in the order the events actually fired.
func recordedEvents(t *testing.T, recordPath string) []map[string]any {
	t.Helper()
	data, err := os.ReadFile(recordPath)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatalf("read record: %v", err)
	}
	var events []map[string]any
	for _, line := range strings.Split(strings.TrimRight(string(data), "\n"), "\n") {
		if line == "" {
			continue
		}
		var payload map[string]any
		if err := json.Unmarshal([]byte(line), &payload); err != nil {
			t.Fatalf("decode recorded payload %q: %v", line, err)
		}
		events = append(events, payload)
	}
	return events
}

// TestSessionStartNeverFiresAtLaunchOnlyAfterTheFirstPrompt is requirement
// 126's own timing contract (Q3d, docs/reports/codex-cli-0.154.0-spike.md):
// "a freshly launched, never-prompted codex TUI is completely
// uninstrumented". Nothing fires merely from starting this fixture up and
// entering the pane-command loop; SessionStart fires only once a "prompt"
// command arrives.
func TestSessionStartNeverFiresAtLaunchOnlyAfterTheFirstPrompt(t *testing.T) {
	directory := t.TempDir()
	recordPath := filepath.Join(directory, "record.jsonl")
	override := recorderOverride(t, "SessionStart", recordPath)
	getenv := func(key string) string {
		if key == commandsEnvironment {
			return "1"
		}
		return ""
	}

	t.Run("nothing fires at launch", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		// Empty input: the loop reaches immediate EOF without any command
		// ever arriving, exactly like a launched-but-never-prompted session.
		if _, err := runWithIO([]string{"-c", override, dangerouslyBypassHookTrustFlag}, strings.NewReader(""), &stdout, &stderr, getenv); err != nil {
			t.Fatalf("runWithIO: %v", err)
		}
		if _, err := os.Stat(recordPath); !os.IsNotExist(err) {
			t.Fatalf("SessionStart fired at launch (record exists, err=%v)", err)
		}
	})

	t.Run("fires after one prompt", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		input := `{"command":"prompt","text":"hello"}` + "\n"
		userPromptOverride := recorderOverride(t, "UserPromptSubmit", recordPath)
		if _, err := runWithIO([]string{"-c", override, "-c", userPromptOverride, dangerouslyBypassHookTrustFlag}, strings.NewReader(input), &stdout, &stderr, getenv); err != nil {
			t.Fatalf("runWithIO: %v, stderr=%q", err, stderr.String())
		}
		events := recordedEvents(t, recordPath)
		if len(events) != 2 || events[0]["hook_event_name"] != "SessionStart" || events[1]["hook_event_name"] != "UserPromptSubmit" {
			t.Fatalf("recorded events = %#v, want SessionStart then UserPromptSubmit", events)
		}
	})
}

// TestSessionLifecycleFiresAllFiveEventsWithRealFieldNames drives
// "prompt"/"permission"/"stop"/"exit" pane commands and proves all five
// codexHookEvents fire, in order, each carrying the real field names
// codex-cli 0.154.0 actually sends (docs/reports/codex-cli-0.154.0-spike.md
// Q3c / internal/hookrecv/receiver_codex_test.go's own corpus): session_id,
// cwd and hook_event_name on every event, plus source (SessionStart),
// prompt (UserPromptSubmit), tool_name (PermissionRequest),
// last_assistant_message (Stop) and reason (SessionEnd).
func TestSessionLifecycleFiresAllFiveEventsWithRealFieldNames(t *testing.T) {
	directory := t.TempDir()
	recordPath := filepath.Join(directory, "record.jsonl")
	overrides := allFiveEventOverrides(t, recordPath)
	args := []string{dangerouslyBypassHookTrustFlag}
	for _, override := range overrides {
		args = append(args, "-c", override)
	}
	getenv := func(key string) string {
		if key == commandsEnvironment {
			return "1"
		}
		return ""
	}
	input := strings.Join([]string{
		`{"command":"prompt","text":"summarise this directory"}`,
		`{"command":"permission","tool_name":"Bash"}`,
		`{"command":"stop","message":"All done."}`,
		`{"command":"exit"}`,
	}, "\n") + "\n"

	var stdout, stderr bytes.Buffer
	if _, err := runWithIO(args, strings.NewReader(input), &stdout, &stderr, getenv); err != nil {
		t.Fatalf("runWithIO: %v, stderr=%q", err, stderr.String())
	}

	events := recordedEvents(t, recordPath)
	wantNames := []string{"SessionStart", "UserPromptSubmit", "PermissionRequest", "Stop", "SessionEnd"}
	if len(events) != len(wantNames) {
		t.Fatalf("recorded %d events, want %d: %#v", len(events), len(wantNames), events)
	}
	var sessionID string
	for index, event := range events {
		if event["hook_event_name"] != wantNames[index] {
			t.Fatalf("event[%d] hook_event_name = %v, want %v", index, event["hook_event_name"], wantNames[index])
		}
		id, _ := event["session_id"].(string)
		if id == "" {
			t.Fatalf("event[%d] %#v has no session_id", index, event)
		}
		if _, err := uuid.Parse(id); err != nil {
			t.Fatalf("event[%d] session_id %q is not a uuid: %v", index, id, err)
		}
		if sessionID == "" {
			sessionID = id
		} else if id != sessionID {
			t.Fatalf("event[%d] session_id %q, want the same id as every other event (%q)", index, id, sessionID)
		}
		if cwd, _ := event["cwd"].(string); cwd == "" {
			t.Fatalf("event[%d] %#v has no cwd", index, event)
		}
	}
	if events[0]["source"] != "startup" {
		t.Fatalf("SessionStart source = %v, want startup", events[0]["source"])
	}
	if events[1]["prompt"] != "summarise this directory" {
		t.Fatalf("UserPromptSubmit prompt = %v", events[1]["prompt"])
	}
	if events[2]["tool_name"] != "Bash" {
		t.Fatalf("PermissionRequest tool_name = %v", events[2]["tool_name"])
	}
	if events[3]["last_assistant_message"] != "All done." {
		t.Fatalf("Stop last_assistant_message = %v", events[3]["last_assistant_message"])
	}
	if events[4]["reason"] != "other" {
		t.Fatalf("SessionEnd reason = %v", events[4]["reason"])
	}

	want := fmt.Sprintf("fake-codex session-id: %s\n", sessionID)
	if !strings.Contains(stdout.String(), want) {
		t.Fatalf("stdout %q does not announce the minted session id", stdout.String())
	}
}

// TestMintsADistinctUUIDPerInvocation proves this fixture mints its own
// fresh conversation id every ordinary (non-resume) invocation, never a
// fixed or caller-supplied one.
func TestMintsADistinctUUIDPerInvocation(t *testing.T) {
	getenv := func(key string) string {
		if key == commandsEnvironment {
			return "1"
		}
		return ""
	}
	input := `{"command":"prompt","text":"hi"}` + "\n"
	ids := make(map[string]bool)
	for i := 0; i < 3; i++ {
		var stdout bytes.Buffer
		if _, err := runWithIO(nil, strings.NewReader(input), &stdout, io.Discard, getenv); err != nil {
			t.Fatalf("runWithIO: %v", err)
		}
		id := sessionIDFromOutput(t, stdout.String())
		if _, err := uuid.Parse(id); err != nil {
			t.Fatalf("minted id %q is not a uuid: %v", id, err)
		}
		if ids[id] {
			t.Fatalf("minted id %q repeated across invocations", id)
		}
		ids[id] = true
	}
}

// sessionIDFromOutput extracts the id submitPrompt announced on stdout.
func sessionIDFromOutput(t *testing.T, output string) string {
	t.Helper()
	for _, line := range strings.Split(output, "\n") {
		if id, ok := strings.CutPrefix(line, "fake-codex session-id: "); ok {
			return id
		}
	}
	t.Fatalf("output %q does not announce a minted session id", output)
	return ""
}

// TestResumeReusesTheGivenIDWithSourceResume proves `resume <id>` never
// mints a fresh id: the first prompt's SessionStart carries the resumed id
// unchanged and source "resume", exactly matching Q4's real capture.
func TestResumeReusesTheGivenIDWithSourceResume(t *testing.T) {
	directory := t.TempDir()
	recordPath := filepath.Join(directory, "record.jsonl")
	override := recorderOverride(t, "SessionStart", recordPath)
	userPromptOverride := recorderOverride(t, "UserPromptSubmit", recordPath)
	getenv := func(key string) string {
		if key == commandsEnvironment {
			return "1"
		}
		return ""
	}
	const resumedID = "01a09616-150d-7252-959c-d72a289dae41"
	input := `{"command":"prompt","text":"continue"}` + "\n"

	var stdout, stderr bytes.Buffer
	args := []string{"resume", resumedID, "-c", override, "-c", userPromptOverride, dangerouslyBypassHookTrustFlag}
	if _, err := runWithIO(args, strings.NewReader(input), &stdout, &stderr, getenv); err != nil {
		t.Fatalf("runWithIO: %v, stderr=%q", err, stderr.String())
	}
	events := recordedEvents(t, recordPath)
	if len(events) != 2 || events[0]["hook_event_name"] != "SessionStart" {
		t.Fatalf("recorded events = %#v, want SessionStart then UserPromptSubmit", events)
	}
	if events[0]["session_id"] != resumedID {
		t.Fatalf("session_id = %v, want the resumed id %q", events[0]["session_id"], resumedID)
	}
	if events[0]["source"] != "resume" {
		t.Fatalf("source = %v, want resume", events[0]["source"])
	}
}

// TestRolloutTranscriptIsWrittenAndLocatedByTranscriptPaths proves this
// fixture writes the real on-disk convention internal/agent/codex.go's own
// TranscriptPaths locates -- <CODEX_HOME>/sessions/<yyyy>/<mm>/<dd>/
// rollout-<ISO>-<id>.jsonl, first line a session_meta object carrying
// session_id and cwd -- and that task 015's own TranscriptPaths finds it
// given nothing but the minted id and CODEX_HOME.
func TestRolloutTranscriptIsWrittenAndLocatedByTranscriptPaths(t *testing.T) {
	codexHome := t.TempDir()
	getenv := func(key string) string {
		switch key {
		case commandsEnvironment:
			return "1"
		case "CODEX_HOME":
			return codexHome
		}
		return ""
	}
	input := `{"command":"prompt","text":"hello"}` + "\n"

	var stdout bytes.Buffer
	if _, err := runWithIO(nil, strings.NewReader(input), &stdout, io.Discard, getenv); err != nil {
		t.Fatalf("runWithIO: %v", err)
	}
	id := sessionIDFromOutput(t, stdout.String())

	wantCWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("os.Getwd: %v", err)
	}

	path, ok := agent.NewCodex().TranscriptPaths(agent.TranscriptInput{CodexHome: codexHome, ConversationID: id})
	if !ok {
		t.Fatalf("TranscriptPaths did not locate the rollout file for id %q under %q", id, codexHome)
	}
	relative, err := filepath.Rel(codexHome, path)
	if err != nil {
		t.Fatalf("filepath.Rel: %v", err)
	}
	if !strings.HasPrefix(relative, "sessions"+string(filepath.Separator)) {
		t.Fatalf("located path %q is not under sessions/", path)
	}
	if got := filepath.Base(filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(path))))); got != "sessions" {
		t.Fatalf("path %q is not three date levels below sessions/ (yyyy/mm/dd)", path)
	}
	name := filepath.Base(path)
	if !strings.HasPrefix(name, "rollout-") || !strings.HasSuffix(name, "-"+id+".jsonl") {
		t.Fatalf("filename %q does not match rollout-<ISO>-<id>.jsonl", name)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read rollout file: %v", err)
	}
	firstLine, _, _ := strings.Cut(string(data), "\n")
	var meta struct {
		Type      string `json:"type"`
		SessionID string `json:"session_id"`
		CWD       string `json:"cwd"`
	}
	if err := json.Unmarshal([]byte(firstLine), &meta); err != nil {
		t.Fatalf("decode first line %q: %v", firstLine, err)
	}
	if meta.Type != "session_meta" || meta.SessionID != id || meta.CWD != wantCWD {
		t.Fatalf("first line = %#v, want session_meta with session_id %q and cwd %q", meta, id, wantCWD)
	}
}

// TestPaneStateCommandsMatchEveryCodexProbeVerdict proves the "state" pane
// command produces text internal/agent's real codex probe rules classify
// exactly as named -- starting, running, waiting, idle and error -- fitted
// to the same markers as the real corpus in
// internal/agent/testdata/probes/codex/ (task 019), not an invented rule.
func TestPaneStateCommandsMatchEveryCodexProbeVerdict(t *testing.T) {
	codex := agent.NewCodex()
	for _, name := range []string{"starting", "running", "waiting", "idle", "error"} {
		t.Run(name, func(t *testing.T) {
			getenv := func(key string) string {
				if key == commandsEnvironment {
					return "1"
				}
				return ""
			}
			input := fmt.Sprintf(`{"command":"state","name":%q}`, name) + "\n"
			var stdout bytes.Buffer
			if _, err := runWithIO(nil, strings.NewReader(input), &stdout, io.Discard, getenv); err != nil {
				t.Fatalf("runWithIO: %v", err)
			}
			status, _ := codex.Probe(stdout.String())
			if status != name {
				t.Fatalf("Probe(%q) = %q, want %q", stdout.String(), status, name)
			}
		})
	}

	if _, err := runWithIO(nil, strings.NewReader(`{"command":"state","name":"not-a-real-state"}`+"\n"), io.Discard, io.Discard, func(string) string { return "1" }); err == nil {
		t.Fatal("an unknown pane state was unexpectedly accepted")
	}
}

// TestPermissionAndStopRequireAPromptFirst proves the session-scoped pane
// commands refuse to run before any session exists, rather than firing a
// hook with an empty/zero-value session.
func TestPermissionAndStopRequireAPromptFirst(t *testing.T) {
	getenv := func(key string) string {
		if key == commandsEnvironment {
			return "1"
		}
		return ""
	}
	for name, command := range map[string]string{
		"permission": `{"command":"permission","tool_name":"Bash"}`,
		"stop":       `{"command":"stop","message":"done"}`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := runWithIO(nil, strings.NewReader(command+"\n"), io.Discard, io.Discard, getenv); err == nil {
				t.Fatalf("%q command without a prior prompt unexpectedly succeeded", name)
			}
		})
	}
}

// TestExitCommandFiresSessionEndOnlyOnceASessionHasStarted proves the
// "exit" pane command's SessionEnd firing is conditional on an actual
// session existing: task 020's own hook-only tests (which never send a
// "prompt" command) must keep behaving exactly as they did before this
// task, and a session that HAS started does get its clean SessionEnd.
func TestExitCommandFiresSessionEndOnlyOnceASessionHasStarted(t *testing.T) {
	directory := t.TempDir()
	recordPath := filepath.Join(directory, "record.jsonl")
	override := recorderOverride(t, "SessionEnd", recordPath)
	args := []string{"-c", override, dangerouslyBypassHookTrustFlag}
	getenv := func(key string) string {
		if key == commandsEnvironment {
			return "1"
		}
		return ""
	}

	t.Run("no session yet: exit is silent", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		if code, err := runWithIO(args, strings.NewReader(`{"command":"exit"}`+"\n"), &stdout, &stderr, getenv); err != nil || code != 0 {
			t.Fatalf("runWithIO = (%d, %v)", code, err)
		}
		if _, err := os.Stat(recordPath); !os.IsNotExist(err) {
			t.Fatalf("exit with no session fired SessionEnd anyway (record exists, err=%v)", err)
		}
	})

	t.Run("session started: exit fires SessionEnd", func(t *testing.T) {
		discardPath := filepath.Join(directory, "discard.jsonl")
		sessionStartOverride := recorderOverride(t, "SessionStart", discardPath)
		userPromptOverride := recorderOverride(t, "UserPromptSubmit", discardPath)
		withSessionArgs := []string{"-c", override, "-c", sessionStartOverride, "-c", userPromptOverride, dangerouslyBypassHookTrustFlag}
		input := `{"command":"prompt","text":"hi"}` + "\n" + `{"command":"exit"}` + "\n"
		var stdout, stderr bytes.Buffer
		if code, err := runWithIO(withSessionArgs, strings.NewReader(input), &stdout, &stderr, getenv); err != nil || code != 0 {
			t.Fatalf("runWithIO = (%d, %v), stderr=%q", code, err, stderr.String())
		}
		events := recordedEvents(t, recordPath)
		if len(events) != 1 || events[0]["hook_event_name"] != "SessionEnd" || events[0]["reason"] != "other" {
			t.Fatalf("recorded events = %#v, want exactly one SessionEnd/other", events)
		}
	})
}

// TestCleanExitModeEndsTheProcessWithStatusZero is fake-codex's analog of
// cmd/fake-claude's clean-exit pane command: an "exit" command ends the
// pane loop (and, via a wrapper's exec, the whole process) with a plain
// zero exit, distinct from both the hang mode below and a nonzero crash.
func TestCleanExitModeEndsTheProcessWithStatusZero(t *testing.T) {
	getenv := func(key string) string {
		if key == commandsEnvironment {
			return "1"
		}
		return ""
	}
	var stdout bytes.Buffer
	code, err := runWithIO(nil, strings.NewReader(`{"command":"exit"}`+"\n"), &stdout, io.Discard, getenv)
	if err != nil || code != 0 {
		t.Fatalf("runWithIO = (%d, %v), want (0, nil)", code, err)
	}
}

// TestHangModeBlocksOnThePaneUntilACommandOrEOFArrives is fake-codex's
// analog of cmd/fake-claude's own "long-running" mode (features'
// installFakeClaudeOnPATH's longRunning script: "reads from the pane until
// it receives a command or EOF, providing an unsignalled live agent"):
// with FAKE_CODEX_COMMANDS=1 and no command yet sent, this fixture blocks
// on the pane rather than returning, exactly like a live agent idling for
// input that never arrives, and only returns once one arrives.
func TestHangModeBlocksOnThePaneUntilACommandOrEOFArrives(t *testing.T) {
	getenv := func(key string) string {
		if key == commandsEnvironment {
			return "1"
		}
		return ""
	}
	reader, writer := io.Pipe()
	done := make(chan error, 1)
	go func() {
		_, err := runWithIO(nil, reader, io.Discard, io.Discard, getenv)
		done <- err
	}()

	select {
	case err := <-done:
		t.Fatalf("runWithIO returned (err=%v) before any command or EOF arrived -- it did not hang", err)
	case <-time.After(100 * time.Millisecond):
	}

	if _, err := writer.Write([]byte(`{"command":"exit"}` + "\n")); err != nil {
		t.Fatalf("write exit command: %v", err)
	}
	writer.Close()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("runWithIO: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("runWithIO never returned after the exit command arrived")
	}
}

// TestCrashModeIsTheSameNonzeroExitCodeControlAsFakeClaude documents that
// fake-codex's "crash" mode is exactly
// TestExitCodeIsControlledOnlyByFixtureEnvironment's own FAKE_CODEX_EXIT_CODE
// contract above -- cmd/fake-claude has no separate crash flag either; a
// scenario simulates a crash either with a nonzero fixture exit code here,
// or (as features/crash.feature does for Claude) by SIGKILLing the process
// outright, which this fixture, being an ordinary process, already
// supports with no special-casing needed.
func TestCrashModeIsTheSameNonzeroExitCodeControlAsFakeClaude(t *testing.T) {
	var output bytes.Buffer
	getenv := func(key string) string {
		if key == exitCodeEnvironment {
			return "1"
		}
		return ""
	}
	if code, err := run(nil, &output, getenv); err != nil || code != 1 {
		t.Fatalf("run = (%d, %v), want (1, nil) -- a crash-shaped nonzero exit", code, err)
	}
}
