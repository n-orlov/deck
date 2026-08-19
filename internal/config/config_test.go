package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func environment(values map[string]string) func(string) string {
	return func(key string) string { return values[key] }
}

func fakeHome() (string, error) { return "/home/deck", nil }

func TestPathsRespectDeckHomeAndXDG(t *testing.T) {
	settings, err := LoadFrom(environment(map[string]string{"DECK_HOME": "/tmp/scenario"}), fakeHome)
	if err != nil {
		t.Fatal(err)
	}
	if settings.Paths.DataDir != "/tmp/scenario" || settings.Paths.LogDir != "/tmp/scenario/log" || settings.Paths.ConfigFile != "/tmp/scenario/config.toml" {
		t.Fatalf("DECK_HOME paths = %+v", settings.Paths)
	}

	settings, err = LoadFrom(environment(map[string]string{"XDG_DATA_HOME": "/data", "XDG_CONFIG_HOME": "/config", "XDG_STATE_HOME": "/state"}), fakeHome)
	if err != nil {
		t.Fatal(err)
	}
	if settings.Paths.DataDir != "/data/deck" || settings.Paths.ConfigFile != "/config/deck/config.toml" || settings.Paths.LogDir != "/state/deck/log" || settings.Paths.StateDB != "/data/deck/state.db" {
		t.Fatalf("XDG paths = %+v", settings.Paths)
	}

	settings, err = LoadFrom(environment(nil), fakeHome)
	if err != nil {
		t.Fatal(err)
	}
	if settings.Paths.DataDir != "/home/deck/.local/share/deck" || settings.Paths.ConfigFile != "/home/deck/.config/deck/config.toml" || settings.Paths.LogDir != "/home/deck/.local/state/deck/log" {
		t.Fatalf("fallback paths = %+v", settings.Paths)
	}
}

func TestControlsAndClock(t *testing.T) {
	settings, err := LoadFrom(environment(map[string]string{
		"DECK_HOME": t.TempDir(), "DECK_TMUX_SOCKET": "scenario_42", "DECK_CLOCK": "2025-01-02T03:04:05Z", "DECK_CLOCK_STEP": "2s",
		"DECK_RECONCILE_MS": "9", "DECK_PREVIEW_MS": "11", "DECK_ASCII": "true", "DECK_ANIM": "false", "NO_COLOR": "1",
	}), fakeHome)
	if err != nil {
		t.Fatal(err)
	}
	if settings.Socket != "scenario_42" || settings.Reconcile != 9*time.Millisecond || settings.Preview != 11*time.Millisecond || !settings.ASCII || settings.Animation || settings.Color {
		t.Fatalf("controls = %+v", settings)
	}
	want := "2025-01-02T03:04:05Z"
	if got := settings.Clock.Now().Format(time.RFC3339); got != want {
		t.Fatalf("frozen now = %s, want %s", got, want)
	}
	if got := settings.Clock.Advance().Format(time.RFC3339); got != "2025-01-02T03:04:07Z" {
		t.Fatalf("advanced now = %s", got)
	}
	before := settings.Clock.Elapsed()
	time.Sleep(2 * time.Millisecond)
	if settings.Clock.Elapsed() <= before {
		t.Fatal("elapsed duration did not advance under frozen wall clock")
	}
}

