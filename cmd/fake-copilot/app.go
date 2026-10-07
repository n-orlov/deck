package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"syscall"
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
	// turnOpen is a held turn: the prompt was taken and the agent has not stopped.
	turnOpen bool
	ended    bool
	// screen is the rendered TUI, nil unless FAKE_COPILOT_SCREEN=1.
	screen *screen
}

type command struct {
	Command          string `json:"command"`
	Text             string `json:"text"`
	Hold             bool   `json:"hold"`
	Title            string `json:"title"`
	Message          string `json:"message"`
	NotificationType string `json:"notification_type"`
	Kind             string `json:"kind"`
}

// dispatch runs one pane command. The first result reports a quit.
func (a *app) dispatch(line string) (bool, error) {
	var request command
	if err := json.Unmarshal([]byte(line), &request); err != nil {
		return false, fmt.Errorf("decode command: %w", err)
	}
	switch request.Command {
	case "prompt":
		return false, a.prompt(request.Text, request.Hold)
	case "stop":
		return false, a.stop()
	case "notification":
		a.fire("notification", map[string]any{
			"message": request.Message, "title": request.Title,
			"hook_event_name": "Notification", "notification_type": request.NotificationType,
		})
		return false, nil
	case "error":
		a.fail(request.Message)
		return false, nil
	case "interrupt":
		a.interrupt()
		return false, nil
	case "dialog":
		return false, a.dialog(request.Kind, request.Message)
	case "dismiss":
		a.dismiss()
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
// A held prompt ("hold":true) leaves the turn open, the agent working, until a
// "stop" command ends it.
func (a *app) prompt(text string, hold bool) error {
	if a.screen != nil {
		a.screen.add(promptMark + text)
		a.screen.setMode(modeWorking, "")
	}
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
	); err != nil {
		return err
	}
	a.turnOpen = true
	if hold {
		return nil
	}
	return a.stop()
}

// stop ends the open turn: the reply is recorded and agentStop fires with the
// transcript path.
func (a *app) stop() error {
	if !a.turnOpen {
		return fmt.Errorf("no turn is in progress")
	}
	a.turnOpen = false
	if a.screen != nil {
		a.screen.add(replyMarker + "fake reply")
		a.screen.setMode(modeIdle, "")
	}
	if err := a.sess.record(
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
	if a.screen != nil {
		a.screen.stop()
	}
}

// fail is an error in the turn: errorOccurred fires, and the transcript shows
// the "✗ " line Copilot draws for a failed model call (the turn's footer
// returns to idle; a hold, if any, is still ended by "stop").
func (a *app) fail(message string) {
	a.fire("errorOccurred", map[string]any{
		"errorContext": "model_call", "recoverable": true,
		"error": map[string]any{"message": message, "name": "Error", "stack": "Error: " + message},
	})
	if a.screen != nil {
		a.screen.add(errorMarker + message)
		a.screen.setMode(modeIdle, "")
	}
}

// interrupt is the user's Ctrl-C during work. Copilot aborts the turn and fires
// no hook for it (the real behaviour deck's pane probe exists to cover), so the
// turn just closes: the transcript records the cancellation and the footer is
// idle again. With no turn open it does nothing.
func (a *app) interrupt() {
	if !a.turnOpen {
		return
	}
	a.turnOpen = false
	if a.screen != nil {
		a.screen.add(replyMarker + abortedLine)
		a.screen.setMode(modeIdle, "")
	} else {
		sayln(a.out, "fake-copilot turn aborted")
	}
	if err := a.sess.record(event{Type: "abort", Data: map[string]any{"reason": "user_initiated"}}); err != nil {
		sayf(a.errOut, "fake-copilot: %v\n", err)
	}
}

// dialog shows one of Copilot's modal prompts in place of the composer: the
// permission dialog (detail names the command), the question dialog (detail is
// the question) or the folder-trust prompt. It draws only; a hook, if the
// scenario wants one, is a separate "notification" command.
func (a *app) dialog(kind, detail string) error {
	modes := map[string]screenMode{"permission": modePermission, "question": modeQuestion, "trust": modeTrust}
	mode, known := modes[kind]
	if !known {
		return fmt.Errorf("unknown dialog kind %q", kind)
	}
	if a.screen == nil {
		sayf(a.out, "fake-copilot dialog: %s\n", kind)
		return nil
	}
	a.screen.setMode(mode, detail)
	return nil
}

// dismiss closes the dialog: back to the working footer while a turn is open,
// else the idle footer.
func (a *app) dismiss() {
	if a.screen == nil {
		return
	}
	if a.turnOpen {
		a.screen.setMode(modeWorking, "")
		return
	}
	a.screen.setMode(modeIdle, "")
}

// onSignal handles a signal the serve loop received. SIGWINCH redraws at the
// new size; SIGINT during an open turn in screen mode is Ctrl-C aborting it;
// anything else (and SIGINT with no turn) is a clean shutdown. The result is
// true when the loop must stop.
func (a *app) onSignal(sig os.Signal) bool {
	switch {
	case sig == syscall.SIGWINCH:
		if a.screen != nil {
			a.screen.resized()
		}
		return false
	case sig == syscall.SIGINT && a.screen != nil && a.turnOpen:
		a.interrupt()
		return false
	}
	a.shutdown()
	return true
}
