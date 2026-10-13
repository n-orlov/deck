package agent

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// claudeTailBytes bounds how much of a transcript's end ReadName reads. A
// transcript grows without bound; the title records are appended as the
// session goes, so the end of the file is where the current one lives.
const claudeTailBytes = 512 * 1024

// claudePlaceholderTitle is the pane title Claude shows before it has a real
// one.
const claudePlaceholderTitle = "Claude Code"

// claudeOpen opens a file for ReadName. It is a seam so a test can record
// every path ReadName touches. It refuses anything that is not a regular
// file (a FIFO, a socket, a device, a symlink) before reading: it Lstats the
// path, then opens it O_NONBLOCK|O_NOFOLLOW so a file swapped in after the
// Lstat cannot block the open or redirect it, and checks the opened file again.
var claudeOpen = func(path string) (io.ReadSeekCloser, int64, error) {
	if info, err := os.Lstat(path); err != nil || !info.Mode().IsRegular() {
		if err == nil {
			err = os.ErrInvalid
		}
		return nil, 0, err
	}
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK|syscall.O_NOFOLLOW, 0) //nolint:gosec // G304: read-only; path is a regular .jsonl, opened without following a link or blocking, and never executed
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
// The hook's path is usable only when it is an absolute, existing, regular
// .jsonl file whose symlink-resolved path lies inside the Claude projects root
// (<claude config dir>/projects); a missing file, a link and a non-regular file
// fall back to the derived path (R271, R279).
func claudeNamePath(in NameProbeInput) string {
	root := claudeConfigRoot(in)
	if root != "" && filepath.IsAbs(in.TranscriptPath) && filepath.Ext(in.TranscriptPath) == ".jsonl" {
		if path := filepath.Clean(in.TranscriptPath); claudeTranscriptConfined(path, filepath.Join(root, "projects")) {
			return path
		}
	}
	if !safeConversationID(in.ConversationID) || in.CWD == "" || root == "" {
		return ""
	}
	project := strings.ReplaceAll(filepath.Clean(in.CWD), string(filepath.Separator), "-")
	return filepath.Join(root, "projects", project, in.ConversationID+".jsonl")
}

// claudeConfigRoot is Claude's config directory: the session's
// CLAUDE_CONFIG_DIR, else <Home>/.claude; empty when neither is known.
func claudeConfigRoot(in NameProbeInput) string {
	if root := in.Env[claudeConfigDirKey]; root != "" {
		return root
	}
	if in.Home == "" {
		return ""
	}
	return filepath.Join(in.Home, ".claude")
}

// claudeTranscriptConfined reports whether path (already cleaned) may be read
// as the hook's transcript: it lies lexically inside projects, it exists, its
// symlink-resolved form lies inside the resolved projects root and is a .jsonl
// file, and the path itself is a regular file. A missing file, a link, a FIFO,
// a socket or a device is not eligible, so the caller falls back to the
// derived transcript path.
func claudeTranscriptConfined(path, projects string) bool {
	projects = filepath.Clean(projects)
	if !filepath.IsAbs(projects) || !pathWithin(projects, path) {
		return false
	}
	realRoot, err := filepath.EvalSymlinks(projects)
	if err != nil {
		return false
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil || filepath.Ext(resolved) != ".jsonl" || !pathWithin(realRoot, resolved) {
		return false
	}
	info, err := os.Lstat(path)
	return err == nil && info.Mode().IsRegular()
}

// pathWithin reports whether path is root or lies below it, comparing cleaned
// paths lexically.
func pathWithin(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
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
