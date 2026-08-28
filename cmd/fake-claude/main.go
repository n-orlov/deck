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
	"sync"
	"syscall"

	"github.com/creack/pty"
	"github.com/google/uuid"
	"golang.org/x/sys/unix"
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
	// sigwinchCountName is where this fixture overwrites the exact running
	// total of SIGWINCH signals it has received, as a bare decimal integer
	// (never appended, unlike sizesLogName above) via write-to-temp-then-
	// rename, so a poller never observes a partial write (identical
	// mechanism to cmd/deck's -tags deckinputcount writeInputCount).
	// requirement 11 / II-2 needs an exact count, not a comparison of
	// before/after sizes: two SIGWINCH that happen to land the pane at the
	// same size would still be one count each here, and this counter never
	// calls pty.Getsize (recordSize's own early-return on failure), so it can
	// never silently undercount a SIGWINCH the sizes log missed.
	sigwinchCountName = "fake-claude-sigwinch-count"
	// repaintModeEnvironment selects this fixture's repaint behaviour
	// (PRD phase3b-interactive-preview.md requirement 1 / II-1): repaintModeSigwinch,
	// repaintModeKeystroke or repaintModeNever. Setting it puts the fixture into a
	// dedicated mode (see runRepaintFixture) mutually exclusive with the
	// silent-fixture mode above and the argv/commands flow below -- it never
	// prints the Claude banner, it only emits repaint markers per the selected
	// behaviour and drains stdin until EOF.
	repaintModeEnvironment = "FAKE_CLAUDE_REPAINT_MODE"
)

// Repaint modes for repaintModeEnvironment. "Repaint" here means the fixture
// writes an observable "repaint #N" marker line to its own pane -- distinct
// from startSizeRecorder's requirement-4 size log, which keeps recording
// unconditionally regardless of the mode selected here. A scenario asserts
// the marker's presence/timing against the pane's screen, not the size log,
// to distinguish the three behaviours.
const (
	// repaintModeSigwinch repaints immediately on every SIGWINCH.
	repaintModeSigwinch = "sigwinch"
	// repaintModeKeystroke ignores SIGWINCH but repaints once on the next byte
	// read from stdin after a SIGWINCH arrived (a keystroke forwarded into the
	// pane, in the real interactive-mode flow this fixture stands in for).
	repaintModeKeystroke = "keystroke"
	// repaintModeNever never repaints, regardless of SIGWINCH or input -- the
	// realistic worst case (a target waiting on a network round-trip), and the
	// mode requirement 49 / II-49's "has not repainted" announcement uses.
	repaintModeNever = "never"
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

	if mode := getenv(repaintModeEnvironment); mode != "" {
		return 0, runRepaintFixture(mode, stdin, stdout)
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
	// OldConversationID and NewConversationID are used only by the "resume"
	// command (see runCommands): the in-session resume end/start pair that
	// SPEC Phase 2b-2 requirement 43's SessionEnd taxonomy exercises. If
	// NewConversationID is empty, a fresh UUID is generated so a scenario
	// does not have to invent one itself.
	OldConversationID string `json:"old_session_id"`
	NewConversationID string `json:"new_session_id"`
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
			if request.Payload == nil {
				request.Payload = make(map[string]any)
			}
			if err := fireHook(stdout, stderr, commands, request.Event, request.Payload); err != nil {
				return err
			}
		case "resume":
			if err := fireResumePair(stdout, stderr, commands, request); err != nil {
				return err
			}
		case "exit":
			// Ends this loop (and, via the caller's exec-replaced wrapper
			// script, this process) with a clean status 0 exit -- the
			// pane-side control a scenario reaches for when it needs this
			// fixture's own tmux pane to actually disappear (deck's
			// remain-on-exit=failed destroys a zero-exit pane and, with it,
			// the session), rather than merely posing a terminal status
			// while the pane a real hook subprocess is independent of stays
			// alive underneath it.
			return nil
		default:
			return fmt.Errorf("unknown command %q", request.Command)
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read command: %w", err)
	}
	return nil
}

// fireResumePair implements the pane-side command that exercises requirement
// 43's in-session resume taxonomy: it fires SessionEnd reason=resume for the
// conversation named by OldConversationID, immediately followed by
// SessionStart reason=resume for a NEW conversation id (NewConversationID, or
// a freshly generated UUID if that is left empty), in the same pane -- the
// tmux session, the pane and this process all survive, exactly as a resumed
// Claude Code session does. It announces the id it picked on stdout so a
// scenario driving a scripted "resume" command without naming its own new id
// can still fire a later Stop/Notification for the right conversation.
func fireResumePair(stdout, stderr io.Writer, commands map[string]string, request fixtureCommand) error {
	if request.OldConversationID == "" {
		return errors.New(`"resume" command requires "old_session_id"`)
	}
	newID := request.NewConversationID
	if newID == "" {
		newID = uuid.NewString()
	}
	if err := fireHook(stdout, stderr, commands, "SessionEnd", map[string]any{"session_id": request.OldConversationID, "reason": "resume"}); err != nil {
		return err
	}
	if err := fireHook(stdout, stderr, commands, "SessionStart", map[string]any{"session_id": newID, "reason": "resume"}); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "fake-claude resume: %s -> %s\n", request.OldConversationID, newID)
	return nil
}

