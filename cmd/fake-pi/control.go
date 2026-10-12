package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/n-orlov/deck/internal/agent"
)

// controlPoll is how often the fixture looks at its control file, the same
// order of magnitude as the extension's own watch.
const controlPoll = 20 * time.Millisecond

// lockedWriter serialises the pane writes of the command loop and the control
// watcher, so one line never interleaves with another.
type lockedWriter struct {
	mu sync.Mutex
	w  io.Writer
}

func (l *lockedWriter) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.w.Write(p)
}

// controlFile plays the deck extension's control file (R263): it creates
// <PI_CONTROL_DIR>/<session id>.jsonl as session_start does, hands each name
// line deck appends to the session as Pi's own setSessionName would (the pane
// shows it and the extension's session_info_changed hook fires), and removes
// the file on stop as session_shutdown does.
type controlFile struct {
	path    string
	hooks   extensionHooks
	out     io.Writer
	stop    chan struct{}
	stopped chan struct{}
}

// openControlFile starts the watcher, or returns nil when the environment names
// no control directory or the session has no id (the extension then does
// nothing and deck falls back to keystrokes).
func openControlFile(hooks extensionHooks, getenv func(string) string, out io.Writer) (*controlFile, error) {
	dir := getenv(agent.PiControlDirEnv)
	if dir == "" || hooks.sessionID == "" || hooks.extension == "" {
		return nil, nil
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create control directory: %w", err)
	}
	path, ok := agent.PiControlFile(parentOf(dir), hooks.sessionID)
	if !ok || agent.PiControlDir(parentOf(dir)) != dir {
		return nil, fmt.Errorf("control directory %q is not <deck home>/pi/control", dir)
	}
	if err := os.WriteFile(path, agent.EncodePiControlPid(os.Getpid()), 0o600); err != nil {
		return nil, fmt.Errorf("create control file: %w", err)
	}
	c := &controlFile{path: path, hooks: hooks, out: out, stop: make(chan struct{}), stopped: make(chan struct{})}
	go c.watch()
	return c, nil
}

// parentOf returns <deck home> for <deck home>/pi/control.
func parentOf(dir string) string {
	return filepath.Dir(filepath.Dir(dir))
}

func (c *controlFile) watch() {
	defer close(c.stopped)
	offset := int64(len(agent.EncodePiControlPid(os.Getpid())))
	var pending []byte
	ticker := time.NewTicker(controlPoll)
	defer ticker.Stop()
	for {
		select {
		case <-c.stop:
			return
		case <-ticker.C:
			offset, pending = c.drain(offset, pending)
		}
	}
}

// drain reads what was appended past offset and applies each complete line.
func (c *controlFile) drain(offset int64, pending []byte) (int64, []byte) {
	data, err := os.ReadFile(c.path)
	if err != nil || int64(len(data)) <= offset {
		return offset, pending
	}
	pending = append(pending, data[offset:]...)
	offset = int64(len(data))
	for {
		end := bytes.IndexByte(pending, '\n')
		if end < 0 {
			return offset, pending
		}
		c.apply(pending[:end])
		pending = pending[end+1:]
	}
}

func (c *controlFile) apply(line []byte) {
	name, ok := agent.DecodePiControlLine(line)
	if !ok {
		return
	}
	sayf(c.out, "fake-pi name: %s\n", name)
	if err := c.hooks.setName(c.out, name); err != nil {
		fmt.Fprintln(os.Stderr, "fake-pi:", err)
	}
}

// close stops the watcher and removes the file, as session_shutdown does.
func (c *controlFile) close() {
	if c == nil {
		return
	}
	close(c.stop)
	<-c.stopped
	_ = os.Remove(c.path) // #nosec G703 -- path is PiControlFile of the fixture's own session id
}
