package tui

import (
	"os"
	"reflect"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/n-orlov/deck/internal/config"
)

// configPollTick fires every Settings.ConfigPoll (SPEC §6.5: 30 s in
// production, shorter only through the test-only DECK_CONFIG_POLL_MS).
type configPollTick time.Time

// configPolled is one poll's outcome. changed is false when the fingerprint
// of config.toml and the user theme files matched the last-seen one, in
// which case nothing was parsed.
type configPolled struct {
	settings config.Settings
	changed  bool
	err      error
}

// newConfigReloader builds the production poller for settings, or nil when
// polling is off (a hand-built Settings with no ConfigPoll). It re-reads
// through the same loader start-up used and holds no writer (SPEC §6.5).
func newConfigReloader(settings config.Settings) *config.Reloader {
	if settings.ConfigPoll <= 0 {
		return nil
	}
	return config.NewReloader(settings, func() (config.Settings, error) {
		return config.LoadFromProfile(os.Getenv, os.UserHomeDir, settings.Profile)
	})
}

func (m Model) configPollTickCmd() tea.Cmd {
	return tea.Tick(m.settings.ConfigPoll, func(t time.Time) tea.Msg { return configPollTick(t) })
}

// onConfigPollTick runs one fingerprint comparison off the update loop. The
// next tick is only scheduled by onConfigPolled, so two polls never overlap
// and the Reloader needs no lock.
func (m Model) onConfigPollTick(_ configPollTick) (tea.Model, tea.Cmd) {
	reloader := m.configReloader
	if reloader == nil {
		return m, nil
	}
	return m, func() tea.Msg {
		settings, changed, err := reloader.Poll()
		return configPolled{settings: settings, changed: changed, err: err}
	}
}

// onConfigPolled records a reload's result and schedules the next poll. An
// unchanged poll leaves the model untouched. A reload that failed to parse
// keeps the running settings exactly as they were and raises a one-line
// notice (configReloadBanner); the next valid write clears it. A good reload
// is applied to the running model (applyReloadedSettings, SPEC §6.5) unless
// the settings takeover holds an unsaved edit, in which case it is held back
// and applied when that edit is saved (merged into the write) or cancelled.
func (m Model) onConfigPolled(msg configPolled) (tea.Model, tea.Cmd) {
	tick := m.configPollTickCmd()
	if !msg.changed {
		return m, tick
	}
	m.reloadErr = msg.err
	if msg.err != nil {
		return m, tick
	}
	reloaded := msg.settings
	m.reloaded = &reloaded
	if m.settingsEditPending() {
		m.reloadHeld = &reloaded
		return m, tick
	}
	m.reloadHeld = nil
	return m, tea.Batch(m.applyReloadedSettings(reloaded), tick)
}

// settingsEditPending reports whether the settings takeover is holding
// something a reload must not clobber: a staged edit not yet saved, the
// discard prompt, or a text editor that is open.
func (m Model) settingsEditPending() bool {
	if !m.settingsOpen {
		return false
	}
	return m.settingsDirty() || m.settingsDiscardConfirm || m.settingsStringEditing || m.settingsEnvEditing
}

// releaseHeldReload applies a reload that was held back for an open edit as
// soon as the edit is no longer pending (it was cancelled or reverted). A
// save consumes the held reload itself (settingsSave), so by then it is nil.
func (m Model) releaseHeldReload(next tea.Model, cmd tea.Cmd) (tea.Model, tea.Cmd) {
	model, ok := next.(Model)
	if !ok || model.reloadHeld == nil || model.settingsEditPending() {
		return next, cmd
	}
	held := *model.reloadHeld
	model.reloadHeld = nil
	return model, tea.Batch(cmd, model.applyReloadedSettings(held))
}

// mergeHeldReload folds a held reload into a save: every key the user did not
// touch in this editing session (edits equals baseline) takes the reloaded
// file's value, every key the user changed keeps their value, so the write
// neither drops the other instance's change nor the user's own.
func mergeHeldReload(edits, baseline, reloaded config.FileConfig) config.FileConfig {
	merged := settingsCloneFileConfig(edits)
	mv := reflect.ValueOf(&merged).Elem()
	ev, bv, rv := reflect.ValueOf(edits), reflect.ValueOf(baseline), reflect.ValueOf(reloaded)
	for i := 0; i < mv.NumField(); i++ {
		if reflect.DeepEqual(ev.Field(i).Interface(), bv.Field(i).Interface()) {
			mv.Field(i).Set(rv.Field(i))
		}
	}
	return settingsCloneFileConfig(merged)
}

// configReloadBanner is the one-line, non-fatal notice raised while the last
// changed config.toml could not be read: the previous settings stay in force.
func (m Model) configReloadBanner(width int) []string {
	if m.reloadErr == nil {
		return nil
	}
	text, _, _ := strings.Cut(m.reloadErr.Error(), "\n")
	text = "config.toml not reloaded, keeping the previous settings: " + text
	return []string{truncateToWidth(text, width), ""}
}

// applyReloadedSettings re-applies a freshly reloaded Settings to the running
// model with no restart (SPEC §6.5). It reuses the settings takeover's own
// live-apply path (settingsApplyLiveFields) with the reloaded file as the
// "saved" side and the running file as "previous", so a key the file did not
// change is untouched, a key the environment overrides stays pinned, and a
// restart-to-apply key only has its file value refreshed. The theme is also
// re-taken from the reload itself so that editing a user theme file's colours
// (same name, new contents) changes the active theme. The returned command
// carries the terminal-level mouse toggle when ui.mouse changed.
func (m *Model) applyReloadedSettings(next config.Settings) tea.Cmd {
	previous := m.settings.File
	cmd := m.settingsApplyLiveFields(next.File, previous)
	m.retakeTheme(next)
	m.settings.File = settingsCloneFileConfig(next.File)
	m.reloadApplied++
	if m.settingsOpen && !m.settingsDirty() {
		// An open but untouched takeover shows the reloaded values, and its
		// later save writes them back rather than the stale ones.
		m.settingsEdits = settingsEditsFromSettings(m.settings)
		m.settingsSavedEdits = settingsEditsFromSettings(m.settings)
	}
	return cmd
}

// retakeTheme adopts the reloaded Settings' resolved theme when it differs
// from the running one, which is how an edit to the active user theme file
// (same configured name, new colours) reaches the running model.
func (m *Model) retakeTheme(next config.Settings) {
	if next.Theme != nil && !reflect.DeepEqual(next.Theme, m.settings.Theme) {
		m.settings.Theme = next.Theme
		m.settings.ThemeReason = next.ThemeReason
	}
}

// applyHeldTheme finishes a save that folded a held reload into its write
// (SPEC §6.5): the file part was merged and applied through the save's own
// path, but a changed user theme file under an unchanged configured name is
// only visible in the held reload's resolved theme. It is adopted unless the
// user chose a different theme in this editing session (savedName differs
// from the name the reload carried), in which case their choice, already
// resolved by the save, stands.
func (m *Model) applyHeldTheme(held *config.Settings, savedName string) {
	if held == nil || held.File.Theme != savedName {
		return
	}
	m.retakeTheme(*held)
}
