package tui

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	upd "github.com/n-orlov/deck/internal/update"
)

// updateIndicatorState is what the update indicator in the footer's note slot
// reads (SPEC §11, "The update indicator").
type updateIndicatorState int

const (
	// updateHidden shows nothing: the check is disabled, offline or failed
	// silently, the build is current, or it is a dev build.
	updateHidden updateIndicatorState = iota
	// updateAvailable offers ctrl+u: a newer release exists and this install
	// can be updated.
	updateAvailable
	// updateRefused names why a newer release cannot be installed from here
	// (an unwritable install directory, a package-managed binary).
	updateRefused
	// updateInstalled says the new release is on disk and a restart applies
	// it. It persists until deck restarts.
	updateInstalled
	// updateFailed says an update was attempted and failed, naming the cause.
	updateFailed
	// updateInstalling is the progress state of a confirmed ctrl+u update
	// (R299): the verified update is running in the background.
	updateInstalling
	// updateSkipped says auto_update is on but this build cannot take a
	// release at all (a dev build, R300): the reason, and no download.
	updateSkipped
)

// updateIndicator is the model's whole update-indicator state.
type updateIndicator struct {
	state  updateIndicatorState
	tag    string // the release the state talks about
	reason string // the refusal reason (updateRefused) or the cause (updateFailed)
}

// updateSource is how the model learns about releases: the throttled daily
// check, and the preflight that says whether this install can be updated.
// Both run off the UI goroutine, in the command that carries them.
type updateSource struct {
	check     func(context.Context) upd.Status
	preflight func() error
}

// updateChecked is the daily check's answer. refusal is the preflight's
// error for a newer release (nil when the install can be updated).
type updateChecked struct {
	status  upd.Status
	refusal error
}

// updateInstalledMsg and updateFailedMsg report an update attempt's end. The
// ctrl+u flow and the auto-install send them.
type updateInstalledMsg struct{ tag string }
type updateFailedMsg struct{ cause string }

// WithUpdateSource wires the daily update check into the model: check is the
// throttled check (update.Checker.Check) and preflight says whether this
// install may be updated (update.Preflight). A model without a source never
// shows an indicator.
func (m Model) WithUpdateSource(check func(context.Context) upd.Status, preflight func() error) Model {
	m.updateSrc = updateSource{check: check, preflight: preflight}
	return m
}

// updateCheckCmd runs the daily check and, for a newer release, the
// preflight, on the command goroutine. It returns nil without a source.
func (m Model) updateCheckCmd() tea.Cmd {
	src := m.updateSrc
	if src.check == nil {
		return nil
	}
	// The effective settings are read here, when the request is made, never
	// from a copy captured at startup (SPEC §6.5): a check switched off by a
	// live save or an external reload makes no request.
	if !m.settings.UpdateCheckEnabled() {
		return nil
	}
	auto := m.settings.AutoUpdate
	return func() tea.Msg {
		st := src.check(context.Background())
		out := updateChecked{status: st}
		switch {
		case src.preflight == nil:
		case st.Newer:
			out.refusal = src.preflight()
		case auto && st.Latest != "":
			// A dev build is never outdated, so the check sets no Newer; with
			// auto_update on, the preflight still names why nothing installs.
			if err := src.preflight(); isDevRefusal(err) {
				out.refusal = err
			}
		}
		return out
	}
}

// updateSettingsApplied runs after a live settings save or an external reload
// changed m.settings. Enabling the check (update_check or auto_update going
// on) runs the daily check now, without a restart; turning auto_update on
// while a release is already on offer installs it at once, as an auto-update
// check that had found it would. Disabling needs no command: the next
// updateCheckCmd sees the check off and makes no request, and an answer
// already in flight is applied without an install (onUpdateChecked reads the
// current auto_update).
func (m *Model) updateSettingsApplied(wasEnabled, wasAuto bool) tea.Cmd {
	if m.settings.AutoUpdate && !wasAuto && m.updateInd.state == updateAvailable {
		return m.startUpdate()
	}
	if m.settings.UpdateCheckEnabled() && !wasEnabled {
		return m.updateCheckCmd()
	}
	return nil
}

// isDevRefusal reports whether err refuses a dev build.
func isDevRefusal(err error) bool {
	r, ok := upd.AsRefusal(err)
	return ok && r.Reason == upd.RefusedDev
}

