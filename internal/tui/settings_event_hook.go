package tui

import (
	"slices"
	"strings"

	"github.com/n-orlov/deck/internal/config"
)

// settingsEventKindsText renders the offered-kinds list as the
// comma-separated text the settings dialog edits (R229, SPEC §11.5).
func settingsEventKindsText(kinds []string) string {
	return strings.Join(kinds, ", ")
}

// settingsParseEventKinds is settingsEventKindsText's inverse: it splits
// text on commas, trims each word and drops empty ones, so "waiting, error,"
// and "waiting,error" both stage ["waiting", "error"] and an emptied field
// stages the empty list.
func settingsParseEventKinds(text string) []string {
	kinds := []string{}
	for _, word := range strings.Split(text, ",") {
		if word = strings.TrimSpace(word); word != "" {
			kinds = append(kinds, word)
		}
	}
	return kinds
}

// settingsApplyLiveEventHook refreshes the running event-hook settings
// (R229, SPEC §10) for every key a save or a config reload changed. All four
// are ScopeGlobal: the dispatcher reads them from m.settings when an event
// fires, so the next event already follows them. None has a DECK_* override.
func (m *Model) settingsApplyLiveEventHook(edited, previous config.FileConfig) {
	if edited.EventHook != previous.EventHook {
		m.settings.EventHook = edited.EventHook
	}
	if edited.EventHookDefault != previous.EventHookDefault {
		m.settings.EventHookDefault = edited.EventHookDefault
	}
	if !slices.Equal(edited.EventHookEvents, previous.EventHookEvents) {
		m.settings.EventHookEvents = slices.Clone(edited.EventHookEvents)
	}
	if edited.EventHookTimeout != previous.EventHookTimeout {
		m.settings.EventHookTimeout = edited.EventHookTimeout
	}
}
