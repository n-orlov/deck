// Command fake-claude is a deterministic, black-box test fixture for Claude Code.
//
// It intentionally implements only the Claude flags deck needs to exercise. It has
// no deck-specific IPC or status channel: observable behavior is its terminal output
// and eventual process exit, just as it is for the real CLI.
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/creack/pty"
	"github.com/google/uuid"
)

const (
	exitCodeEnvironment         = "FAKE_CLAUDE_EXIT_CODE"
	commandsEnvironment         = "FAKE_CLAUDE_COMMANDS"
	fixtureDirectoryEnvironment = "FAKE_AGENT_FIXTURE_DIR"
	// silentFixtureEnvironment names a fixture (relative to
	// FAKE_AGENT_FIXTURE_DIR) to render verbatim and then go silent forever:
	// no banner, no argv record, no further bytes of any kind. This is the
	// deterministic fake pane the preview needs (SPEC Phase 2b-1 requirement
	// 5) -- byte-stable across repeated capture-pane ticks. It is orthogonal
	// to commandsEnvironment's interactive "fixture"/"hook" command loop,
	// which keeps talking.
	silentFixtureEnvironment = "FAKE_CLAUDE_FIXTURE"
	// sizesLogName is where this fixture appends its initial terminal size
	// and every SIGWINCH-observed size, one "COLSxROWS" line per observation
	// (SPEC Phase 2b-1 requirement 4). This is what lets a scenario assert
	// the agent's own experience of a resize rather than infer it from tmux
	// bookkeeping.
	sizesLogName = "fake-claude-sizes.log"
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
	settings       string
	message        string
}

func main() {
	code, err := runWithIO(os.Args[1:], os.Stdin, os.Stdout, os.Stderr, os.Getenv, os.Getwd)
	if err != nil {
		fmt.Fprintln(os.Stderr, "fake-claude:", err)
		code = 2
	}
	os.Exit(code)
}

func run(args []string, stdout io.Writer, getenv func(string) string, getwd func() (string, error)) (int, error) {
	return runWithIO(args, strings.NewReader(""), stdout, io.Discard, getenv, getwd)
}