// onUpdateChecked records the daily check's answer. With auto_update on and
// an Available result (a newer release this install may take) it installs the
// release through the same verified update ctrl+u runs, with no prompt. A
// refusal (unwritable directory, managed prefix, dev build) or a check that
// found nothing newer starts no download; a dev build's refusal reads
// Skipped (updateSkipped) while auto_update is on.
func (m Model) onUpdateChecked(msg updateChecked) (tea.Model, tea.Cmd) {
	m.applyUpdateCheck(msg.status, msg.refusal)
	if m.settings.AutoUpdate && m.updateInd.state == updateAvailable {
		return m, m.startUpdate()
	}
	return m, nil
}

func (m Model) onUpdateInstalled(msg updateInstalledMsg) (tea.Model, tea.Cmd) {
	m.updateInd = updateIndicator{state: updateInstalled, tag: msg.tag}
	m.footerNoteSets[noteUpdate]++
	return m, nil
}

func (m Model) onUpdateFailed(msg updateFailedMsg) (tea.Model, tea.Cmd) {
	if m.updateInd.state == updateInstalled {
		return m, nil
	}
	m.updateInd = updateIndicator{state: updateFailed, tag: m.updateInd.tag, reason: msg.cause}
	m.footerNoteSets[noteUpdate]++
	return m, nil
}

// applyUpdateCheck records a check result. Installed stays for the life of
// the process: a later check never replaces it. A refusal that is not a
// Refusal (the binary cannot be located) hides the indicator.
func (m *Model) applyUpdateCheck(st upd.Status, refusal error) {
	if m.updateInd.state == updateInstalled || m.updateInd.state == updateInstalling {
		return
	}
	next := m.classifyUpdateCheck(st, refusal)
	if next != m.updateInd {
		m.footerNoteSets[noteUpdate]++
	}
	m.updateInd = next
}

// classifyUpdateCheck maps a check result and its preflight refusal to the
// indicator state it leaves.
func (m *Model) classifyUpdateCheck(st upd.Status, refusal error) updateIndicator {
	switch {
	case st.Latest != "" && m.settings.AutoUpdate && isDevRefusal(refusal):
		return updateIndicator{state: updateSkipped, tag: st.Latest, reason: "dev build, install a release with install.sh"}
	case !st.Newer || st.Latest == "":
		return updateIndicator{state: updateHidden}
	case refusal == nil:
		return updateIndicator{state: updateAvailable, tag: st.Latest}
	case upd.IsRefusal(refusal):
		return updateIndicator{state: updateRefused, tag: st.Latest, reason: refusalPhrase(refusal)}
	}
	return updateIndicator{state: updateHidden}
}

// refusalPhrase is the short reason the indicator names for a refusal.
func refusalPhrase(err error) string {
	r, ok := upd.AsRefusal(err)
	if !ok {
		return ""
	}
	switch r.Reason {
	case upd.RefusedManaged:
		return "installed by a package manager, update it there"
	case upd.RefusedUnwritable:
		return "install directory is not writable, see deck update"
	}
	return "cannot be updated here, see deck update"
}

// updateIndicatorText is the indicator's note, or "" when it is hidden. The
// check being off hides every state but Installed (the binary on disk is new
// whatever the setting now says). An Available offer is also hidden while
// auto_update is on: deck installs it without asking. Skipped shows only
// while auto_update is on.
func (m Model) updateIndicatorText() string {
	ind := m.updateInd
	if ind.state != updateInstalled && ind.state != updateInstalling && !m.settings.UpdateCheckEnabled() {
		return ""
	}
	return m.updateStateText(ind)
}

// updateStateText is the note for one indicator state under the current
// auto_update setting.
func (m Model) updateStateText(ind updateIndicator) string {
	switch ind.state {
	case updateAvailable:
		if m.settings.AutoUpdate {
			return ""
		}
		return fmt.Sprintf("update available: %s — press ctrl+u to install", ind.tag)
	case updateRefused:
		return fmt.Sprintf("update available: %s — %s", ind.tag, ind.reason)
	case updateSkipped:
		if !m.settings.AutoUpdate {
			return ""
		}
		return fmt.Sprintf("auto-update skipped: %s — %s", ind.tag, ind.reason)
	case updateInstalling:
		return fmt.Sprintf("updating to %s — downloading and verifying…", ind.tag)
	case updateInstalled:
		return fmt.Sprintf("update installed: %s — restart deck to apply", ind.tag)
	case updateFailed:
		return updateFailedText(ind.reason)
	}
	return ""
}

// updateFailedText is the Failed note: the fixed phrase plus the cause, with
// its whitespace collapsed to one line, when there is one.
func updateFailedText(reason string) string {
	text := "update failed — see deck update"
	if cause := strings.Join(strings.Fields(upd.RedactText(reason)), " "); cause != "" {
		text += " (" + cause + ")"
	}
	return text
}
