package tui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/n-orlov/deck/internal/config"
	upd "github.com/n-orlov/deck/internal/update"
)

const (
	indicatorTag      = "v1.2.3"
	indicatorAvailble = "update available: v1.2.3 — press ctrl+u to install"
)

// indicatorRefusal is the Refusal the preflight returns for one reason.
func indicatorRefusal(reason string) error {
	return &upd.Refusal{Reason: reason, Message: "refused (" + reason + ")"}
}

// updateIndicatorRows is one row per R298 state: how the model reaches it
// through the real message path, and the footer text it must show ("" for a
// hidden state). The four hidden rows are disabled, offline, current and a
// dev build.
var updateIndicatorRows = []struct {
	name string
	show string
	set  func(t *testing.T, m Model) Model
}{
	{"available", indicatorAvailble, func(t *testing.T, m Model) Model {
		return checkedModel(t, m, true, upd.Status{Latest: indicatorTag, Newer: true}, nil)
	}},
	{"installed", "update installed: v1.2.3 — restart deck to apply", func(t *testing.T, m Model) Model {
		return sendUpdateMsg(t, m, updateInstalledMsg{tag: indicatorTag})
	}},
	{"failed", "update failed — see deck update (checksum mismatch)", func(t *testing.T, m Model) Model {
		m.settings.UpdateCheck = true
		return sendUpdateMsg(t, m, updateFailedMsg{cause: "checksum mismatch"})
	}},
	{"unwritable", "update available: v1.2.3 — install directory is not writable, see deck update", func(t *testing.T, m Model) Model {
		return checkedModel(t, m, true, upd.Status{Latest: indicatorTag, Newer: true}, indicatorRefusal(upd.RefusedUnwritable))
	}},
	{"managed", "update available: v1.2.3 — installed by a package manager, update it there", func(t *testing.T, m Model) Model {
		return checkedModel(t, m, true, upd.Status{Latest: indicatorTag, Newer: true}, indicatorRefusal(upd.RefusedManaged))
	}},
	{"hidden-disabled", "", func(t *testing.T, m Model) Model {
		return checkedModel(t, m, false, upd.Status{Latest: indicatorTag, Newer: true}, nil)
	}},
	{"hidden-offline", "", func(t *testing.T, m Model) Model {
		return checkedModel(t, m, true, upd.Status{}, nil)
	}},
	{"hidden-current", "", func(t *testing.T, m Model) Model {
		return checkedModel(t, m, true, upd.Status{Latest: indicatorTag, Newer: upd.IsNewer(indicatorTag, indicatorTag)}, nil)
	}},
	{"hidden-dev-build", "", func(t *testing.T, m Model) Model {
		return checkedModel(t, m, true, upd.Status{Latest: indicatorTag, Newer: upd.IsNewer(indicatorTag, upd.DevVersion)}, nil)
	}},
}

func sendUpdateMsg(t *testing.T, m Model, msg tea.Msg) Model {
	t.Helper()
	next, _ := m.Update(msg)
	out, ok := next.(Model)
	if !ok {
		t.Fatalf("Update returned %T, want Model", next)
	}
	return out
}

// checkedModel delivers one daily-check answer to m with update_check set to
// enabled.
func checkedModel(t *testing.T, m Model, enabled bool, st upd.Status, refusal error) Model {
	t.Helper()
	m.settings.UpdateCheck = enabled
	return sendUpdateMsg(t, m, updateChecked{status: st, refusal: refusal})
}

// footerOf is the last line of a view: the footer.
func footerOf(view string) string {
	lines := strings.Split(view, "\n")
	return stripANSI(lines[len(lines)-1])
}

