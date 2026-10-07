package features

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/cucumber/godog"
)

// copilotFixtureDir holds the real-capture pane fixtures the fake is held to.
const copilotFixtureDir = "internal/agent/testdata/probes/copilot"

const driftSessionID = "11111111-2222-4333-8444-555555555555"

// copilotScreenKeys are, per state, the key substrings of the matching
// fixture: each one is read out of the recorded capture first (so the table
// cannot drift from the fixture) and then looked for in the fake's own screen.
var copilotScreenKeys = map[string][]string{
	"idle":       {"Interactive · Manual Approval · / commands"},
	"working":    {"◉ Working · ", "esc interrupt"},
	"permission": {"Do you want to run this command?", "↑/↓ to navigate · enter to select · esc to cancel"},
	"question":   {"Copilot needs information.", "↑/↓ select · enter accept · ctrl+d decline · esc cancel"},
	"trust":      {"Confirm folder trust", "Do you trust the files in this folder?", "↑/↓ to navigate · enter to select · esc to cancel"},
	"error":      {"✗ Failed to get response from the AI model"},
}

// copilotPayloadKeys are the keys the #72 spike comment records for each
// event's payload (every payload also has sessionId, timestamp and cwd).
// errorOccurred's "error" object carries message, name and stack.
var copilotPayloadKeys = map[string][]string{
	"userPromptSubmitted": {"sessionId", "timestamp", "cwd", "prompt"},
	"sessionStart":        {"sessionId", "timestamp", "cwd", "source", "initialPrompt"},
	"notification":        {"sessionId", "timestamp", "cwd", "message", "title", "hook_event_name", "notification_type"},
	"agentStop":           {"sessionId", "timestamp", "cwd", "transcriptPath", "stopReason", "stop_hook_active"},
	"errorOccurred":       {"sessionId", "timestamp", "cwd", "errorContext", "recoverable", "error"},
	"sessionEnd":          {"sessionId", "timestamp", "cwd", "reason"},
}

var copilotErrorObjectKeys = []string{"message", "name", "stack"}

var driftSocketCounter atomic.Int64

type fakeCopilotDriftScenario struct {
	dir      string
	binary   string
	payloads map[string]map[string]json.RawMessage
}

func registerFakeCopilotDriftSteps(sc *godog.ScenarioContext) {
	scenario := &fakeCopilotDriftScenario{}
	sc.After(func(ctx context.Context, _ *godog.Scenario, _ error) (context.Context, error) {
		if scenario.dir != "" {
			_ = os.RemoveAll(scenario.dir)
		}
		return ctx, nil
	})
	sc.Step(`^the fake Copilot fixture is built$`, scenario.fixtureIsBuilt)
	sc.Step(`^the fake Copilot "([^"]*)" screen carries the key strings of the "([^"]*)" fixture$`, scenario.screenCarriesFixtureKeys)
	sc.Step(`^the fake Copilot plays a turn, a notification, an error and a clean exit$`, scenario.playsEveryHookEvent)
	sc.Step(`^the fake Copilot "([^"]*)" payload carries the #72 keys$`, scenario.payloadCarriesKeys)
}

func (s *fakeCopilotDriftScenario) fixtureIsBuilt(ctx context.Context) error {
	root, err := repositoryRoot()
	if err != nil {
		return err
	}
	s.dir, err = os.MkdirTemp("", "deck-copilot-drift-")
	if err != nil {
		return fmt.Errorf("create drift scratch dir: %w", err)
	}
	s.binary = filepath.Join(s.dir, "fake-copilot")
	build := exec.CommandContext(ctx, "go", "build", "-o", s.binary, "./cmd/fake-copilot")
	build.Dir = root
	if output, err := build.CombinedOutput(); err != nil {
		return fmt.Errorf("build fake copilot fixture: %w\n%s", err, output)
	}
	return nil
}

