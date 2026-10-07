// Command fake-copilot is a deterministic, black-box test fixture for the
// GitHub Copilot CLI. It implements only the Copilot flags deck uses and the
// parts of Copilot's behaviour deck depends on: the session directory under
// $COPILOT_HOME/session-state/<id>/, and the plugin hooks Copilot runs. It has
// no deck-specific IPC or status channel: observable behaviour is its terminal
// output, its files, the hook commands it runs and its eventual exit.
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
)

const (
	exitCodeEnvironment = "FAKE_COPILOT_EXIT_CODE"
	commandsEnvironment = "FAKE_COPILOT_COMMANDS"
)

// uuidPattern is the shape Copilot accepts for --session-id: a canonical
// 8-4-4-4-12 hex UUID. Anything else exits 1, so a session can never be
// created under a name that is not an id.
var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

type options struct {
	sessionID    string
	sessionIDSet bool
	pluginDir    string
	allowTools   []string
	allowAll     bool
	noAutoUpdate bool
}

func main() {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGHUP, syscall.SIGTERM, syscall.SIGINT)
	os.Exit(runWithIO(os.Args[1:], os.Stdin, os.Stdout, os.Stderr, os.Getenv, os.Getwd, signals))
}

// isHelpRequest reports whether args is exactly one --help or -h.
func isHelpRequest(args []string) bool {
	return len(args) == 1 && (args[0] == "--help" || args[0] == "-h")
}

// runWithIO is the whole program with its process-level inputs injected. It
// returns the exit status: 1 for an argv or session id Copilot rejects, 2 for
// a fixture fault, otherwise the configured exit code.
func runWithIO(args []string, stdin io.Reader, stdout, stderr io.Writer, getenv func(string) string, getwd func() (string, error), signals <-chan os.Signal) int {
	if isHelpRequest(args) {
		say(stdout, helpText)
		return 0
	}
	opts, rejection := parse(args)
	if rejection != "" {
		say(stderr, rejection)
		return 1
	}
	exit, err := configuredExitCode(getenv(exitCodeEnvironment))
	if err != nil {
		sayf(stderr, "fake-copilot: %v\n", err)
		return 2
	}
	app, code := startApp(opts, args, stdout, stderr, getenv, getwd)
	if app == nil {
		return code
	}
	if getenv(commandsEnvironment) == "1" {
		if err := app.serve(stdin, signals); err != nil {
			sayf(stderr, "fake-copilot: %v\n", err)
			return 2
		}
		return exit
	}
	app.shutdown()
	return exit
}

// startApp resolves the session and announces the launch. A nil app means
// the launch was refused and code is the exit status.
func startApp(opts options, args []string, stdout, stderr io.Writer, getenv func(string) string, getwd func() (string, error)) (*app, int) {
	id := opts.sessionID
	if !opts.sessionIDSet {
		id = newUUID()
	}
	if !uuidPattern.MatchString(id) {
		sayf(stderr, "Error: No session or task matched '%s'.\nThe value is not a valid UUID, so a new session cannot be created.\n\n"+
			"To resume an existing session or task:  copilot --session-id=<id>\n"+
			"To start a new session with ID:        copilot --session-id=<valid-uuid>\n", id)
		return nil, 1
	}
	cwd, err := getwd()
	if err != nil {
		sayf(stderr, "fake-copilot: resolve cwd: %v\n", err)
		return nil, 2
	}
	root := copilotRoot(getenv)
	if root == "" {
		sayf(stderr, "fake-copilot: neither COPILOT_HOME nor HOME is set\n")
		return nil, 2
	}
	sess, err := openSession(root, id, cwd)
	if err != nil {
		sayf(stderr, "fake-copilot: %v\n", err)
		return nil, 2
	}
	encoded, err := json.Marshal(args)
	if err != nil {
		sayf(stderr, "fake-copilot: encode argv: %v\n", err)
		return nil, 2
	}
	announceLaunch(stdout, opts, sess, encoded)
	return &app{opts: opts, sess: sess, out: stdout, errOut: stderr, hooks: newHookRunner(opts.pluginDir, cwd)}, 0
}

// copilotRoot is Copilot's own home: $COPILOT_HOME, else $HOME/.copilot.
func copilotRoot(getenv func(string) string) string {
	if home := getenv("COPILOT_HOME"); home != "" {
		return home
	}
	if home := getenv("HOME"); home != "" {
		return filepath.Join(home, ".copilot")
	}
	return ""
}

// announceLaunch prints the fixture's startup banner: small and deterministic
// so a pane assertion can prove both that the fixture started and which id and
// argv reached it.
func announceLaunch(stdout io.Writer, opts options, sess *session, encodedArgv []byte) {
	sayln(stdout, "Fake copilot")
	sayf(stdout, "fake-copilot argv: %s\n", encodedArgv)
	sayf(stdout, "fake-copilot session-id: %s\n", sess.id)
	if sess.resumed {
		sayln(stdout, "fake-copilot session: resume")
	} else {
		sayln(stdout, "fake-copilot session: new")
	}
	if opts.pluginDir != "" {
		sayf(stdout, "fake-copilot plugin-dir: %s\n", opts.pluginDir)
	}
	if opts.allowAll {
		sayln(stdout, "fake-copilot allow-all: true")
	}
	for _, tool := range opts.allowTools {
		sayf(stdout, "fake-copilot allow-tool: %s\n", tool)
	}
}

