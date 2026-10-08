package tui

import (
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
)

// reloadHarness is a Model built from a real config.toml in a temp DECK_HOME
// whose poller re-reads that same directory through the production loader,
// so a test edits the files and then delivers exactly the messages
// bubbletea would.
type reloadHarness struct {
	t     *testing.T
	m     Model
	dir   string
	clock time.Time
}

func newReloadHarness(t *testing.T, initial string) *reloadHarness {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("DECK_HOME", dir)
	t.Setenv("HOME", dir)
	for _, name := range []string{"DECK_ASCII", "DECK_MOUSE", "DECK_PREVIEW_FIT", "DECK_PREVIEW_PAINT", "DECK_PROFILE", "DECK_CONFIG_POLL_MS"} {
		t.Setenv(name, "")
		if err := os.Unsetenv(name); err != nil {
			t.Fatal(err)
		}
	}
	h := &reloadHarness{t: t, dir: dir, clock: time.Now().Add(time.Hour)}
	h.write(filepath.Join(dir, "config.toml"), initial)
	settings, err := config.LoadFrom(os.Getenv, os.UserHomeDir)
	if err != nil {
		t.Fatalf("seed load: %v", err)
	}
	settings.Color = true
	h.m = New(nil, settings, "")
	h.m.width, h.m.height = 120, 30
	if h.m.configReloader == nil {
		t.Fatal("the model built no config poller")
	}
	return h
}

func (h *reloadHarness) write(path, body string) {
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

func (h *reloadHarness) writeConfig(body string) {
	h.t.Helper()
	h.write(filepath.Join(h.dir, "config.toml"), body)
}

// reload delivers one poll the way the program does: the tick, the poll
// command's result, then the model's answer (which applies the reload).
func (h *reloadHarness) reload() {
	h.t.Helper()
	next, cmd := h.m.Update(configPollTick(time.Now()))
	h.m = next.(Model)
	polled, ok := cmd().(configPolled)
	if !ok {
		h.t.Fatal("the poll command did not produce a configPolled message")
	}
	if polled.err != nil {
		h.t.Fatalf("reload failed: %v", polled.err)
	}
	next, _ = h.m.Update(polled)
	h.m = next.(Model)
}

func TestReloadAppliesUiThemeLive(t *testing.T) {
	h := newReloadHarness(t, "[ui]\ntheme = \"empire\"\n")
	before := h.m.settings.Theme.Name
	h.writeConfig("[ui]\ntheme = \"matrix\"\n")
	h.reload()
	if got := h.m.settings.Theme.Name; got != "matrix" || got == before {
		t.Fatalf("active theme = %q after one reload, want matrix (was %q)", got, before)
	}
}

func TestReloadAppliesUiASCIILive(t *testing.T) {
	h := newReloadHarness(t, "[ui]\nascii = false\n")
	if h.m.settings.ASCII {
		t.Fatal("setup: ascii starts on")
	}
	viewBefore := h.m.View()
	h.writeConfig("[ui]\nascii = true\n")
	h.reload()
	if !h.m.settings.ASCII {
		t.Fatal("ui.ascii did not apply after one reload")
	}
	if h.m.View() == viewBefore {
		t.Fatal("the rendered frame did not change after ui.ascii was reloaded")
	}
}

func TestReloadAppliesUiMouseLive(t *testing.T) {
	h := newReloadHarness(t, "[ui]\nmouse = true\n")
	h.writeConfig("[ui]\nmouse = false\n")
	h.reload()
	if h.m.settings.Mouse {
		t.Fatal("ui.mouse did not apply after one reload")
	}
	next, err := config.LoadFrom(os.Getenv, os.UserHomeDir)
	if err != nil {
		t.Fatal(err)
	}
	next.File.Mouse = true
	cmd := h.m.applyReloadedSettings(next)
	if !h.m.settings.Mouse {
		t.Fatal("ui.mouse did not turn back on")
	}
	if cmd == nil {
		t.Fatal("turning the mouse on returned no terminal command")
	}
	if got := reflect.TypeOf(cmd()); got != reflect.TypeOf(tea.EnableMouseCellMotion()) {
		t.Fatalf("the mouse command produced %v, want the enable-mouse message", got)
	}
}

func TestReloadAppliesUiSortOrderLive(t *testing.T) {
	h := newReloadHarness(t, "[ui]\nsort_order = \"attention\"\n")
	var sessions []store.Session
	for i, name := range []string{"bravo", "alpha", "charlie"} {
		sessions = append(sessions, store.Session{ID: "s" + name, Name: name, Agent: "shell", Status: "idle", CreatedAt: int64(i), StatusAt: int64(i)})
	}
	next, _ := h.m.Update(sessionsLoaded{sessions: sessions})
	h.m = next.(Model)
	h.writeConfig("[ui]\nsort_order = \"name\"\n")
	h.reload()
	if h.m.settings.SortOrder != SortOrderName {
		t.Fatalf("sort_order = %q after one reload, want %q", h.m.settings.SortOrder, SortOrderName)
	}
	var names []string
	for _, s := range h.m.sessions {
		names = append(names, s.Name)
	}
	if want := []string{"alpha", "bravo", "charlie"}; !reflect.DeepEqual(names, want) {
		t.Fatalf("sessions are %v after the reload, want %v", names, want)
	}
}

func TestReloadUserThemeFileEditChangesActiveTheme(t *testing.T) {
	h := newReloadHarness(t, "[ui]\ntheme = \"midnight\"\n")
	userTheme := func(accent string) string {
		return strings.Replace(builtinEmpireTheme(t), "name = \"empire\"", "name = \"midnight\"", 1) + "\n# " + accent + "\n"
	}
	path := filepath.Join(h.dir, "themes", "midnight.toml")
	h.write(path, userTheme("one"))
	h.reload()
	first := h.m.settings.Theme
	if first == nil || first.Name != "midnight" {
		t.Fatalf("setup: the user theme was not picked up: %+v", first)
	}
	var token string
	var colour string
	for tok, c := range first.Colors {
		token, colour = string(tok), c
		break
	}
	edited := strings.Replace(userTheme("two"), colour, "#123456", 1)
	if edited == userTheme("two") {
		t.Fatalf("setup: colour %s of %s not found in the theme file", colour, token)
	}
	h.write(path, edited)
	h.reload()
	if reflect.DeepEqual(h.m.settings.Theme.Colors, first.Colors) {
		t.Fatal("editing the user theme file's colours left the active theme unchanged after one poll")
	}
}

// builtinEmpireTheme returns the embedded empire theme's source so a user
// theme can be derived from a known-complete one.
func builtinEmpireTheme(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "theme", "builtin", "empire.toml"))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// reloadRestartRequired is the one list of config.toml keys a running TUI
// does NOT re-apply on reload. SPEC §6.5 names exactly these.
var reloadRestartRequired = []string{
	"agent", "capture_min_interval", "event_retention_days", "interactive_ms",
	"interactive_transport", "post_destroy", "pre_launch", "stale_after",
	"tmux_mouse", "ui.recent_cwd_limit", "[env]",
}

