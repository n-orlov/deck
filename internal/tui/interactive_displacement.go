package tui

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/n-orlov/deck/internal/interactive"
)

// interactiveDisplacementChecked carries task 118's claim-still-ours /
// SessionAttachedCount backstop poll's own account back into Update.
// windowTarget is the target the poll actually ran against, captured at
// SCHEDULING time rather than trusted to still equal
// m.interactiveWindowTarget by the time the round trip completes: the
// fast path may have already raised the dialog this same tick, or
// interactive mode may have been left (Ctrl+Q) and re-entered against a
// different session entirely, and this message must be ignored rather
// than misattributed to whichever window happens to be live when it
// lands (case interactiveDisplacementChecked below checks this).
type interactiveDisplacementChecked struct {
	windowTarget string
	displaced    bool
	sessionName  string
}

// interactiveDisplacementFastPath is task 118's cheap, no-tmux-call check
// against SPEC §11.9's own fast path: the pipe transport's displacement
// signal (interactive.StatusDisplaced). A stolen claim (`F` on another
// process) re-arms a competing `pipe-pane -IO` on the very same target as
// part of the new holder's own entry (interactive.StartWithTransport),
// which displaces this holder's own pipe immediately -- so under
// interactive.TransportPipe the F-steal flavour is caught here, on the
// very first previewTick after the steal, with no tmux round trip at all.
//
// It can never fire under interactive.TransportCapture (Transport's own
// doc: "There is no live byte stream to displace... Status stays
// StatusLive for a capture Session's whole life"), and it can never fire
// for the client-attached flavour either (a plain `tmux attach` never
// touches pipe-pane) -- checkInteractiveDisplacementBackstop below is
// what catches both of those instead.
func (m Model) interactiveDisplacementFastPath() bool {
	return m.interactive && m.interactiveGrid != nil && m.interactiveGrid.Status() == interactive.StatusDisplaced
}

// checkInteractiveDisplacementBackstop is task 118's backstop poll,
// issued alongside the fast path above on every previewTick while
// m.interactive is true (never per keystroke -- SPEC §11.9: "not paid
// for with a tmux round-trip per keystroke", and updateInteractive itself
// gains no check at all). It answers "displaced" unless BOTH hold:
//
//   - claimStillMine (task 103's probe): a stolen claim (ClaimForeignLive)
//     is the `F`-flavour, and this is the only way to catch it under
//     interactive.TransportCapture, where the fast path never fires at
//     all;
//   - SessionAttachedCount(target) == 0: deck's own interactive mode is
//     never itself a tmux "attached client" (it only pipes/captures the
//     pane), so a nonzero count means some OTHER client has attached
//     directly -- SPEC §11.9's "full attach arriving and re-expressing
//     its own size" flavour, which never touches deck's own ownership
//     option and so is invisible to the claim probe on its own.
//
// A transport error from either tmux call is treated as "not displaced"
// (best-effort, like previewFit's own no-live-pane skip): acting on an
// unconfirmed read is never safer here than leaving the dialog closed
// for one more tick, and the very next tick tries again.
func (m Model) checkInteractiveDisplacementBackstop() tea.Cmd {
	if !m.interactive || m.interactiveOwnership == nil {
		return nil
	}
	client := m.tmuxClient
	ownership := m.interactiveOwnership
	target := m.interactiveWindowTarget
	name := ""
	if session, ok := m.selectedSession(); ok {
		name = session.Name
	}
	return func() tea.Msg {
		ctx := context.Background()
		displaced := !claimStillMine(ctx, ownership)
		if !displaced {
			if attached, err := client.SessionAttachedCount(ctx, target); err == nil && attached != 0 {
				displaced = true
			}
		}
		return interactiveDisplacementChecked{windowTarget: target, displaced: displaced, sessionName: name}
	}
}

// raiseLostAttach is both displacement flavours' one shared exit: leave
// interactive mode through exitInteractive's own still-mine-gated
// teardown (so a stolen claim's new holder is touched by neither the
// geometry restore nor the release -- task 112), then raise SPEC §11.9's
// lost-attach dialog naming the session that just lost it.
func (m Model) raiseLostAttach(name string) (Model, tea.Cmd) {
	next, cmd := m.exitInteractive()
	m = next.(Model)
	m.lostAttach = true
	m.lostAttachSession = name
	return m, cmd
}
