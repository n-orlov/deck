//go:build deckinputcount

package main

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// countingStubModel is the smallest possible tea.Model: it records nothing
// itself (inputCountingModel is the thing under test), it only needs to
// exist so wrapForInputCounting has something real to wrap and delegate to,
// exactly as deck's own tui.Model does in production.
type countingStubModel struct{ updates int }

func (m countingStubModel) Init() tea.Cmd { return nil }
func (m countingStubModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	m.updates++
	return m, nil
}
func (m countingStubModel) View() string { return "" }

func readCount(t *testing.T, path string) int {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read count file: %v", err)
	}
	total, err := strconv.Atoi(string(data))
	if err != nil {
		t.Fatalf("count file %q is not a bare integer: %v", string(data), err)
	}
	return total
}

// TestInputCountingModelCountsKeysNotUpdateCalls is the "proves the counter
// is real" test: it drives the wrapper directly (no pty, no process --
// inputCountingModel.Update is a plain method call) and checks the file
// after every single message, so a regression that increments on the wrong
// message, double-counts, or forgets to persist is caught at the exact call
// that introduced it, not just at the end.
func TestInputCountingModelCountsKeysNotUpdateCalls(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "count")
	t.Setenv(inputCountFileEnvironment, path)

	wrapped := wrapForInputCounting(countingStubModel{})
	counting, ok := wrapped.(*inputCountingModel)
	if !ok {
		t.Fatalf("wrapForInputCounting did not return an *inputCountingModel with %s set", inputCountFileEnvironment)
	}

	// A single named key (no Runes, e.g. Up/Enter) weighs 1.
	counting.Update(tea.KeyMsg{Type: tea.KeyUp})
	if got := readCount(t, path); got != 1 {
		t.Fatalf("after one named key: count = %d, want 1", got)
	}

	// A single-rune key also weighs 1.
	counting.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	if got := readCount(t, path); got != 2 {
		t.Fatalf("after a second, single-rune key: count = %d, want 2", got)
	}

	// A message that is not a key at all (e.g. a resize) must NOT move the
	// counter -- if it did, this instrument would be counting "Update calls",
	// which task 003 explicitly does not want (a resize or a render tick is
	// not a key the program "received" from the user).
	counting.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	if got := readCount(t, path); got != 2 {
		t.Fatalf("after a non-key message: count = %d, want unchanged 2", got)
	}

	// A coalesced multi-rune KeyMsg -- exactly the shape bubbletea's own PTY
	// reader produces when two keystrokes land in the same read
	// (internal/tui/tui.go's own coalescing case) -- must weigh THREE, one
	// per keystroke it represents, not one per Update call. This is the
	// case that would go silently wrong (count under-reported by exactly the
	// coalescing amount) if the implementation counted messages instead of
	// keystrokes; withholding this case's own weighting (hard-coding weight
	// = 1 for every KeyMsg) is the mutation this test is designed to catch.
	counting.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a', 'b', 'c'}})
	if got := readCount(t, path); got != 5 {
		t.Fatalf("after a 3-rune coalesced key: count = %d, want 5 (2 + 3)", got)
	}

	// A bracketed paste is one input event regardless of its length, the
	// same exemption deck's own splitting gives it (msg.Paste) -- it must
	// weigh 1, not len(Runes).
	counting.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("pasted text"), Paste: true})
	if got := readCount(t, path); got != 6 {
		t.Fatalf("after an 11-rune paste: count = %d, want 6 (5 + 1)", got)
	}
}

// TestInputCountingModelGoesRedWhenAKeyIsWithheld is the mutation-style
// half of "proves the counter is real": it drives two keys through the
// wrapper but polls the file after only the first, and asserts the second
// key's weight has NOT yet landed -- i.e. the test would fail (go red) if a
// future change made writeInputCount count in advance, batch writes, or
// otherwise decouple "a key was delivered" from "the file reflects it". It
// is the same shape as task 114's byte-count poll, applied to input instead
// of output.
func TestInputCountingModelGoesRedWhenAKeyIsWithheld(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "count")
	t.Setenv(inputCountFileEnvironment, path)

	counting := wrapForInputCounting(countingStubModel{}).(*inputCountingModel)

	counting.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if got := readCount(t, path); got != 1 {
		t.Fatalf("after the first key: count = %d, want 1", got)
	}

	// The second key is deliberately never sent (withheld). If the counter
	// were not real -- if it were, say, a constant or derived from something
	// other than actual Update calls -- this assertion could not distinguish
	// "one key arrived" from "two keys arrived"; it must still read 1.
	if got := readCount(t, path); got != 1 {
		t.Fatalf("with a key withheld: count = %d, want still 1 (no phantom increment)", got)
	}
}

// TestWrapForInputCountingIsInertWithoutTheEnvironmentVariable proves the
// no-crash, no-surface guarantee documented on wrapForInputCounting: with
// DECK_INPUT_COUNT_FILE unset (as it is for every real user, even one who
// happens to run a -tags deckinputcount build by mistake), the returned
// model is the original, uninstrumented one -- no file, no counting.
func TestWrapForInputCountingIsInertWithoutTheEnvironmentVariable(t *testing.T) {
	t.Setenv(inputCountFileEnvironment, "")
	stub := countingStubModel{}
	got := wrapForInputCounting(stub)
	if _, ok := got.(*inputCountingModel); ok {
		t.Fatalf("wrapForInputCounting wrapped the model even with %s unset", inputCountFileEnvironment)
	}
	if got != tea.Model(stub) {
		t.Fatalf("wrapForInputCounting returned a different value than its input when inert")
	}
}
