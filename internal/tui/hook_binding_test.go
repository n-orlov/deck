package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/service"
	"github.com/n-orlov/deck/internal/store"
)

// hookBindingModel is a list model running as deck binary `running`, with one
// selected running session whose agent was launched with `bound`.
func hookBindingModel(t *testing.T, running, bound string) Model {
	t.Helper()
	model := New(nil, config.Settings{}, "")
	model.deckExecutable = running
	model.sessions = []store.Session{{ID: "s1", Name: "bound-claude", Agent: "claude", Status: "running", StatusReason: "tool running", HookExecutable: bound}}
	model.selected = rowCursor(0)
	return model
}

// existingBinary writes an executable file and returns its path.
func existingBinary(t *testing.T, name string) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "d") // short, so the hint is not ellipsised in the dialog
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

// flat collapses a rendered dialog's frame and line wrapping to single spaces.
func flat(view string) string {
	return strings.Join(strings.Fields(strings.NewReplacer("\u2502", " ", "\u256d", " ", "\u256e", " ", "\u2570", " ", "\u256f", " ", "\u2500", " ").Replace(view)), " ")
}

func staleHint(path string) string {
	return "hooks: bound to " + path + " \u2014 restart (R) to refresh"
}

// TestStaleHookBindingHintWhenTheLaunchExecutableDiffers: a session launched by
// another deck binary shows the hint in `i` and in the row's status reason.
func TestStaleHookBindingHintWhenTheLaunchExecutableDiffers(t *testing.T) {
	older := existingBinary(t, "deck-old")
	model := hookBindingModel(t, existingBinary(t, "deck-new"), older)
	want := staleHint(older)

	model.detail = true
	if view := model.View(); !strings.Contains(flat(view), want) {
		t.Fatalf("`i` detail lacks %q:\n%s", want, view)
	}
	if reason := model.selectedRowReason(); !strings.Contains(reason, want) || !strings.Contains(reason, "tool running") {
		t.Fatalf("row status reason = %q, want the stored reason and %q", reason, want)
	}
}

// TestStaleHookBindingHintWhenTheLaunchExecutableIsMissing: the same binary as
// the running deck is not stale while its file exists, and is once it is gone.
func TestStaleHookBindingHintWhenTheLaunchExecutableIsMissing(t *testing.T) {
	binary := existingBinary(t, "deck")
	model := hookBindingModel(t, binary, binary)
	if reason := model.selectedRowReason(); strings.Contains(reason, "hooks:") {
		t.Fatalf("existing, identical binary shows a hint: %q", reason)
	}

	if err := os.Remove(binary); err != nil {
		t.Fatal(err)
	}
	want := staleHint(binary)
	if reason := model.selectedRowReason(); !strings.Contains(reason, want) {
		t.Fatalf("row status reason = %q, want %q for a missing file", reason, want)
	}
	model.detail = true
	if view := model.View(); !strings.Contains(flat(view), want) {
		t.Fatalf("`i` detail lacks %q for a missing file:\n%s", want, view)
	}
}

// TestNoStaleHookBindingHintWithoutARecordedExecutable: a row that never
// recorded a hook executable (created before schema 9, or a shell) has
// nothing to compare, so it shows no hint.
func TestNoStaleHookBindingHintWithoutARecordedExecutable(t *testing.T) {
	model := hookBindingModel(t, existingBinary(t, "deck"), "")
	if reason := model.selectedRowReason(); strings.Contains(reason, "hooks:") {
		t.Fatalf("unrecorded binding shows a hint: %q", reason)
	}
}

// TestStaleHookBindingHintIsNotAnErrorState: the hint is advice on a healthy
// row: the status stays what the hooks said, and the row is still `running`.
func TestStaleHookBindingHintIsNotAnErrorState(t *testing.T) {
	model := hookBindingModel(t, existingBinary(t, "deck-new"), existingBinary(t, "deck-old"))
	session, _ := model.selectedSession()
	if session.Status != "running" {
		t.Fatalf("status = %q, want running", session.Status)
	}
	if reason := model.selectedRowReason(); !strings.HasPrefix(reason, "running") {
		t.Fatalf("reason = %q, want the row's own status first", reason)
	}
	if view := model.View(); strings.Contains(strings.ToLower(view), "error") {
		t.Fatalf("a stale binding put an error on screen:\n%s", view)
	}
}

// TestStaleHookBindingHintClearsAfterRestartAndResume: restart (R) and resume
// both relaunch under the running deck, which records it as the session's hook
// executable; the refreshed row carries no hint and is not in an error state.
func TestStaleHookBindingHintClearsAfterRestartAndResume(t *testing.T) {
	running := existingBinary(t, "deck-new")
	for name, msg := range map[string]func(store.Session) tea.Msg{
		"restart": func(s store.Session) tea.Msg { return sessionRestarted{session: s, outcome: service.ResumeStarted} },
		"resume":  func(s store.Session) tea.Msg { return sessionResumed{session: s, outcome: service.ResumeStarted} },
	} {
		t.Run(name, func(t *testing.T) {
			model := hookBindingModel(t, running, existingBinary(t, "deck-old"))
			if !strings.Contains(model.selectedRowReason(), "hooks: bound to") {
				t.Fatal("precondition: the stale binding must show the hint")
			}
			refreshed := model.sessions[0]
			refreshed.HookExecutable = running
			refreshed.Status = "starting"
			refreshed.StatusReason = ""
			updated, _ := model.Update(msg(refreshed))
			// resume relaunches through the store, and the TUI picks the new
			// row up with its next reload.
			updated, _ = updated.Update(sessionsLoaded{sessions: []store.Session{refreshed}})
			after := updated.(Model)
			if reason := after.selectedRowReason(); strings.Contains(reason, "hooks:") {
				t.Fatalf("hint survives %s: %q", name, reason)
			}
			after.detail = true
			if view := after.View(); strings.Contains(view, "hooks: bound to") || strings.Contains(strings.ToLower(view), "error") {
				t.Fatalf("detail after %s still hints or errors:\n%s", name, view)
			}
		})
	}
}