// screenCarriesFixtureKeys launches the fake in a 120x40 pane of a private tmux
// server (never a deck socket), drives it into the state, and requires every
// key substring of the recorded fixture on its screen.
func (s *fakeCopilotDriftScenario) screenCarriesFixtureKeys(ctx context.Context, state, fixture string) error {
	keys, ok := copilotScreenKeys[state]
	if !ok {
		return fmt.Errorf("no key strings are defined for the %q screen", state)
	}
	root, err := repositoryRoot()
	if err != nil {
		return err
	}
	recorded, err := os.ReadFile(filepath.Join(root, copilotFixtureDir, fixture+".txt"))
	if err != nil {
		return fmt.Errorf("read the %q fixture: %w", fixture, err)
	}
	for _, key := range keys {
		if !strings.Contains(string(recorded), key) {
			return fmt.Errorf("key %q is not in the recorded %q fixture; the drift table is wrong", key, fixture)
		}
	}
	pane, err := s.startPane(state)
	if err != nil {
		return err
	}
	defer pane.close()
	if err := pane.drive(state); err != nil {
		return err
	}
	if _, err := pane.waitFor(ctx, keys); err != nil {
		return fmt.Errorf("fake Copilot %q screen drifted from the %q fixture: %w", state, fixture, err)
	}
	return nil
}

type driftPane struct {
	socket string
	dir    string
}

func (s *fakeCopilotDriftScenario) startPane(state string) (*driftPane, error) {
	pane := &driftPane{socket: fmt.Sprintf("drift-copilot-%d-%d", os.Getpid(), driftSocketCounter.Add(1)), dir: filepath.Join(s.dir, "pane-"+state)}
	cwd := filepath.Join(pane.dir, "work")
	for _, dir := range []string{cwd, filepath.Join(pane.dir, "home"), filepath.Join(pane.dir, "deck")} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return nil, err
		}
	}
	environment := []string{
		"FAKE_COPILOT_COMMANDS=1", "FAKE_COPILOT_SCREEN=1",
		"COPILOT_HOME=" + filepath.Join(pane.dir, "home"), "DECK_HOME=" + filepath.Join(pane.dir, "deck"),
	}
	if state == "trust" {
		environment = append(environment, "FAKE_COPILOT_TRUST=1")
	}
	args := []string{"-L", pane.socket, "new-session", "-d", "-x", "120", "-y", "40", "-c", cwd, "env"}
	args = append(args, environment...)
	args = append(args, s.binary, "--session-id", driftSessionID, "--no-auto-update")
	if output, err := exec.Command("tmux", args...).CombinedOutput(); err != nil {
		return nil, fmt.Errorf("start the fake Copilot pane: %w: %s", err, output)
	}
	return pane, nil
}

func (p *driftPane) close() {
	_ = exec.Command("tmux", "-L", p.socket, "kill-server").Run()
}

func (p *driftPane) tmux(args ...string) ([]byte, error) {
	output, err := exec.Command("tmux", append([]string{"-L", p.socket}, args...)...).CombinedOutput()
	if err != nil {
		return output, fmt.Errorf("tmux %s: %w: %s", strings.Join(args, " "), err, output)
	}
	return output, nil
}

// paneCommands maps a state to the JSON pane commands that reach it.
var paneCommands = map[string][]string{
	"working":    {`{"command":"prompt","text":"hello","hold":true}`},
	"permission": {`{"command":"dialog","kind":"permission","message":"sleep 90"}`},
	"question":   {`{"command":"dialog","kind":"question","message":"Which colour do you prefer?"}`},
	"error":      {`{"command":"error","message":"Failed to get response from the AI model"}`},
}

func (p *driftPane) drive(state string) error {
	for _, command := range paneCommands[state] {
		if _, err := p.tmux("send-keys", "-l", command); err != nil {
			return err
		}
		if _, err := p.tmux("send-keys", "Enter"); err != nil {
			return err
		}
	}
	return nil
}

