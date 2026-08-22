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
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/creack/pty"
)

const (
	exitCodeEnvironment         = "FAKE_PI_EXIT_CODE"
	commandsEnvironment         = "FAKE_PI_COMMANDS"
	fixtureDirectoryEnvironment = "FAKE_AGENT_FIXTURE_DIR"
	// silentFixtureEnvironment names a fixture (relative to
	// FAKE_AGENT_FIXTURE_DIR) to render verbatim and then go silent forever
	// (requirement 5), identical in contract to cmd/fake-claude's own knob of
	// the same purpose.
	silentFixtureEnvironment = "FAKE_PI_FIXTURE"
	// sizesLogName is where this fixture appends its initial terminal size
	// and every SIGWINCH-observed size, one "COLSxROWS" line per observation
	// (SPEC Phase 2b-1 requirement 4).
	sizesLogName = "fake-pi-sizes.log"
)

type options struct {
	sessionID string
	approve   bool
	message   string
}

func main() {
	code, err := runWithIO(os.Args[1:], os.Stdin, os.Stdout, os.Getenv, os.Getwd)
	if err != nil {
		fmt.Fprintln(os.Stderr, "fake-pi:", err)
		code = 2
	}
	os.Exit(code)
}

func run(args []string, stdout io.Writer, getenv func(string) string, getwd func() (string, error)) (int, error) {
	return runWithIO(args, os.Stdin, stdout, getenv, getwd)
}

func runWithIO(args []string, stdin io.Reader, stdout io.Writer, getenv func(string) string, getwd func() (string, error)) (int, error) {
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

	if err := replayAndRecord(opts, getenv, getwd, stdout); err != nil {
		// Transcript persistence is best-effort scaffolding around the fixture's
		// real observable contract (the banner, argv record, and exit status);
		// an unwritable HOME must not turn an otherwise-accepted invocation into
		// a fixture crash. Identical reasoning to cmd/fake-claude's own copy of
		// this guard.
		fmt.Fprintln(os.Stderr, "fake-pi: transcript unavailable:", err)
	}

	return configuredExitCode(getenv(exitCodeEnvironment))
}

// replayAndRecord implements the per-conversation transcript persisted at the
// real pi CLI's on-disk path/naming convention (see transcriptDir and
// createTranscript below for the convention itself and its provenance).
// Unlike Claude Code's separate --session-id/--resume flags, pi reuses a
// single --session-id flag for both creating and resuming a conversation
// (see the comment where opts.sessionID is echoed above), so "resuming" here
// means the caller passed a --session-id that already has a transcript file
// on disk: this fixture then replays that conversation's own last recorded
// message before appending anything new, exactly as cmd/fake-claude's
// --resume path does.
func replayAndRecord(opts options, getenv func(string) string, getwd func() (string, error), stdout io.Writer) error {
	if opts.sessionID == "" {
		return nil
	}

	dir, err := transcriptDir(getenv, getwd)
	if err != nil {
		return err
	}
	if dir == "" {
		return nil
	}

	path, err := findExistingTranscript(dir, opts.sessionID)
	if err != nil {
		return err
	}
	if path != "" {
		last, err := lastMessage(path)
		if err != nil {
			return err
		}
		if last != "" {
			fmt.Fprintf(stdout, "fake-pi replay: %s\n", last)
		}
	} else {
		cwd, err := getwd()
		if err != nil {
			return fmt.Errorf("resolve cwd: %w", err)
		}
		path, err = createTranscript(dir, opts.sessionID, cwd)
		if err != nil {
			return err
		}
	}

	if opts.message != "" {
		if err := appendMessage(path, opts.message); err != nil {
			return err
		}
	}
	return nil
}

// transcriptDir returns pi's own session-storage directory for the current
// working directory: $HOME/.pi/agent/sessions/--<encoded-cwd>--, per pi's
// documented layout (docs/session-format.md's "File Location" section,
// ~/.pi/agent/sessions/--<path>--/<timestamp>_<uuid>.jsonl) and confirmed by
// running a real pi 0.84.1 binary (see docs/reports/phase3-fake-pi-transcript-provenance.md for
// the capture). Returns "" when HOME is unset, exactly like
// cmd/fake-claude's transcriptPath degrading to "no transcript" rather than
// erroring: replay/record is best-effort, not the fixture's core observable
// contract.
func transcriptDir(getenv func(string) string, getwd func() (string, error)) (string, error) {
	home := getenv("HOME")
	if home == "" {
		return "", nil
	}
	cwd, err := getwd()
	if err != nil {
		return "", fmt.Errorf("resolve cwd: %w", err)
	}
	return filepath.Join(home, ".pi", "agent", "sessions", encodeCwd(cwd)), nil
}

