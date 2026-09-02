package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/n-orlov/deck/internal/theme"
)

// lostAttachView renders SPEC \u00a711.9's lost-attach dialog inside
// framedDialog (task 078's shared box, same as restartChoiceView/
// eventLogView): a displaced client's interactive mode has already ended
// by the time this raises (its own claim stolen by `F`, or a full attach
// arriving and re-expressing its own size, task 118), so there is nothing
// underneath it to cancel back to -- only \u21b5 dismisses it.
func (m Model) lostAttachView() string {
	return m.framedDialog(m.styledLostAttachBody())
}

// lostAttachBody is lostAttachView's plain-text content, split out the
// same way restartChoiceBody/eventLogBody are so styledLostAttachBody can
// re-derive the identical structure with colour. It reads only
// m.lostAttachSession -- the display name captured at the moment the
// dialog opened -- never m.sessions/m.selected, which may have moved on
// by the time this renders (SPEC \u00a711.4: "a dialog never reads the store
// from its render path", and by extension never a possibly-stale
// selection either).
func (m Model) lostAttachBody() string {
	var b strings.Builder
	fmt.Fprintf(&b, "Lost attach: %s\n\n", m.lostAttachSession)
	b.WriteString("Another client took over this session's window.\n")
	b.WriteString("\nEnter dismisses\n")
	return b.String()
}

// styledLostAttachBody re-derives lostAttachBody's exact structure with
// SPEC \u00a711.4's token mapping (SPEC.md:1355): the title line in `title`,
// the explanatory sentence in `dimmed`, the footer's one contract key
// ("Enter") in `key` over `hint` prose -- the same split
// styledRestartChoiceBody/styledEventLogBody already draw.
func (m Model) styledLostAttachBody() string {
	wrap := m.wrapDialogLines
	var out []string
	colorWhole := func(tok theme.Token, line string) {
		for _, l := range wrap(line) {
			out = append(out, m.colorToken(tok, l))
		}
	}
	colorWhole(theme.Title, fmt.Sprintf("Lost attach: %s", m.lostAttachSession))
	out = append(out, "")
	colorWhole(theme.Dimmed, "Another client took over this session's window.")
	out = append(out, "")
	for _, l := range wrap("Enter dismisses") {
		fields := strings.Fields(l)
		for i, f := range fields {
			if f == "Enter" {
				fields[i] = m.colorToken(theme.Key, f)
			} else {
				fields[i] = m.colorToken(theme.Hint, f)
			}
		}
		out = append(out, strings.Join(fields, " "))
	}
	return strings.Join(out, "\n")
}

// updateLostAttachView handles every key while SPEC \u00a711.9's lost-attach
// dialog is open. It swallows every key -- "that is its point" (SPEC.md:
// ~1770) -- except \u21b5, which applyDialogContract's Submit closes it on;
// there is no Cancel/esc binding here (Cancel nil is left unhandled by
// the contract, same as detailView/helpView's single-case dialogs when a
// key falls outside their own vocabulary too), so esc simply does
// nothing, matching "dismisses on \u21b5" literally rather than also on esc.
func (m Model) updateLostAttachView(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	cmd, handled := applyDialogContract(msg, dialogContract{
		Submit: func() tea.Cmd {
			m.lostAttach = false
			m.lostAttachSession = ""
			return nil
		},
	})
	if handled {
		return m, cmd
	}
	return m, nil
}
