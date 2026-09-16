// Command fake-codex is a deterministic, black-box test fixture for the Codex CLI.
//
// It intentionally implements only the codex-cli 0.154.0 flags deck needs to
// exercise (SPEC §5, §8.2), in cmd/fake-claude's own idiom: no deck-specific IPC
// or status channel, only terminal output and eventual process exit. Codex's own
// argv shape differs from Claude's in exactly the ways SPEC §5/§8.2 document: no
// id at launch (codex mints its own), `resume <id>` as a positional subcommand
// rather than a `--resume` flag, `-a`/`-s` rather than `--permission-mode`, and
// hooks arriving as repeated `-c hooks.<Event>=…` inline TOML overrides rather
// than a `--settings` JSON blob.
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
	"regexp"
	"strconv"
	"strings"
)

const (
	// exitCodeEnvironment lets a scenario control this fixture's exit status,
	// exactly like fake-claude's own FAKE_CLAUDE_EXIT_CODE.
	exitCodeEnvironment = "FAKE_CODEX_EXIT_CODE"
	// commandsEnvironment enables the pane-side control surface (runCommands
	// below), read from stdin one JSON line at a time, exactly like
	// fake-claude's own FAKE_CLAUDE_COMMANDS.
	commandsEnvironment = "FAKE_CODEX_COMMANDS"
)

// askForApprovalValues is codex-cli 0.154.0's own accepted set for
// `-a`/`--ask-for-approval` (SPEC §5, verified against that release): only
// on-request and never. The older untrusted/on-failure values are gone
// upstream and are not accepted here either.
var askForApprovalValues = map[string]bool{
	"on-request": true,
	"never":      true,
}

// sandboxValues is codex-cli 0.154.0's own accepted set for `-s`/`--sandbox`
// (SPEC §5).
var sandboxValues = map[string]bool{
	"read-only":          true,
	"workspace-write":    true,
	"danger-full-access": true,
}

// dangerouslyBypassHookTrustFlag is the one flag that buys hook trust for the
// whole invocation (SPEC §8.2): without it every inline hook override this
// fixture parses is honoured exactly the way real codex-cli treats an
// untrusted hook -- skipped silently, no warning, no error, no exit code.
const dangerouslyBypassHookTrustFlag = "--dangerously-bypass-hook-trust"

type options struct {
	// resume is set only by the positional `resume <id>` subcommand form
	// (never a --resume flag: codex has none).
	resume string
	// askForApproval and sandbox hold -a/-s's own validated values.
	askForApproval string
	sandbox        string
	// configOverrides is every -c value, in the order given, repeats
	// included -- this fixture never collapses or rejects a repeated key.
	configOverrides []string
	// trustHooks is true only when dangerouslyBypassHookTrustFlag was
	// present in argv.
	trustHooks bool
	message    string
}

func main() {
	code, err := runWithIO(os.Args[1:], os.Stdin, os.Stdout, os.Stderr, os.Getenv)
	if err != nil {
		fmt.Fprintln(os.Stderr, "fake-codex:", err)
		code = 2
	}
	os.Exit(code)
}

// run is the no-stdin, no-stderr convenience form used by tests that don't
// drive the pane-command loop.
func run(args []string, stdout io.Writer, getenv func(string) string) (int, error) {
	return runWithIO(args, strings.NewReader(""), stdout, io.Discard, getenv)
}

func runWithIO(args []string, stdin io.Reader, stdout, stderr io.Writer, getenv func(string) string) (int, error) {
	if len(args) == 1 && (args[0] == "--help" || args[0] == "-h") {
		fmt.Fprint(stdout, helpText)
		return 0, nil
	}

	parsed, err := parse(args)
	if err != nil {
		return 0, err
	}

	encoded, err := json.Marshal(args)
	if err != nil {
		return 0, fmt.Errorf("encode argv: %w", err)
	}

	// Keep this output deliberately small and deterministic, exactly like
	// fake-claude's own banner/argv record: proof that the fixture started
	// and which argv reached it.
	fmt.Fprintln(stdout, "Fake Codex")
	fmt.Fprintf(stdout, "fake-codex argv: %s\n", encoded)
	if parsed.resume != "" {
		fmt.Fprintf(stdout, "fake-codex resume: %s\n", parsed.resume)
	}
	if parsed.askForApproval != "" {
		fmt.Fprintf(stdout, "fake-codex ask-for-approval: %s\n", parsed.askForApproval)
	}
	if parsed.sandbox != "" {
		fmt.Fprintf(stdout, "fake-codex sandbox: %s\n", parsed.sandbox)
	}

	hooks := hookCommandsFromOverrides(parsed.configOverrides)

	if getenv(commandsEnvironment) == "1" {
		if err := runCommands(stdin, stdout, stderr, hooks, parsed.trustHooks); err != nil {
			return 0, err
		}
	}

	return configuredExitCode(getenv(exitCodeEnvironment))
}

