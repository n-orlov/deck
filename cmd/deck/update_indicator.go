package main

import (
	"context"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/update"
)

// updateIndicatorSource builds what the TUI's update indicator reads: the
// throttled daily check (cache shared by every profile, SPEC §4) and the
// preflight that says whether this install can be updated. With update_check
// and auto_update both off the check makes no request.
func updateIndicatorSource(settings config.Settings) (func(context.Context) update.Status, func() error) {
	opts := update.Options{Current: version}
	checker := &update.Checker{
		Enabled:   settings.UpdateCheckEnabled,
		CachePath: settings.Paths.UpdateCache,
		Options:   opts,
	}
	return checker.Check, func() error { return update.Preflight(opts) }
}
