package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite" // the driver for the fake's own copy of state_5.sqlite
)

// Codex keeps a thread's name in two places, and these are the shapes the
// real CLI writes (issue for this harness, "Disk"):
//
//	<codex home>/state_5.sqlite, table threads, column name  NULL while unnamed
//	<codex home>/session_index.jsonl
//	    {"id":"...","thread_name":"...","updated_at":"..."}  latest line per id wins
//
// The rollout transcript carries no name record. The thread row appears with
// the first prompt, so a /rename typed before it is held in memory (the pane
// title shows it at once) and persisted when the thread is created, and the
// generated title that follows the first prompt never replaces it. Codex gives
// no hook payload a name. The terminal title it sets is
// "<name> | <cwd basename>".
const (
	stateFile          = "state_5.sqlite"
	sessionIndexFile   = "session_index.jsonl"
	renameLinePrefix   = "/rename "
	threadsDDL         = `CREATE TABLE IF NOT EXISTS threads (id TEXT PRIMARY KEY, rollout_path TEXT, cwd TEXT, name TEXT, created_at INTEGER, updated_at INTEGER)`
	paneTitleSeparator = " | "
)

// threadNames is the fake's own persisted name state for its one conversation.
// A nil *threadNames is valid and does nothing (no Codex home to write to).
type threadNames struct {
	home     string
	cwd      string
	out      io.Writer
	threadID string
	// held is a name typed before the thread exists.
	held string
	// settled is true once the thread has a name, whether a human chose it or
	// the generated title took the slot: Codex generates a title once.
	settled bool
}

// newThreadNames returns the name state for a Codex home, or nil when there is
// none.
func newThreadNames(home string, out io.Writer) *threadNames {
	if home == "" {
		return nil
	}
	cwd, _ := os.Getwd()
	return &threadNames{home: home, cwd: cwd, out: out}
}

// typedRename recognises the line deck types to push a name ("/rename <name>"
// submitted with Enter).
func typedRename(line string) (string, bool) {
	rest, ok := strings.CutPrefix(strings.TrimSpace(line), renameLinePrefix)
	rest = strings.TrimSpace(rest)
	return rest, ok && rest != ""
}

// start creates the thread row with the conversation's first prompt (or finds
// the one a resumed conversation already has) and persists a held name.
func (n *threadNames) start(id, rolloutPath string) error {
	if n == nil {
		return nil
	}
	n.threadID = id
	now := time.Now().UTC().Unix()
	err := n.withDB(func(db *sql.DB) error {
		if _, err := db.Exec(`INSERT OR IGNORE INTO threads (id, rollout_path, cwd, name, created_at, updated_at) VALUES (?, ?, ?, NULL, ?, ?)`, id, rolloutPath, n.cwd, now, now); err != nil {
			return err
		}
		var existing sql.NullString
		if err := db.QueryRow(`SELECT name FROM threads WHERE id = ?`, id).Scan(&existing); err != nil {
			return err
		}
		n.settled = existing.Valid && existing.String != ""
		return nil
	})
	if err != nil {
		return err
	}
	if held := n.held; held != "" {
		n.held = ""
		return n.store(held)
	}
	return nil
}

// rename is a human naming the thread (a typed /rename): it supersedes any
// generated title and any held name.
func (n *threadNames) rename(name string) error {
	name = strings.TrimSpace(name)
	if n == nil || name == "" {
		return nil
	}
	if n.threadID == "" {
		n.held = name
		n.setPaneTitle(name)
		return nil
	}
	return n.store(name)
}

// generate is the background auto-title: it names a thread that has no name
// yet, once, and is ignored for a thread a human already named.
func (n *threadNames) generate(title string) error {
	title = strings.TrimSpace(title)
	if n == nil || title == "" {
		return nil
	}
	if n.threadID == "" {
		return errors.New(`"ai-title" requires a "prompt" command first: the thread does not exist yet`)
	}
	if n.settled {
		return nil
	}
	return n.store(title)
}

// store writes the name to the database and the session index, then sets the
// pane title.
func (n *threadNames) store(name string) error {
	now := time.Now().UTC()
	err := n.withDB(func(db *sql.DB) error {
		_, err := db.Exec(`UPDATE threads SET name = ?, updated_at = ? WHERE id = ?`, name, now.Unix(), n.threadID)
		return err
	})
	if err != nil {
		return err
	}
	if err := n.appendIndex(name, now); err != nil {
		return err
	}
	n.settled = true
	n.setPaneTitle(name)
	return nil
}

func (n *threadNames) withDB(apply func(*sql.DB) error) (err error) {
	if err := os.MkdirAll(n.home, 0o750); err != nil {
		return fmt.Errorf("create codex home: %w", err)
	}
	// busy_timeout: deck and the scenario read this file while the fake writes
	// it; without a busy handler a write that overlaps a reader fails at once
	// with "database is locked", the fake exits and the pane dies.
	db, err := sql.Open("sqlite", "file:"+filepath.Join(n.home, stateFile)+"?_pragma=busy_timeout(5000)")
	if err != nil {
		return fmt.Errorf("open %s: %w", stateFile, err)
	}
	defer func() {
		if closeErr := db.Close(); err == nil && closeErr != nil {
			err = closeErr
		}
	}()
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(threadsDDL); err != nil {
		return fmt.Errorf("create threads table: %w", err)
	}
	return apply(db)
}

func (n *threadNames) appendIndex(name string, at time.Time) error {
	line, err := json.Marshal(map[string]string{"id": n.threadID, "thread_name": name, "updated_at": at.Format(time.RFC3339Nano)})
	if err != nil {
		return err
	}
	file, err := os.OpenFile(filepath.Join(n.home, sessionIndexFile), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("open %s: %w", sessionIndexFile, err)
	}
	if _, err := file.Write(append(line, '\n')); err != nil {
		_ = file.Close()
		return fmt.Errorf("append %s: %w", sessionIndexFile, err)
	}
	return file.Close()
}

// setPaneTitle sets the terminal title the way Codex does, with OSC 0.
func (n *threadNames) setPaneTitle(name string) {
	sayf(n.out, "\x1b]0;%s%s%s\x07", name, paneTitleSeparator, filepath.Base(n.cwd))
}

// nameCommand runs the name pane commands: "rename" (a human /rename inside
// Codex) and "ai-title" (the generated title).
func (n *threadNames) nameCommand(request paneRequest) (handled bool, err error) {
	switch request.Command {
	case "rename", "ai-title":
	default:
		return false, nil
	}
	if n == nil {
		return true, fmt.Errorf("%q needs a Codex home: set CODEX_HOME or HOME", request.Command)
	}
	if strings.TrimSpace(request.Name) == "" {
		return true, errors.New(request.Command + ` requires a non-blank "name"`)
	}
	if request.Command == "rename" {
		return true, n.rename(request.Name)
	}
	return true, n.generate(request.Name)
}
