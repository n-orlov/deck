package main

import (
	"encoding/json"
	"fmt"
	"io"
	"time"
)

// app is one running fake Copilot: its session, its hooks and the pane.
type app struct {
	opts   options
	sess   *session
	out    io.Writer
	errOut io.Writer
	hooks  hookRunner
	// prompted is set by the first prompt, the moment sessionStart fires.
	prompted bool
	ended    bool
}

type command struct {
	Command          string `json:"command"`
	Text             string `json:"text"`
	Title            string `json:"title"`
	Message          string `json:"message"`
	NotificationType string `json:"notification_type"`
}

// dispatch runs one pane command. The first result reports a quit.
func (a *app) dispatch(line string) (bool, error) {
	var request command
	if err := json.Unmarshal([]byte(line), &request); err != nil {
		return false, fmt.Errorf("decode command: %w", err)
	}
	switch request.Command {
	case "prompt":
		return false, a.prompt(request.Text)
	case "notification":
		a.fire("notification", map[string]any{
			"message": request.Message, "title": request.Title,
			"hook_event_name": "Notification", "notification_type": request.NotificationType,
		})
		return false, nil
	case "error":
		a.fire("errorOccurred", map[string]any{
			"errorContext": "model_call", "recoverable": true,
			"error": map[string]any{"message": request.Message, "name": "Error", "stack": "Error: " + request.Message},
		})
		return false, nil
	case "exit":
		a.shutdown()
		return true, nil
	default:
		return false, fmt.Errorf("unknown command %q", request.Command)
	}
}

// base is the part of every hook payload: sessionId, timestamp (epoch ms), cwd.
func (a *app) base() map[string]any {
	return map[string]any{"sessionId": a.sess.id, "timestamp": time.Now().UnixMilli(), "cwd": a.sess.cwd}
}

// fire runs the hooks of one event and reports each in the pane.
func (a *app) fire(event string, fields map[string]any) {
	payload := a.base()
	for key, value := range fields {
		payload[key] = value
	}
	result := a.hooks.fire(event, payload)
	if result.ran+result.failed == 0 {
		return
	}
	if result.failed > 0 {
		sayf(a.out, "fake-copilot hook failed: %s\n", event)
		return
	}
	sayf(a.out, "fake-copilot hook fired: %s\n", event)
	if result.stdout != "" {
		sayf(a.out, "fake-copilot hook stdout: %s\n", result.stdout)
	}
}

// prompt plays one turn: userPromptSubmitted, then sessionStart if this is the
// process's first prompt (Copilot fires it at the first prompt, never at
// launch), then agentStop with the transcript path. events.jsonl appears here.
func (a *app) prompt(text string) error {
	a.fire("userPromptSubmitted", map[string]any{"prompt": text})
	if !a.prompted {
		a.prompted = true
		source := "new"
		if a.sess.resumed {
			source = "resume"
		}
		a.fire("sessionStart", map[string]any{"source": source, "initialPrompt": text})
	}
	if err := a.sess.record(
		event{Type: "user.message", Data: map[string]any{"content": text}},
		event{Type: "assistant.turn_start", Data: map[string]any{}},
		event{Type: "assistant.message", Data: map[string]any{"content": "fake reply"}},
		event{Type: "assistant.turn_end", Data: map[string]any{}},
	); err != nil {
		return err
	}
	a.fire("agentStop", map[string]any{"transcriptPath": a.sess.eventsPath(), "stopReason": "end_turn", "stop_hook_active": false})
	return nil
}

// shutdown is a clean exit (reason user_exit, what tmux SIGHUP, Ctrl-C twice and /exit all give): the transcript is completed and sessionEnd fires.
// It runs at most once. A SIGKILL never reaches it.
func (a *app) shutdown() {
	if a.ended {
		return
	}
	a.ended = true
	if err := a.sess.record(event{Type: "session.shutdown", Data: map[string]any{"shutdownType": "routine"}}); err != nil {
		sayf(a.errOut, "fake-copilot: %v\n", err)
	}
	a.fire("sessionEnd", map[string]any{"reason": "user_exit"})
}
