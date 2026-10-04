package main

import (
	"io"
	"os/exec"
)

// pumpTerminal copies a pseudo-terminal's output into dst until the terminal
// closes. The read side ends with EIO on every ordinary exit of the process on
// the other end, so the copy's error carries no signal and is dropped here.
func pumpTerminal(dst io.Writer, src io.Reader) { _, _ = io.Copy(dst, src) }

// killTmuxServer stops the test's private tmux server on socket. A server that
// has already exited makes kill-server fail, which is the expected state after
// a test that shut it down itself, so the error is dropped.
func killTmuxServer(socket string) {
	_ = exec.Command("tmux", "-L", socket, "kill-server").Run()
}
