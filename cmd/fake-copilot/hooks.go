package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// defaultHookTimeout applies to an entry that declares no timeoutSec.
const defaultHookTimeout = 30 * time.Second

type hookEntry struct {
	Type       string `json:"type"`
	Bash       string `json:"bash"`
	TimeoutSec int    `json:"timeoutSec"`
}

type hooksFile struct {
	Hooks map[string][]hookEntry `json:"hooks"`
}

// hookRunner plays Copilot's plugin hook mechanism: for an event it runs every
// registered command entry under `bash -c`, in the session's cwd, with this
// process's own environment (so $DECK_EXE and the rest reach it, as they reach
// a real Copilot hook) and the camelCase payload on stdin.
type hookRunner struct {
	pluginDir string
	cwd       string
}

func newHookRunner(pluginDir, cwd string) hookRunner {
	return hookRunner{pluginDir: pluginDir, cwd: cwd}
}

// entries reads the plugin's hooks.json each time, as Copilot does at the point
// of firing. No plugin directory, or none with a hooks.json, registers nothing.
func (h hookRunner) entries(event string) []hookEntry {
	if h.pluginDir == "" {
		return nil
	}
	data, err := os.ReadFile(filepath.Join(h.pluginDir, "hooks.json"))
	if err != nil {
		return nil
	}
	var parsed hooksFile
	if json.Unmarshal(data, &parsed) != nil {
		return nil
	}
	var commands []hookEntry
	for _, entry := range parsed.Hooks[event] {
		if entry.Type == "command" && entry.Bash != "" {
			commands = append(commands, entry)
		}
	}
	return commands
}

// hookResult is what firing one event produced.
type hookResult struct {
	// ran counts the entries that exited 0.
	ran int
	// failed counts entries that did not start, timed out or exited non-zero.
	failed int
	// stdout is everything the entries printed. Deck's entries print nothing.
	stdout string
}

// fire runs the event's entries one after another and never fails the fixture
// for a failing hook: Copilot carries on after one, and so does this.
func (h hookRunner) fire(event string, payload map[string]any) hookResult {
	var result hookResult
	entries := h.entries(event)
	if len(entries) == 0 {
		return result
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		result.failed = len(entries)
		return result
	}
	var out bytes.Buffer
	for _, entry := range entries {
		if h.runEntry(entry, encoded, &out) {
			result.ran++
		} else {
			result.failed++
		}
	}
	result.stdout = out.String()
	return result
}

func (h hookRunner) runEntry(entry hookEntry, payload []byte, stdout *bytes.Buffer) bool {
	timeout := defaultHookTimeout
	if entry.TimeoutSec > 0 {
		timeout = time.Duration(entry.TimeoutSec) * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	command := exec.CommandContext(ctx, "bash", "-c", entry.Bash) //nolint:gosec // G204: the command is the hooks.json entry of the plugin directory this fixture was launched with, exactly what Copilot runs
	command.Dir = h.cwd
	command.Stdin = bytes.NewReader(payload)
	command.Stdout = stdout
	err := command.Run()
	var exitErr *exec.ExitError
	return err == nil && !errors.As(err, &exitErr)
}
