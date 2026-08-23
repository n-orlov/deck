package tui

import (
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
)

// TestIsSecretShapedKeyMatchesSPEC64Pattern proves isSecretShapedKey --
// the ONE predicate every masking call site uses -- matches SPEC §6.4's
// exact five substrings case-insensitively by substring, and that an
// ordinary key is left alone.
func TestIsSecretShapedKeyMatchesSPEC64Pattern(t *testing.T) {
	cases := map[string]bool{
		"AUDIT_ENV_TOKEN": true,
		"api_key":         true,
		"DB_PASSWORD":     true,
		"my_secret_thing": true,
		"AWS_CREDENTIAL":  true,
		"PATH":            false,
		"HOME":            false,
		"NODE_ENV":        false,
	}
	for key, want := range cases {
		if got := isSecretShapedKey(key); got != want {
			t.Errorf("isSecretShapedKey(%q) = %v, want %v", key, got, want)
		}
	}
}

// TestMaskEnvValueMasksBySDefaultAndRevealsOnToggle proves maskEnvValue --
// the shared function every rendering call site uses -- returns the fixed
// placeholder for a secret-shaped key while not revealed, the real value
// once revealed, and always the real value for a non-secret-shaped key
// (revealed makes no difference there).
func TestMaskEnvValueMasksBySDefaultAndRevealsOnToggle(t *testing.T) {
	m := New(nil, config.Settings{}, "")

	if got := m.maskEnvValue("API_TOKEN", "super-secret-do-not-log", false); got == "super-secret-do-not-log" {
		t.Fatalf("maskEnvValue with revealed=false returned the real value: %q", got)
	}
	if strings.Contains(m.maskEnvValue("API_TOKEN", "super-secret-do-not-log", false), "super-secret-do-not-log") {
		t.Fatal("masked placeholder contains the real value as a substring")
	}
	if got := m.maskEnvValue("API_TOKEN", "super-secret-do-not-log", true); got != "super-secret-do-not-log" {
		t.Fatalf("maskEnvValue with revealed=true = %q, want the real value", got)
	}
	if got := m.maskEnvValue("PATH", "/usr/bin:/bin", false); got != "/usr/bin:/bin" {
		t.Fatalf("maskEnvValue masked a non-secret-shaped key: %q", got)
	}
}

// TestEnvEditorMasksSecretShapedValueByDefaultAndRevealsOnR drives the
// real `e` env editor view end to end: a secret-shaped row's value never
// appears on screen until "r" is pressed, and pressing "r" again re-masks
// it -- the toggle, not a one-way reveal.
func TestEnvEditorMasksSecretShapedValueByDefaultAndRevealsOnR(t *testing.T) {
	model := New(nil, config.Settings{}, "")
	model.sessions = []store.Session{{
		ID: "s1", Name: "sess", Agent: "claude",
		Env: map[string]string{"AUDIT_ENV_TOKEN": "super-secret-do-not-log-8675309"},
	}}
	updated, _ := model.Update(key("e"))
	m := updated.(Model)
	if !m.envEditing {
		t.Fatal("e did not open the env editor")
	}

	view := m.View()
	if strings.Contains(view, "super-secret-do-not-log-8675309") {
		t.Fatalf("env editor showed the secret-shaped value unmasked by default:\n%s", view)
	}

	updated, _ = m.Update(key("r"))
	m = updated.(Model)
	view = m.View()
	if !strings.Contains(view, "super-secret-do-not-log-8675309") {
		t.Fatalf("env editor did not reveal the value after r:\n%s", view)
	}

	updated, _ = m.Update(key("r"))
	m = updated.(Model)
	view = m.View()
	if strings.Contains(view, "super-secret-do-not-log-8675309") {
		t.Fatalf("a second r did not re-mask the value:\n%s", view)
	}
}

// TestEnvEditorRevealResetsToMaskedOnReopen proves the reveal toggle never
// sticks across a close/reopen of the dialog -- SPEC §6.4's "masked ...
// by default" holds every time the dialog is opened, not just the first.
func TestEnvEditorRevealResetsToMaskedOnReopen(t *testing.T) {
	model := New(nil, config.Settings{}, "")
	model.sessions = []store.Session{{
		ID: "s1", Name: "sess", Agent: "claude",
		Env: map[string]string{"API_KEY": "leak-me-not"},
	}}
	updated, _ := model.Update(key("e"))
	m := updated.(Model)
	updated, _ = m.Update(key("r"))
	m = updated.(Model)
	if !m.envReveal {
		t.Fatal("r did not set envReveal")
	}

	// Close (esc, nothing being edited) and reopen.
	updated, _ = m.Update(key("esc"))
	m = updated.(Model)
	if m.envEditing {
		t.Fatal("esc with nothing being edited did not close the env editor")
	}
	updated, _ = m.Update(key("e"))
	m = updated.(Model)
	if m.envReveal {
		t.Fatal("envReveal still true after closing and reopening the env editor")
	}
	if strings.Contains(m.View(), "leak-me-not") {
		t.Fatalf("reopened env editor shows the value unmasked:\n%s", m.View())
	}
}

// TestSettingsEnvEntriesMaskSecretShapedValueByDefaultAndRevealOnR mirrors
// the env-editor test above for the settings takeover's `[env]` entries
// list -- the other of the two surfaces that render an env key/value pair
// today.
func TestSettingsEnvEntriesMaskSecretShapedValueByDefaultAndRevealOnR(t *testing.T) {
	cfg := config.Settings{Env: map[string]string{"DB_PASSWORD": "leak-me-not-either"}}
	cfg.File.Env = cfg.Env
	model := New(nil, cfg, "")

	categories := settingsCategories()
	catIdx, fieldIdx := -1, -1
	for ci, cat := range categories {
		for fi, f := range cat.Fields {
			if f.FullKey() == "[env]" {
				catIdx, fieldIdx = ci, fi
			}
		}
	}
	if catIdx < 0 {
		t.Fatal("no [env] field found in settingsCategories()")
	}
	updated, _ := model.Update(key(","))
	m := updated.(Model)
	m.settingsCategoryIndex = catIdx
	m.settingsFieldIndex = fieldIdx
	m.settingsFocus = settingsFocusFields

	updated, _ = m.Update(key("enter")) // open the [env] entries list
	m = updated.(Model)
	if !m.settingsEnvOpen {
		t.Fatal("enter on [env] did not open the entries list")
	}

	view := m.View()
	if strings.Contains(view, "leak-me-not-either") {
		t.Fatalf("[env] entries list showed the secret-shaped value unmasked by default:\n%s", view)
	}

	updated, _ = m.Update(key("r"))
	m = updated.(Model)
	view = m.View()
	if !strings.Contains(view, "leak-me-not-either") {
		t.Fatalf("[env] entries list did not reveal the value after r:\n%s", view)
	}

	updated, _ = m.Update(key("r"))
	m = updated.(Model)
	view = m.View()
	if strings.Contains(view, "leak-me-not-either") {
		t.Fatalf("a second r did not re-mask the value:\n%s", view)
	}
}
