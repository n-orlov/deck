package tui

import (
	"os"
	"reflect"
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

// onConfigPolled records a reload's result, applies every live key of a
// good one to the running model (applyReloadedSettings, SPEC §6.5) and
// schedules the next poll. An unchanged poll leaves the model untouched; a
// reload that failed to parse keeps the running settings as they were.
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
	return m, tea.Batch(m.applyReloadedSettings(reloaded), tick)
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
	if next.Theme != nil && !reflect.DeepEqual(next.Theme, m.settings.Theme) {
		m.settings.Theme = next.Theme
		m.settings.ThemeReason = next.ThemeReason
	}
	m.settings.File = settingsCloneFileConfig(next.File)
	return cmd
}
