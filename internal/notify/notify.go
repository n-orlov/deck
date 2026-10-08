package notify

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"syscall"
	"time"
	"unicode/utf8"

	"github.com/n-orlov/deck/internal/config"
)

// DefaultOutputCap bounds the captured output tail (SPEC §10.3).
const DefaultOutputCap = 2048

// killGrace bounds how long Spawn waits for a script's pipes after the
// process group was killed or the script itself exited.
const killGrace = 500 * time.Millisecond

// Request is one event to hand to the configured script.
type Request struct {
	// Command is the executable followed by any fixed arguments; the event
	// kind is appended as the final argument. It is never run by a shell.
	Command []string
	Session Session
	Event   Event
	Deck    Deck
	// BaseEnv is the inherited environment (os.Environ() in production).
	BaseEnv []string
	// SessionEnv is the session's env map. Its values are scrubbed from the
	// message and the output record (SPEC §6.4) and are never exported.
	SessionEnv map[string]string
	// Timeout, OutputCap and MessageCap default to the config default (3 s),
	// DefaultOutputCap and DefaultMessageCap when not positive.
	Timeout    time.Duration
	OutputCap  int
	MessageCap int
}

// Result is the recordable fact of one spawn (SPEC §10.3).
type Result struct {
	// ExitCode is the script's exit status, -1 when it did not exit normally
	// (timed out, killed by a signal) or never started.
	ExitCode int
	TimedOut bool
	// Output is the capped tail of stdout and stderr, scrubbed per §6.4.
	Output    string
	Truncated bool
	Duration  time.Duration
}

// Failed reports whether the result is a visible failure fact: a non-zero
// exit or a timeout.
func (r Result) Failed() bool { return r.TimedOut || r.ExitCode != 0 }

// Spawn runs the configured script for one event, without a shell, under the
// request's timeout, and kills its whole process group when that expires. An
// error means the script was never started (invalid request, missing or
// non-executable script); it is safe to record. A started script's non-zero
// exit or timeout is reported in the Result, not as an error.
func Spawn(ctx context.Context, req Request) (Result, error) {
	notStarted := Result{ExitCode: -1}
	if err := validate(req); err != nil {
		return notStarted, err
	}
	message := safeMessage(req)
	body, err := buildPayload(req, message)
	if err != nil {
		return notStarted, fmt.Errorf("event hook: encode payload: %w", err)
	}
	outCap := req.OutputCap
	if outCap <= 0 {
		outCap = DefaultOutputCap
	}
	margin := longest(scrubValues(req.SessionEnv))
	out := newTailBuffer(outCap + margin)
	timeout := req.Timeout
	if timeout <= 0 {
		timeout = config.DefaultEventHookTimeout
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := command(runCtx, req, body, buildEnv(req, message), out)
	start := time.Now()
	if err := cmd.Start(); err != nil {
		return notStarted, fmt.Errorf("event hook: start %s: %w", req.Command[0], unwrapPathError(err))
	}
	waitErr := cmd.Wait()
	return finish(req, outCap, out, errors.Is(runCtx.Err(), context.DeadlineExceeded), waitErr, time.Since(start)), nil
}

// command builds the process: no shell, own process group, payload on stdin,
// stdout and stderr folded into the tail buffer. Cancelling ctx kills the
// whole group, so a script that forked a child leaves nothing behind.
func command(ctx context.Context, req Request, body []byte, env []string, out *tailBuffer) *exec.Cmd {
	args := append(slices.Clone(req.Command[1:]), req.Event.Kind)
	cmd := exec.CommandContext(ctx, req.Command[0], args...) //nolint:gosec // G204: the configured event hook is a user-owned executable run without a shell by design (SPEC §10.1)
	cmd.Env = env
	cmd.Stdin = bytes.NewReader(body)
	cmd.Stdout = out
	cmd.Stderr = out
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	cmd.WaitDelay = killGrace
	return cmd
}

// finish turns the wait outcome into the recordable Result.
func finish(req Request, outCap int, out *tailBuffer, timedOut bool, waitErr error, took time.Duration) Result {
	res := Result{ExitCode: exitCode(waitErr), Duration: took}
	res.TimedOut = timedOut
	if res.TimedOut {
		res.ExitCode = -1
	}
	raw, dropped := out.tail()
	res.Output, res.Truncated = capTail(Redact(raw, req.SessionEnv), outCap, dropped)
	return res
}

// exitCode is the exit status of a finished wait: 0 on success, the process's
// own status on a non-zero exit, -1 for a signal death or any other failure.
func exitCode(waitErr error) int {
	if waitErr == nil {
		return 0
	}
	var exit *exec.ExitError
	if errors.As(waitErr, &exit) {
		return exit.ExitCode()
	}
	return -1
}

// capTail keeps the last limit bytes of text on a rune boundary.
func capTail(text string, limit int, dropped bool) (string, bool) {
	if len(text) <= limit {
		return text, dropped
	}
	cut := len(text) - limit
	for cut < len(text) && !utf8.RuneStart(text[cut]) {
		cut++
	}
	return text[cut:], true
}

// longest is the length of the longest string in values.
func longest(values []string) int {
	longest := 0
	for _, v := range values {
		longest = max(longest, len(v))
	}
	return longest
}

// validate refuses a request that cannot start a script, with an error that
// is safe to record: a missing or non-executable script is named by path.
func validate(req Request) error {
	if len(req.Command) == 0 || req.Command[0] == "" {
		return errors.New("event hook: no script configured")
	}
	if !slices.Contains(config.EventHookKinds, req.Event.Kind) {
		return fmt.Errorf("event hook: %q is not an offered event kind", req.Event.Kind)
	}
	return checkExecutable(req.Command[0])
}

// checkExecutable reports why path cannot be run, resolving a bare name on
// PATH like an agent binary.
func checkExecutable(name string) error {
	path, err := exec.LookPath(name)
	if err != nil {
		return fmt.Errorf("event hook: %s: %w", name, unwrapPathError(err))
	}
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return fmt.Errorf("event hook: %s is not a runnable file", name)
	}
	return nil
}

// unwrapPathError drops the duplicated path from an *fs.PathError or
// *exec.Error so the recorded text reads once.
func unwrapPathError(err error) error {
	var pe *os.PathError
	if errors.As(err, &pe) {
		return pe.Err
	}
	var ee *exec.Error
	if errors.As(err, &ee) {
		return ee.Err
	}
	return err
}
