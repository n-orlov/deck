// Command fake-claude is a deterministic, black-box test fixture for Claude Code.
//
// It intentionally implements only the Claude flags deck needs to exercise. It has
// no deck-specific IPC or status channel: observable behavior is its terminal output
// and eventual process exit, just as it is for the real CLI.
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
	"strings"

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
	message        string
}

func main() {
	code, err := run(os.Args[1:], os.Stdout, os.Getenv, os.Getwd)
	if err != nil {
		fmt.Fprintln(os.Stderr, "fake-claude:", err)
		code = 2
	}
	os.Exit(code)
}

func run(args []string, stdout io.Writer, getenv func(string) string, getwd func() (string, error)) (int, error) {
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

	if err := replayAndRecord(options, getenv, getwd, stdout); err != nil {
		// Transcript persistence is best-effort scaffolding around the fixture's
		// real observable contract (the banner, argv record, and exit status);
		// an unwritable HOME (e.g. no real home directory provisioned for the
		// caller) must not turn an otherwise-accepted invocation into a fixture
		// crash.
		fmt.Fprintln(os.Stderr, "fake-claude: transcript unavailable:", err)
	}

	return configuredExitCode(getenv(exitCodeEnvironment))
}

// replayAndRecord implements the per-conversation transcript persisted at the real
// Claude on-disk path/naming convention: $HOME/.claude/projects/<escaped-cwd>/<id>.jsonl,
// one JSON object per line. On --resume it prints ("replays") the resumed id's own last
// recorded message before appending anything new, and on either --session-id or --resume
// with trailing prompt text it appends that text as a new message keyed to that id.
func replayAndRecord(options options, getenv func(string) string, getwd func() (string, error), stdout io.Writer) error {
	conversationID := options.sessionID
	if conversationID == "" {
		conversationID = options.resume
	}
	if conversationID == "" {
		return nil
	}

	path, err := transcriptPath(getenv, getwd, conversationID)
	if err != nil {
		return err
	}
	if path == "" {
		return nil
	}

	if options.resume != "" {
		last, err := lastMessage(path)
		if err != nil {
			return err
		}
		if last != "" {
			fmt.Fprintf(stdout, "fake-claude replay: %s\n", last)
		}
	}

	if options.message != "" {
		if err := appendMessage(path, options.message); err != nil {
			return err
		}
	}

	return nil
}

// transcriptPath mirrors the real Claude Code layout: a project directory derived from
// the current working directory (path separators replaced with "-"), holding one
// "<conversation id>.jsonl" file per conversation.
func transcriptPath(getenv func(string) string, getwd func() (string, error), conversationID string) (string, error) {
	home := getenv("HOME")
	if home == "" {
		// A missing HOME means this fixture is running in an environment that
		// hasn't provided one (deck's own service layer always resolves and
		// passes one; a bare launcher may not). Degrade to "no transcript"
		// rather than erroring: replay/record is best-effort, not the fixture's
		// core observable contract.
		return "", nil
	}
	cwd, err := getwd()
	if err != nil {
		return "", fmt.Errorf("resolve cwd: %w", err)
	}
	project := strings.ReplaceAll(cwd, string(filepath.Separator), "-")
	return filepath.Join(home, ".claude", "projects", project, conversationID+".jsonl"), nil
}

type transcriptEntry struct {
	Message string `json:"message"`
}

func appendMessage(path, message string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create transcript directory: %w", err)
	}
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open transcript: %w", err)
	}
	defer file.Close()

	encoded, err := json.Marshal(transcriptEntry{Message: message})
	if err != nil {
		return fmt.Errorf("encode transcript entry: %w", err)
	}
	if _, err := file.Write(append(encoded, '\n')); err != nil {
		return fmt.Errorf("write transcript entry: %w", err)
	}
	return nil
}

func lastMessage(path string) (string, error) {
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("open transcript: %w", err)
	}
	defer file.Close()

	var last string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		var entry transcriptEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			return "", fmt.Errorf("decode transcript entry: %w", err)
		}
		last = entry.Message
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("read transcript: %w", err)
	}
	return last, nil
}

func parse(args []string) (options, error) {
	var result options
	var message []string
	for index := 0; index < len(args); index++ {
		argument := args[index]
		if argument == "--" {
			message = append(message, args[index+1:]...) // Remaining positional prompt text is accepted by Claude.
			break
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
			continue
		}
		message = append(message, argument)
	}
	result.message = strings.Join(message, " ")
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

When $HOME is set and writable, trailing prompt text given with --session-id or
--resume is appended to a per-conversation transcript at the real Claude Code
path and naming convention ($HOME/.claude/projects/<escaped-cwd>/<id>.jsonl),
and --resume replays that conversation's own last recorded message before
accepting any new one.
`
