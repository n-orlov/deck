// Command fake-pi is a deterministic, black-box test fixture for the pi coding
// agent. It intentionally implements only the pi flags deck uses. It has no
// deck-specific IPC or status channel: observable behavior is its terminal
// output and eventual process exit, just as it is for the real CLI.
package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
)

const (
	exitCodeEnvironment         = "FAKE_PI_EXIT_CODE"
	commandsEnvironment         = "FAKE_PI_COMMANDS"
	fixtureDirectoryEnvironment = "FAKE_AGENT_FIXTURE_DIR"
)

type options struct {
	sessionID string
	approve   bool
	message   string
}

func main() {
	code, err := runWithIO(os.Args[1:], os.Stdin, os.Stdout, os.Getenv)
	if err != nil {
		fmt.Fprintln(os.Stderr, "fake-pi:", err)
		code = 2
	}
	os.Exit(code)
}

func run(args []string, stdout io.Writer) (int, error) {
	return runWithIO(args, os.Stdin, stdout, os.Getenv)
}

func runWithIO(args []string, stdin io.Reader, stdout io.Writer, getenv func(string) string) (int, error) {
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

	if getenv(commandsEnvironment) == "1" {
		if err := runCommands(stdin, stdout, getenv(fixtureDirectoryEnvironment)); err != nil {
			return 0, err
		}
	}

	return configuredExitCode(getenv(exitCodeEnvironment))
}

type fixtureCommand struct {
	Command string `json:"command"`
	Name    string `json:"name"`
}

func runCommands(input io.Reader, output io.Writer, fixtureDirectory string) error {
	scanner := bufio.NewScanner(input)
	for scanner.Scan() {
		var request fixtureCommand
		if err := json.Unmarshal(scanner.Bytes(), &request); err != nil {
			return fmt.Errorf("decode command: %w", err)
		}
		if request.Command != "fixture" {
			return fmt.Errorf("unknown command %q", request.Command)
		}
		if err := renderFixture(output, fixtureDirectory, request.Name); err != nil {
			return err
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read command: %w", err)
	}
	return nil
}

// renderFixture copies the named corpus file without adding a marker or newline.
// This lets probe golden tests and pane-driven scenarios consume identical bytes.
func renderFixture(output io.Writer, directory, name string) error {
	if directory == "" {
		return errors.New("FAKE_AGENT_FIXTURE_DIR is not set")
	}
	if name == "" || !filepath.IsLocal(name) {
		return fmt.Errorf("invalid fixture name %q", name)
	}
	contents, err := os.ReadFile(filepath.Join(directory, name))
	if err != nil {
		return fmt.Errorf("read fixture %q: %w", name, err)
	}
	if _, err := output.Write(contents); err != nil {
		return fmt.Errorf("render fixture %q: %w", name, err)
	}
	return nil
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
Set FAKE_PI_COMMANDS=1 to read newline-delimited commands from the pane. A fixture
command has the form {"command":"fixture","name":"pi/waiting.txt"} and copies that
file from FAKE_AGENT_FIXTURE_DIR to the pane without changing its bytes.
`