// waitFor polls the pane until every key is on it, and returns the last screen.
func (p *driftPane) waitFor(ctx context.Context, keys []string) (string, error) {
	deadline := time.Now().Add(10 * time.Second)
	for {
		output, err := p.tmux("capture-pane", "-p", "-t", "0")
		if err != nil {
			return "", err
		}
		var missing []string
		for _, key := range keys {
			if !strings.Contains(string(output), key) {
				missing = append(missing, fmt.Sprintf("%q", key))
			}
		}
		if len(missing) == 0 {
			return string(output), nil
		}
		if time.Now().After(deadline) || ctx.Err() != nil {
			return "", fmt.Errorf("missing %s on the pane:\n%s", strings.Join(missing, ", "), output)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// playsEveryHookEvent runs the fake with a plugin whose hooks write each
// event's stdin payload to a file, and plays all six events.
func (s *fakeCopilotDriftScenario) playsEveryHookEvent(ctx context.Context) error {
	work := filepath.Join(s.dir, "hooks")
	plugin := filepath.Join(work, "plugin")
	out := filepath.Join(work, "out")
	cwd := filepath.Join(work, "cwd")
	for _, dir := range []string{plugin, out, cwd, filepath.Join(work, "home")} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return err
		}
	}
	hooks := map[string][]map[string]any{}
	for event := range copilotPayloadKeys {
		hooks[event] = []map[string]any{{"type": "command", "bash": `cat > "$DRIFT_OUT/` + event + `.json"`, "timeoutSec": 5}}
	}
	encoded, err := json.Marshal(map[string]any{"version": 1, "hooks": hooks})
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(plugin, "hooks.json"), encoded, 0o600); err != nil {
		return err
	}
	commands := strings.Join([]string{
		`{"command":"prompt","text":"hello"}`,
		`{"command":"notification","notification_type":"permission_prompt","title":"Permission needed","message":"Run command: sleep 90"}`,
		`{"command":"error","message":"model call failed"}`,
		`{"command":"exit"}`,
	}, "\n") + "\n"
	runCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	command := exec.CommandContext(runCtx, s.binary, "--session-id", driftSessionID, "--plugin-dir", plugin, "--no-auto-update")
	command.Dir = cwd
	command.Stdin = strings.NewReader(commands)
	command.Env = append(os.Environ(), "FAKE_COPILOT_COMMANDS=1", "COPILOT_HOME="+filepath.Join(work, "home"), "DRIFT_OUT="+out)
	var combined bytes.Buffer
	command.Stdout, command.Stderr = &combined, &combined
	if err := command.Run(); err != nil {
		return fmt.Errorf("run the fake Copilot: %w\n%s", err, combined.String())
	}
	s.payloads = map[string]map[string]json.RawMessage{}
	for event := range copilotPayloadKeys {
		raw, err := os.ReadFile(filepath.Join(out, event+".json"))
		if err != nil {
			return fmt.Errorf("the fake Copilot never fired %s: %w\n%s", event, err, combined.String())
		}
		var payload map[string]json.RawMessage
		if err := json.Unmarshal(raw, &payload); err != nil {
			return fmt.Errorf("%s payload is not a JSON object: %w: %s", event, err, raw)
		}
		s.payloads[event] = payload
	}
	return nil
}

func (s *fakeCopilotDriftScenario) payloadCarriesKeys(event string) error {
	want, ok := copilotPayloadKeys[event]
	if !ok {
		return fmt.Errorf("no #72 keys are defined for %s", event)
	}
	payload, ok := s.payloads[event]
	if !ok {
		return errors.New("no payloads were captured")
	}
	for _, key := range want {
		if _, ok := payload[key]; !ok {
			return fmt.Errorf("%s payload lacks the #72 key %q", event, key)
		}
	}
	if event != "errorOccurred" {
		return nil
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(payload["error"], &object); err != nil {
		return fmt.Errorf("errorOccurred error is not an object: %w", err)
	}
	for _, key := range copilotErrorObjectKeys {
		if _, ok := object[key]; !ok {
			return fmt.Errorf("errorOccurred error object lacks the #72 key %q", key)
		}
	}
	return nil
}
