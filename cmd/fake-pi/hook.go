package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"slices"
	"strings"

	"github.com/n-orlov/deck/internal/agent"
)

// extensionHooks plays the deck-owned Pi extension a real pi would load with
// -e: it never invokes "deck _hook" itself, it runs the hook command the
// launch put in the environment, for the events the extension file really
// subscribes, exactly as cmd/fake-claude and cmd/fake-codex run the commands
// their launch argv injected.
type extensionHooks struct {
	extension string
	sessionID string
	command   string
}

func newExtensionHooks(opts options, getenv func(string) string) extensionHooks {
	return extensionHooks{extension: opts.extension, sessionID: opts.sessionID, command: getenv(agent.PiHookCommandEnv)}
}

// checkInstalled fails, like fake-claude's own fireHook, when event could not
// have been installed: no extension was loaded, the file is unreadable, or it
// does not subscribe the event.
func (h extensionHooks) checkInstalled(event string) error {
	if h.extension == "" {
		return fmt.Errorf("hook event %q was not installed: no extension was loaded with -e", event)
	}
	source, err := os.ReadFile(h.extension)
	if err != nil {
		return fmt.Errorf("hook event %q was not installed: read extension: %w", event, err)
	}
	if !slices.Contains(agent.PiHookEvents, event) || !strings.Contains(string(source), `"`+event+`"`) {
		return fmt.Errorf("hook event %q is not subscribed by the loaded extension", event)
	}
	return nil
}

// encodePayload builds the JSON the extension writes to the hook's stdin: the
// event's own fields, its name, and the id of the session pi was launched with.
func (h extensionHooks) encodePayload(event string, payload map[string]any) ([]byte, error) {
	if payload == nil {
		payload = make(map[string]any)
	}
	payload["hook_event_name"] = event
	if _, exists := payload["session_id"]; !exists && h.sessionID != "" {
		payload["session_id"] = h.sessionID
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("encode %s payload: %w", event, err)
	}
	return append(encoded, '\n'), nil
}

// fire runs one hook event. With no hook command in the environment it does
// nothing, as the extension does.
func (h extensionHooks) fire(stdout io.Writer, event string, payload map[string]any) error {
	if err := h.checkInstalled(event); err != nil {
		return err
	}
	if h.command == "" {
		return nil
	}
	encoded, err := h.encodePayload(event, payload)
	if err != nil {
		return err
	}
	var captured bytes.Buffer
	runErr := runHookCommand(hookShell, h.command, encoded, &captured)
	var exitErr *exec.ExitError
	switch {
	case runErr == nil:
		sayf(stdout, "fake-pi hook fired: %s\n", event)
	case errors.As(runErr, &exitErr):
		// The real extension shows the failed hook's stderr as a Pi warning
		// and carries on; so does this fixture.
		sayf(stdout, "fake-pi hook failed: %s\n", event)
		sayf(stdout, "fake-pi notify: %s\n", strings.TrimSpace(captured.String()))
	default:
		return fmt.Errorf("fire %s hook: %w", event, runErr)
	}
	return nil
}

// hookShell is the shell that runs the hook line, as the real extension does.
const hookShell = "sh"

// hookScript runs the hook line from the environment variable the launch put
// it in, so the command line itself is data and never part of the argv.
const hookScript = `eval "$` + agent.PiHookCommandEnv + `"`

// runHookCommand runs the shell line deck itself put in the agent's
// environment, as the real extension would: the payload on stdin and the
// failed hook's stderr collected into stderr. The shell is a parameter and the
// script a constant; the command rides in the child's environment.
func runHookCommand(shell, command string, payload []byte, stderr io.Writer) error {
	process := exec.Command(shell, "-c", hookScript)
	process.Env = append(os.Environ(), agent.PiHookCommandEnv+"="+command)
	process.Stdin = bytes.NewReader(payload)
	process.Stderr = stderr
	return process.Run()
}
