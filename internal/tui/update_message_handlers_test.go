package tui

import (
	"context"
	"errors"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/n-orlov/deck/internal/config"
)

// Behaviour of the per-message handlers Update dispatches to, driven through
// Update itself (no tmux): each one is a message a background command can
// deliver late, so the interesting cases are the stale ones.

func TestEntryRefusalHolderRecheckClearsOnlyTheRefusalItWasIssuedFor(t *testing.T) {
	tests := []struct {
		name      string
		msg       func(m Model) entryRefusalHolderRecheckDone
		wantClear bool
	}{
		{"the same refusal whose holder left clears", func(m Model) entryRefusalHolderRecheckDone {
			return entryRefusalHolderRecheckDone{sessionID: "s1", kind: entryRefusalOther, reasonGone: true, generation: m.entryRefusal.generation}
		}, true},
		{"a holder still there keeps it", func(m Model) entryRefusalHolderRecheckDone {
			return entryRefusalHolderRecheckDone{sessionID: "s1", kind: entryRefusalOther, reasonGone: false, generation: m.entryRefusal.generation}
		}, false},
		{"a reply for an older generation of the same session and kind keeps it", func(m Model) entryRefusalHolderRecheckDone {
			return entryRefusalHolderRecheckDone{sessionID: "s1", kind: entryRefusalOther, reasonGone: true, generation: m.entryRefusal.generation - 1}
		}, false},
		{"a reply for another session keeps it", func(m Model) entryRefusalHolderRecheckDone {
			return entryRefusalHolderRecheckDone{sessionID: "s2", kind: entryRefusalOther, reasonGone: true, generation: m.entryRefusal.generation}
		}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := New(nil, config.Settings{}, "")
			m.setEntryRefusal("s1", entryRefusalOther, "boom")
			next, cmd := m.Update(tc.msg(m))
			if cmd != nil {
				t.Fatalf("the recheck reply returned a command, want none")
			}
			if got := next.(Model).entryRefusal.active; got == tc.wantClear {
				t.Fatalf("refusal active after the reply = %v, want %v", got, !tc.wantClear)
			}
		})
	}
}

func TestInteractiveDisplacementCheckLeavesInteractiveOnlyForItsOwnWindow(t *testing.T) {
	interactiveModel := func() Model {
		m := New(nil, config.Settings{}, "")
		m.interactive = true
		m.interactiveWindowTarget = "deck:1"
		return m
	}
	t.Run("a displaced owner of the polled window raises the lost-attach view", func(t *testing.T) {
		next, _ := interactiveModel().Update(interactiveDisplacementChecked{windowTarget: "deck:1", displaced: true, sessionName: "alpha"})
		got := next.(Model)
		if got.interactive || !got.lostAttach || got.lostAttachSession != "alpha" {
			t.Fatalf("interactive=%v lostAttach=%v session=%q, want interactive left and lost-attach raised for alpha", got.interactive, got.lostAttach, got.lostAttachSession)
		}
	})
	t.Run("a reply for a window since left or replaced is ignored", func(t *testing.T) {
		next, cmd := interactiveModel().Update(interactiveDisplacementChecked{windowTarget: "deck:2", displaced: true, sessionName: "alpha"})
		got := next.(Model)
		if cmd != nil || !got.interactive || got.lostAttach {
			t.Fatalf("interactive=%v lostAttach=%v cmd=%v, want interactive untouched", got.interactive, got.lostAttach, cmd != nil)
		}
	})
	t.Run("a reply after interactive mode ended is ignored", func(t *testing.T) {
		m := interactiveModel()
		m.interactive = false
		next, _ := m.Update(interactiveDisplacementChecked{windowTarget: "deck:1", displaced: true, sessionName: "alpha"})
		if next.(Model).lostAttach {
			t.Fatalf("a displacement reply raised lost-attach with interactive mode already off")
		}
	})
	t.Run("a poll that found nothing changes nothing", func(t *testing.T) {
		next, _ := interactiveModel().Update(interactiveDisplacementChecked{windowTarget: "deck:1"})
		if got := next.(Model); !got.interactive || got.lostAttach {
			t.Fatalf("interactive=%v lostAttach=%v, want interactive untouched", got.interactive, got.lostAttach)
		}
	})
}

