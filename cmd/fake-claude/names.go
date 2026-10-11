package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

// Claude names a session through records appended to its transcript
// (<config>/projects/<cwd>/<id>.jsonl) and through the terminal title it sets
// with OSC 0 ("✳ <title>"). These are the shapes the real CLI writes (issue
// for this harness, "What's on disk"):
//
//	{"type":"custom-title","customTitle":"...","sessionId":"..."}  --name, /rename
//	{"type":"agent-name","agentName":"...","sessionId":"..."}      paired with custom-title
//	{"type":"ai-title","aiTitle":"...","sessionId":"..."}          generated, re-appended
//
// A name a human chose (--name, /rename) is explicit: it suppresses generated
// titles, and SessionStart/UserPromptSubmit carry it as session_title. A
// generated title never rides a hook payload.
const (
	recordCustomTitle = "custom-title"
	recordAgentName   = "agent-name"
	recordAITitle     = "ai-title"
	// paneTitleGlyph is the activity glyph Claude puts before its title.
	paneTitleGlyph = "✳"
	// renameLinePrefix is what deck types into the pane to push a name.
	renameLinePrefix = "/rename "
)

// titleRecord is one name-bearing transcript line.
type titleRecord struct {
	Type        string `json:"type"`
	SessionID   string `json:"sessionId"`
	CustomTitle string `json:"customTitle,omitempty"`
	AgentName   string `json:"agentName,omitempty"`
	AITitle     string `json:"aiTitle,omitempty"`
}

// names is the fake's own persisted name state for one conversation. A nil
// *names is valid and does nothing (no conversation id, or no HOME).
type names struct {
	sessionID string
	path      string
	out       io.Writer
	explicit  string
	title     string
	// unwritten is a launch name not yet in the transcript: Claude creates a
	// conversation's transcript with its first prompt, so a --name given to a
	// conversation that has none is held until then (flush).
	unwritten string
}

// newNames returns the name state for the conversation the options key, or nil
// when the fake has nowhere to persist one.
func newNames(o options, getenv func(string) string, getwd func() (string, error), out io.Writer) *names {
	id := o.conversationID()
	if id == "" {
		return nil
	}
	path, err := transcriptPath(getenv, getwd, id)
	if err != nil || path == "" {
		return nil
	}
	n := &names{sessionID: id, path: path, out: out}
	n.load()
	return n
}

// load restores the last title a resumed conversation already carries, as the
// real CLI does when it reopens a transcript.
func (n *names) load() {
	file, err := os.Open(n.path)
	if err != nil {
		return
	}
	defer func() { _ = file.Close() }() // read-only handle
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var rec titleRecord
		if json.Unmarshal(scanner.Bytes(), &rec) != nil {
			continue
		}
		switch {
		case rec.Type == recordCustomTitle && rec.CustomTitle != "":
			n.explicit, n.title = rec.CustomTitle, rec.CustomTitle
		case rec.Type == recordAITitle && rec.AITitle != "" && n.explicit == "":
			n.title = rec.AITitle
		}
	}
	if n.title != "" {
		n.setPaneTitle()
	}
}

// launch applies --name. On a conversation with no transcript yet the name
// shows in the pane title at once and reaches the transcript with the first
// prompt; on a resumed one it is appended immediately.
func (n *names) launch(name string) error {
	name = strings.TrimSpace(name)
	if n == nil || name == "" {
		return nil
	}
	if _, err := os.Stat(n.path); err == nil { //nolint:gosec // G703: the transcript path comes from this fake's own session id and home
		return n.rename(name)
	}
	n.explicit, n.title, n.unwritten = name, name, name
	n.setPaneTitle()
	return nil
}

// flush writes a held launch name to the transcript; the first prompt calls it.
func (n *names) flush() error {
	if n == nil || n.unwritten == "" {
		return nil
	}
	name := n.unwritten
	n.unwritten = ""
	return n.writeExplicit(name)
}

// rename is a human naming the session (a typed /rename, a hook's
// sessionTitle): it supersedes any held launch name.
func (n *names) rename(name string) error {
	name = strings.TrimSpace(name)
	if n == nil || name == "" {
		return nil
	}
	n.unwritten = ""
	return n.writeExplicit(name)
}

// writeExplicit appends the custom-title and its paired agent-name record,
// then sets the pane title.
func (n *names) writeExplicit(name string) error {
	for _, rec := range []titleRecord{
		{Type: recordCustomTitle, SessionID: n.sessionID, CustomTitle: name},
		{Type: recordAgentName, SessionID: n.sessionID, AgentName: name},
	} {
		if err := appendJSON(n.path, rec); err != nil {
			return err
		}
	}
	n.explicit, n.title = name, name
	n.setPaneTitle()
	return nil
}

// generate is the background auto-title: an ai-title record, re-appended
// whenever the title changes. An explicit name suppresses it, as in Claude.
func (n *names) generate(title string) error {
	title = strings.TrimSpace(title)
	if n == nil || title == "" || n.explicit != "" {
		return nil
	}
	if err := appendJSON(n.path, titleRecord{Type: recordAITitle, SessionID: n.sessionID, AITitle: title}); err != nil {
		return err
	}
	n.title = title
	n.setPaneTitle()
	return nil
}

// setPaneTitle sets the terminal title the way Claude does: OSC 0 with the
// activity glyph in front.
func (n *names) setPaneTitle() {
	sayf(n.out, "\x1b]0;%s %s\x07", paneTitleGlyph, n.title)
}

// annotate adds what a real hook payload carries once the session has a name:
// transcript_path, and session_title only for an explicit name on the events
// that carry it.
func (n *names) annotate(event string, payload map[string]any) {
	if n == nil || n.title == "" {
		return
	}
	if _, ok := payload["transcript_path"]; !ok {
		payload["transcript_path"] = n.path
	}
	if event != "SessionStart" && event != "UserPromptSubmit" || n.explicit == "" {
		return
	}
	if _, ok := payload["session_title"]; !ok {
		payload["session_title"] = n.explicit
	}
}

// typedRename recognises the line deck types to push a name ("/rename <name>"
// after Ctrl-U, which the tty line discipline has already consumed).
func typedRename(line string) (string, bool) {
	rest, ok := strings.CutPrefix(strings.TrimSpace(line), renameLinePrefix)
	rest = strings.TrimSpace(rest)
	return rest, ok && rest != ""
}

// nameCommand runs the name-related pane commands: "rename" (a human /rename
// inside Claude) and "ai-title" (a generated title).
func (n *names) nameCommand(request fixtureCommand) (handled bool, err error) {
	switch request.Command {
	case "rename":
		return true, n.requireConversation(request, n.rename)
	case "ai-title":
		return true, n.requireConversation(request, n.generate)
	}
	return false, nil
}

func (n *names) requireConversation(request fixtureCommand, apply func(string) error) error {
	if n == nil {
		return fmt.Errorf("%q needs a conversation: launch with --session-id or --resume and a HOME", request.Command)
	}
	if strings.TrimSpace(request.Name) == "" {
		return errors.New(request.Command + " requires a non-blank \"name\"")
	}
	return apply(request.Name)
}
