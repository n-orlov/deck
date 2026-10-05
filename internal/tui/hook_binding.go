package tui

import (
	"os"
	"path/filepath"

	"github.com/n-orlov/deck/internal/store"
)

// runningExecutable is the absolute path of the deck binary this TUI is, in
// the same form the launcher records for a session's hook command
// (filepath.Abs of os.Executable). Empty when it cannot be resolved.
func runningExecutable() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	abs, err := filepath.Abs(exe)
	if err != nil {
		return ""
	}
	return abs
}

// staleHookBinding is R204c's stale-binding hint, or "" when there is none.
// A running agent keeps the hook command it was launched with, so a session
// launched by a deck binary other than this one (or whose binary has since
// been removed) still calls that old path for every hook. The hint says so
// and names the way out; it is advice, never a status: nothing here changes
// the row's status, and a restart or resume (which records this binary as
// the session's hook executable) clears it. A row that never recorded
// a path (created before schema 9, or an adapter without hooks) has nothing
// to compare and shows no hint; a stopped row keeps its hint until resume
// relaunches it, because the recorded path is the one a resume replaces.
func (m Model) staleHookBinding(session store.Session) string {
	path := session.HookExecutable
	if path == "" {
		return ""
	}
	differs := m.deckExecutable != "" && path != m.deckExecutable
	if !differs && fileExists(path) {
		return ""
	}
	return "hooks: bound to " + path + m.glyph(" — ", " - ") + "restart (R) to refresh"
}

// fileExists reports whether path names an existing file (symlinks followed).
func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// withStaleHookBinding appends the stale-binding hint to a row's reason, the
// footer's status-reason slot, as a further " · " clause. A row with no other
// reason shows `<status> · <hint>`.
func (m Model) withStaleHookBinding(session store.Session, reason string) string {
	hint := m.staleHookBinding(session)
	switch {
	case hint == "":
		return reason
	case reason == "":
		return session.Status + m.glyph(" · ", " - ") + hint
	default:
		return reason + m.glyph(" · ", " - ") + hint
	}
}
