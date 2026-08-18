// Command fake-pi is a deterministic, black-box test fixture for the pi coding
// agent. It intentionally implements only the pi flags deck uses. It has no
// deck-specific IPC or status channel: observable behavior is its terminal
// output and eventual process exit, just as it is for the real CLI.
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
)

const exitCodeEnvironment = "FAKE_PI_EXIT_CODE"

type options struct {
	sessionID string
	approve   bool
	message   string
}

func main() {
	code, err := run(os.Args[1:], os.Stdout)
	if err != nil {
		fmt.Fprintln(os.Stderr, "fake-pi:", err)
		code = 2
	}
	os.Exit(code)
}

func run(args []string, stdout io.Writer) (int, error) {
	if len(args) == 1 && (args[0] == "--help" || args[0] == "-h") {
		fmt.Fprint(stdout, helpText)
		return 0, nil
	}

	opts, err := parse(args)
	if err != nil {
		return 0, err
	}

	encoded, err := json.Marshal(args)
	if err != nil {
		return 0, fmt.Errorf("encode argv: %w", err)
	}

	// Keep this output deliberately small and deterministic so a real
	// terminal/pane assertion can prove both that the fixture started and
	// which argv reached it.
	fmt.Fprintln(stdout, "Fake pi")
	fmt.Fprintf(stdout, "fake-pi argv: %s\n", encoded)
	if opts.sessionID != "" {
		// pi uses the same --session-id flag for both launch and resume: the
		// id is caller-assigned, and pi creates the conversation if it does
		// not already exist.
		fmt.Fprintf(stdout, "fake-pi session-id: %s\n", opts.sessionID)
	}
	if opts.approve {
		fmt.Fprintln(stdout, "fake-pi approve: true")
	}

	return configuredExitCode(os.Getenv(exitCodeEnvironment))
}

func parse(args []string) (options, error) {
	var result options
	var message []string
	for index := 0; index < len(args); index++ {
		argument := args[index]
		if argument == "--" {
			message = append(message, args[index+1:]...) // Remaining positional prompt text is accepted by pi.
			break
		}
		switch argument {
		case "--session-id":
			if index+1 == len(args) {
				return result, fmt.Errorf("option %q requires a value", argument)
			}
			index++
			result.sessionID = args[index]
			continue
		case "--approve":
			result.approve = true
			continue
		}
		if len(argument) > 1 && argument[0] == '-' {
			return result, fmt.Errorf("unknown option %q", argument)
		}
		message = append(message, argument)
	}
	_ = message
	if len(message) > 0 {
		result.message = message[0]
		for _, part := range message[1:] {
			result.message += " " + part
		}
	}
	return result, nil
}

func configuredExitCode(value string) (int, error) {
	if value == "" {
		return 0, nil
	}
	code, err := strconv.Atoi(value)
	if err != nil || code < 0 || code > 125 {
		return 0, errors.New("FAKE_PI_EXIT_CODE must be an integer from 0 through 125")
	}
	return code, nil
}

const helpText = `Fake pi fixture

Usage: fake-pi [options] [prompt]

Options:
  --session-id <id>   Use a caller-assigned conversation id (created if missing).
  --approve           Accept edits/actions without further prompting.
  --help, -h          Show this help.

Set FAKE_PI_EXIT_CODE to an integer from 0 through 125 to control this fixture's exit status.
`