// fireHook fires event's injected --settings command with payload on its
// stdin, exactly as runCommands's "hook" pane command always has: the
// subprocess inherits this fixture's own (deck-injected) environment and is
// invoked directly, never via "deck _hook". It is shared by the plain "hook"
// command and by fireResumePair's SessionEnd/SessionStart pair so both paths
// have identical, single-sourced hook-firing behaviour.
func fireHook(stdout, stderr io.Writer, commands map[string]string, event string, payload map[string]any) error {
	if !supportedHookEvents[event] {
		return fmt.Errorf("unsupported hook event %q", event)
	}
	command := commands[event]
	if command == "" {
		return fmt.Errorf("hook event %q was not injected in --settings", event)
	}
	if payload == nil {
		payload = make(map[string]any)
	}
	if _, exists := payload["hook_event_name"]; !exists {
		payload["hook_event_name"] = event
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode %s payload: %w", event, err)
	}
	process := exec.Command("sh", "-c", command)
	process.Stdin = bytes.NewReader(append(encoded, '\n'))
	process.Stdout = stdout
	process.Stderr = stderr
	if err := process.Run(); err != nil {
		return fmt.Errorf("fire %s hook: %w", event, err)
	}
	fmt.Fprintf(stdout, "fake-claude hook fired: %s\n", event)
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

// runRepaintFixture is a dedicated, self-contained mode (mutually exclusive
// with the argv-parsing banner flow, the commands loop and the silent-fixture
// mode above) that exists purely to give a scenario a selectable, observable
// repaint behaviour (requirement 1 / II-1). It never terminates on its own;
// it exits when stdin reaches EOF, exactly like the other blocking modes in
// this file, so the harness's normal teardown (closing/killing the pane)
// still works.
func runRepaintFixture(mode string, stdin io.Reader, stdout io.Writer) error {
	if mode != repaintModeSigwinch && mode != repaintModeKeystroke && mode != repaintModeNever {
		return fmt.Errorf("invalid %s %q: want %q, %q or %q", repaintModeEnvironment, mode, repaintModeSigwinch, repaintModeKeystroke, repaintModeNever)
	}

	// A real full-screen coding agent draws its own representation of
	// typed input rather than relying on the tty's own local echo (PRD
	// phase3b requirement 49 states plainly that a frozen agent's frame
	// is "byte-identical before and after typing" -- true only if the
	// tty itself never echoes the keystroke back on deck's behalf).
	// disableStdinEcho is a no-op unless stdin is a real *os.File backed
	// by an actual tty (every unit test in this package drives
	// watchAndRepaint directly over a plain io.Pipe, never this
	// function, so none of them are affected).
	disableStdinEcho(stdin)

	// Registered here, synchronously, before any goroutine starts reading from
	// the (buffered, capacity-1) channel: a SIGWINCH delivered any time after
	// this call returns is queued in the channel regardless of whether a
	// consumer has started selecting on it yet, so there is no race between
	// "registration happened" and "the test's signal arrives" (watchAndRepaint
	// below is what a test drives directly, with its own Notify call made the
	// same way, to keep that guarantee visible at the call site).
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGWINCH)
	defer signal.Stop(signals)
	return watchAndRepaint(mode, stdin, stdout, signals)
}

// disableStdinEcho turns off local ECHO/ECHONL on stdin's own tty, when
// stdin is in fact a real tty -- a bare unix.IoctlGetTermios error
// (stdin is a pipe, a regular file, or otherwise not a tty) degrades to a
// silent no-op rather than an error, since neither case is a fixture
// misconfiguration worth failing the process over. Only ECHO/ECHONL are
// cleared; every other terminal mode (canonical line editing, signal
// generation, etc.) is left exactly as the pty already had it, since
// this fixture reads a byte at a time from a raw pipe-pane stream and
// has no need to change anything else about how the kernel line
// discipline behaves.
func disableStdinEcho(stdin io.Reader) {
	file, ok := stdin.(*os.File)
	if !ok {
		return
	}
	fd := int(file.Fd())
	term, err := unix.IoctlGetTermios(fd, unix.TCGETS)
	if err != nil {
		return
	}
	term.Lflag &^= unix.ECHO | unix.ECHONL
	_ = unix.IoctlSetTermios(fd, unix.TCSETS, term)
}

