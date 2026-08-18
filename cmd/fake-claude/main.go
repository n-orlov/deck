// Command fake-claude is a deterministic, black-box test fixture for Claude Code.
//
// It intentionally implements only the Claude flags deck needs to exercise. It has
// no deck-specific IPC or status channel: observable behavior is its terminal output
// and eventual process exit, just as it is for the real CLI.
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"

	"github.com/google/uuid"
)

const (
	exitCodeEnvironment = "FAKE_CLAUDE_EXIT_CODE"
)

var permissionModes = map[string]bool{
	"manual":            true,
	"plan":              true,
	"acceptEdits":       true,
	"auto":              true,
	"dontAsk":           true,
	"bypassPermissions": true,
}

type options struct {
	sessionID      string
	resume         string
	permissionMode string
}

func main() {
	code, err := run(os.Args[1:], os.Stdout, os.Getenv)
	if err != nil {
		fmt.Fprintln(os.Stderr, "fake-claude:", err)
		code = 2
	}
	os.Exit(code)
}

func run(args []string, stdout io.Writer, getenv func(string) string) (int, error) {
	if len(args) == 1 && (args[0] == "--help" || args[0] == "-h") {
		fmt.Fprint(stdout, helpText)
		return 0, nil
	}

	options, err := parse(args)
	if err != nil {
		return 0, err
	}
	encoded, err := json.Marshal(args)
	if err != nil {
		return 0, fmt.Errorf("encode argv: %w", err)
	}

	// Keep this output deliberately small and deterministic so a real terminal/pane
	// assertion can prove both that the fixture started and which argv reached it.
	fmt.Fprintln(stdout, "Fake Claude Code")
	fmt.Fprintf(stdout, "fake-claude argv: %s\n", encoded)
	if options.sessionID != "" {
		fmt.Fprintf(stdout, "fake-claude session-id: %s\n", options.sessionID)
	}
	if options.resume != "" {
		fmt.Fprintf(stdout, "fake-claude resume: %s\n", options.resume)
	}
	if options.permissionMode != "" {
		fmt.Fprintf(stdout, "fake-claude permission-mode: %s\n", options.permissionMode)
	}

	return configuredExitCode(getenv(exitCodeEnvironment))
}

func parse(args []string) (options, error) {
	var result options
	for index := 0; index < len(args); index++ {
		argument := args[index]
		if argument == "--" {
			return result, nil // Remaining positional prompt text is accepted by Claude.
		}
		if len(argument) > 1 && argument[0] == '-' {
			if index+1 == len(args) {
				return result, fmt.Errorf("option %q requires a value", argument)
			}
			value := args[index+1]
			index++
			switch argument {
			case "--session-id":
				if err := validUUID("--session-id", value); err != nil {
					return result, err
				}
				result.sessionID = value
			case "--resume":
				if err := validUUID("--resume", value); err != nil {
					return result, err
				}
				result.resume = value
			case "--permission-mode":
				if !permissionModes[value] {
					return result, fmt.Errorf("invalid value for --permission-mode: %q", value)
				}
				result.permissionMode = value
			default:
				return result, fmt.Errorf("unknown option %q", argument)
			}
		}
	}
	return result, nil
}

func validUUID(name, value string) error {
	if _, err := uuid.Parse(value); err != nil {
		return fmt.Errorf("invalid UUID for %s: %q", name, value)
	}
	return nil
}

func configuredExitCode(value string) (int, error) {
	if value == "" {
		return 0, nil
	}
	code, err := strconv.Atoi(value)
	if err != nil || code < 0 || code > 125 {
		return 0, errors.New("FAKE_CLAUDE_EXIT_CODE must be an integer from 0 through 125")
	}
	return code, nil
}

const helpText = `Fake Claude Code fixture

Usage: fake-claude [options] [prompt]

Options:
  --session-id <uuid>             Use a UUID assigned by the caller.
  --resume <uuid>                 Resume a UUID conversation.
  --permission-mode <mode>        One of: manual, plan, acceptEdits, auto, dontAsk, bypassPermissions.
  --help, -h                      Show this help.

Set FAKE_CLAUDE_EXIT_CODE to an integer from 0 through 125 to control this fixture's exit status.
`