func TestFrozenClockUsesResolvedDataRootWithoutDeckHome(t *testing.T) {
	data := t.TempDir()
	env := environment(map[string]string{
		"XDG_DATA_HOME": data,
		"DECK_CLOCK":    "2025-01-02T03:04:05Z",
	})
	first, err := LoadFrom(env, fakeHome)
	if err != nil {
		t.Fatal(err)
	}
	clockFile := filepath.Join(data, "deck", "clock.now")
	if err := os.MkdirAll(filepath.Dir(clockFile), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(clockFile, []byte("2025-01-02T03:08:05Z\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := first.Clock.Now().Format(time.RFC3339); got != "2025-01-02T03:08:05Z" {
		t.Fatalf("clock.now under resolved data root = %s", got)
	}
	second, err := LoadFrom(env, fakeHome)
	if err != nil {
		t.Fatal(err)
	}
	if got := second.Clock.Now().Format(time.RFC3339); got != "2025-01-02T03:08:05Z" {
		t.Fatalf("second process now = %s", got)
	}
}

func TestDeckHomeClocksShareOnDemandAdvance(t *testing.T) {
	home := t.TempDir()
	env := environment(map[string]string{"DECK_HOME": home, "DECK_CLOCK": "2025-01-02T03:04:05Z", "DECK_CLOCK_STEP": "2m"})
	first, err := LoadFrom(env, fakeHome)
	if err != nil {
		t.Fatal(err)
	}
	second, err := LoadFrom(env, fakeHome)
	if err != nil {
		t.Fatal(err)
	}
	advanced, err := first.Clock.AdvanceShared()
	if err != nil {
		t.Fatal(err)
	}
	want := "2025-01-02T03:06:05Z"
	if got := advanced.Format(time.RFC3339); got != want {
		t.Fatalf("advanced = %s, want %s", got, want)
	}
	if got := second.Clock.Now().Format(time.RFC3339); got != want {
		t.Fatalf("second process now = %s, want %s", got, want)
	}
	third, err := LoadFrom(env, fakeHome)
	if err != nil {
		t.Fatal(err)
	}
	if got := third.Clock.Now().Format(time.RFC3339); got != want {
		t.Fatalf("later subprocess now = %s, want %s", got, want)
	}
}

func TestSeededUUIDsAreStableAndValid(t *testing.T) {
	left, right := NewIDGenerator("same"), NewIDGenerator("same")
	first, err := left.UUID()
	if err != nil {
		t.Fatal(err)
	}
	matching, err := right.UUID()
	if err != nil {
		t.Fatal(err)
	}
	second, err := left.UUID()
	if err != nil {
		t.Fatal(err)
	}
	if first != matching || first == second {
		t.Fatalf("seeded ids first=%q matching=%q second=%q", first, matching, second)
	}
	if len(first) != 36 || first[14] != '4' || !strings.Contains("89ab", string(first[19])) {
		t.Fatalf("not an RFC4122 v4 UUID: %q", first)
	}
}

func writeConfigFile(t *testing.T, contents string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestConfigFileMissingDefaultsToNoYoloAndNoEnv(t *testing.T) {
	settings, err := LoadFrom(environment(map[string]string{"DECK_HOME": t.TempDir()}), fakeHome)
	if err != nil {
		t.Fatal(err)
	}
	if settings.AllowYolo {
		t.Fatal("AllowYolo should default to false when config.toml is absent")
	}
	if settings.StaleAfter != DefaultStaleAfter {
		t.Fatalf("StaleAfter = %s, want default %s", settings.StaleAfter, DefaultStaleAfter)
	}
	if settings.Env != nil {
		t.Fatalf("Env should be nil when config.toml is absent, got %+v", settings.Env)
	}
}

func TestConfigFileAllowYoloTrue(t *testing.T) {
	dir := writeConfigFile(t, "allow_yolo = true\n")
	settings, err := LoadFrom(environment(map[string]string{"DECK_HOME": dir}), fakeHome)
	if err != nil {
		t.Fatal(err)
	}
	if !settings.AllowYolo {
		t.Fatal("AllowYolo should be true when allow_yolo = true")
	}
}

func TestConfigFileAllowYoloFalse(t *testing.T) {
	dir := writeConfigFile(t, "allow_yolo = false\n")
	settings, err := LoadFrom(environment(map[string]string{"DECK_HOME": dir}), fakeHome)
	if err != nil {
		t.Fatal(err)
	}
	if settings.AllowYolo {
		t.Fatal("AllowYolo should be false when allow_yolo = false")
	}
}

func TestConfigFileStaleAfter(t *testing.T) {
	for name, value := range map[string]string{"seconds": "90", "duration": `"1m30s"`} {
		t.Run(name, func(t *testing.T) {
			dir := writeConfigFile(t, "stale_after = "+value+"\n")
			settings, err := LoadFrom(environment(map[string]string{"DECK_HOME": dir}), fakeHome)
			if err != nil {
				t.Fatal(err)
			}
			if settings.StaleAfter != 90*time.Second {
				t.Fatalf("StaleAfter = %s, want 90s", settings.StaleAfter)
			}
		})
	}
}

func TestConfigFileEnvTable(t *testing.T) {
	dir := writeConfigFile(t, "allow_yolo = true\n\n[env]\nPATH = \"/opt/tools:/usr/bin\"\nFOO = \"bar\"\n")
	settings, err := LoadFrom(environment(map[string]string{"DECK_HOME": dir}), fakeHome)
	if err != nil {
		t.Fatal(err)
	}
	if !settings.AllowYolo {
		t.Fatal("AllowYolo should be true")
	}
	want := map[string]string{"PATH": "/opt/tools:/usr/bin", "FOO": "bar"}
	if len(settings.Env) != len(want) {
		t.Fatalf("Env = %+v, want %+v", settings.Env, want)
	}
	for k, v := range want {
		if settings.Env[k] != v {
			t.Fatalf("Env[%s] = %q, want %q", k, settings.Env[k], v)
		}
	}
}

func TestConfigFileMalformedIsRejected(t *testing.T) {
	for name, contents := range map[string]string{
		"no equals":            "allow_yolo true\n",
		"bad bool":             "allow_yolo = maybe\n",
		"bad stale duration":   "stale_after = \"0s\"\n",
		"unterminated section": "[env\nFOO = \"bar\"\n",
		"unquoted env value":   "[env]\nFOO = bar\n",
		"empty key":            "= \"x\"\n",
	} {
		t.Run(name, func(t *testing.T) {
			dir := writeConfigFile(t, contents)
			if _, err := LoadFrom(environment(map[string]string{"DECK_HOME": dir}), fakeHome); err == nil {
				t.Fatalf("contents %q were accepted", contents)
			}
		})
	}
}

func TestInvalidControlsAreRejected(t *testing.T) {
	for key, value := range map[string]string{
		"DECK_CLOCK": "tomorrow", "DECK_CLOCK_STEP": "0s", "DECK_RECONCILE_MS": "0", "DECK_PREVIEW_MS": "soon", "DECK_TMUX_SOCKET": "bad/name", "DECK_ASCII": "perhaps",
	} {
		t.Run(key, func(t *testing.T) {
			if _, err := LoadFrom(environment(map[string]string{key: value}), fakeHome); err == nil {
				t.Fatalf("%s=%q was accepted", key, value)
			}
		})
	}
}