// reloadEdits gives, for every schema key, a config.toml body that changes
// that key away from its default.
var reloadEdits = map[string]string{
	"allow_yolo":             "allow_yolo = true\n",
	"yolo_default":           "yolo_default = true\n",
	"stale_after":            "stale_after = 99\n",
	"capture_min_interval":   "capture_min_interval = 9\n",
	"event_retention_days":   "event_retention_days = 7\n",
	"tmux_mouse":             "tmux_mouse = false\n",
	"interactive_ms":         "interactive_ms = 90\n",
	"interactive_transport":  "interactive_transport = \"capture\"\n",
	"ui.theme":               "[ui]\ntheme = \"matrix\"\n",
	"ui.ascii":               "[ui]\nascii = true\n",
	"ui.mouse":               "[ui]\nmouse = false\n",
	"ui.preview_fit":         "[ui]\npreview_fit = false\n",
	"ui.preview_paint":       "[ui]\npreview_paint = \"nofit\"\n",
	"ui.recent_cwd_limit":    "[ui]\nrecent_cwd_limit = 9\n",
	"ui.sort_order":          "[ui]\nsort_order = \"name\"\n",
	"ui.default_group_first": "[ui]\ndefault_group_first = true\n",
	"ui.attach_on_new":       "[ui]\nattach_on_new = false\n",
	"ui.attach_on_click":     "[ui]\nattach_on_click = false\n",
	"ui.select_on_drag":      "[ui]\nselect_on_drag = false\n",
	"ui.attach_on_resume":    "[ui]\nattach_on_resume = true\n",
	"pre_launch":             "pre_launch = \"echo hi\"\n",
	"agent":                  "agent = \"shell\"\n",
	"post_destroy":           "post_destroy = \"echo bye\"\n",
	"[env]":                  "[env]\nDECK_RELOAD_PROBE = \"1\"\n",
}

