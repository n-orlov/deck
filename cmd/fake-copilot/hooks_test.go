package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/n-orlov/deck/internal/agent"
)

func commandsEnv() map[string]string { return map[string]string{commandsEnvironment: "1"} }

func (h *harness) runCommands(t *testing.T, plugin string, commands ...string) outcome {
	t.Helper()
	args := []string{"--session-id", testSessionID}
	if plugin != "" {
		args = append(args, "--plugin-dir", plugin)
	}
	return h.run(args, strings.NewReader(strings.Join(commands, "\n")+"\n"), commandsEnv())
}

func requireBase(t *testing.T, call capturedHook, cwd string) {
	t.Helper()
	if call.payload["sessionId"] != testSessionID || call.payload["cwd"] != cwd {
		t.Errorf("%s payload %v lacks sessionId %q or cwd %q", call.event, call.payload, testSessionID, cwd)
	}
	if stamp, ok := call.payload["timestamp"].(float64); !ok || int64(stamp) < time.Now().Add(-time.Minute).UnixMilli() {
		t.Errorf("%s timestamp = %v, want epoch milliseconds", call.event, call.payload["timestamp"])
	}
	for key := range call.payload {
		if key == "session_id" || key == "hook_event_name" && call.event != "notification" {
			t.Errorf("%s payload carries snake_case key %q: %v", call.event, key, call.payload)
		}
	}
}

// R224 item 3: the six events, with #72's payload shapes, through deck's real
// hooks.json entries and the DECK_HOOK_EVENT they set.
func TestRunsThePluginHooksForTheSixEventsWithTheRealPayloadShapes(t *testing.T) {
	h := newHarness(t)
	plugin := h.installPlugin(t)
	got := h.runCommands(t, plugin,
		`{"command":"prompt","text":"say hi"}`,
		`{"command":"notification","notification_type":"permission_prompt","title":"Permission needed","message":"Run command: sleep 90"}`,
		`{"command":"error","message":"Could not connect"}`,
		`{"command":"exit"}`,
	)
	if got.code != 0 {
		t.Fatalf("exit %d: %s", got.code, got.stderr)
	}
	want := []string{"userPromptSubmitted", "sessionStart", "agentStop", "notification", "errorOccurred", "sessionEnd"}
	if events := h.capturedEvents(t); !equal(events, want) {
		t.Fatalf("hook events = %v, want %v", events, want)
	}
	calls := h.captured(t)
	for _, call := range calls {
		requireBase(t, call, h.cwd)
	}
	if calls[0].payload["prompt"] != "say hi" {
		t.Errorf("userPromptSubmitted = %v, want the prompt", calls[0].payload)
	}
	if calls[1].payload["source"] != "new" || calls[1].payload["initialPrompt"] != "say hi" {
		t.Errorf("sessionStart = %v, want source new and the initial prompt", calls[1].payload)
	}
	transcript := filepath.Join(h.sessionDir(testSessionID), "events.jsonl")
	if calls[2].payload["transcriptPath"] != transcript || calls[2].payload["stopReason"] != "end_turn" || calls[2].payload["stop_hook_active"] != false {
		t.Errorf("agentStop = %v, want transcriptPath %s", calls[2].payload, transcript)
	}
	if !h.exists(transcript) {
		t.Error("agentStop named a transcript that does not exist")
	}
	note := calls[3].payload
	if note["notification_type"] != "permission_prompt" || note["hook_event_name"] != "Notification" || note["title"] != "Permission needed" || note["message"] != "Run command: sleep 90" {
		t.Errorf("notification = %v", note)
	}
	failure, _ := calls[4].payload["error"].(map[string]any)
	if calls[4].payload["errorContext"] != "model_call" || calls[4].payload["recoverable"] != true || failure["message"] != "Could not connect" || failure["name"] != "Error" {
		t.Errorf("errorOccurred = %v", calls[4].payload)
	}
	if calls[5].payload["reason"] != "user_exit" {
		t.Errorf("sessionEnd = %v, want reason user_exit", calls[5].payload)
	}
}

