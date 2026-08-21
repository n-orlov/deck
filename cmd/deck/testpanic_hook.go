//go:build decktestpanic

package main

import (
	"os"

	tea "github.com/charmbracelet/bubbletea"
)

// wrapForDeliberateTestPanic exists only to let requirement 36 be proven
// against a real deck process through a real PTY: mouse reporting must be
// disabled on every exit path, "including a panic," and a fake claim of that
// is worthless. This file is compiled in only when a test explicitly builds
// with `-tags decktestpanic` (see features/mouse_panic_test.go); it is never
// part of `go build ./...`, `go vet ./...`, `go test ./...` or a release, and
// even when built in it stays inert unless DECK_TEST_PANIC_KEY is set, which
// no documented deck control ever sets on a user's behalf.
func wrapForDeliberateTestPanic(model tea.Model) tea.Model {
	key := os.Getenv("DECK_TEST_PANIC_KEY")
	if key == "" {
		return model
	}
	return panicOnKeyModel{Model: model, key: key}
}

// panicOnKeyModel panics the instant it observes the configured key, before
// delegating to the wrapped model, so the panic happens inside the same
// Update call chain a real bug would use — proving Bubble Tea's own
// recover-and-restore path (which deck relies on rather than reimplementing)
// actually disables mouse reporting before the process exits.
type panicOnKeyModel struct {
	tea.Model
	key string
}

func (m panicOnKeyModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if k, ok := msg.(tea.KeyMsg); ok && k.String() == m.key {
		panic("deliberate test panic (DECK_TEST_PANIC_KEY=" + m.key + ") for requirement 36's disable-on-panic proof")
	}
	inner, cmd := m.Model.Update(msg)
	return panicOnKeyModel{Model: inner, key: m.key}, cmd
}