// reloadGaps returns every schema entry the test has no classification for:
// each entry needs an edit, and is applied live unless it is in
// reloadRestartRequired. A new schema key lands here until classified.
func reloadGaps(schema []config.Field, edits map[string]string) []string {
	var gaps []string
	for _, f := range schema {
		if _, ok := edits[f.FullKey()]; !ok {
			gaps = append(gaps, f.FullKey())
		}
	}
	return gaps
}

func TestReloadClassifiesEverySchemaEntry(t *testing.T) {
	if gaps := reloadGaps(config.Schema, reloadEdits); len(gaps) > 0 {
		t.Fatalf("settings-schema keys neither applied live nor listed restart-required: %v", gaps)
	}
	inSchema := map[string]config.Field{}
	for _, f := range config.Schema {
		inSchema[f.FullKey()] = f
	}
	for _, k := range reloadRestartRequired {
		if _, ok := inSchema[k]; !ok {
			t.Fatalf("restart-required key %q is not in the settings schema", k)
		}
	}
	for k := range reloadEdits {
		if _, ok := inSchema[k]; !ok {
			t.Fatalf("edit for %q: not in the settings schema", k)
		}
	}
}

func TestReloadGapDetectorFlagsAnUnclassifiedEntry(t *testing.T) {
	extra := append(append([]config.Field{}, config.Schema...), config.Field{Key: "brand_new_key", Kind: config.KindToggle, Scope: config.ScopeGlobal})
	gaps := reloadGaps(extra, reloadEdits)
	if !reflect.DeepEqual(gaps, []string{"brand_new_key"}) {
		t.Fatalf("gaps = %v, want exactly [brand_new_key]", gaps)
	}
}

func settingsWithoutFile(s config.Settings) config.Settings {
	s.File = config.FileConfig{}
	return s
}

func TestReloadEverySchemaEntryAppliesLiveOrIsRestartRequired(t *testing.T) {
	restart := map[string]bool{}
	for _, k := range reloadRestartRequired {
		restart[k] = true
	}
	for _, f := range config.Schema {
		key := f.FullKey()
		t.Run(key, func(t *testing.T) {
			edit, ok := reloadEdits[key]
			if !ok {
				t.Fatalf("schema key %q is neither applied live nor restart-required: add it to the test's lists", key)
			}
			h := newReloadHarness(t, "")
			before := settingsWithoutFile(h.m.settings)
			h.writeConfig(edit)
			h.reload()
			after := settingsWithoutFile(h.m.settings)
			if restart[key] {
				if f.Scope != config.ScopeRestartToApply {
					t.Fatalf("%s is listed restart-required but its schema Scope is %q", key, f.Scope)
				}
				if !reflect.DeepEqual(before, after) {
					t.Fatalf("restart-required key %s changed the running settings on reload", key)
				}
				return
			}
			if f.Scope == config.ScopeRestartToApply {
				t.Fatalf("%s is applied live but its schema Scope is restart-to-apply", key)
			}
			if reflect.DeepEqual(before, after) {
				t.Fatalf("%s did not change the running settings after one reload", key)
			}
		})
	}
}

// specRestartRequired reads the key list from SPEC §6.5's one
// "Restart-required keys:" line.
func specRestartRequired(t *testing.T) []string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "SPEC.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(string(data), "\n") {
		if !strings.HasPrefix(line, "**Restart-required keys.**") {
			continue
		}
		var keys []string
		for _, m := range regexp.MustCompile("`([^`]+)`").FindAllStringSubmatch(line, -1) {
			keys = append(keys, m[1])
		}
		return keys
	}
	t.Fatal("SPEC.md has no **Restart-required keys.** line")
	return nil
}

func TestReloadSpecNamesEveryRestartRequiredKey(t *testing.T) {
	got := append([]string{}, specRestartRequired(t)...)
	want := append([]string{}, reloadRestartRequired...)
	sort.Strings(got)
	sort.Strings(want)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("SPEC §6.5 restart-required keys %v do not match the test's list %v", got, want)
	}
}