// commandLines feeds stdin's lines to the serve loop until EOF.
func commandLines(stdin io.Reader) <-chan string {
	lines := make(chan string)
	go func() {
		defer close(lines)
		scanner := bufio.NewScanner(stdin)
		for scanner.Scan() {
			lines <- scanner.Text()
		}
	}()
	return lines
}

// serve reads newline-delimited JSON pane commands until stdin closes, an
// "exit" command arrives or a termination signal does; each of the last three
// is a clean shutdown (Copilot writes its events and fires sessionEnd on them).
func (a *app) serve(stdin io.Reader, signals <-chan os.Signal) error {
	lines := commandLines(stdin)
	for {
		select {
		case line, open := <-lines:
			if !open {
				a.shutdown()
				return nil
			}
			quit, err := a.dispatch(line)
			if err != nil {
				return err
			}
			if quit {
				return nil
			}
		case <-signals:
			a.shutdown()
			return nil
		}
	}
}

func configuredExitCode(value string) (int, error) {
	if value == "" {
		return 0, nil
	}
	code, err := strconv.Atoi(value)
	if err != nil || code < 0 || code > 125 {
		return 0, fmt.Errorf("%s must be an integer from 0 through 125", exitCodeEnvironment)
	}
	return code, nil
}

// parse reads the argv deck builds. The second result is the complete stderr
// text of a rejection (exit 1), empty when the argv is accepted. Copilot
// itself accepts --continue, --resume, --connect, --remote, --acp and --yolo;
// deck never builds them, so this fixture treats them as unknown, in Copilot's
// own wording for an argument its parser does not know, which makes any deck
// regression that emits one fail loudly instead of resuming by luck.
func parse(args []string) (options, string) {
	var result options
	for index := 0; index < len(args); index++ {
		name, value, hasValue := strings.Cut(args[index], "=")
		if !knownFlags[name] {
			return result, unexpectedArgument(args[index])
		}
		if flagTakesValue(name) && !hasValue {
			if index+1 >= len(args) {
				return result, fmt.Sprintf("error: a value is required for '%s <VALUE>' but none was supplied\n\nFor more information, try '--help'.\n", name)
			}
			index++
			value = args[index]
		}
		assign(&result, name, value)
	}
	return result, ""
}

var knownFlags = map[string]bool{
	"--session-id": true, "--plugin-dir": true, "--allow-tool": true, "--allow-all": true, "--no-auto-update": true,
}

func flagTakesValue(name string) bool {
	return name == "--session-id" || name == "--plugin-dir" || name == "--allow-tool"
}

func assign(result *options, name, value string) {
	switch name {
	case "--session-id":
		result.sessionID, result.sessionIDSet = value, true
	case "--plugin-dir":
		result.pluginDir = value
	case "--allow-tool":
		result.allowTools = append(result.allowTools, value)
	case "--allow-all":
		result.allowAll = true
	case "--no-auto-update":
		result.noAutoUpdate = true
	}
}

func unexpectedArgument(argument string) string {
	return fmt.Sprintf("error: unexpected argument '%s' found\n\nFor more information, try '--help'.\n", argument)
}

const helpText = `Fake copilot fixture

Usage: fake-copilot [options]

Options:
  --session-id <uuid>   Create the session with this id, or resume it when it exists.
  --plugin-dir <dir>    Load hooks.json from this plugin directory and run its hook commands.
  --allow-tool=<tool>   Accepted (echoed in the banner), grants nothing.
  --allow-all           Accepted (echoed in the banner), grants nothing.
  --no-auto-update      Accepted.
  --help, -h            Show this help.

Every other argument, including --continue, --resume, --connect, --remote, --acp and --yolo,
is rejected with exit 1.

The session lives in $COPILOT_HOME/session-state/<id>/ (HOME/.copilot when COPILOT_HOME is unset):
workspace.yaml at launch, events.jsonl at the first prompt or at a clean shutdown, never after SIGKILL.

Set FAKE_COPILOT_EXIT_CODE to an integer from 0 through 125 to control this fixture's exit status.
Set FAKE_COPILOT_COMMANDS=1 to stay up and read newline-delimited JSON commands from the pane:
  {"command":"prompt","text":"...","hold":true}   (hold keeps the turn open: no agentStop)
  {"command":"stop"}                               (ends a held turn: agentStop)
  {"command":"notification","notification_type":"permission_prompt","title":"...","message":"..."}
  {"command":"error","message":"..."}
  {"command":"exit"}
A prompt fires userPromptSubmitted, sessionStart (the first prompt only) and, unless held, agentStop.
`
