//go:build decktestpanic

package main

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

type panicStubModel struct{ updates int }

func (m panicStubModel) Init() tea.Cmd { return nil }
func (m panicStubModel) Update(tea.Msg) (tea.Model, tea.Cmd) {
	m.updates++
	return m, nil
}
func (m panicStubModel) View() string { return "" }

// TestPanicOnKeyModelPanicsOnlyOnTheConfiguredKey proves the deliberate
// panic fires for the configured key before the wrapped model sees it, and
// that every other key and every non-key message passes through to the
// wrapped model unharmed.
func TestPanicOnKeyModelPanicsOnlyOnTheConfiguredKey(t *testing.T) {
	model := panicOnKeyModel{Model: panicStubModel{}, key: "x"}

	for _, msg := range []tea.Msg{
		tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")},
		tea.WindowSizeMsg{Width: 80, Height: 24},
	} {
		next, _ := model.Update(msg)
		wrapped, ok := next.(panicOnKeyModel)
		if !ok {
			t.Fatalf("Update(%T) returned %T, want panicOnKeyModel", msg, next)
		}
		if got := wrapped.Model.(panicStubModel).updates; got != 1 {
			t.Fatalf("Update(%T) delegated %d times, want 1", msg, got)
		}
	}

	defer func() {
		recovered := recover()
		text, _ := recovered.(string)
		if !strings.Contains(text, "DECK_TEST_PANIC_KEY=x") {
			t.Fatalf("recovered %v, want the deliberate test panic naming the key", recovered)
		}
	}()
	model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	t.Fatal("the configured key did not panic")
}