// parse implements codex-cli 0.154.0's own argv contract, narrowed to
// exactly what deck launches with (SPEC §5, §8.2):
//
//   - `resume <id>` as a positional subcommand, and only as the very first
//     argument -- codex has no --resume flag at all.
//   - `-a`/`--ask-for-approval <value>`, value restricted to
//     askForApprovalValues.
//   - `-s`/`--sandbox <value>`, value restricted to sandboxValues.
//   - `-c`/`--config <key>=<value>`, repeatable, never deduplicated.
//   - dangerouslyBypassHookTrustFlag, a bare boolean.
//   - `--session-id` and `--full-auto` are explicitly rejected: both are
//     Claude-shaped flags codex-cli 0.154.0 does not have (the older
//     --full-auto is gone upstream, and codex never took a caller-assigned
//     session id to begin with -- SPEC §5).
//
// Anything else starting with '-' is an unknown option and is rejected the
// same way. Remaining positional tokens are joined as the trailing prompt.
func parse(args []string) (options, error) {
	var result options
	var message []string
	for index := 0; index < len(args); index++ {
		argument := args[index]

		if index == 0 && argument == "resume" {
			if index+1 == len(args) {
				return result, errors.New(`"resume" requires a conversation id`)
			}
			result.resume = args[index+1]
			index++
			continue
		}

		if argument == "--session-id" {
			return result, errors.New("--session-id is not a codex-cli flag: codex mints its own conversation id")
		}
		if argument == "--full-auto" {
			return result, errors.New("--full-auto was removed from codex-cli 0.154.0")
		}
		if argument == dangerouslyBypassHookTrustFlag {
			result.trustHooks = true
			continue
		}

		if len(argument) > 1 && argument[0] == '-' {
			if index+1 == len(args) {
				return result, fmt.Errorf("option %q requires a value", argument)
			}
			value := args[index+1]
			index++
			switch argument {
			case "-a", "--ask-for-approval":
				if !askForApprovalValues[value] {
					return result, fmt.Errorf("invalid value for %s: %q", argument, value)
				}
				result.askForApproval = value
			case "-s", "--sandbox":
				if !sandboxValues[value] {
					return result, fmt.Errorf("invalid value for %s: %q", argument, value)
				}
				result.sandbox = value
			case "-c", "--config":
				result.configOverrides = append(result.configOverrides, value)
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

// hookOverrideValuePattern matches exactly the -c VALUE shape
// internal/agent/codex.go's own codexHookOverride produces: a single hook
// group containing exactly one hook of type "command" (SPEC §8.2). This
// fixture parses only that shape -- it is not a general TOML parser, and
// has no need to be one.
var hookOverrideValuePattern = regexp.MustCompile(`^\[\{hooks=\[\{type="command",command="((?:[^"\\]|\\.)*)"\}\]\}\]$`)

// parseHookOverride decodes one -c value of the form
// `hooks.<Event>=[{hooks=[{type="command",command="…"}]}]` into the event
// name and the (TOML-unescaped) command string. ok is false for any -c
// value that isn't a hooks.* override in that exact shape -- a config
// override this fixture doesn't otherwise interpret, never an error.
func parseHookOverride(raw string) (event, command string, ok bool) {
	key, value, found := strings.Cut(raw, "=")
	if !found || !strings.HasPrefix(key, "hooks.") {
		return "", "", false
	}
	match := hookOverrideValuePattern.FindStringSubmatch(value)
	if match == nil {
		return "", "", false
	}
	return strings.TrimPrefix(key, "hooks."), codexTOMLUnescapeString(match[1]), true
}

// codexTOMLUnescapeString reverses internal/agent/codex.go's own
// codexTOMLEscapeString: a backslash always escapes exactly the byte that
// follows it (only backslash and double-quote are ever produced by the
// encoder, but this reversal is correct for any escaped byte).
func codexTOMLUnescapeString(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '\\' && i+1 < len(s) {
			i++
			b.WriteByte(s[i])
			continue
		}
		b.WriteByte(c)
	}
	return b.String()
}

// hookCommandsFromOverrides extracts every hooks.<Event> command this
// invocation's -c overrides carried, keyed by event name. Non-hook -c
// overrides are parsed and silently ignored -- this fixture has no other
// config surface to apply them to.
func hookCommandsFromOverrides(overrides []string) map[string]string {
	hooks := make(map[string]string)
	for _, raw := range overrides {
		if event, command, ok := parseHookOverride(raw); ok {
			hooks[event] = command
		}
	}
	return hooks
}

// fireHook is the silent trust gate itself (SPEC §8.2): when trusted is
// false -- dangerouslyBypassHookTrustFlag was absent from argv -- this is a
// complete no-op, whether or not event was ever injected via -c: no error,
// no output, no exit code, exactly as real codex-cli skips an untrusted
// hook. Only when trusted is true does it actually run the injected
// command, on "sh -c", with payload on its stdin and this process's own
// (deck-injected) environment inherited -- never invoking "deck _hook"
// itself, exactly as fake-claude's own fireHook never does.
func fireHook(stdout, stderr io.Writer, hooks map[string]string, trusted bool, event string, payload map[string]any) error {
	if !trusted {
		return nil
	}
	command, ok := hooks[event]
	if !ok || command == "" {
		return fmt.Errorf("hook event %q was not injected via -c", event)
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
	fmt.Fprintf(stdout, "fake-codex hook fired: %s\n", event)
	return nil
}

// runCommands is the pane-side control surface used by black-box scenarios,
// in fake-claude's own idiom: each input line asks this fixture to fire an
// injected hook. Unlike fake-claude, there is no "fixture"/"resume" pane
// command here yet -- session lifecycle (minting an id, firing on first
// prompt, the rollout transcript) is a later task's own deliverable; this
// fixture's job is exactly the argv contract and the trust gate.
func runCommands(input io.Reader, stdout, stderr io.Writer, hooks map[string]string, trusted bool) error {
	scanner := bufio.NewScanner(input)
	for scanner.Scan() {
		var request struct {
			Command string         `json:"command"`
			Event   string         `json:"event"`
			Payload map[string]any `json:"payload"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &request); err != nil {
			return fmt.Errorf("decode command: %w", err)
		}
		switch request.Command {
		case "hook":
			if err := fireHook(stdout, stderr, hooks, trusted, request.Event, request.Payload); err != nil {
				return err
			}
		case "exit":
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

func configuredExitCode(value string) (int, error) {
	if value == "" {
		return 0, nil
	}
	code, err := strconv.Atoi(value)
	if err != nil || code < 0 || code > 125 {
		return 0, errors.New("FAKE_CODEX_EXIT_CODE must be an integer from 0 through 125")
	}
	return code, nil
}

const helpText = `Fake Codex fixture

Usage: fake-codex [options] [prompt]
       fake-codex resume <id> [options]

Options:
  -a, --ask-for-approval <value>   One of: on-request, never.
  -s, --sandbox <value>            One of: read-only, workspace-write, danger-full-access.
  -c, --config <key>=<value>       Set a config override; repeatable. hooks.<Event>=… overrides
                                    are parsed for this fixture's own hook-firing surface below.
  --dangerously-bypass-hook-trust  Trust every hooks.* override for this invocation.
  --help, -h                       Show this help.

--session-id and --full-auto are not codex-cli 0.154.0 flags and are rejected.

Set FAKE_CODEX_EXIT_CODE to an integer from 0 through 125 to control this fixture's exit status.
Set FAKE_CODEX_COMMANDS=1 to read newline-delimited commands from the pane. A hook command has
the form {"command":"hook","event":"SessionStart","payload":{...}}. Without
--dangerously-bypass-hook-trust this is always a silent no-op -- no output, no error, no exit
code -- exactly as real codex-cli skips an untrusted hook. With it present, the event's own -c
hooks.<Event>=… command runs on "sh -c" with payload on its stdin and this process's inherited
environment, never invoking "deck _hook" directly. An exit command has the form
{"command":"exit"} and ends the loop with a clean status 0 exit.
`
