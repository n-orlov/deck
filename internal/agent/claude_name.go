package agent

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// claudeTailBytes bounds how much of a transcript's end ReadName reads. A
// transcript grows without bound; the title records are appended as the
// session goes, so the end of the file is where the current one lives.
const claudeTailBytes = 512 * 1024

// claudePlaceholderTitle is the pane title Claude shows before it has a real
// one.
const claudePlaceholderTitle = "Claude Code"

// claudeOpen opens a file for ReadName. It is a seam so a test can record
// every path ReadName touches.
var claudeOpen = func(path string) (io.ReadSeekCloser, int64, error) {
	f, err := os.Open(path) //nolint:gosec // G304: read-only; path is a .jsonl the hook named, opened as a regular file and never executed
	if err != nil {
		return nil, 0, err
	}
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		_ = f.Close()
		return nil, 0, os.ErrInvalid
	}
	return f, info.Size(), nil
}

// ReadName returns Claude's current name for the conversation: the last
// custom-title, agent-name or ai-title record in the tail of the transcript
// (the path the hook named, else the conversation's own transcript), else the
// pane title with Claude's glyph prefix stripped. custom-title and agent-name
// are names a human chose (explicit); an ai-title and a pane title are not.
// ~/.claude/sessions/<pid>.json is never opened.
func (Claude) ReadName(in NameProbeInput) (string, bool, bool) {
	if name, explicit, ok := claudeTranscriptName(claudeNamePath(in)); ok {
		return name, explicit, true
	}
	if in.PaneTitle == nil {
		return "", false, false
	}
	title := claudePaneName(in.PaneTitle())
	return title, false, title != ""
}

// claudeNamePath is the transcript file to read: the hook's path when it is
// usable, else the path Claude derives from the cwd and the conversation id.
func claudeNamePath(in NameProbeInput) string {
	if filepath.IsAbs(in.TranscriptPath) && filepath.Ext(in.TranscriptPath) == ".jsonl" {
		return filepath.Clean(in.TranscriptPath)
	}
	if !safeConversationID(in.ConversationID) || in.CWD == "" {
		return ""
	}
	root := in.Env[claudeConfigDirKey]
	if root == "" {
		if in.Home == "" {
			return ""
		}
		root = filepath.Join(in.Home, ".claude")
	}
	project := strings.ReplaceAll(filepath.Clean(in.CWD), string(filepath.Separator), "-")
	return filepath.Join(root, "projects", project, in.ConversationID+".jsonl")
}

// claudePaneName cleans a pane title. Claude always puts an activity glyph
// (✳, or a spinner frame while it works) and a space in front of its title; a
// title without one is not Claude's (tmux shows the host name until Claude sets
// its own) and is no name. The glyph goes, and the placeholder is no name.
func claudePaneName(title string) string {
	glyph, rest, found := strings.Cut(strings.TrimSpace(title), " ")
	if !found || !claudePaneGlyphs(glyph) {
		return ""
	}
	rest = strings.TrimSpace(rest)
	if rest == claudePlaceholderTitle {
		return ""
	}
	return rest
}

// claudePaneGlyphs reports whether glyph is one rune of Claude's activity
// glyphs: the ✳ family or a braille spinner frame.
func claudePaneGlyphs(glyph string) bool {
	runes := []rune(glyph)
	if len(runes) != 1 {
		return false
	}
	r := runes[0]
	return strings.ContainsRune("✳✶✻✽✢·", r) || (r >= 0x2800 && r <= 0x28FF)
}

// claudeTranscriptName reads the title records at the end of path.
func claudeTranscriptName(path string) (name string, explicit, ok bool) {
	if path == "" {
		return "", false, false
	}
	tail, ok := claudeReadTail(path)
	if !ok {
		return "", false, false
	}
	return claudeLastTitle(tail)
}

// claudeReadTail returns up to claudeTailBytes from the end of path. When the
// read starts mid-file, the first (cut) line is dropped.
func claudeReadTail(path string) ([]byte, bool) {
	f, size, err := claudeOpen(path)
	if err != nil {
		return nil, false
	}
	defer func() { _ = f.Close() }()
	offset := int64(0)
	if size > claudeTailBytes {
		offset = size - claudeTailBytes
	}
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return nil, false
	}
	data, err := io.ReadAll(io.LimitReader(f, claudeTailBytes))
	if err != nil {
		return nil, false
	}
	if offset > 0 {
		cut := bytes.IndexByte(data, '\n')
		if cut < 0 {
			return nil, false
		}
		data = data[cut+1:]
	}
	return data, true
}

// claudeTitleRecord is the part of a transcript line ReadName understands.
type claudeTitleRecord struct {
	Type        string `json:"type"`
	CustomTitle string `json:"customTitle"`
	AgentName   string `json:"agentName"`
	AITitle     string `json:"aiTitle"`
}

// title is the record's title, and whether a human chose it.
func (r claudeTitleRecord) title() (string, bool, bool) {
	switch r.Type {
	case "custom-title":
		return r.CustomTitle, true, true
	case "agent-name":
		return r.AgentName, true, true
	case "ai-title":
		return r.AITitle, false, true
	}
	return "", false, false
}

// claudeLastTitle scans lines from the end and returns the last non-blank
// title record. A line that is not a complete JSON object (a partial last
// line) is skipped.
func claudeLastTitle(data []byte) (string, bool, bool) {
	lines := bytes.Split(data, []byte{'\n'})
	for i := len(lines) - 1; i >= 0; i-- {
		var rec claudeTitleRecord
		if json.Unmarshal(lines[i], &rec) != nil {
			continue
		}
		if title, explicit, ok := rec.title(); ok && strings.TrimSpace(title) != "" {
			return title, explicit, true
		}
	}
	return "", false, false
}
