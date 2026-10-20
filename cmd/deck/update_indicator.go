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
	// No Enabled gate here: the model reads the current effective update_check
	// and auto_update each time it asks for a check (a live save or an
	// external reload changes them), so a copy of the startup settings would
	// keep a check off after the user turns it on.
	checker := &update.Checker{
		CachePath: settings.Paths.UpdateCache,
		Options:   opts,
	}
	return checker.Check, func() error { return update.Preflight(opts) }
}

// updateInstallSource is what ctrl+u runs once confirmed: the running build's
// version and the same verified update `deck update` performs.
func updateInstallSource(settings config.Settings) (string, func(context.Context) (update.Result, error)) {
	opts := updateInstallOptions(settings)
	return version, func(ctx context.Context) (update.Result, error) { return update.Run(ctx, opts) }
}

// updateInstallOptions is what ctrl+u and the auto-update hand update.Run: the
// install lock sits beside the shared update cache.
func updateInstallOptions(settings config.Settings) update.Options {
	return update.Options{Current: version, OnlyIfNewer: true, InstallLock: update.InstallLockPath(settings.Paths.UpdateCache)}
}
