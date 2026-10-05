package tui

import (
	"os"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/service"
	"github.com/n-orlov/deck/internal/store"
)

// hookBindingModelFor is hookBindingModel for a session of any agent kind.
func hookBindingModelFor(kind, running, bound string) Model {
	model := New(nil, config.Settings{}, "")
	model.deckExecutable = running
	model.sessions = []store.Session{{ID: "s1", Name: "bound-" + kind, Agent: kind, Status: "running", StatusReason: "tool running", HookExecutable: bound}}
	model.selected = rowCursor(0)
	return model
}

// R204c (#56) for the other harnesses: a Codex session launched by another
// deck binary, or by one that is gone, shows the hint in `i` and the row's
// reason exactly like Claude; the hint is advice, not an error; restart (R)
// and resume clear it. A Pi session records no hook executable (its launch
// builds no hook command), so it shows no hint even beside a deck that
// differs from whatever launched it.
func TestStaleHookBindingHintForCodexAndPi(t *testing.T) {
	running := existingBinary(t, "deck-new")
	older := existingBinary(t, "deck-old")

	t.Run("codex differs", func(t *testing.T) {
		model := hookBindingModelFor("codex", running, older)
		want := staleHint(older)
		model.detail = true
		if view := model.View(); !strings.Contains(flat(view), want) || strings.Contains(strings.ToLower(view), "error") {
			t.Fatalf("`i` detail lacks %q (or shows an error):\n%s", want, view)
		}
		if reason := model.selectedRowReason(); !strings.HasPrefix(reason, "running") || !strings.Contains(reason, want) || !strings.Contains(reason, "tool running") {
			t.Fatalf("row status reason = %q, want the stored reason and %q", reason, want)
		}
	})

	t.Run("codex missing", func(t *testing.T) {
		gone := existingBinary(t, "deck-gone")
		model := hookBindingModelFor("codex", gone, gone)
		if reason := model.selectedRowReason(); strings.Contains(reason, "hooks:") {
			t.Fatalf("existing, identical binary shows a hint: %q", reason)
		}
		if err := os.Remove(gone); err != nil {
			t.Fatal(err)
		}
		if reason := model.selectedRowReason(); !strings.Contains(reason, staleHint(gone)) {
			t.Fatalf("row status reason = %q, want %q for a missing file", reason, staleHint(gone))
		}
	})

	t.Run("pi never shows it", func(t *testing.T) {
		model := hookBindingModelFor("pi", running, "")
		model.detail = true
		if view := model.View(); strings.Contains(flat(view), "hooks: bound to") {
			t.Fatalf("a Pi session shows the hint:\n%s", view)
		}
		if reason := model.selectedRowReason(); strings.Contains(reason, "hooks:") {
			t.Fatalf("a Pi session shows the hint in its reason: %q", reason)
		}
	})

	for name, msg := range map[string]func(store.Session) tea.Msg{
		"restart": func(s store.Session) tea.Msg { return sessionRestarted{session: s, outcome: service.ResumeStarted} },
		"resume":  func(s store.Session) tea.Msg { return sessionResumed{session: s, outcome: service.ResumeStarted} },
	} {
		t.Run("codex clears after "+name, func(t *testing.T) {
			model := hookBindingModelFor("codex", running, older)
			if !strings.Contains(model.selectedRowReason(), "hooks: bound to") {
				t.Fatal("precondition: the stale binding must show the hint")
			}
			refreshed := model.sessions[0]
			refreshed.HookExecutable = running
			refreshed.Status = "starting"
			refreshed.StatusReason = ""
			updated, _ := model.Update(msg(refreshed))
			updated, _ = updated.Update(sessionsLoaded{sessions: []store.Session{refreshed}})
			after := updated.(Model)
			if reason := after.selectedRowReason(); strings.Contains(reason, "hooks:") {
				t.Fatalf("hint survives %s: %q", name, reason)
			}
			if session, _ := after.selectedSession(); session.Status == "error" {
				t.Fatalf("%s left the row in an error state", name)
			}
			after.detail = true
			if view := after.View(); strings.Contains(view, "hooks: bound to") || strings.Contains(strings.ToLower(view), "error") {
				t.Fatalf("detail after %s still hints or errors:\n%s", name, view)
			}
		})
	}
}
