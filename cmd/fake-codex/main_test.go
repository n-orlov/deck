package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRejectsSessionIDAndFullAuto proves both Claude-shaped flags codex-cli
// 0.154.0 does not have are rejected outright (SPEC §5): codex mints its own
// conversation id, and --full-auto was removed upstream.
func TestRejectsSessionIDAndFullAuto(t *testing.T) {
	for name, arguments := range map[string][]string{
		"--session-id": {"--session-id", "123e4567-e89b-12d3-a456-426614174000"},
		"--full-auto":  {"--full-auto"},
	} {
		t.Run(name, func(t *testing.T) {
			var output bytes.Buffer
			if _, err := run(arguments, &output, func(string) string { return "" }); err == nil {
				t.Fatalf("run(%q) unexpectedly succeeded", arguments)
			}
		})
	}
}

// TestAcceptsResumeAsPositionalSubcommand proves `resume <id>` works as a
// positional subcommand -- codex has no --resume flag at all -- and that it
// is only recognised as the very first argument.
func TestAcceptsResumeAsPositionalSubcommand(t *testing.T) {
	var output bytes.Buffer
	code, err := run([]string{"resume", "some-conversation-id", "-a", "never"}, &output, func(string) string { return "" })
	if err != nil || code != 0 {
		t.Fatalf("run = (%d, %v)", code, err)
	}
	if !strings.Contains(output.String(), "fake-codex resume: some-conversation-id") {
		t.Fatalf("output %q does not record the resumed id", output.String())
	}

	// "resume" is not treated as a subcommand once other tokens precede it;
	// it lands as ordinary trailing prompt text instead.
	var later bytes.Buffer
	if _, err := run([]string{"-a", "never", "resume", "not-a-subcommand-here"}, &later, func(string) string { return "" }); err != nil {
		t.Fatalf("run: %v", err)
	}
	if strings.Contains(later.String(), "fake-codex resume:") {
		t.Fatalf("output %q treated a non-leading \"resume\" as the subcommand", later.String())
	}

	if _, err := run([]string{"resume"}, &output, func(string) string { return "" }); err == nil {
		t.Fatal(`"resume" with no id unexpectedly succeeded`)
	}
}

// TestAcceptsOnlyDocumentedApprovalValues proves -a accepts exactly
// on-request and never, and rejects anything else -- including the older
// untrusted/on-failure values that codex-cli 0.154.0 removed.
func TestAcceptsOnlyDocumentedApprovalValues(t *testing.T) {
	for _, value := range []string{"on-request", "never"} {
		t.Run("accepts "+value, func(t *testing.T) {
			var output bytes.Buffer
			if code, err := run([]string{"-a", value}, &output, func(string) string { return "" }); err != nil || code != 0 {
				t.Fatalf("run = (%d, %v)", code, err)
			}
			if !strings.Contains(output.String(), "fake-codex ask-for-approval: "+value) {
				t.Fatalf("output %q does not record -a %s", output.String(), value)
			}
		})
	}
	for _, value := range []string{"untrusted", "on-failure", "always"} {
		t.Run("rejects "+value, func(t *testing.T) {
			var output bytes.Buffer
			if _, err := run([]string{"-a", value}, &output, func(string) string { return "" }); err == nil {
				t.Fatalf("run(-a %s) unexpectedly succeeded", value)
			}
		})
	}
}

