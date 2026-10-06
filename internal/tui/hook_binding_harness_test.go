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

// hookBindingHarnesses are the three agent harnesses R204 (#56) covers.
var hookBindingHarnesses = []string{"claude", "codex", "pi"}

// hookBindingModelFor is hookBindingModel for a session of any agent kind.
func hookBindingModelFor(kind, running, bound string) Model {
	model := New(nil, config.Settings{}, "")
	model.deckExecutable = running
	model.sessions = []store.Session{{ID: "s1", Name: "bound-" + kind, Agent: kind, Status: "running", StatusReason: "tool running", HookExecutable: bound}}
	model.selected = rowCursor(0)
	return model
}

// relaunchMsgs are the two relaunches (restart R, resume) that record the
// running deck as the session's hook executable.
var relaunchMsgs = map[string]func(store.Session) tea.Msg{
	"restart": func(s store.Session) tea.Msg { return sessionRestarted{session: s, outcome: service.ResumeStarted} },
	"resume":  func(s store.Session) tea.Msg { return sessionResumed{session: s, outcome: service.ResumeStarted} },
}

// relaunched applies a relaunch message carrying `refreshed`, then the reload
// that picks the relaunched row up, and returns the resulting model.
func relaunched(model Model, msg tea.Msg, refreshed store.Session) Model {
	updated, _ := model.Update(msg)
	updated, _ = updated.Update(sessionsLoaded{sessions: []store.Session{refreshed}})
	return updated.(Model)
}

// TestStaleHookBindingHintPerHarnessWhenTheLaunchExecutableDiffers mirrors
// TestStaleHookBindingHintWhenTheLaunchExecutableDiffers for every harness:
// a Claude, Codex or Pi row bound to another deck binary shows the hint in
// `i` and in the row's status reason. The hint reads only the row's recorded
// binding, never its agent kind.
func TestStaleHookBindingHintPerHarnessWhenTheLaunchExecutableDiffers(t *testing.T) {
	for _, kind := range hookBindingHarnesses {
		t.Run(kind, func(t *testing.T) {
			older := existingBinary(t, "deck-old")
			model := hookBindingModelFor(kind, existingBinary(t, "deck-new"), older)
			want := staleHint(older)
			model.detail = true
			if view := model.View(); !strings.Contains(flat(view), want) {
				t.Fatalf("%s: `i` detail lacks %q:\n%s", kind, want, view)
			}
			if reason := model.selectedRowReason(); !strings.Contains(reason, want) || !strings.Contains(reason, "tool running") {
				t.Fatalf("%s: row status reason = %q, want the stored reason and %q", kind, reason, want)
			}
		})
	}
}

// TestStaleHookBindingHintPerHarnessWhenTheLaunchExecutableIsMissing mirrors
// TestStaleHookBindingHintWhenTheLaunchExecutableIsMissing for every harness.
func TestStaleHookBindingHintPerHarnessWhenTheLaunchExecutableIsMissing(t *testing.T) {
	for _, kind := range hookBindingHarnesses {
		t.Run(kind, func(t *testing.T) {
			binary := existingBinary(t, "deck")
			model := hookBindingModelFor(kind, binary, binary)
			if reason := model.selectedRowReason(); strings.Contains(reason, "hooks:") {
				t.Fatalf("%s: existing, identical binary shows a hint: %q", kind, reason)
			}
			if err := os.Remove(binary); err != nil {
				t.Fatal(err)
			}
			want := staleHint(binary)
			if reason := model.selectedRowReason(); !strings.Contains(reason, want) {
				t.Fatalf("%s: row status reason = %q, want %q for a missing file", kind, reason, want)
			}
			model.detail = true
			if view := model.View(); !strings.Contains(flat(view), want) {
				t.Fatalf("%s: `i` detail lacks %q for a missing file:\n%s", kind, want, view)
			}
		})
	}
}

// TestNoStaleHookBindingHintPerHarnessWithoutARecordedExecutable mirrors
// TestNoStaleHookBindingHintWithoutARecordedExecutable for every harness: a
// row that never recorded a binding (one created before the binding existed;
// every launch of Claude, Codex and Pi records one, see
// TestCodexAndPiRecordAHookExecutableAndShellDoesNot in internal/service)
// has nothing to compare and shows no hint.
func TestNoStaleHookBindingHintPerHarnessWithoutARecordedExecutable(t *testing.T) {
	for _, kind := range hookBindingHarnesses {
		t.Run(kind, func(t *testing.T) {
			model := hookBindingModelFor(kind, existingBinary(t, "deck"), "")
			if reason := model.selectedRowReason(); strings.Contains(reason, "hooks:") {
				t.Fatalf("%s: unrecorded binding shows a hint: %q", kind, reason)
			}
			model.detail = true
			if view := model.View(); strings.Contains(flat(view), "hooks: bound to") {
				t.Fatalf("%s: unrecorded binding shows a hint in `i`:\n%s", kind, view)
			}
		})
	}
}

// TestStaleHookBindingHintPerHarnessIsNotAnErrorState mirrors
// TestStaleHookBindingHintIsNotAnErrorState for every harness.
func TestStaleHookBindingHintPerHarnessIsNotAnErrorState(t *testing.T) {
	for _, kind := range hookBindingHarnesses {
		t.Run(kind, func(t *testing.T) {
			model := hookBindingModelFor(kind, existingBinary(t, "deck-new"), existingBinary(t, "deck-old"))
			session, _ := model.selectedSession()
			if session.Status != "running" {
				t.Fatalf("%s: status = %q, want running", kind, session.Status)
			}
			if reason := model.selectedRowReason(); !strings.HasPrefix(reason, "running") {
				t.Fatalf("%s: reason = %q, want the row's own status first", kind, reason)
			}
			if view := model.View(); strings.Contains(strings.ToLower(view), "error") {
				t.Fatalf("%s: a stale binding put an error on screen:\n%s", kind, view)
			}
		})
	}
}

// TestStaleHookBindingHintPerHarnessClearsAfterRestartAndResume mirrors
// TestStaleHookBindingHintClearsAfterRestartAndResume for every harness: the
// relaunched row is bound to the running deck (every harness records it, see
// TestRestartRebindsCodexAndPi in internal/service), and so carries no hint and
// is not in an error state.
func TestStaleHookBindingHintPerHarnessClearsAfterRestartAndResume(t *testing.T) {
	running := existingBinary(t, "deck-new")
	for _, kind := range hookBindingHarnesses {
		rebound := running
		for name, msg := range relaunchMsgs {
			t.Run(kind+" "+name, func(t *testing.T) {
				model := hookBindingModelFor(kind, running, existingBinary(t, "deck-old"))
				if !strings.Contains(model.selectedRowReason(), "hooks: bound to") {
					t.Fatal("precondition: the stale binding must show the hint")
				}
				refreshed := model.sessions[0]
				refreshed.HookExecutable = rebound
				refreshed.Status = "starting"
				refreshed.StatusReason = ""
				after := relaunched(model, msg(refreshed), refreshed)
				if reason := after.selectedRowReason(); strings.Contains(reason, "hooks:") {
					t.Fatalf("%s: hint survives %s: %q", kind, name, reason)
				}
				if session, _ := after.selectedSession(); session.Status == "error" {
					t.Fatalf("%s: %s left the row in an error state", kind, name)
				}
				after.detail = true
				if view := after.View(); strings.Contains(view, "hooks: bound to") || strings.Contains(strings.ToLower(view), "error") {
					t.Fatalf("%s: detail after %s still hints or errors:\n%s", kind, name, view)
				}
			})
		}
	}
}
