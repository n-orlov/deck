package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/n-orlov/deck/internal/store"
)

// hookReexecEnv marks a process that is itself a re-exec of an older `_hook`.
// It guards the loop: a re-exec'd process that still meets a newer schema
// prints the R204 message and exits, it never re-execs again (SPEC §3.1).
const hookReexecEnv = "DECK_HOOK_REEXEC"

// writerProbeTimeout bounds the `_schema` probe of the recorded writer, so a
// wedged binary cannot hold the agent's hook past one short wait.
const writerProbeTimeout = 3 * time.Second

// writerRunTimeout bounds the whole re-exec'd writer run (a package variable
// so a test can shorten it). A writer that outlives it is killed and the hook
// falls back to the R204 restart message, so a wedged writer cannot hold the
// agent's hook open.
var writerRunTimeout = 10 * time.Second

// writerKillGrace bounds how long a killed writer's inherited output pipes
// may keep the hook waiting (a grandchild holding stdout open).
const writerKillGrace = time.Second

// hookVerb is the whole argv (past argv[0]) of every hook invocation: run()
// takes the hook path only for exactly `deck _hook`, so the re-exec passes the
// same single verb on unchanged.
const hookVerb = "_hook"

// hookReexecExit is returned when the re-exec'd writer ran and exited
// non-zero. Its own stderr already said why, so run() prints nothing more and
// exits with the same code.
type hookReexecExit struct{ code int }

func (e *hookReexecExit) Error() string {
	return "re-exec'd hook exited " + strconv.Itoa(e.code)
}

// printSchema answers the hidden `_schema` verb: the newest state-database
// schema this binary supports, one integer on stdout. `_hook` asks the
// recorded writer binary for it before re-execing it.
func printSchema(w io.Writer) { sayf(w, "%d\n", store.SupportedSchemaVersion()) }

func isSchemaRequest(args []string) bool { return len(args) == 2 && args[1] == "_schema" }

// healHook is runHook's answer to a store open that failed. When err is a
// newer-schema refusal and the recorded writer binary can take over, it
// re-execs that binary and reports handled=true with the outcome (nil, or a
// *hookReexecExit); otherwise handled=false and the caller reports err.
func healHook(ctx context.Context, err error, payload []byte) (handled bool, outcome error) {
	var newer *store.NewerSchemaError
	if !errors.As(err, &newer) || os.Getenv(hookReexecEnv) != "" {
		return false, nil
	}
	self, selfErr := os.Executable()
	if selfErr != nil {
		return false, nil
	}
	return reexecHook(ctx, newer, self, payload, os.Stdout, os.Stderr)
}

// reexecHook runs the recorded writer with the same argv (the single hook verb), the buffered stdin
// payload and the loop-guard marker, once, when the writer is usable.
func reexecHook(ctx context.Context, newer *store.NewerSchemaError, self string, payload []byte, stdout, stderr io.Writer) (bool, error) {
	writer := newer.WriterBinary
	if !usableWriter(ctx, writer, self, newer.DB) {
		return false, nil
	}
	return runWriter(ctx, writer, payload, stdout, stderr)
}

// runWriter runs the vetted writer binary with the hook verb, the buffered
// payload on stdin and the loop-guard marker in its environment, and maps its
// exit to the outcome reexecHook reports. The run is bounded by
// writerRunTimeout: a writer still running at that point is killed and the
// result is handled=false, so the caller prints the R204 restart message.
func runWriter(ctx context.Context, writer string, payload []byte, stdout, stderr io.Writer) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, writerRunTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, writer, hookVerb)
	cmd.WaitDelay = writerKillGrace
	cmd.Env = append(os.Environ(), hookReexecEnv+"=1")
	cmd.Stdin = bytes.NewReader(payload)
	cmd.Stdout, cmd.Stderr = stdout, stderr
	err := cmd.Run()
	var exit *exec.ExitError
	switch {
	case ctx.Err() != nil:
		return false, nil
	case err == nil:
		return true, nil
	case errors.As(err, &exit):
		return true, &hookReexecExit{code: exit.ExitCode()}
	}
	return false, nil
}

// usableWriter reports whether the recorded writer exists, is an executable
// regular file, is not self, and reports a schema at least as new as the
// database's. Any failure means no re-exec and the R204 message instead.
func usableWriter(ctx context.Context, writer, self string, dbSchema int) bool {
	if writer == "" || !executableFile(writer) || sameFile(writer, self) {
		return false
	}
	schema, err := writerSchema(ctx, writer)
	return err == nil && schema >= dbSchema
}

func executableFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular() && info.Mode().Perm()&0o111 != 0
}

func sameFile(a, b string) bool {
	ai, aErr := os.Stat(a)
	bi, bErr := os.Stat(b)
	return aErr == nil && bErr == nil && os.SameFile(ai, bi)
}

// writerSchema asks the writer binary for its supported schema through the
// hidden `_schema` verb.
func writerSchema(ctx context.Context, writer string) (int, error) {
	ctx, cancel := context.WithTimeout(ctx, writerProbeTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, writer, "_schema")
	cmd.Env = append(os.Environ(), hookReexecEnv+"=1")
	out, err := cmd.Output()
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(strings.TrimSpace(string(out)))
}