func TestDeleteGraceExpiryReapsOnlyTheCurrentGeneration(t *testing.T) {
	var reaped []string
	reapErr := errors.New("reap failed")
	m := New(nil, config.Settings{}, "")
	m.deleteUndoGeneration = 3
	m.deleteUndoSessionID = "s1"
	m.deleteUndoSessionName = "alpha"
	m.reapSvc = func(_ context.Context, id string) error { reaped = append(reaped, id); return reapErr }

	if _, cmd := m.Update(deleteGraceExpired(2)); cmd != nil {
		t.Fatalf("a stale generation returned a command")
	}
	next, cmd := m.Update(deleteGraceExpired(3))
	if cmd == nil {
		t.Fatalf("the current generation returned no reap command")
	}
	if got := next.(Model); got.deleteUndoSessionID != "" || got.deleteUndoSessionName != "" {
		t.Fatalf("undo state after expiry = %q/%q, want cleared", got.deleteUndoSessionID, got.deleteUndoSessionName)
	}
	msg, ok := cmd().(sessionReaped)
	if !ok || !errors.Is(msg.err, reapErr) || len(reaped) != 1 || reaped[0] != "s1" {
		t.Fatalf("reap command result = %#v, reaped %v, want s1 reaped with the service's error", msg, reaped)
	}
}

func TestBatchDeleteGraceExpiryReapsEverySessionOfTheCurrentGeneration(t *testing.T) {
	var reaped []string
	m := New(nil, config.Settings{}, "")
	m.batchDeleteUndoGeneration = 5
	m.batchDeleteUndoSessionIDs = []string{"a", "b"}
	m.reapSvc = func(_ context.Context, id string) error { reaped = append(reaped, id); return nil }

	if _, cmd := m.Update(batchDeleteGraceExpired(4)); cmd != nil {
		t.Fatalf("a stale generation returned a command")
	}
	next, cmd := m.Update(batchDeleteGraceExpired(5))
	if cmd == nil {
		t.Fatalf("the current generation returned no reap command")
	}
	if got := next.(Model); got.batchDeleteUndoSessionIDs != nil {
		t.Fatalf("batch undo ids after expiry = %v, want cleared", got.batchDeleteUndoSessionIDs)
	}
	result, ok := cmd().(sessionsBulkReaped)
	if !ok || len(result.errs) != 2 || len(reaped) != 2 || reaped[0] != "a" || reaped[1] != "b" {
		t.Fatalf("reap command result = %#v, reaped %v, want a and b reaped", result, reaped)
	}
}

func TestBulkReapResultReportsTheFirstFailureAndLeavesSuccessSilent(t *testing.T) {
	m := New(nil, config.Settings{}, "")

	next, cmd := m.Update(sessionsBulkReaped{errs: []error{nil, errors.New("boom"), errors.New("later")}})
	if cmd != nil {
		t.Fatalf("a bulk reap result returned a command")
	}
	if got := next.(Model).attachError; got != "Cannot reap: boom" {
		t.Fatalf("attachError after a failed reap = %q, want the first failure", got)
	}

	m.attachError = "unrelated"
	next, _ = m.Update(sessionsBulkReaped{errs: []error{nil, nil}})
	if got := next.(Model).attachError; got != "unrelated" {
		t.Fatalf("attachError after a clean reap = %q, want it untouched", got)
	}
}

func TestMouseReportsAreIgnoredWhenTheMouseIsOff(t *testing.T) {
	m := New(nil, config.Settings{}, "")
	m.settings.Mouse = false
	m.width, m.height = 100, 30
	next, cmd := m.Update(tea.MouseMsg{X: 3, Y: 3, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	if cmd != nil || next.(Model).selected != m.selected {
		t.Fatalf("a mouse press with [ui] mouse off changed state or returned a command")
	}
}
