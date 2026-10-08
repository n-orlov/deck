package tui

import (
	"os"
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

// onConfigPolled records a reload's result and schedules the next poll.
// Applying the reloaded keys live is R226b and the safety rules R227; this
// is the detection half, so an unchanged poll leaves the model untouched.
func (m Model) onConfigPolled(msg configPolled) (tea.Model, tea.Cmd) {
	if msg.changed {
		m.reloadErr = msg.err
		if msg.err == nil {
			reloaded := msg.settings
			m.reloaded = &reloaded
		}
	}
	return m, m.configPollTickCmd()
}