// watchAndRepaint is runRepaintFixture's behaviour with signal registration
// factored out, so a test can register its own channel (guaranteeing no race
// against the signal it is about to send) and drive this loop directly.
func watchAndRepaint(mode string, stdin io.Reader, stdout io.Writer, signals <-chan os.Signal) error {
	var mu sync.Mutex
	counter := 0
	pending := false
	repaint := func() {
		mu.Lock()
		counter++
		n := counter
		mu.Unlock()
		fmt.Fprintf(stdout, "repaint #%d\n", n)
	}

	done := make(chan struct{})
	defer close(done)
	go func() {
		for {
			select {
			case <-signals:
				switch mode {
				case repaintModeSigwinch:
					repaint()
				case repaintModeKeystroke:
					// Ignore the SIGWINCH itself; remember it happened so the
					// next byte read from stdin (a forwarded keystroke) triggers
					// the repaint instead.
					mu.Lock()
					pending = true
					mu.Unlock()
				case repaintModeNever:
					// Never repaints, whatever arrives.
				}
			case <-done:
				return
			}
		}
	}()

	buffer := make([]byte, 4096)
	for {
		n, err := stdin.Read(buffer)
		if n > 0 && mode == repaintModeKeystroke {
			mu.Lock()
			fire := pending
			pending = false
			mu.Unlock()
			if fire {
				repaint()
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
	}
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
	countPath := sigwinchCountPath(getenv)
	if path == "" && countPath == "" {
		return func() {}
	}
	if path != "" {
		recordSize(path)
	}

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGWINCH)
	done := make(chan struct{})
	go func() {
		var count int64
		for {
			select {
			case <-signals:
				if path != "" {
					recordSize(path)
				}
				if countPath != "" {
					count++
					recordSigwinchCount(countPath, count)
				}
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

// sigwinchCountPath resolves the SIGWINCH-count path under DECK_HOME,
// identically to sizesLogPath, or "" when DECK_HOME is not set.
func sigwinchCountPath(getenv func(string) string) string {
	home := getenv("DECK_HOME")
	if home == "" {
		return ""
	}
	return filepath.Join(home, "log", sigwinchCountName)
}

// recordSigwinchCount overwrites path with total as a bare decimal integer,
// via write-to-temp-then-rename, exactly as cmd/deck's inputcount_hook.go
// writeInputCount does for the same reason: a concurrent reader (the test
// harness polling this file) must never observe a partially written value.
// Any failure is silently swallowed -- counting is scaffolding for a test
// harness, never part of this fixture's observable contract.
func recordSigwinchCount(path string, total int64) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}
	tmp, err := os.CreateTemp(dir, ".fake-claude-sigwinch-count-*")
	if err != nil {
		return
	}
	name := tmp.Name()
	_, writeErr := fmt.Fprint(tmp, strconv.FormatInt(total, 10))
	closeErr := tmp.Close()
	if writeErr != nil || closeErr != nil {
		os.Remove(name)
		return
	}
	if err := os.Rename(name, path); err != nil {
		os.Remove(name)
	}
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
Set FAKE_CLAUDE_REPAINT_MODE=sigwinch|keystroke|never to switch this fixture into a dedicated
repaint-observation mode (mutually exclusive with FAKE_CLAUDE_FIXTURE and FAKE_CLAUDE_COMMANDS):
it prints no banner and reads no commands, it only ever writes "repaint #N" lines to the pane
per the selected behaviour (sigwinch: on every SIGWINCH; keystroke: on the next byte read from
stdin after a SIGWINCH; never: not at all) until stdin reaches EOF.
Set FAKE_CLAUDE_COMMANDS=1 to read newline-delimited commands from the pane. A hook
command has the form {"command":"hook","event":"SessionStart","payload":{...}}.
It invokes that event's command from --settings with the payload on stdin and with
this process's injected environment; it never calls deck _hook directly. A fixture
command has the form {"command":"fixture","name":"claude/running.txt"} and copies
that file from FAKE_AGENT_FIXTURE_DIR to the pane without changing its bytes.
An exit command has the form {"command":"exit"} and ends the loop (and, via
the wrapper's exec, this process) with a clean status 0 exit -- the pane-side
control for a scenario that needs this fixture's own tmux pane to actually
disappear rather than merely pose a terminal status while the pane stays alive.
A resume command has the form {"command":"resume","old_session_id":"<uuid>",
"new_session_id":"<uuid, optional>"} and fires SessionEnd reason=resume for
old_session_id immediately followed by SessionStart reason=resume for
new_session_id (a fresh UUID is generated and announced on stdout as
"fake-claude resume: <old> -> <new>" if new_session_id is omitted), both via
the same injected --settings commands the "hook" command uses, in the same
pane: the tmux session, the pane and this process all survive, exactly like a
real in-session Claude Code resume.

When $HOME is set and writable, trailing prompt text given with --session-id or
--resume is appended to a per-conversation transcript at the real Claude Code
path and naming convention ($HOME/.claude/projects/<escaped-cwd>/<id>.jsonl),
and --resume replays that conversation's own last recorded message before
accepting any new one.
`