// TestUpdateIndicatorStates is R298's table: nine states, each at 80 and 160
// columns in list and interactive mode (36 cases). A shown state puts its
// text in the footer line; a hidden state puts none there; and the frame has
// the same number of lines with the indicator shown as hidden.
func TestUpdateIndicatorStates(t *testing.T) {
	widths := []int{80, 160}
	for _, row := range updateIndicatorRows {
		for _, width := range widths {
			for _, interactive := range []bool{false, true} {
				mode := "list"
				if interactive {
					mode = "interactive"
				}
				t.Run(fmt.Sprintf("%s/%dcols/%s", row.name, width, mode), func(t *testing.T) {
					hidden := footerNoteFrameModel(width, 24, interactive)
					shown := row.set(t, footerNoteFrameModel(width, 24, interactive))

					hiddenView, shownView := hidden.mainView(), shown.mainView()
					if got, want := len(strings.Split(shownView, "\n")), len(strings.Split(hiddenView, "\n")); got != want {
						t.Errorf("line count with the indicator = %d, hidden = %d: it took its own row", got, want)
					}
					footer := footerOf(shownView)
					if row.show == "" {
						if strings.Contains(footer, "update ") {
							t.Errorf("footer %q shows an indicator, want it hidden", footer)
						}
						return
					}
					if !strings.Contains(footer, row.show) {
						t.Errorf("footer %q does not show %q", footer, row.show)
					}
					if strings.Contains(footerOf(hiddenView), "update ") {
						t.Errorf("hidden footer %q shows an indicator", footerOf(hiddenView))
					}
				})
			}
		}
	}
}

// TestFooterLegendDropsWholeEntriesAt80x24 is R298's fit rule: at the 80x24
// minimum with the Available indicator shown, the footer line is 80 columns,
// holds the indicator whole, and its legend is a run of whole entries with at
// least one entry dropped for lack of room -- never one cut mid-entry.
func TestFooterLegendDropsWholeEntriesAt80x24(t *testing.T) {
	for _, interactive := range []bool{false, true} {
		m := footerNoteFrameModel(80, 24, interactive)
		m = checkedModel(t, m, true, upd.Status{Latest: indicatorTag, Newer: true}, nil)
		line := stripANSI(m.footerLineContent())
		if got := stringWidth(line); got != 80 {
			t.Fatalf("interactive=%v: footer is %d columns, want 80: %q", interactive, got, line)
		}
		if !strings.HasSuffix(line, indicatorAvailble) {
			t.Fatalf("interactive=%v: footer %q does not end with the indicator", interactive, line)
		}
		legend := strings.TrimRight(strings.TrimSuffix(line, indicatorAvailble), " ")
		if interactive {
			const quit = "Ctrl+Q leave interactive mode"
			if legend != "" && !strings.Contains(legend, quit) {
				t.Fatalf("interactive: clipped text %q left of the indicator", legend)
			}
			continue
		}
		if !wholeLegendPrefixes(m)[legend] {
			t.Fatalf("list: legend part %q is not a run of whole entries", legend)
		}
		full, _ := m.footerLegendSegments()
		if legend == stripANSI(strings.Join(full, m.glyph(" · ", " - "))) {
			t.Fatalf("list: the whole legend fits beside the indicator at 80 columns; the test no longer exercises dropping")
		}
	}
}

// TestInstalledIndicatorPersists: Installed replaces Available, and no later
// check result, failure or setting change takes it back for the life of the
// process.
func TestInstalledIndicatorPersists(t *testing.T) {
	m := footerNoteFrameModel(160, 24, false)
	m = checkedModel(t, m, true, upd.Status{Latest: indicatorTag, Newer: true}, nil)
	if got := m.footerNote(); got != indicatorAvailble {
		t.Fatalf("before install: note = %q, want %q", got, indicatorAvailble)
	}
	m = sendUpdateMsg(t, m, updateInstalledMsg{tag: indicatorTag})
	const want = "update installed: v1.2.3 — restart deck to apply"
	if got := m.footerNote(); got != want {
		t.Fatalf("after install: note = %q, want %q", got, want)
	}
	later := []struct {
		name string
		msg  tea.Msg
	}{
		{"newer release", updateChecked{status: upd.Status{Latest: "v1.3.0", Newer: true}}},
		{"same release", updateChecked{status: upd.Status{Latest: indicatorTag, Newer: true}}},
		{"current", updateChecked{status: upd.Status{Latest: indicatorTag}}},
		{"offline", updateChecked{}},
		{"refusal", updateChecked{status: upd.Status{Latest: "v1.3.0", Newer: true}, refusal: indicatorRefusal(upd.RefusedManaged)}},
		{"failure", updateFailedMsg{cause: "boom"}},
	}
	for _, step := range later {
		m = sendUpdateMsg(t, m, step.msg)
		if got := m.footerNote(); got != want {
			t.Fatalf("after %s: note = %q, want %q", step.name, got, want)
		}
	}
	m.settings.UpdateCheck, m.settings.AutoUpdate = false, false
	if got := m.footerNote(); got != want {
		t.Fatalf("with update_check off: note = %q, want %q", got, want)
	}
}

