package main

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// session is one conversation's directory under Copilot's home.
type session struct {
	id      string
	cwd     string
	dir     string
	resumed bool
	// eventsOpened is set once this process has put its first records into
	// events.jsonl (session.start for a new file, session.resume for one a
	// previous process wrote).
	eventsOpened bool
	lastEvent    string
}

// openSession creates the session directory and workspace.yaml when the id is
// new, and reports a resume when the directory already holds a workspace.yaml.
// Both happen at launch, before any prompt, exactly as Copilot does.
func openSession(root, id, cwd string) (*session, error) {
	dir := filepath.Join(root, "session-state", id)
	sess := &session{id: id, cwd: cwd, dir: dir}
	if _, err := os.Stat(filepath.Join(dir, "workspace.yaml")); err == nil {
		sess.resumed = true
		return sess, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("stat workspace.yaml: %w", err)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create session directory: %w", err)
	}
	stamp := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	yaml := fmt.Sprintf("id: %s\ncwd: %s\nclient_name: github/cli\nuser_named: false\ncreated_at: %s\nupdated_at: %s\n", id, cwd, stamp, stamp)
	if err := os.WriteFile(filepath.Join(dir, "workspace.yaml"), []byte(yaml), 0o600); err != nil {
		return nil, fmt.Errorf("write workspace.yaml: %w", err)
	}
	return sess, nil
}

// eventsPath is the transcript Copilot hands to agentStop hooks.
func (s *session) eventsPath() string { return filepath.Join(s.dir, eventsName) }

// eventsName is the transcript's file name inside the session directory.
const eventsName = "events.jsonl"

// openAppend opens (creating) the file called name in dir for appending,
// through an os.Root on dir so name can only ever resolve inside it.
func openAppend(dir, name string) (*os.File, error) {
	scope, err := os.OpenRoot(dir)
	if err != nil {
		return nil, err
	}
	defer func() { _ = scope.Close() }() // the opened file stays valid after the root closes
	return scope.OpenFile(name, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
}

// record appends events to events.jsonl, first opening the file for this
// process: a missing file gets session.start and session.model_change, an
// existing one a session.resume. Each event is {type, data, id, timestamp,
// parentId}, the shape the real file has.
func (s *session) record(events ...event) error {
	file, err := openAppend(s.dir, eventsName)
	if err != nil {
		return fmt.Errorf("open %s: %w", eventsName, err)
	}
	defer func() { _ = file.Close() }() // the write errors below are the ones that matter
	if !s.eventsOpened {
		s.eventsOpened = true
		events = append(s.openingEvents(file), events...)
	}
	for _, evt := range events {
		if err := s.write(file, evt); err != nil {
			return err
		}
	}
	return nil
}

// openingEvents is what a process writes before its own first event.
func (s *session) openingEvents(file *os.File) []event {
	if info, err := file.Stat(); err == nil && info.Size() > 0 {
		return []event{{Type: "session.resume", Data: map[string]any{"sessionId": s.id, "context": map[string]any{"cwd": s.cwd}}}}
	}
	return []event{
		{Type: "session.start", Data: map[string]any{"sessionId": s.id, "copilotVersion": "fake", "context": map[string]any{"cwd": s.cwd}, "alreadyInUse": false}},
		{Type: "session.model_change", Data: map[string]any{"newModel": "fake"}},
	}
}

type event struct {
	Type string
	Data map[string]any
}

func (s *session) write(file *os.File, evt event) error {
	id := newUUID()
	record := map[string]any{"type": evt.Type, "data": evt.Data, "id": id, "timestamp": time.Now().UTC().Format("2006-01-02T15:04:05.000Z"), "parentId": nil}
	if s.lastEvent != "" {
		record["parentId"] = s.lastEvent
	}
	s.lastEvent = id
	encoded, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("encode %s event: %w", evt.Type, err)
	}
	if _, err := file.Write(append(encoded, '\n')); err != nil {
		return fmt.Errorf("write %s event: %w", evt.Type, err)
	}
	return nil
}

// newUUID returns a random version 4 UUID.
func newUUID() string {
	var b [16]byte
	_, _ = rand.Read(b[:]) // crypto/rand.Read never returns an error on supported platforms
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}
