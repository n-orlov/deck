package tui

import (
	"context"
	"errors"
	"reflect"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/service"
	"github.com/n-orlov/deck/internal/store"
)

// These tests call updateRestartChoice directly, so every one of its
// closures (Cycle, Cancel, both Submit branches and their nil-service
// notes) is covered by this file alone, not by whichever dialog features
// happen to run alongside it.

func restartChoiceTestModel() Model {
	m := New(nil, config.Settings{}, "")
	m.width, m.height = 80, 24
	m.sessions = []store.Session{{ID: "s1", Name: "alpha", Agent: "shell", Status: "running"}}
	m.selected = rowCursor(0)
	m.restartChoosing = true
	m.restartChoiceValue = "restart"
	return m
}

func pressRestartChoice(t *testing.T, m Model, key tea.KeyMsg) (Model, tea.Cmd) {
	t.Helper()
	updated, cmd := m.updateRestartChoice(key)
	return updated.(Model), cmd
}

func TestUpdateRestartChoiceCyclesTheCandidate(t *testing.T) {
	m := restartChoiceTestModel()
	for _, step := range []struct {
		key  tea.KeyType
		want string
	}{
		{tea.KeyRight, "inject"},
		{tea.KeyRight, "restart"},
		{tea.KeyLeft, "inject"},
		{tea.KeyLeft, "restart"},
	} {
		var cmd tea.Cmd
		m, cmd = pressRestartChoice(t, m, tea.KeyMsg{Type: step.key})
		if cmd != nil {
			t.Fatalf("cycling returned a command; want none")
		}
		if m.restartChoiceValue != step.want {
			t.Fatalf("after %v candidate = %q, want %q", step.key, m.restartChoiceValue, step.want)
		}
	}
	if !m.restartChoosing {
		t.Fatalf("cycling closed the choice dialog")
	}
}

func TestUpdateRestartChoiceEscCancels(t *testing.T) {
	m := restartChoiceTestModel()
	m.restartChoiceNote = "restarting is unavailable"
	m, cmd := pressRestartChoice(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if cmd != nil {
		t.Fatalf("esc returned a command; want none")
	}
	if m.restartChoosing || m.restartChoiceNote != "" {
		t.Fatalf("after esc choosing=%v note=%q, want closed with no note", m.restartChoosing, m.restartChoiceNote)
	}
}

func TestUpdateRestartChoiceIgnoresKeysOutsideTheContract(t *testing.T) {
	m := restartChoiceTestModel()
	m, cmd := pressRestartChoice(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	if cmd != nil || !m.restartChoosing || m.restartChoiceValue != "restart" {
		t.Fatalf("unbound key changed state: cmd=%v choosing=%v value=%q", cmd != nil, m.restartChoosing, m.restartChoiceValue)
	}
}

func TestUpdateRestartChoiceSubmitRestart(t *testing.T) {
	m := restartChoiceTestModel()
	want := store.Session{ID: "s1", Status: "starting"}
	wantErr := errors.New("boom")
	var gotID string
	m.restart = func(_ context.Context, id string) (store.Session, service.ResumeOutcome, error) {
		gotID = id
		return want, service.ResumeStarted, wantErr
	}
	m.inject = func(context.Context, string) (store.Session, []string, error) {
		t.Fatalf("restart candidate dispatched to inject")
		return store.Session{}, nil, nil
	}
	m, cmd := pressRestartChoice(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatalf("enter on restart returned no command")
	}
	msg, ok := cmd().(sessionRestarted)
	if !ok {
		t.Fatalf("restart command produced %T, want sessionRestarted", msg)
	}
	if gotID != "s1" || !reflect.DeepEqual(msg.session, want) || msg.outcome != service.ResumeStarted || !errors.Is(msg.err, wantErr) {
		t.Fatalf("restart dispatched id=%q msg=%+v", gotID, msg)
	}
	if m.restartChoiceNote != "" {
		t.Fatalf("successful dispatch set note %q", m.restartChoiceNote)
	}
}

func TestUpdateRestartChoiceSubmitInject(t *testing.T) {
	m := restartChoiceTestModel()
	m.restartChoiceValue = "inject"
	want := store.Session{ID: "s1"}
	var gotID string
	m.inject = func(_ context.Context, id string) (store.Session, []string, error) {
		gotID = id
		return want, []string{"PATH"}, nil
	}
	m.restart = func(context.Context, string) (store.Session, service.ResumeOutcome, error) {
		t.Fatalf("inject candidate dispatched to restart")
		return store.Session{}, 0, nil
	}
	m, cmd := pressRestartChoice(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatalf("enter on inject returned no command")
	}
	msg, ok := cmd().(envInjected)
	if !ok {
		t.Fatalf("inject command produced %T, want envInjected", msg)
	}
	if gotID != "s1" || !reflect.DeepEqual(msg.session, want) || !reflect.DeepEqual(msg.keys, []string{"PATH"}) || msg.err != nil {
		t.Fatalf("inject dispatched id=%q msg=%+v", gotID, msg)
	}
	if m.restartChoiceNote != "" {
		t.Fatalf("successful dispatch set note %q", m.restartChoiceNote)
	}
}

// TestUpdateRestartChoiceSubmitWithoutService covers both nil-service
// branches: the dialog stays open with a note and dispatches nothing.
func TestUpdateRestartChoiceSubmitWithoutService(t *testing.T) {
	for _, tc := range []struct {
		value, note string
	}{
		{"restart", "restarting is unavailable"},
		{"inject", "injecting the environment is unavailable"},
	} {
		t.Run(tc.value, func(t *testing.T) {
			m := restartChoiceTestModel()
			m.restart, m.inject = nil, nil
			m.restartChoiceValue = tc.value
			m, cmd := pressRestartChoice(t, m, tea.KeyMsg{Type: tea.KeyEnter})
			if cmd != nil {
				t.Fatalf("enter with no %s service returned a command", tc.value)
			}
			if !m.restartChoosing || m.restartChoiceNote != tc.note {
				t.Fatalf("choosing=%v note=%q, want open with %q", m.restartChoosing, m.restartChoiceNote, tc.note)
			}
		})
	}
}