func runWithIO(args []string, stdin io.Reader, stdout, stderr io.Writer, getenv func(string) string, getwd func() (string, error)) (int, error) {
	if len(args) == 1 && (args[0] == "--help" || args[0] == "-h") {
		fmt.Fprint(stdout, helpText)
		return 0, nil
	}

	// Recording starts before anything else is parsed so this fixture's
	// initial size is captured even if the invocation goes on to fail, and so
	// a SIGWINCH racing the earliest possible moment is never missed.
	stopSizeRecorder := startSizeRecorder(getenv)
	defer stopSizeRecorder()

	if name := getenv(silentFixtureEnvironment); name != "" {
		return 0, renderThenFallSilent(stdin, stdout, getenv(fixtureDirectoryEnvironment), name)
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

	if getenv(commandsEnvironment) == "1" {
		if err := runCommands(stdin, stdout, stderr, options.settings, getenv(fixtureDirectoryEnvironment)); err != nil {
			return 0, err
		}
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
			case "--settings":
				if _, err := hookCommands(value); err != nil {
					return result, fmt.Errorf("invalid --settings: %w", err)
				}
				result.settings = value
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

var supportedHookEvents = map[string]bool{
	"SessionStart":     true,
	"UserPromptSubmit": true,
	"Notification":     true,
	"Stop":             true,
	"StopFailure":      true,
	"SessionEnd":       true,
}

type fixtureCommand struct {
	Command string         `json:"command"`
	Event   string         `json:"event"`
	Payload map[string]any `json:"payload"`
	Name    string         `json:"name"`
}

type claudeSettings struct {
	Hooks map[string][]struct {
		Hooks []struct {
			Type    string `json:"type"`
			Command string `json:"command"`
		} `json:"hooks"`
	} `json:"hooks"`
}

func hookCommands(raw string) (map[string]string, error) {
	if raw == "" {
		return nil, nil
	}
	var settings claudeSettings
	if err := json.Unmarshal([]byte(raw), &settings); err != nil {
		return nil, err
	}
	commands := make(map[string]string)
	for event, groups := range settings.Hooks {
		for _, group := range groups {
			for _, hook := range group.Hooks {
				if hook.Type == "command" && hook.Command != "" {
					commands[event] = hook.Command
					break
				}
			}
		}
	}
	return commands, nil
}

// runCommands is the pane-side control surface used by black-box scenarios. Each
// input line asks the fake agent to fire an injected hook or render a corpus file.
// Hook subprocesses inherit the fake agent's environment, exactly as a Claude hook
// does; the harness does not invoke deck _hook itself.
func runCommands(input io.Reader, stdout, stderr io.Writer, rawSettings, fixtureDirectory string) error {
	commands, err := hookCommands(rawSettings)
	if err != nil {
		return fmt.Errorf("decode hook settings: %w", err)
	}
	scanner := bufio.NewScanner(input)
	for scanner.Scan() {
		var request fixtureCommand
		if err := json.Unmarshal(scanner.Bytes(), &request); err != nil {
			return fmt.Errorf("decode command: %w", err)
		}
		switch request.Command {
		case "fixture":
			if err := renderFixture(stdout, fixtureDirectory, request.Name); err != nil {
				return err
			}
		case "hook":
			if !supportedHookEvents[request.Event] {
				return fmt.Errorf("unsupported hook event %q", request.Event)
			}
			command := commands[request.Event]
			if command == "" {
				return fmt.Errorf("hook event %q was not injected in --settings", request.Event)
			}
			if request.Payload == nil {
				request.Payload = make(map[string]any)
			}
			if _, exists := request.Payload["hook_event_name"]; !exists {
				request.Payload["hook_event_name"] = request.Event
			}
			payload, err := json.Marshal(request.Payload)
			if err != nil {
				return fmt.Errorf("encode %s payload: %w", request.Event, err)
			}
			process := exec.Command("sh", "-c", command)
			process.Stdin = bytes.NewReader(append(payload, '\n'))
			process.Stdout = stdout
			process.Stderr = stderr
			if err := process.Run(); err != nil {
				return fmt.Errorf("fire %s hook: %w", request.Event, err)
			}
			fmt.Fprintf(stdout, "fake-claude hook fired: %s\n", request.Event)
		default:
			return fmt.Errorf("unknown command %q", request.Command)
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

// renderThenFallSilent renders the named fixture exactly once and then blocks
// forever, producing no further output of any kind (requirement 5). It reads
// stdin to end-of-file rather than selecting on a channel, so the blocking
// call is a real syscall wait tied to the pane's own lifetime -- when the
// pane is killed and the pty closes, the read returns and this process exits
// cleanly, exactly like a real agent idling for input that never arrives.
func renderThenFallSilent(input io.Reader, output io.Writer, directory, name string) error {
	if err := renderFixture(output, directory, name); err != nil {
		return err
	}
	_, err := io.Copy(io.Discard, input)
	return err
}

func validUUID(name, value string) error {
	if _, err := uuid.Parse(value); err != nil {
		return fmt.Errorf("invalid UUID for %s: %q", name, value)
	}
	return nil
}

// startSizeRecorder appends this process's initial terminal size, and then
// every subsequent SIGWINCH-observed size, to $DECK_HOME/log/fake-claude-sizes.log,
// one "COLSxROWS" line per observation (requirement 4). Sizes are always
// read from the kernel via the pane's own pty, never inferred from
// COLUMNS/LINES or any other environment hint.
//
// Like transcript persistence above, this is best-effort scaffolding around
// the fixture's real observable contract: an unresolved DECK_HOME, or a
// stdout that is not a terminal (for example, a unit test), degrades to "no
// recording" rather than a fixture crash. The returned stop function releases
// the signal handler and its goroutine; it does not need to run before the
// process exits, only before a test process reuses this fixture's code in a
// loop.
func startSizeRecorder(getenv func(string) string) func() {
	path := sizesLogPath(getenv)
	if path == "" {
		return func() {}
	}
	recordSize(path)

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGWINCH)
	done := make(chan struct{})
	go func() {
		for {
			select {
			case <-signals:
				recordSize(path)
			case <-done:
				return
			}
		}
	}()
	return func() {
		signal.Stop(signals)
		close(done)
	}
}

// sizesLogPath resolves the size-log path under DECK_HOME, or "" when
// DECK_HOME is not set (a bare invocation with no deck-managed environment).
func sizesLogPath(getenv func(string) string) string {
	home := getenv("DECK_HOME")
	if home == "" {
		return ""
	}
	return filepath.Join(home, "log", sizesLogName)
}

// recordSize reads the current size of this process's own controlling
// terminal (its pty slave, inherited as stdout) and appends it to path. Any
// failure (no controlling terminal, an unwritable path) is silently
// swallowed: recording is scaffolding, never the fixture's observable
// contract.
func recordSize(path string) {
	rows, cols, err := pty.Getsize(os.Stdout)
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer file.Close()
	fmt.Fprintf(file, "%dx%d\n", cols, rows)
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
  --settings <json>               Accept Claude-compatible per-session hook settings.
  --help, -h                      Show this help.

Set FAKE_CLAUDE_EXIT_CODE to an integer from 0 through 125 to control this fixture's exit status.
Set FAKE_CLAUDE_FIXTURE=<name> to render that fixture from FAKE_AGENT_FIXTURE_DIR verbatim and
then produce no further output (this process idles forever, exactly like a real agent waiting
for input that never arrives). Mutually exclusive with FAKE_CLAUDE_COMMANDS's interactive loop.
Set FAKE_CLAUDE_COMMANDS=1 to read newline-delimited commands from the pane. A hook
command has the form {"command":"hook","event":"SessionStart","payload":{...}}.
It invokes that event's command from --settings with the payload on stdin and with
this process's injected environment; it never calls deck _hook directly. A fixture
command has the form {"command":"fixture","name":"claude/running.txt"} and copies
that file from FAKE_AGENT_FIXTURE_DIR to the pane without changing its bytes.

When $HOME is set and writable, trailing prompt text given with --session-id or
--resume is appended to a per-conversation transcript at the real Claude Code
path and naming convention ($HOME/.claude/projects/<escaped-cwd>/<id>.jsonl),
and --resume replays that conversation's own last recorded message before
accepting any new one.
`