// TestAcceptsOnlyDocumentedSandboxValues proves -s accepts exactly the three
// codex-cli 0.154.0 sandbox modes and rejects anything else.
func TestAcceptsOnlyDocumentedSandboxValues(t *testing.T) {
	for _, value := range []string{"read-only", "workspace-write", "danger-full-access"} {
		t.Run("accepts "+value, func(t *testing.T) {
			var output bytes.Buffer
			if code, err := run([]string{"-s", value}, &output, func(string) string { return "" }); err != nil || code != 0 {
				t.Fatalf("run = (%d, %v)", code, err)
			}
			if !strings.Contains(output.String(), "fake-codex sandbox: "+value) {
				t.Fatalf("output %q does not record -s %s", output.String(), value)
			}
		})
	}
	for _, value := range []string{"full-access", "none", "danger-full-access-plus"} {
		t.Run("rejects "+value, func(t *testing.T) {
			var output bytes.Buffer
			if _, err := run([]string{"-s", value}, &output, func(string) string { return "" }); err == nil {
				t.Fatalf("run(-s %s) unexpectedly succeeded", value)
			}
		})
	}
}

// TestAcceptsRepeatedConfigOverrides proves -c is accepted repeatedly, never
// deduplicated or rejected on a repeated key.
func TestAcceptsRepeatedConfigOverrides(t *testing.T) {
	arguments := []string{
		"-c", "model=gpt-5",
		"-c", "model=gpt-5-again",
		"-c", `hooks.SessionStart=[{hooks=[{type="command",command="echo hi"}]}]`,
	}
	parsed, err := parse(arguments)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(parsed.configOverrides) != 3 {
		t.Fatalf("configOverrides = %v, want 3 entries", parsed.configOverrides)
	}
	if parsed.configOverrides[0] != "model=gpt-5" || parsed.configOverrides[1] != "model=gpt-5-again" {
		t.Fatalf("configOverrides = %v, want both repeated model= values preserved in order", parsed.configOverrides)
	}
}

