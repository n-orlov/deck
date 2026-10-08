package tui

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/n-orlov/deck/internal/config"
)

// pollHarness is a Model whose config poller watches a temp config.toml and
// themes dir, with a load function that counts every re-parse.
type pollHarness struct {
	t         *testing.T
	m         Model
	config    string
	themesDir string
	loads     int
	clock     time.Time
}

func newPollHarness(t *testing.T) *pollHarness {
	t.Helper()
	dir := t.TempDir()
	h := &pollHarness{t: t, config: filepath.Join(dir, "config.toml"), themesDir: filepath.Join(dir, "themes"), clock: time.Now().Add(time.Hour)}
	h.write(h.config, "[ui]\nsort_order = \"name\"\n")
	settings := config.Settings{ConfigPoll: 30 * time.Second}
	settings.Paths.ConfigFile = h.config
	settings.ThemesDir = h.themesDir
	h.m = New(nil, settings, "")
	h.m.configReloader = config.NewReloader(settings, func() (config.Settings, error) {
		h.loads++
		reloaded := settings
		reloaded.ASCII = true
		return reloaded, nil
	})
	return h
}

func (h *pollHarness) write(path, body string) {
	h.t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		h.t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		h.t.Fatal(err)
	}
	h.clock = h.clock.Add(time.Second)
	if err := os.Chtimes(path, h.clock, h.clock); err != nil {
		h.t.Fatal(err)
	}
}

// poll delivers one configPollTick the way bubbletea would: the tick's
// command runs, its configPolled result is fed back, and the model must
// answer by scheduling the next tick. It returns how many parses it caused.
func (h *pollHarness) poll() int {
	h.t.Helper()
	before := h.loads
	next, cmd := h.m.Update(configPollTick(time.Now()))
	h.m = next.(Model)
	if cmd == nil {
		h.t.Fatal("configPollTick returned no command")
	}
	polled, ok := cmd().(configPolled)
	if !ok {
		h.t.Fatal("the poll command did not produce a configPolled message")
	}
	next, cmd = h.m.Update(polled)
	h.m = next.(Model)
	if cmd == nil {
		h.t.Fatal("configPolled did not schedule the next poll tick")
	}
	return h.loads - before
}

func TestConfigPollUnchangedFingerprintParsesZeroTimesAcrossPolls(t *testing.T) {
	h := newPollHarness(t)
	for i := 1; i <= 4; i++ {
		if got := h.poll(); got != 0 {
			t.Fatalf("poll %d with an unchanged fingerprint parsed %d times, want 0", i, got)
		}
	}
	if h.m.reloaded != nil {
		t.Fatal("an unchanged fingerprint recorded a reload")
	}
}

func TestConfigPollConfigChangeParsesExactlyOnce(t *testing.T) {
	h := newPollHarness(t)
	h.poll()
	h.write(h.config, "[ui]\nsort_order = \"created\"\n")
	if got := h.poll(); got != 1 {
		t.Fatalf("poll after a config.toml change parsed %d times, want exactly 1", got)
	}
	if h.m.reloaded == nil || !h.m.reloaded.ASCII {
		t.Fatalf("the reloaded settings were not recorded: %+v", h.m.reloaded)
	}
	for i := 0; i < 3; i++ {
		if got := h.poll(); got != 0 {
			t.Fatalf("poll %d after the reload parsed %d times, want 0", i, got)
		}
	}
}

func TestConfigPollThemeFileAddEditRemoveEachReload(t *testing.T) {
	h := newPollHarness(t)
	theme := filepath.Join(h.themesDir, "midnight.toml")
	steps := []struct {
		name string
		do   func()
	}{
		{"add", func() { h.write(theme, "name = \"midnight\"\n") }},
		{"edit", func() { h.write(theme, "name = \"midnight\"\nappearance = \"dark\"\n") }},
		{"remove", func() {
			if err := os.Remove(theme); err != nil {
				t.Fatal(err)
			}
		}},
	}
	for _, step := range steps {
		step.do()
		if got := h.poll(); got != 1 {
			t.Fatalf("poll after a theme file %s parsed %d times, want exactly 1", step.name, got)
		}
		if got := h.poll(); got != 0 {
			t.Fatalf("second poll after a theme file %s parsed %d times, want 0", step.name, got)
		}
	}
}

func TestConfigPollReloadNeverWritesConfigToml(t *testing.T) {
	h := newPollHarness(t)
	h.write(h.config, "[ui]\nsort_order = \"activity\"\n")
	beforeBytes, _ := os.ReadFile(h.config)
	beforeInfo, err := os.Stat(h.config)
	if err != nil {
		t.Fatal(err)
	}
	if got := h.poll(); got != 1 {
		t.Fatalf("setup: the poll must reload, parsed %d times", got)
	}
	afterBytes, _ := os.ReadFile(h.config)
	afterInfo, err := os.Stat(h.config)
	if err != nil {
		t.Fatal(err)
	}
	if string(beforeBytes) != string(afterBytes) || !afterInfo.ModTime().Equal(beforeInfo.ModTime()) {
		t.Fatalf("a reloading poll rewrote config.toml (mtime %v -> %v)", beforeInfo.ModTime(), afterInfo.ModTime())
	}
}

func TestConfigPollReloaderExistsOnlyWhenAnIntervalIsConfigured(t *testing.T) {
	settings := config.Settings{ConfigPoll: 30 * time.Second}
	m := New(nil, settings, "")
	if m.configReloader == nil {
		t.Fatal("a Model with ConfigPoll set has no config reloader")
	}
	if m.settings.ConfigPoll > 30*time.Second {
		t.Fatalf("poll interval %v exceeds the 30 s bound", m.settings.ConfigPoll)
	}
	if New(nil, config.Settings{}, "").configReloader != nil {
		t.Fatal("a Model with no ConfigPoll must not poll")
	}
	if m.configPollTickCmd() == nil {
		t.Fatal("no poll tick command")
	}
}
