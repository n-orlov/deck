package config

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"
)

const reloadTheme = `name = "midnight"
appearance = "dark"
`

// reloadFixture is a DECK_HOME with a config.toml, a Reloader over it whose
// load counts how many times it re-parsed, and the paths a test edits.
type reloadFixture struct {
	t         *testing.T
	home      string
	config    string
	themesDir string
	loads     int
	reloader  *Reloader
}

func newReloadFixture(t *testing.T) *reloadFixture {
	t.Helper()
	home := t.TempDir()
	f := &reloadFixture{t: t, home: home, config: filepath.Join(home, "config.toml"), themesDir: filepath.Join(home, "themes")}
	f.write(f.config, "[ui]\nsort_order = \"name\"\n")
	getenv := environment(map[string]string{"DECK_HOME": home})
	settings, err := LoadFrom(getenv, fakeHome)
	if err != nil {
		t.Fatal(err)
	}
	f.reloader = NewReloader(settings, func() (Settings, error) {
		f.loads++
		return LoadFrom(getenv, fakeHome)
	})
	return f
}

// write replaces path's bytes and moves its mtime forward, so an edit is
// visible to a fingerprint whatever the filesystem's timestamp granularity.
func (f *reloadFixture) write(path, body string) {
	f.t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		f.t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		f.t.Fatal(err)
	}
	f.bump(path)
}

var reloadClock = time.Now().Add(time.Hour)

func (f *reloadFixture) bump(path string) {
	f.t.Helper()
	reloadClock = reloadClock.Add(time.Second)
	if err := os.Chtimes(path, reloadClock, reloadClock); err != nil {
		f.t.Fatal(err)
	}
}

// poll runs one Poll and returns the load count it added.
func (f *reloadFixture) poll() (loadsAdded int, settings Settings, changed bool) {
	f.t.Helper()
	before := f.loads
	settings, changed, err := f.reloader.Poll()
	if err != nil {
		f.t.Fatal(err)
	}
	return f.loads - before, settings, changed
}

func TestReloaderUnchangedFingerprintParsesZeroTimesAcrossPolls(t *testing.T) {
	f := newReloadFixture(t)
	for i := 1; i <= 5; i++ {
		if added, _, changed := f.poll(); added != 0 || changed {
			t.Fatalf("poll %d on an unchanged fingerprint: loads=%d changed=%v, want 0/false", i, added, changed)
		}
	}
	if f.loads != 0 {
		t.Fatalf("config.toml parsed %d times across 5 unchanged polls, want 0", f.loads)
	}
}

func TestReloaderConfigChangeParsesExactlyOnce(t *testing.T) {
	f := newReloadFixture(t)
	f.write(f.config, "[ui]\nsort_order = \"created\"\n")
	added, settings, changed := f.poll()
	if added != 1 || !changed {
		t.Fatalf("poll after a config.toml edit: loads=%d changed=%v, want exactly 1/true", added, changed)
	}
	if settings.File.SortOrder != "created" {
		t.Fatalf("reloaded sort_order = %q, want the edited value created", settings.File.SortOrder)
	}
	for i := 0; i < 3; i++ {
		if added, _, _ := f.poll(); added != 0 {
			t.Fatalf("poll %d after the reload re-parsed %d times, want 0", i, added)
		}
	}
}