// TestUpdateIndicatorYieldsToTransientNotes: any other footer note shows over
// the indicator, never the reverse, and the indicator is back when it clears.
func TestUpdateIndicatorYieldsToTransientNotes(t *testing.T) {
	m := footerNoteFrameModel(160, 24, false)
	m.undoSessionID, m.undoSessionName = "s1", "alpha"
	m = checkedModel(t, m, true, upd.Status{Latest: indicatorTag, Newer: true}, nil)
	if got := m.footerNote(); !strings.HasPrefix(got, "Killed") {
		t.Fatalf("an indicator arriving over an undo window took the slot: %q", got)
	}
	m.undoSessionID = ""
	if got := m.footerNote(); got != indicatorAvailble {
		t.Fatalf("after the undo window: note = %q, want the indicator", got)
	}
}

// TestUpdateIndicatorAutoUpdateOnReplacesTheOffer: with auto_update on the
// Available offer is never drawn (deck installs without asking, so the
// indicator goes straight to the progress state and a command is returned);
// a refusal still is, because nothing will install. The install itself is
// covered end to end by TestAutoUpdate (this replaces the earlier pin that
// the note was empty while the check result was handled).
func TestUpdateIndicatorAutoUpdateOnReplacesTheOffer(t *testing.T) {
	m := footerNoteFrameModel(160, 24, false)
	m.settings = config.Settings{AutoUpdate: true}
	next, cmd := m.Update(updateChecked{status: upd.Status{Latest: indicatorTag, Newer: true}})
	offer := next.(Model)
	if got := offer.footerNote(); strings.Contains(got, "ctrl+u") || !strings.HasPrefix(got, "updating to "+indicatorTag) || cmd == nil {
		t.Fatalf("auto_update on: note = %q (cmd %v), want the progress state and no offer", got, cmd != nil)
	}
	refused := sendUpdateMsg(t, m, updateChecked{status: upd.Status{Latest: indicatorTag, Newer: true}, refusal: indicatorRefusal(upd.RefusedUnwritable)})
	if got := refused.footerNote(); !strings.Contains(got, "not writable") {
		t.Fatalf("auto_update on, unwritable: note = %q, want the reason", got)
	}
}

// TestUpdateCheckCmdAsksPreflightOnlyForANewerRelease: the command runs the
// check off the UI goroutine, and consults the preflight only when there is
// something to install.
func TestUpdateCheckCmdAsksPreflightOnlyForANewerRelease(t *testing.T) {
	boom := errors.New("preflight")
	for _, newer := range []bool{true, false} {
		calls := 0
		m := New(nil, config.Settings{UpdateCheck: true}, "").WithUpdateSource(
			func(context.Context) upd.Status { return upd.Status{Latest: indicatorTag, Newer: newer} },
			func() error { calls++; return boom },
		)
		msg, ok := m.updateCheckCmd()().(updateChecked)
		if !ok {
			t.Fatalf("newer=%v: command did not return updateChecked", newer)
		}
		if newer && (calls != 1 || !errors.Is(msg.refusal, boom)) {
			t.Fatalf("newer: preflight calls = %d, refusal = %v", calls, msg.refusal)
		}
		if !newer && calls != 0 {
			t.Fatalf("not newer: preflight ran %d times", calls)
		}
	}
	if cmd := New(nil, config.Settings{}, "").updateCheckCmd(); cmd != nil {
		t.Fatal("a model with no update source scheduled a check")
	}
}