// R224 item 3: sessionStart fires at the first prompt only, never at launch.
func TestSessionStartFiresAtTheFirstPromptOnly(t *testing.T) {
	h := newHarness(t)
	plugin := h.installPlugin(t)
	cmd, stdin := h.startProcess(t, "--session-id", testSessionID, "--plugin-dir", plugin)
	waitFor(t, "workspace.yaml", func() bool { return h.exists(h.sessionDir(testSessionID), "workspace.yaml") })
	time.Sleep(200 * time.Millisecond)
	if events := h.capturedEvents(t); len(events) != 0 {
		t.Fatalf("hooks fired at launch, before any prompt: %v", events)
	}
	for _, text := range []string{"one", "two"} {
		if _, err := stdin.Write([]byte(`{"command":"prompt","text":"` + text + `"}` + "\n")); err != nil {
			t.Fatal(err)
		}
	}
	waitFor(t, "second agentStop", func() bool {
		count := 0
		for _, event := range h.capturedEvents(t) {
			if event == "agentStop" {
				count++
			}
		}
		return count == 2
	})
	_ = cmd.Process.Kill()
	_ = cmd.Wait()
	want := []string{"userPromptSubmitted", "sessionStart", "agentStop", "userPromptSubmitted", "agentStop"}
	if events := h.capturedEvents(t); !equal(events, want) {
		t.Fatalf("hook events = %v, want %v", events, want)
	}
}

func TestSessionStartReportsAResume(t *testing.T) {
	h := newHarness(t)
	plugin := h.installPlugin(t)
	h.runCommands(t, plugin, `{"command":"prompt","text":"a"}`)
	if err := os.Remove(h.capture); err != nil {
		t.Fatal(err)
	}
	h.runCommands(t, plugin, `{"command":"prompt","text":"b"}`)
	for _, call := range h.captured(t) {
		if call.event == "sessionStart" {
			if call.payload["source"] != "resume" {
				t.Fatalf("sessionStart = %v, want source resume", call.payload)
			}
			return
		}
	}
	t.Fatal("no sessionStart on the resumed session's first prompt")
}

// Only what the plugin registers runs: deck registers no preToolUse and no
// permissionRequest, and the fixture never fires them either.
func TestFiresOnlyWhatThePluginRegisters(t *testing.T) {
	h := newHarness(t)
	plugin := h.installPlugin(t)
	write(t, filepath.Join(plugin, "hooks.json"), `{"version":1,"hooks":{"agentStop":[{"type":"command","bash":"printf agentStop >> \"$HOOK_CAPTURE.only\"; exit 0"}]}}`)
	h.runCommands(t, plugin, `{"command":"prompt","text":"x"}`, `{"command":"exit"}`)
	data, err := os.ReadFile(h.capture + ".only")
	if err != nil || string(data) != "agentStop" {
		t.Fatalf("registered-only capture = %q (%v), want exactly one agentStop", data, err)
	}
	if h.exists(h.capture) {
		t.Fatal("deck's hook command ran although the plugin no longer registers it")
	}
}

func TestEveryDeckHookEntryPrintsNothing(t *testing.T) {
	h := newHarness(t)
	plugin := h.installPlugin(t)
	got := h.runCommands(t, plugin, `{"command":"prompt","text":"x"}`, `{"command":"exit"}`)
	if strings.Contains(got.stdout, "hook stdout") || !strings.Contains(got.stdout, "fake-copilot hook fired: agentStop\n") {
		t.Fatalf("pane = %q, want fired lines and no hook output", got.stdout)
	}
}

func TestAHookThatPrintsIsReportedAndAFailingOneDoesNotStopTheFixture(t *testing.T) {
	h := newHarness(t)
	plugin := h.installPlugin(t)
	write(t, filepath.Join(plugin, "hooks.json"), `{"version":1,"hooks":{`+
		`"userPromptSubmitted":[{"type":"command","bash":"echo spoken","timeoutSec":5}],`+
		`"agentStop":[{"type":"command","bash":"exit 3","timeoutSec":5}],`+
		`"sessionStart":[{"type":"prompt","bash":"exit 0"},{"type":"command","bash":""}]}}`)
	got := h.runCommands(t, plugin, `{"command":"prompt","text":"x"}`, `{"command":"exit"}`)
	if got.code != 0 {
		t.Fatalf("exit %d: %s", got.code, got.stderr)
	}
	for _, want := range []string{"fake-copilot hook fired: userPromptSubmitted\n", "fake-copilot hook stdout: spoken\n", "fake-copilot hook failed: agentStop\n"} {
		if !strings.Contains(got.stdout, want) {
			t.Errorf("pane %q lacks %q", got.stdout, want)
		}
	}
	if strings.Contains(got.stdout, "sessionStart") {
		t.Errorf("pane %q reports a sessionStart hook, but no command entry is registered for it", got.stdout)
	}
}

