package notify

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"syscall"
)

// Start launches the configured script for one event and returns as soon as
// it is running: the detached form of Spawn (SPEC §10.3, "Skipped on the
// session-end path"). It validates and builds the argv, environment and
// stdin payload exactly as Spawn does, but the script gets no timeout, its
// output goes nowhere and nothing is recorded about how it ends, so the
// caller can exit while it keeps running. The script is its own process
// group leader, so the agent being shut down does not take it with it.
//
// The payload is handed over through a pipe the caller fills and closes
// before returning (a payload is bounded far below the pipe buffer), so the
// script never depends on this process staying alive to feed its stdin.
// An error means the script was never started and is safe to record.
func Start(req Request) error {
	if err := validate(req); err != nil {
		return redactError(err, req.SessionEnv)
	}
	message := safeMessage(req)
	safe := sanitize(req)
	body, err := buildPayload(safe, message)
	if err != nil {
		return fmt.Errorf("event hook: encode payload: %w", err)
	}
	reader, err := payloadPipe(body)
	if err != nil {
		return err
	}
	defer func() { _ = reader.Close() }()
	cmd := exec.Command(req.Command[0], argv(req)...) //nolint:gosec // G204: the configured event hook is a user-owned executable run without a shell by design (SPEC §10.1)
	cmd.Env = buildEnv(safe, message)
	cmd.Stdin = reader
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		return redactError(fmt.Errorf("event hook: start %s: %w", req.Command[0], unwrapPathError(err)), req.SessionEnv)
	}
	return cmd.Process.Release()
}

// payloadPipe returns the read end of a pipe already holding body and
// closed for writing.
func payloadPipe(body []byte) (*os.File, error) {
	reader, writer, err := os.Pipe()
	if err != nil {
		return nil, fmt.Errorf("event hook: open stdin pipe: %w", err)
	}
	_, writeErr := writer.Write(body)
	closeErr := writer.Close()
	if writeErr != nil || closeErr != nil {
		_ = reader.Close()
		return nil, fmt.Errorf("event hook: write payload: %w", errors.Join(writeErr, closeErr))
	}
	return reader, nil
}
