package notify

import (
	"errors"
	"fmt"
	"io"
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
// The payload is handed over as an already-unlinked temporary file opened
// at its start, not a pipe: writing it never waits on a reader however long
// the payload is (a pipe holds only its buffer, and a payload carrying a long
// reason exceeds it), and the script never depends on this process staying
// alive to feed its stdin. Nothing is left on disk: the file is removed
// before the script starts and lives only as long as the script's stdin.
// An error means the script was never started and is safe to record: every
// error it returns, a payload-file failure included, is scrubbed of session
// env values (SPEC §10.1).
func Start(req Request) error {
	req = stripRequestNUL(req)
	if err := validate(req); err != nil {
		return redactError(err, req.SessionEnv)
	}
	message := safeMessage(req)
	safe := sanitize(req)
	body, err := buildPayload(safe, message)
	if err != nil {
		return redactError(fmt.Errorf("event hook: encode payload: %w", err), req.SessionEnv)
	}
	reader, err := payloadFile(body)
	if err != nil {
		return redactError(err, req.SessionEnv)
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

// payloadFile returns a read handle, positioned at the start, on an unlinked
// temporary file holding body. Every step is a plain file operation, so it
// cannot block on a consumer.
func payloadFile(body []byte) (*os.File, error) {
	file, err := os.CreateTemp("", "deck-event-hook-*")
	if err != nil {
		return nil, fmt.Errorf("event hook: create payload file: %w", err)
	}
	removeErr := os.Remove(file.Name())
	_, writeErr := file.Write(body)
	_, seekErr := file.Seek(0, io.SeekStart)
	if err := errors.Join(removeErr, writeErr, seekErr); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("event hook: write payload: %w", err)
	}
	return file, nil
}