// TestParseHookOverrideRoundTripsAgainstTheRealEncoder proves this fixture's
// decoder correctly reverses internal/agent/codex.go's own -c value shape,
// including a command carrying a backslash and a quote.
func TestParseHookOverrideRoundTripsAgainstTheRealEncoder(t *testing.T) {
	for name, command := range map[string]string{
		"plain path":          `/usr/local/bin/deck _hook`,
		"space in path":       `/opt/My Deck/deck _hook`,
		"quote and backslash": `/opt/"weird"\deck _hook`,
	} {
		t.Run(name, func(t *testing.T) {
			escaped := strings.ReplaceAll(command, `\`, `\\`)
			escaped = strings.ReplaceAll(escaped, `"`, `\"`)
			raw := `hooks.SessionStart=[{hooks=[{type="command",command="` + escaped + `"}]}]`
			event, decoded, ok := parseHookOverride(raw)
			if !ok {
				t.Fatalf("parseHookOverride(%q) declined to match", raw)
			}
			if event != "SessionStart" {
				t.Fatalf("event = %q, want SessionStart", event)
			}
			if decoded != command {
				t.Fatalf("decoded command = %q, want %q", decoded, command)
			}
		})
	}

	if _, _, ok := parseHookOverride("model=gpt-5"); ok {
		t.Fatal("a non-hooks -c override was unexpectedly decoded as a hook")
	}
}

// TestHookTrustGateFiresOnlyWhenBypassFlagIsPresent is the trust gate's own
// contract test, in both directions (SPEC §8.2): without
// --dangerously-bypass-hook-trust, a pane "hook" command is a complete,
// silent no-op -- no subprocess runs, no output, no error, exit 0. With the
// flag present, the same command actually runs the injected command.
func TestHookTrustGateFiresOnlyWhenBypassFlagIsPresent(t *testing.T) {
	directory := t.TempDir()
	record := filepath.Join(directory, "hook-record.txt")
	hook := filepath.Join(directory, "injected-hook")
	script := fmt.Sprintf("#!/bin/sh\ncat >> %q\n", record)
	if err := os.WriteFile(hook, []byte(script), 0o755); err != nil {
		t.Fatalf("write hook recorder: %v", err)
	}

	override := fmt.Sprintf(`hooks.SessionStart=[{hooks=[{type="command",command="%s"}]}]`, strings.ReplaceAll(hook, `\`, `\\`))
	input := `{"command":"hook","event":"SessionStart","payload":{"session_id":"abc"}}` + "\n"
	getenv := func(key string) string {
		if key == commandsEnvironment {
			return "1"
		}
		return ""
	}

	t.Run("untrusted: silent no-op", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code, err := runWithIO([]string{"-c", override}, strings.NewReader(input), &stdout, &stderr, getenv)
		if err != nil || code != 0 {
			t.Fatalf("runWithIO = (%d, %v), stderr %q", code, err, stderr.String())
		}
		if strings.Contains(stdout.String(), "hook fired") {
			t.Fatalf("untrusted invocation reported a fired hook: %q", stdout.String())
		}
		if stderr.String() != "" {
			t.Fatalf("untrusted invocation warned on stderr: %q", stderr.String())
		}
		if _, err := os.Stat(record); !os.IsNotExist(err) {
			t.Fatalf("untrusted invocation ran the injected hook (record exists, err=%v)", err)
		}
	})

	t.Run("trusted: fires", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code, err := runWithIO([]string{"-c", override, dangerouslyBypassHookTrustFlag}, strings.NewReader(input), &stdout, &stderr, getenv)
		if err != nil || code != 0 {
			t.Fatalf("runWithIO = (%d, %v), stderr %q", code, err, stderr.String())
		}
		if !strings.Contains(stdout.String(), "fake-codex hook fired: SessionStart") {
			t.Fatalf("trusted invocation did not acknowledge the fired hook: %q", stdout.String())
		}
		recorded, err := os.ReadFile(record)
		if err != nil {
			t.Fatalf("read hook record: %v", err)
		}
		if !strings.Contains(string(recorded), `"session_id":"abc"`) {
			t.Fatalf("hook record %q does not carry the payload", recorded)
		}
		if !strings.Contains(string(recorded), `"hook_event_name":"SessionStart"`) {
			t.Fatalf("hook record %q does not carry hook_event_name", recorded)
		}
	})
}

// TestPaneHookCommandRequiresInjectedOverrideEvenWhenTrusted proves that
// trust alone is not enough: an event that was never injected via -c still
// errors, even with --dangerously-bypass-hook-trust present.
func TestPaneHookCommandRequiresInjectedOverrideEvenWhenTrusted(t *testing.T) {
	input := `{"command":"hook","event":"Stop","payload":{}}` + "\n"
	getenv := func(key string) string {
		if key == commandsEnvironment {
			return "1"
		}
		return ""
	}
	var stdout, stderr bytes.Buffer
	_, err := runWithIO([]string{dangerouslyBypassHookTrustFlag}, strings.NewReader(input), &stdout, &stderr, getenv)
	if err == nil || !strings.Contains(err.Error(), `was not injected via -c`) {
		t.Fatalf("runWithIO error = %v, want missing injected override", err)
	}
}

// TestExitCodeIsControlledOnlyByFixtureEnvironment mirrors fake-claude's own
// exit-code contract test.
func TestExitCodeIsControlledOnlyByFixtureEnvironment(t *testing.T) {
	var output bytes.Buffer
	getenv := func(key string) string {
		if key == exitCodeEnvironment {
			return "17"
		}
		return ""
	}
	if code, err := run(nil, &output, getenv); err != nil || code != 17 {
		t.Fatalf("run = (%d, %v), want (17, nil)", code, err)
	}
	if _, err := configuredExitCode("126"); err == nil {
		t.Fatal("out-of-range fixture exit code was accepted")
	}
}

// TestUnknownOptionIsRejected proves an arbitrary unrecognised flag is
// rejected the same way --session-id/--full-auto are, rather than silently
// accepted or treated as a positional prompt.
func TestUnknownOptionIsRejected(t *testing.T) {
	var output bytes.Buffer
	if _, err := run([]string{"--not-a-real-flag", "value"}, &output, func(string) string { return "" }); err == nil {
		t.Fatal("run with an unknown flag unexpectedly succeeded")
	}
}