// encodeCwd reproduces pi's own encoding exactly (pi-mono's
// session-manager.ts getDefaultSessionDirPath: strip a single leading "/" or
// "\\", then replace every remaining "/", "\\" or ":" with "-", and wrap the
// result in a literal "--" prefix/suffix).
func encodeCwd(cwd string) string {
	trimmed := cwd
	if len(trimmed) > 0 && (trimmed[0] == '/' || trimmed[0] == '\\') {
		trimmed = trimmed[1:]
	}
	replaced := strings.Map(func(r rune) rune {
		if r == '/' || r == '\\' || r == ':' {
			return '-'
		}
		return r
	}, trimmed)
	return "--" + replaced + "--"
}

// findExistingTranscript looks for a file already ending in "_<conversationID>.jsonl"
// in dir. Real pi's filename embeds a creation timestamp that a caller who
// only knows the conversation id cannot predict, so resuming means globbing
// for the id suffix rather than recomputing the exact name (this fixture's
// stand-in for what a real deck adapter's TranscriptPaths lookup will also
// have to do).
func findExistingTranscript(dir, conversationID string) (string, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("read transcript directory: %w", err)
	}
	suffix := "_" + conversationID + ".jsonl"
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), suffix) {
			return filepath.Join(dir, entry.Name()), nil
		}
	}
	return "", nil
}

// createTranscript creates a new transcript file named exactly as real pi
// names one (<fileTimestamp>_<conversationID>.jsonl, fileTimestamp being an
// ISO-8601 UTC timestamp with every ":" and "." replaced by "-") and writes
// its session header line, mirroring the header a real pi always writes
// immediately on session creation (observed in the capture recorded in
// docs/reports/phase3-fake-pi-transcript-provenance.md), before any message exists.
func createTranscript(dir, conversationID, cwd string) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create transcript directory: %w", err)
	}
	now := time.Now().UTC()
	stamp := strings.NewReplacer(":", "-", ".", "-").Replace(now.Format("2006-01-02T15:04:05.000Z"))
	path := filepath.Join(dir, stamp+"_"+conversationID+".jsonl")

	header := map[string]any{
		"type":      "session",
		"version":   3,
		"id":        conversationID,
		"timestamp": now.Format("2006-01-02T15:04:05.000Z"),
		"cwd":       cwd,
	}
	encoded, err := json.Marshal(header)
	if err != nil {
		return "", fmt.Errorf("encode transcript header: %w", err)
	}
	if err := os.WriteFile(path, append(encoded, '\n'), 0o644); err != nil {
		return "", fmt.Errorf("write transcript header: %w", err)
	}
	return path, nil
}

type transcriptEntry struct {
	Message string `json:"message"`
}

func appendMessage(path, message string) error {
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
			continue // The header line has no "message" field; skip lines that are not message entries.
		}
		if entry.Message != "" {
			last = entry.Message
		}
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("read transcript: %w", err)
	}
	return last, nil
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
//
// fake-pi carries no fixture data of its own (requirement 38's derivation,
// task 008): named fixtures such as "pi/running.txt" and "pi/error.txt" are
// the exact same files internal/agent/probe_test.go's probeGoldens table
// pins and internal/agent/testdata/probes/pi-PROVENANCE.md documents as
// captures of a real pi binary. Pointing FAKE_AGENT_FIXTURE_DIR at
// internal/agent/testdata/probes (as features/status_probe_test.go and
// cmd/fake-pi/main_test.go's TestRenderedFixturesProbeToTheRealPiVerdict both
// do) is therefore sufficient for a fake-pi pane to probe to the same
// verdict a real pi pane would — there is no second corpus to keep in sync.
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
// forever, producing no further output of any kind (requirement 5), for the
// identical reason and by the identical mechanism as cmd/fake-claude's own
// copy of this function.
func renderThenFallSilent(input io.Reader, output io.Writer, directory, name string) error {
	if err := renderFixture(output, directory, name); err != nil {
		return err
	}
	_, err := io.Copy(io.Discard, input)
	return err
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

// startSizeRecorder appends this process's initial terminal size, and then
// every subsequent SIGWINCH-observed size, to $DECK_HOME/log/fake-pi-sizes.log,
// one "COLSxROWS" line per observation (requirement 4). Sizes are always
// read from the kernel via the pane's own pty, never inferred from
// COLUMNS/LINES or any other environment hint.
//
// Like cmd/fake-claude's identical recorder, this is best-effort
// scaffolding: an unresolved DECK_HOME, or a stdout that is not a terminal
// (for example, a unit test), degrades to "no recording" rather than a
// fixture crash.
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
Set FAKE_PI_FIXTURE=<name> to render that fixture from FAKE_AGENT_FIXTURE_DIR verbatim and then
produce no further output (this process idles forever, exactly like a real agent waiting for
input that never arrives). Mutually exclusive with FAKE_PI_COMMANDS's interactive loop.
Set FAKE_PI_COMMANDS=1 to read newline-delimited commands from the pane. A fixture
command has the form {"command":"fixture","name":"pi/waiting.txt"} and copies that
file from FAKE_AGENT_FIXTURE_DIR to the pane without changing its bytes.
`
