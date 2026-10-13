package tui

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	upd "github.com/n-orlov/deck/internal/update"
)

// R297, R302 / SPEC §6.5: the daily check and the automatic install read the
// CURRENT effective update_check and auto_update, not a copy captured at
// startup. These tests wire the model the way cmd/deck does (a Checker with no
// startup-bound gate, the real Preflight and verified installer) against a
// local fake release server, and change the settings through the real
// settings save and the real external reload.

type liveUpdateRig struct {
	t   *testing.T
	h   *reloadHarness
	srv *autoServer
	exe string
}

func newLiveUpdateRig(t *testing.T, config string) *liveUpdateRig {
	t.Helper()
	srv := newAutoServer(t, false)
	exe, _ := autoBinary(t)
	t.Setenv("DECK_UPDATE_URL", srv.URL)
	t.Setenv("PATH", t.TempDir()) // no gh: checksum-verified only
	t.Setenv("DECK_UPDATE_CHECK", "")
	t.Setenv("DECK_AUTO_UPDATE", "")
	_ = os.Unsetenv("DECK_UPDATE_CHECK")
	_ = os.Unsetenv("DECK_AUTO_UPDATE")
	h := newReloadHarness(t, config)
	opts := upd.Options{Exe: exe, Current: ctrlUCurrent}
	checker := &upd.Checker{CachePath: filepath.Join(t.TempDir(), "update-check.json"), Options: opts}
	install := opts
	install.OnlyIfNewer = true
	h.m = h.m.WithUpdateSource(checker.Check, func() error { return upd.Preflight(opts) })
	h.m = h.m.WithUpdateInstaller(ctrlUCurrent, func(ctx context.Context) (upd.Result, error) { return upd.Run(ctx, install) })
	return &liveUpdateRig{t: t, h: h, srv: srv, exe: exe}
}

// pump runs cmd (flattening batches) and feeds every update message it yields
// back into the model, repeating until nothing is left.
func (r *liveUpdateRig) pump(cmd tea.Cmd) {
	r.t.Helper()
	pending := []tea.Cmd{cmd}
	for steps := 0; len(pending) > 0; steps++ {
		if steps > 12 {
			r.t.Fatal("the model kept returning commands")
		}
		c := pending[0]
		pending = pending[1:]
		if c == nil {
			continue
		}
		switch msg := c().(type) {
		case tea.BatchMsg:
			pending = append(pending, msg...)
		case nil:
		default:
			next, more := r.h.m.Update(msg)
			r.h.m = next.(Model)
			pending = append(pending, more)
		}
	}
}

// saveToggle opens the settings takeover, flips the named toggle and saves.
func (r *liveUpdateRig) saveToggle(field string) {
	r.t.Helper()
	next, _ := r.h.m.Update(key(","))
	m := next.(Model)
	m = settingsFocusFieldByKey(r.t, m, field)
	next, _ = m.Update(key("enter"))
	next, cmd := next.(Model).Update(key("ctrl+s"))
	r.h.m = next.(Model)
	next, _ = r.h.m.Update(key("esc"))
	r.h.m = next.(Model)
	r.pump(cmd)
}

// reloadFile rewrites config.toml and applies it as the poller would.
func (r *liveUpdateRig) reloadFile(body string) {
	r.t.Helper()
	r.h.writeConfig(body)
	next, cmd := r.h.m.Update(configPollTick(r.h.clock))
	r.h.m = next.(Model)
	polled, ok := cmd().(configPolled)
	if !ok || polled.err != nil {
		r.t.Fatalf("poll = %+v", polled)
	}
	r.pump(r.h.m.applyReloadedSettings(polled.settings))
}

func (r *liveUpdateRig) startup() {
	r.t.Helper()
	r.pump(r.h.m.updateCheckCmd())
}

func (r *liveUpdateRig) assertRequests(want int32) {
	r.t.Helper()
	if got := r.srv.checks.Load(); got != want {
		r.t.Fatalf("release-check requests = %d, want %d", got, want)
	}
}

func (r *liveUpdateRig) binary() string {
	b, _ := os.ReadFile(r.exe)
	return string(b)
}

func TestLiveSaveEnablingUpdateCheckChecksWithoutRestart(t *testing.T) {
	r := newLiveUpdateRig(t, "update_check = false\n")
	r.startup()
	r.assertRequests(0)
	r.saveToggle("update_check")
	r.assertRequests(1)
	if got := r.h.m.footerNote(); got != indicatorAvailble {
		t.Fatalf("after enabling update_check: note = %q, want the offer", got)
	}
	if r.binary() != ctrlUOldBody {
		t.Fatal("update_check alone installed a release")
	}
}