func TestAHookThatOutlastsItsTimeoutFails(t *testing.T) {
	h := newHarness(t)
	plugin := h.installPlugin(t)
	write(t, filepath.Join(plugin, "hooks.json"), `{"version":1,"hooks":{"agentStop":[{"type":"command","bash":"sleep 30","timeoutSec":1}]}}`)
	start := time.Now()
	got := h.runCommands(t, plugin, `{"command":"prompt","text":"x"}`, `{"command":"exit"}`)
	if !strings.Contains(got.stdout, "fake-copilot hook failed: agentStop\n") || time.Since(start) > 20*time.Second {
		t.Fatalf("pane %q after %v, want the slow hook failed at its timeout", got.stdout, time.Since(start))
	}
}

func TestNoPluginOrAnUnreadableOneRegistersNothing(t *testing.T) {
	for name, setup := range map[string]func(*harness, string){
		"none":      nil,
		"missing":   func(_ *harness, dir string) { _ = os.Remove(filepath.Join(dir, "hooks.json")) },
		"malformed": func(_ *harness, dir string) { write(t, filepath.Join(dir, "hooks.json"), "{") },
	} {
		t.Run(name, func(t *testing.T) {
			h := newHarness(t)
			plugin := h.installPlugin(t)
			if setup == nil {
				plugin = ""
			} else {
				setup(h, plugin)
			}
			got := h.runCommands(t, plugin, `{"command":"prompt","text":"x"}`, `{"command":"exit"}`)
			if got.code != 0 || strings.Contains(got.stdout, "hook fired") || len(h.capturedEvents(t)) != 0 {
				t.Fatalf("pane %q, hooks %v: want nothing fired", got.stdout, h.capturedEvents(t))
			}
		})
	}
}

func TestDeckPluginCommandsAreTheOnesRun(t *testing.T) {
	h := newHarness(t)
	plugin := h.installPlugin(t)
	data, err := os.ReadFile(filepath.Join(plugin, "hooks.json"))
	if err != nil || string(data) != agent.CopilotHooksConfig() {
		t.Fatalf("plugin hooks.json drifted from agent.CopilotHooksConfig (err %v)", err)
	}
	runner := newHookRunner(plugin, h.cwd)
	for _, event := range agent.CopilotHookEvents {
		if entries := runner.entries(event); len(entries) != 1 || entries[0].TimeoutSec != agent.CopilotHookTimeoutSec {
			t.Errorf("entries(%s) = %v, want deck's one entry", event, entries)
		}
	}
	if entries := runner.entries("preToolUse"); len(entries) != 0 {
		t.Errorf("preToolUse has entries %v", entries)
	}
}

func TestAHookPayloadThatCannotBeEncodedFailsTheEntry(t *testing.T) {
	h := newHarness(t)
	plugin := h.installPlugin(t)
	result := newHookRunner(plugin, h.cwd).fire("agentStop", map[string]any{"bad": make(chan int)})
	if result.failed != 1 || result.ran != 0 {
		t.Fatalf("result = %+v, want one failed entry", result)
	}
}

func TestAnUnknownPaneCommandIsAFixtureFault(t *testing.T) {
	h := newHarness(t)
	got := h.runCommands(t, "", `{"command":"nope"}`)
	if got.code != 2 || !strings.Contains(got.stderr, `unknown command "nope"`) {
		t.Fatalf("run = (%d, %q), want exit 2", got.code, got.stderr)
	}
	got = h.runCommands(t, "", `not json`)
	if got.code != 2 || !strings.Contains(got.stderr, "decode command") {
		t.Fatalf("run = (%d, %q), want exit 2", got.code, got.stderr)
	}
}

func TestSessionEndFiresOnceOnACleanShutdown(t *testing.T) {
	h := newHarness(t)
	plugin := h.installPlugin(t)
	h.runCommands(t, plugin, `{"command":"exit"}`)
	if events := h.capturedEvents(t); !equal(events, []string{"sessionEnd"}) {
		t.Fatalf("hook events = %v, want exactly one sessionEnd", events)
	}
}

func TestASignalShutsDownThroughTheServeLoop(t *testing.T) {
	h := newHarness(t)
	plugin := h.installPlugin(t)
	signals := make(chan os.Signal, 1)
	signals <- os.Interrupt
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	defer writer.Close()
	var stdout, stderr strings.Builder
	code := runWithIO([]string{"--session-id", testSessionID, "--plugin-dir", plugin}, reader, &stdout, &stderr, h.getenv(commandsEnv()), func() (string, error) { return h.cwd, nil }, signals)
	if code != 0 || !equal(h.capturedEvents(t), []string{"sessionEnd"}) {
		t.Fatalf("run = %d, hooks %v, want a clean shutdown with sessionEnd", code, h.capturedEvents(t))
	}
}
