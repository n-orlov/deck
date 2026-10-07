package config

import (
	"path/filepath"
	"testing"
)

// R220.3: the config file's top-level agent key is declared, parsed into
// Settings.Agent, written back, and empty when absent. Every agent kind is
// accepted verbatim here; whether a kind is registered is checked where the key
// is consumed (cmd/deck), with the create service's own diagnostic.
func TestConfigAgentKeyIsParsedAndRoundTrips(t *testing.T) {
	if _, ok := FieldByFullKey("agent"); !ok {
		t.Fatal("agent is not declared in Schema")
	}
	cases := map[string]string{"copilot": "copilot", "codex": "codex", "claude": "claude", "shell": "shell", "ghost": "ghost", "": ""}
	for value, want := range cases {
		dir := writeConfigFile(t, "agent = \""+value+"\"\nallow_yolo = true\n")
		settings, err := LoadFrom(environment(map[string]string{"DECK_HOME": dir}), fakeHome)
		if err != nil {
			t.Fatalf("agent = %q: %v", value, err)
		}
		if settings.Agent != want || !settings.AllowYolo {
			t.Errorf("agent = %q: Settings.Agent=%q AllowYolo=%v, want %q and true", value, settings.Agent, settings.AllowYolo, want)
		}
	}
	settings, err := LoadFrom(environment(map[string]string{"DECK_HOME": t.TempDir()}), fakeHome)
	if err != nil || settings.Agent != "" {
		t.Fatalf("absent key: Agent=%q err=%v, want empty", settings.Agent, err)
	}

	path := filepath.Join(t.TempDir(), "config.toml")
	cfg, err := loadConfigFile(path)
	if err != nil {
		t.Fatal(err)
	}
	cfg.Agent = "copilot"
	if err := WriteConfigFile(path, cfg); err != nil {
		t.Fatal(err)
	}
	reread, err := loadConfigFile(path)
	if err != nil || reread.Agent != "copilot" {
		t.Fatalf("written agent key re-read as %q, %v", reread.Agent, err)
	}
}

func TestConfigAgentKeyMustBeAQuotedString(t *testing.T) {
	dir := writeConfigFile(t, "agent = copilot\n")
	if _, err := LoadFrom(environment(map[string]string{"DECK_HOME": dir}), fakeHome); err == nil {
		t.Fatal("an unquoted agent value must be rejected like every other string key")
	}
}