func TestReloaderSizeAloneChangesTheFingerprint(t *testing.T) {
	f := newReloadFixture(t)
	info, err := os.Stat(f.config)
	if err != nil {
		t.Fatal(err)
	}
	// Same mtime, different size: the size half of the fingerprint.
	if err := os.WriteFile(f.config, []byte("[ui]\nsort_order = \"activity\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(f.config, info.ModTime(), info.ModTime()); err != nil {
		t.Fatal(err)
	}
	if added, _, changed := f.poll(); added != 1 || !changed {
		t.Fatalf("size-only edit: loads=%d changed=%v, want 1/true", added, changed)
	}
}

func TestReloaderThemeFileAddEditRemoveEachChangeTheFingerprint(t *testing.T) {
	f := newReloadFixture(t)
	theme := filepath.Join(f.themesDir, "midnight.toml")

	f.write(theme, reloadTheme)
	if added, _, changed := f.poll(); added != 1 || !changed {
		t.Fatalf("adding a theme file: loads=%d changed=%v, want 1/true", added, changed)
	}
	if added, _, _ := f.poll(); added != 0 {
		t.Fatalf("poll after the add re-parsed %d times, want 0", added)
	}

	f.write(theme, reloadTheme+"# edited\n")
	if added, _, changed := f.poll(); added != 1 || !changed {
		t.Fatalf("editing a theme file: loads=%d changed=%v, want 1/true", added, changed)
	}

	f.bump(theme) // mtime alone
	if added, _, changed := f.poll(); added != 1 || !changed {
		t.Fatalf("touching a theme file's mtime: loads=%d changed=%v, want 1/true", added, changed)
	}

	if err := os.Remove(theme); err != nil {
		t.Fatal(err)
	}
	if added, _, changed := f.poll(); added != 1 || !changed {
		t.Fatalf("removing a theme file: loads=%d changed=%v, want 1/true", added, changed)
	}
	if added, _, _ := f.poll(); added != 0 {
		t.Fatalf("poll after the remove re-parsed %d times, want 0", added)
	}
}

func TestComputeFingerprintCoversNamesMtimesAndSizes(t *testing.T) {
	f := newReloadFixture(t)
	base := ComputeFingerprint(f.config, f.themesDir)
	if ComputeFingerprint(f.config, f.themesDir) != base {
		t.Fatal("fingerprint of untouched files is not stable")
	}
	a := filepath.Join(f.themesDir, "a.toml")
	f.write(a, reloadTheme)
	withA := ComputeFingerprint(f.config, f.themesDir)
	if withA == base {
		t.Fatal("adding a theme entry did not change the fingerprint")
	}
	if err := os.Rename(a, filepath.Join(f.themesDir, "b.toml")); err != nil {
		t.Fatal(err)
	}
	if ComputeFingerprint(f.config, f.themesDir) == withA {
		t.Fatal("renaming a theme entry (same mtime and size) did not change the fingerprint")
	}
	if err := os.Remove(f.config); err != nil {
		t.Fatal(err)
	}
	if ComputeFingerprint(f.config, f.themesDir) == withA {
		t.Fatal("removing config.toml did not change the fingerprint")
	}
}

func TestReloaderPollNeverWritesConfigToml(t *testing.T) {
	f := newReloadFixture(t)
	f.write(f.config, "[ui]\nsort_order = \"activity\"\n")
	beforeBytes, err := os.ReadFile(f.config)
	if err != nil {
		t.Fatal(err)
	}
	beforeInfo, err := os.Stat(f.config)
	if err != nil {
		t.Fatal(err)
	}
	if added, _, changed := f.poll(); added != 1 || !changed {
		t.Fatalf("setup: the poll must reload (loads=%d changed=%v)", added, changed)
	}
	afterBytes, err := os.ReadFile(f.config)
	if err != nil {
		t.Fatal(err)
	}
	afterInfo, err := os.Stat(f.config)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(beforeBytes, afterBytes) || !afterInfo.ModTime().Equal(beforeInfo.ModTime()) {
		t.Fatalf("a reloading poll changed config.toml: bytes equal=%v mtime %v -> %v", bytes.Equal(beforeBytes, afterBytes), beforeInfo.ModTime(), afterInfo.ModTime())
	}
	themes, err := os.ReadDir(f.themesDir)
	if err == nil && len(themes) != 0 {
		t.Fatalf("a poll created theme entries: %v", themes)
	}
}

func TestReloaderRecordsAnInvalidFileOnceAndPicksUpTheNextWrite(t *testing.T) {
	f := newReloadFixture(t)
	f.write(f.config, "[ui\nthis is not toml")
	if _, changed, err := f.reloader.Poll(); !changed || err == nil {
		t.Fatalf("invalid file: changed=%v err=%v, want true and an error", changed, err)
	}
	if _, changed, _ := f.reloader.Poll(); changed {
		t.Fatal("an unchanged invalid file was re-parsed on the next poll")
	}
	f.write(f.config, "[ui]\nsort_order = \"name\"\n")
	if added, _, changed := f.poll(); added != 1 || !changed {
		t.Fatalf("the next valid write: loads=%d changed=%v, want 1/true", added, changed)
	}
}

func TestConfigPollIntervalDefaultIsAtMostThirtySecondsAndEnvOverrides(t *testing.T) {
	settings, err := LoadFrom(environment(map[string]string{"DECK_HOME": t.TempDir()}), fakeHome)
	if err != nil {
		t.Fatal(err)
	}
	if settings.ConfigPoll <= 0 || settings.ConfigPoll > 30*time.Second {
		t.Fatalf("production ConfigPoll = %v, want in (0, 30s]", settings.ConfigPoll)
	}
	settings, err = LoadFrom(environment(map[string]string{"DECK_HOME": t.TempDir(), "DECK_CONFIG_POLL_MS": "50"}), fakeHome)
	if err != nil {
		t.Fatal(err)
	}
	if settings.ConfigPoll != 50*time.Millisecond {
		t.Fatalf("DECK_CONFIG_POLL_MS=50 -> %v, want 50ms", settings.ConfigPoll)
	}
	if _, err = LoadFrom(environment(map[string]string{"DECK_HOME": t.TempDir(), "DECK_CONFIG_POLL_MS": "soon"}), fakeHome); err == nil {
		t.Fatal("DECK_CONFIG_POLL_MS=soon must be rejected like every other interval variable")
	}
}

func TestReloaderMarkOwnWriteSwallowsOnlyTheConfigWrite(t *testing.T) {
	f := newReloadFixture(t)
	f.write(f.config, "[ui]\nsort_order = \"created\"\n")
	f.reloader.MarkOwnWrite()
	if loads, _, changed := f.poll(); changed || loads != 0 {
		t.Fatalf("an own config.toml write was reported back (changed=%v, loads=%d)", changed, loads)
	}
	f.write(f.config, "[ui]\nsort_order = \"activity\"\n")
	if loads, _, changed := f.poll(); !changed || loads != 1 {
		t.Fatalf("a later external write was not seen (changed=%v, loads=%d)", changed, loads)
	}
	f.write(filepath.Join(f.themesDir, "mine.toml"), "name = \"mine\"\n")
	f.write(f.config, "[ui]\nsort_order = \"name\"\n")
	f.reloader.MarkOwnWrite()
	if loads, _, changed := f.poll(); !changed || loads != 1 {
		t.Fatalf("a theme file added before an own write was swallowed (changed=%v, loads=%d)", changed, loads)
	}
}
