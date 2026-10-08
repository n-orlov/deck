package tui

import (
	"context"
	"slices"
	"strings"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
)

// This file holds what the launch-inputs editor and the create dialog share
// for a session's own event-hook controls (SPEC §10.2, R233a):
// event_hook_enabled (inherit / on / off) and event_hook_events (an optional
// list of offered kinds). Both are read at dispatch, so neither is a launch
// input: editing them on a running session applies at once and sets neither
// env_dirty nor launch_dirty.

// eventHookMode is event_hook_enabled's tri-state as a cycled selection.
type eventHookMode int

const (
	eventHookInherit eventHookMode = iota
	eventHookOn
	eventHookOff
)

// eventHookModeCount is how many values the selection cycles through.
const eventHookModeCount = 3

// eventKindsNone is the word that stands for the empty list in the kinds
// field, where an empty field already means "inherit the global list".
const eventKindsNone = "none"

// eventHookModeName is a mode's drawn value.
func eventHookModeName(mode eventHookMode) string {
	switch mode {
	case eventHookOn:
		return "on"
	case eventHookOff:
		return "off"
	default:
		return "inherit"
	}
}

// cycleEventHookMode steps mode by delta through inherit, on, off, wrapping.
func cycleEventHookMode(mode eventHookMode, delta int) eventHookMode {
	return eventHookMode((int(mode) + delta + eventHookModeCount) % eventHookModeCount)
}

// eventHookModeOf maps a stored tri-state onto the selection.
func eventHookModeOf(enabled *bool) eventHookMode {
	switch {
	case enabled == nil:
		return eventHookInherit
	case *enabled:
		return eventHookOn
	default:
		return eventHookOff
	}
}

// enabled is the stored tri-state a mode stands for: nil inherits.
func (mode eventHookMode) enabled() *bool {
	if mode == eventHookInherit {
		return nil
	}
	on := mode == eventHookOn
	return &on
}

// eventKindsText renders a session's own list as the text its field holds:
// nil (inherit) is empty, the empty list is "none", otherwise comma-separated.
func eventKindsText(events []string) string {
	if events == nil {
		return ""
	}
	if len(events) == 0 {
		return eventKindsNone
	}
	return settingsEventKindsText(events)
}

// parseEventKinds is eventKindsText's inverse. Empty text inherits the global
// list (nil), "none" is the empty list, and every other word must be in the
// offered set of SPEC §10.1.
func parseEventKinds(text string) ([]string, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, nil
	}
	if text == eventKindsNone {
		return []string{}, nil
	}
	kinds := settingsParseEventKinds(text)
	if len(kinds) == 0 {
		return nil, nil
	}
	if err := config.CheckEventKinds(kinds); err != nil {
		return nil, err
	}
	return kinds, nil
}

// sameEventHook reports whether a session already holds exactly the given
// controls, nil and the empty list being different values.
func sameEventHook(session store.Session, enabled *bool, events []string) bool {
	if eventHookModeOf(session.EventHookEnabled) != eventHookModeOf(enabled) {
		return false
	}
	if (session.EventHookEvents == nil) != (events == nil) {
		return false
	}
	return slices.Equal(session.EventHookEvents, events)
}

// WithEventHookSetter attaches the action that stores a session's own
// event-hook controls (service.Service.SetEventHook). It sets no dirty flag.
// A Model without it reports the launch-inputs editor's event-hook edit as
// unavailable instead of dropping it.
func (m Model) WithEventHookSetter(setter func(context.Context, string, *bool, []string) (store.Session, error)) Model {
	m.eventHookSetter = setter
	return m
}
