package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"
)

// Pi keeps a session's name in its session file as `session_info` entries
// (docs/session-format.md, "SessionInfoEntry"): one JSON line per name change,
// {"type":"session_info","id":"<8 hex>","parentId":<previous entry id|null>,
// "timestamp":"<ISO-8601>","name":"<name>"}. The last one wins; a launch with
// --name appends one before session_start, and Pi's own setSessionName (the
// built-in /name, or deck's control file) appends another and emits
// session_info_changed. Pi never titles a session itself, so these are the only
// source of a name.

// sessionInfoEntry is the fields of a session_info line this fixture reads.
type sessionInfoEntry struct {
	Type string `json:"type"`
	ID   string `json:"id"`
	Name string `json:"name"`
}

// appendSessionInfo appends one session_info entry to the session file at path.
func appendSessionInfo(path, name string, now time.Time) error {
	previous, count, err := lastEntryID(path)
	if err != nil {
		return err
	}
	entry := map[string]any{
		"type":      "session_info",
		"id":        fmt.Sprintf("%08x", count+1),
		"parentId":  nil,
		"timestamp": now.UTC().Format("2006-01-02T15:04:05.000Z"),
		"name":      name,
	}
	if previous != "" {
		entry["parentId"] = previous
	}
	encoded, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("encode session_info: %w", err)
	}
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600) //nolint:gosec // G304: the session file path is derived by this fake from its own session id and home
	if err != nil {
		return fmt.Errorf("open session file: %w", err)
	}
	_, writeErr := file.Write(append(encoded, '\n'))
	closeErr := file.Close()
	if writeErr != nil {
		return fmt.Errorf("write session_info: %w", writeErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close session file: %w", closeErr)
	}
	return nil
}

// lastEntryID returns the id of the last tree entry in the session file (the
// header has none of its own) and how many entries have one.
func lastEntryID(path string) (last string, count int, err error) {
	err = eachSessionEntry(path, func(entry sessionInfoEntry) {
		if entry.Type != "session" && entry.ID != "" {
			last = entry.ID
			count++
		}
	})
	return last, count, err
}

// sessionInfoName is the session's current name: the last session_info entry's.
func sessionInfoName(path string) (name string, err error) {
	err = eachSessionEntry(path, func(entry sessionInfoEntry) {
		if entry.Type == "session_info" {
			name = strings.TrimSpace(entry.Name)
		}
	})
	return name, err
}

// eachSessionEntry calls visit for every JSON line of the session file; lines
// that are not JSON objects are skipped, a missing file has no entries.
func eachSessionEntry(path string, visit func(sessionInfoEntry)) error {
	file, err := os.Open(path) //nolint:gosec // G304: the session file path is derived by this fake from its own session id and home
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("open session file: %w", err)
	}
	defer func() { _ = file.Close() }() // read-only handle: Close cannot lose data
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var entry sessionInfoEntry
		if json.Unmarshal(scanner.Bytes(), &entry) == nil {
			visit(entry)
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read session file: %w", err)
	}
	return nil
}

// sessionFile finds or creates the session's file and returns its path; "" when
// the fixture has no id or no HOME to keep it in.
func sessionFile(opts options, getenv func(string) string, getwd func() (string, error)) (string, error) {
	if opts.sessionID == "" {
		return "", nil
	}
	dir, err := transcriptDir(getenv, getwd)
	if err != nil || dir == "" {
		return "", err
	}
	existing, err := findExistingTranscript(dir, opts.sessionID)
	if err != nil || existing != "" {
		return existing, err
	}
	cwd, err := getwd()
	if err != nil {
		return "", fmt.Errorf("resolve cwd: %w", err)
	}
	return createTranscript(dir, opts.sessionID, cwd)
}

// recordLaunchName persists --name as Pi does at startup: a session_info entry
// before session_start, appended again on a resume that passes the flag again.
// It fires no session_info_changed. Like the transcript, it is best-effort
// scaffolding: a failure is reported on stderr and never stops the fixture.
func recordLaunchName(opts options, getenv func(string) string, getwd func() (string, error)) {
	if opts.name == "" {
		return
	}
	path, err := sessionFile(opts, getenv, getwd)
	if err == nil && path != "" {
		err = appendSessionInfo(path, opts.name, time.Now())
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "fake-pi: session name unavailable:", err)
	}
}

// sessionState is what the loaded extension holds between pane commands: the
// control file session_start opened.
type sessionState struct {
	mu      sync.Mutex
	control *controlFile
	open    func(extensionHooks, io.Writer) (*controlFile, error)
}

// withSession gives the hooks the session file of this launch and the
// extension's session_start/session_shutdown state.
func (h extensionHooks) withSession(opts options, getenv func(string) string, getwd func() (string, error)) extensionHooks {
	h.transcript = func() (string, error) { return sessionFile(opts, getenv, getwd) }
	h.session = &sessionState{open: func(hooks extensionHooks, w io.Writer) (*controlFile, error) {
		return openControlFile(hooks, getenv, w)
	}}
	return h
}

// close plays session_shutdown: the control file is closed and removed.
func (h extensionHooks) close() {
	if h.session == nil {
		return
	}
	h.session.mu.Lock()
	defer h.session.mu.Unlock()
	h.session.control.close()
	h.session.control = nil
}

// currentName is the name Pi holds now, "" when it has none.
func (h extensionHooks) currentName() (string, error) {
	if h.transcript == nil {
		return "", nil
	}
	path, err := h.transcript()
	if err != nil || path == "" {
		return "", err
	}
	return sessionInfoName(path)
}

// setName plays Pi's setSessionName: the session_info entry is appended, then
// the extension reports the change. A cleared name appends an entry with no name.
func (h extensionHooks) setName(out io.Writer, name string) error {
	if h.transcript != nil {
		path, err := h.transcript()
		if err != nil {
			return err
		}
		if path != "" {
			if err := appendSessionInfo(path, name, time.Now()); err != nil {
				return err
			}
		}
	}
	return h.renamed(out, name)
}

// start plays session_start: the extension opens its control file and reports
// SessionStart with the name Pi already holds (session_name, omitted when blank).
func (h extensionHooks) start(out io.Writer) error {
	if err := h.openControl(out); err != nil {
		return err
	}
	name, err := h.currentName()
	if err != nil {
		return err
	}
	payload := map[string]any{"source": "startup"}
	if name != "" {
		payload["session_name"] = name
	}
	return h.fire(out, "SessionStart", payload)
}

func (h extensionHooks) openControl(out io.Writer) error {
	if h.session == nil {
		return nil
	}
	h.session.mu.Lock()
	defer h.session.mu.Unlock()
	if h.session.control != nil {
		return nil
	}
	control, err := h.session.open(h, out)
	h.session.control = control
	return err
}

// typedName is the name in a line typed into the pane as Pi's built-in /name,
// which is also deck's keystroke fallback (Ctrl-U, "/name <name>", Enter).
func typedName(line []byte) (name string, ok bool) {
	rest := strings.TrimLeft(string(bytes.TrimSpace(line)), "\x15")
	rest, found := strings.CutPrefix(rest, "/name")
	if !found || (rest != "" && rest[0] != ' ') {
		return "", false
	}
	return strings.TrimSpace(rest), true
}
