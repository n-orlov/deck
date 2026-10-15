package tui

import (
	"context"
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/n-orlov/deck/internal/theme"
	upd "github.com/n-orlov/deck/internal/update"
)

// updateInstaller is how ctrl+u installs a release: current is the running
// build's version (the confirmation shows it beside the target), run is the
// same verified update `deck update` performs. It runs on a command
// goroutine, never on the UI goroutine.
type updateInstaller struct {
	current string
	run     func(context.Context) (upd.Result, error)
}

// WithUpdateInstaller wires ctrl+u's confirmed update into the model. A model
// without an installer still shows the confirmation, but confirming it fails
// with "updating is unavailable".
func (m Model) WithUpdateInstaller(current string, run func(context.Context) (upd.Result, error)) Model {
	m.updateInstall = updateInstaller{current: current, run: run}
	return m
}

// updateOffered reports whether the update indicator is showing its Available
// offer: the only state in which ctrl+u does anything, and the only one in
// which the footer and `?` list it.
func (m Model) updateOffered() bool {
	return m.updateInd.state == updateAvailable && m.settings.UpdateCheckEnabled() && !m.settings.AutoUpdate
}

// updateFooterHints is the footer legend's ctrl+u entry: present only while
// the offer is up, so no other state advertises a key that does nothing.
func (m Model) updateFooterHints() []footerKeyHint {
	if !m.updateOffered() {
		return nil
	}
	return []footerKeyHint{{"ctrl+u", "ctrl+u", "update deck", nil}}
}

// updateHelpLines are the `?` overlay's ctrl+u lines, listed (like the
// bookmark keys) only while the Available offer is up.
const updateHelpLines = `  ctrl+u install the available deck update: a confirmation shows the current
    and the new version, then the verified update runs in the background.
    deck never restarts itself; restart it to apply. Shown only while a
    newer release is on offer
`

// keyUpdate handles "ctrl+u": while the indicator reads Available it opens
// the confirmation; in every other state it is inert.
func (m Model) keyUpdate(_ tea.KeyMsg) (tea.Model, tea.Cmd) {
	if !m.updateOffered() {
		return m, nil
	}
	m.updateConfirming = true
	return m, nil
}

// updateUpdateConfirm handles keys while the confirmation is open. Esc
// cancels and changes nothing; Enter starts the update in the background and
// closes the dialog, leaving the progress state in the indicator. Every other
// key is swallowed, which is what keeps a second ctrl+u from re-entering.
func (m Model) updateUpdateConfirm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	cmd, handled := applyDialogContract(msg, dialogContract{
		Cancel: func() { m.updateConfirming = false },
		Submit: func() tea.Cmd {
			m.updateConfirming = false
			if !m.updateOffered() {
				return nil
			}
			return m.startUpdate()
		},
	})
	if handled {
		return m, cmd
	}
	return m, nil
}

// startUpdate flips the indicator to its progress state and returns the
// command that runs the update off the UI goroutine.
func (m *Model) startUpdate() tea.Cmd {
	tag := m.updateInd.tag
	m.updateInd = updateIndicator{state: updateInstalling, tag: tag}
	m.footerNoteSets[noteUpdate]++
	run := m.updateInstall.run
	return func() tea.Msg {
		if run == nil {
			return updateFailedMsg{cause: "updating is unavailable"}
		}
		res, err := run(context.Background())
		if err != nil {
			return updateFailedMsg{cause: upd.RedactText(upd.StripControl(err.Error()))}
		}
		if res.Unchanged {
			return updateFailedMsg{cause: "no newer release to install"}
		}
		if res.Tag == "" {
			res.Tag = tag
		}
		return updateInstalledMsg{tag: res.Tag}
	}
}

func (m Model) updateConfirmView() string {
	return m.framedDialog(m.styledUpdateConfirmBody())
}

// updateConfirmBody is the confirmation's text: the running and the target
// version, what confirming does and what it leaves alone.
func (m Model) updateConfirmBody() string {
	current := upd.StripControl(m.updateInstall.current)
	if current == "" {
		current = "unknown"
	}
	return fmt.Sprintf(`Update deck

Current version: %s
Target version:  %s

Confirming downloads the release, verifies its checksum and build
attestation and replaces the deck binary on disk, in the background.
deck keeps running, does not restart and does not touch your sessions:
restart deck to run the new version.

Enter installs · Esc cancels
`, current, upd.StripControl(m.updateInd.tag))
}

func (m Model) styledUpdateConfirmBody() string {
	plain := m.updateConfirmBody()
	lines := m.wrapDialogLines(plain)
	for i, l := range lines {
		switch {
		case i == 0:
			lines[i] = m.colorToken(theme.Title, l)
		case l == "Enter installs · Esc cancels":
			lines[i] = m.colorToken(theme.Hint, l)
		}
	}
	out := ""
	for i, l := range lines {
		if i > 0 {
			out += "\n"
		}
		out += l
	}
	return out
}
