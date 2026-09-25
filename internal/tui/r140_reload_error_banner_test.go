package tui

import (
	"errors"
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
)

// TestSessionsReloadErrorRendersWithoutTmuxUnavailablePrefixOrInstallLine
// proves R140/GH #36's transient-error half: a sessionsLoaded carrying a
// non-nil err is a RUNTIME read failure, not the start-up
// missing/too-old-tmux condition, so it must render its own short
// "Cannot read sessions: <err>" line -- never borrowing startupBanner's
// "tmux unavailable:" prefix or its "Install tmux 3.2 or newer" advice
// line, both of which are reserved for the genuine start-up note built
// from tmuxNote (New's third argument). Proven red on the unfixed tree
// (26cdbfd), where every reload error -- regardless of tmuxNote -- lands
// in the single shared m.startupNote field and startupBanner
// unconditionally prefixes/suffixes it.
func TestSessionsReloadErrorRendersWithoutTmuxUnavailablePrefixOrInstallLine(t *testing.T) {
	model := New(nil, config.Settings{}, "")
	model.sessions = []store.Session{{Name: "only-session", Agent: "shell", Status: "running"}}

	updated, _ := model.Update(sessionsLoaded{err: errors.New("exit status 1")})
	m := updated.(Model)

	view := m.View()
	if !strings.Contains(view, "Cannot read sessions: exit status 1") {
		t.Fatalf("view missing the runtime reload error message:\n%s", view)
	}
	if strings.Contains(view, "tmux unavailable:") {
		t.Fatalf("runtime reload error wrongly carries the start-up \"tmux unavailable:\" prefix:\n%s", view)
	}
	if strings.Contains(view, "Install tmux") {
		t.Fatalf("runtime reload error wrongly carries the start-up install-tmux line:\n%s", view)
	}
}

// TestSessionsReloadErrorClearsOnNextSuccessfulReload proves the second
// half of R140's transient-error fix: unlike the start-up note (which
// nothing ever clears -- see
// TestStartupTmuxNoteRendersAndPersistsAcrossReloads below), a runtime
// reload error is scoped to the single sessionsLoaded that reported it. A
// following sessionsLoaded with a nil err must leave no trace of the
// earlier failure text anywhere in the rendered frame. Proven red on the
// unfixed tree (26cdbfd), where m.startupNote is written by the failing
// reload and nothing in the success branch ever resets it.
func TestSessionsReloadErrorClearsOnNextSuccessfulReload(t *testing.T) {
	model := New(nil, config.Settings{}, "")
	model.sessions = []store.Session{{Name: "only-session", Agent: "shell", Status: "running"}}

	failed, _ := model.Update(sessionsLoaded{err: errors.New("exit status 1")})
	m := failed.(Model)
	if !strings.Contains(m.View(), "Cannot read sessions: exit status 1") {
		t.Fatalf("setup: reload error did not render before the successful reload")
	}

	succeeded, _ := m.Update(sessionsLoaded{sessions: m.sessions})
	m = succeeded.(Model)

	view := m.View()
	if strings.Contains(view, "Cannot read sessions") {
		t.Fatalf("reload error note survived a subsequent successful reload:\n%s", view)
	}
	if strings.Contains(view, "tmux unavailable") {
		t.Fatalf("a successful reload wrongly introduced a tmux-unavailable note:\n%s", view)
	}
	if strings.Contains(view, "Install tmux") {
		t.Fatalf("a successful reload wrongly introduced an install-tmux line:\n%s", view)
	}
}

// TestStartupTmuxNoteRendersAndPersistsAcrossReloads proves R140's fix
// leaves the genuine start-up note untouched: the note built from
// tmuxNote at New/construction time (the missing/too-old-tmux condition
// features/tmux_contract.feature exercises end-to-end) still renders
// "tmux unavailable:" plus the install-tmux advice line, and -- unlike the
// runtime reload-error note -- survives across reloads, successful or
// not, since it reports a condition detected once at start-up rather than
// per-reload.
func TestStartupTmuxNoteRendersAndPersistsAcrossReloads(t *testing.T) {
	model := New(nil, config.Settings{}, `tmux 3.1c is too old`)
	model.sessions = []store.Session{{Name: "only-session", Agent: "shell", Status: "running"}}

	assertStartupNote := func(t *testing.T, view string) {
		t.Helper()
		if !strings.Contains(view, "tmux unavailable: tmux 3.1c is too old") {
			t.Fatalf("view missing the start-up tmux note:\n%s", view)
		}
		if !strings.Contains(view, "Install tmux 3.2 or newer") {
			t.Fatalf("view missing the start-up install-tmux advice line:\n%s", view)
		}
	}
	assertStartupNote(t, model.View())

	// A successful reload must not clear the start-up note.
	succeeded, _ := model.Update(sessionsLoaded{sessions: model.sessions})
	m := succeeded.(Model)
	assertStartupNote(t, m.View())

	// Nor must a subsequent runtime reload error, or that error's own
	// later clearing, ever touch it.
	failed, _ := m.Update(sessionsLoaded{err: errors.New("exit status 1")})
	m = failed.(Model)
	assertStartupNote(t, m.View())

	recovered, _ := m.Update(sessionsLoaded{sessions: m.sessions})
	m = recovered.(Model)
	assertStartupNote(t, m.View())
}
