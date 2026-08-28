//go:build deckinputcount

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"sync/atomic"

	tea "github.com/charmbracelet/bubbletea"
)

// inputCountFileEnvironment names the file this build records the running
// total of keys received into, one plain decimal integer, overwritten (not
// appended) after every tea.KeyMsg the wrapped model's Update sees. It is
// deliberately a file rather than stdout/stderr: deck's own stdout is the
// terminal being driven through a real pty in every scenario that would use
// this, so a second channel is needed for the harness to poll without
// disturbing what is under test. See docs/reports/phase3d-input-count.md
// for how a scenario or test reads it.
const inputCountFileEnvironment = "DECK_INPUT_COUNT_FILE"

// wrapForInputCounting is compiled in only when a test explicitly builds
// `-tags deckinputcount` (see cmd/deck/inputcount_default.go for the no-op
// every other build sees), and even then stays inert unless
// DECK_INPUT_COUNT_FILE is set, which no documented deck control ever sets
// on a user's behalf. It exists so I-1's keystroke-drop investigation can
// distinguish "the pty never delivered the byte" from "deck received the
// key but did something else with it" -- a question no amount of screen-
// content assertion can answer, because both failure modes render an
// unchanged frame.
func wrapForInputCounting(model tea.Model) tea.Model {
	path := os.Getenv(inputCountFileEnvironment)
	if path == "" {
		return model
	}
	return &inputCountingModel{Model: model, path: path}
}

// inputCountingModel counts at the outermost Update call this process makes
// -- before deck's own coalesced-KeyMsg splitting (internal/tui/tui.go,
// tea.KeyMsg case) ever runs, and before any dialog or key binding decides
// whether to act on it. This is deliberately the earliest programmatic
// point reachable without patching bubbletea itself: everything below it,
// including the splitting task 118 added, is deck's own application logic,
// which this instrument exists to be independent of.
//
// A tea.KeyMsg's weight is the number of keystrokes it represents, not the
// number of Update calls it costs: a coalesced KeyMsg{Type: KeyRunes, Runes:
// "jx"} is bubbletea's PTY reader reporting that TWO keys landed in one
// read, so it counts as 2, mirroring exactly the len(msg.Runes) test
// internal/tui/tui.go's own coalescing case already uses. A bracketed paste
// is exempted from that, the same way deck's own splitting exempts it
// (msg.Paste): a paste is one input event, not N keystrokes, regardless of
// how many runes it carries. Every other message type (mouse, resize,
// ticks, ...) is passed through uncounted; this instrument answers only
// "how many keys", not "how many messages".
type inputCountingModel struct {
	tea.Model
	path  string
	total int64
}

// Unwrap lets cmd/deck's own always-on interactiveShutdownGuard
// (interactive_shutdown.go) see through this test-only wrapper to reach
// the real tui.Model underneath, in case a panic reaches it while a
// -tags deckinputcount build is also in use.
func (m *inputCountingModel) Unwrap() tea.Model { return m.Model }

func (m *inputCountingModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		weight := int64(1)
		if !key.Paste && len(key.Runes) > 0 {
			weight = int64(len(key.Runes))
		}
		total := atomic.AddInt64(&m.total, weight)
		writeInputCount(m.path, total)
	}
	inner, cmd := m.Model.Update(msg)
	m.Model = inner
	return m, cmd
}

// writeInputCount overwrites path with total as a bare decimal integer,
// via a write-to-temp-then-rename so a concurrent reader (the test harness
// polling this file) never observes a partially written value: os.Rename
// within the same directory is atomic on every platform this project ships
// for. Any failure (an unwritable directory, a path that was never set up)
// is silently swallowed -- like the sizes recorder this mirrors
// (cmd/fake-claude/main.go's recordSize), counting is scaffolding for a test
// harness, never part of deck's own observable contract, and must never
// turn an otherwise-working process into a crash.
func writeInputCount(path string, total int64) {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".deck-input-count-*")
	if err != nil {
		return
	}
	name := tmp.Name()
	_, writeErr := fmt.Fprint(tmp, strconv.FormatInt(total, 10))
	closeErr := tmp.Close()
	if writeErr != nil || closeErr != nil {
		os.Remove(name)
		return
	}
	if err := os.Rename(name, path); err != nil {
		os.Remove(name)
	}
}