func TestLiveSaveDisablingUpdateCheckStopsRequests(t *testing.T) {
	r := newLiveUpdateRig(t, "update_check = true\n")
	r.startup()
	r.assertRequests(1)
	r.saveToggle("update_check")
	if r.h.m.settings.UpdateCheckEnabled() {
		t.Fatal("setup: the save did not disable the check")
	}
	if cmd := r.h.m.updateCheckCmd(); cmd != nil {
		r.pump(cmd)
	}
	r.assertRequests(1)
	if got := r.h.m.footerNote(); got != "" {
		t.Fatalf("with the check off the indicator shows %q", got)
	}
}

func TestLiveSaveEnablingAutoUpdateInstallsWithoutRestart(t *testing.T) {
	r := newLiveUpdateRig(t, "update_check = false\nauto_update = false\n")
	r.startup()
	r.assertRequests(0)
	r.saveToggle("auto_update")
	if got := r.h.m.footerNote(); got != "update installed: v1.2.3 — restart deck to apply" {
		t.Fatalf("after enabling auto_update: note = %q", got)
	}
	if r.binary() != ctrlUNewBody || r.srv.archives.Load() != 1 {
		t.Fatalf("auto_update saved live did not install (archives %d)", r.srv.archives.Load())
	}
}

func TestLiveSaveAutoUpdateOnAnOfferInstallsIt(t *testing.T) {
	// The offer is already up (update_check on); turning auto_update on later
	// installs that release (the install resolves the release itself).
	r := newLiveUpdateRig(t, "update_check = true\n")
	r.startup()
	r.assertRequests(1)
	r.saveToggle("auto_update")
	if r.binary() != ctrlUNewBody {
		t.Fatal("enabling auto_update left the offered release uninstalled")
	}
}

func TestLiveSaveDisablingAutoUpdateStopsAutomaticInstalls(t *testing.T) {
	r := newLiveUpdateRig(t, "update_check = false\nauto_update = true\n")
	r.saveToggle("auto_update") // now off, and update_check is off too
	if r.h.m.settings.AutoUpdate || r.h.m.settings.UpdateCheckEnabled() {
		t.Fatal("setup: the save did not turn both off")
	}
	r.startup()
	// An answer already in flight when the setting went off must not install.
	next, cmd := r.h.m.Update(updateChecked{status: upd.Status{Latest: indicatorTag, Newer: true}})
	r.h.m = next.(Model)
	r.pump(cmd)
	r.assertRequests(0)
	if r.srv.archives.Load() != 0 || r.binary() != ctrlUOldBody {
		t.Fatal("an install ran after auto_update was switched off")
	}
}

func TestLiveReloadEnablingUpdateCheckChecksWithoutRestart(t *testing.T) {
	r := newLiveUpdateRig(t, "update_check = false\n")
	r.startup()
	r.assertRequests(0)
	r.reloadFile("update_check = true\n")
	r.assertRequests(1)
	if got := r.h.m.footerNote(); got != indicatorAvailble {
		t.Fatalf("after the reload: note = %q, want the offer", got)
	}
}

func TestLiveReloadDisablingUpdateCheckStopsRequests(t *testing.T) {
	r := newLiveUpdateRig(t, "update_check = true\n")
	r.reloadFile("update_check = false\n")
	r.startup()
	r.assertRequests(0)
}

func TestLiveReloadEnablingAutoUpdateInstalls(t *testing.T) {
	r := newLiveUpdateRig(t, "update_check = false\n")
	r.startup()
	r.reloadFile("update_check = false\nauto_update = true\n")
	if r.binary() != ctrlUNewBody || r.srv.archives.Load() != 1 {
		t.Fatalf("the reload enabling auto_update did not install (archives %d)", r.srv.archives.Load())
	}
}

func TestLiveReloadDisablingAutoUpdateStopsInstalls(t *testing.T) {
	r := newLiveUpdateRig(t, "update_check = false\nauto_update = true\n")
	r.reloadFile("update_check = false\nauto_update = false\n")
	r.startup()
	r.assertRequests(0)
	if r.srv.archives.Load() != 0 {
		t.Fatal("a download started after auto_update was reloaded off")
	}
}

func TestLiveEnvOverridePinsUpdateCheckAgainstASave(t *testing.T) {
	t.Setenv("DECK_UPDATE_CHECK", "false")
	srv := newAutoServer(t, false)
	t.Setenv("DECK_UPDATE_URL", srv.URL)
	h := newReloadHarness(t, "update_check = false\n")
	exe, _ := autoBinary(t)
	opts := upd.Options{Exe: exe, Current: ctrlUCurrent}
	checker := &upd.Checker{CachePath: filepath.Join(t.TempDir(), "c.json"), Options: opts}
	h.m = h.m.WithUpdateSource(checker.Check, func() error { return upd.Preflight(opts) })
	r := &liveUpdateRig{t: t, h: h, srv: srv, exe: exe}
	r.saveToggle("update_check")
	r.assertRequests(0)
	if r.h.m.settings.UpdateCheckEnabled() {
		t.Fatal("a save overrode DECK_UPDATE_CHECK in the running client")
	}
}
