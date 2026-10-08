package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// loadEventHookBody parses body as a config.toml and returns the resulting
// FileConfig, so a test names the key it exercises and nothing else.
func loadEventHookBody(t *testing.T, body string) (FileConfig, error) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return loadConfigFile(path)
}

// TestEventHookKeysDefaultWhenAbsent pins the R229 defaults: no script, the
// hook off, waiting/error/ended, 3 seconds.
func TestEventHookKeysDefaultWhenAbsent(t *testing.T) {
	cfg, err := loadEventHookBody(t, "")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.EventHook != "" {
		t.Errorf("EventHook = %q, want empty", cfg.EventHook)
	}
	if cfg.EventHookDefault {
		t.Error("EventHookDefault = true, want false")
	}
	if want := []string{"waiting", "error", "ended"}; !reflect.DeepEqual(cfg.EventHookEvents, want) {
		t.Errorf("EventHookEvents = %v, want %v", cfg.EventHookEvents, want)
	}
	if cfg.EventHookTimeout != 3*time.Second {
		t.Errorf("EventHookTimeout = %s, want 3s", cfg.EventHookTimeout)
	}
}

// TestEventHookDefaultsDoNotAliasTheSchema proves a caller editing the
// loaded list cannot corrupt the schema's own default for the next load.
func TestEventHookDefaultsDoNotAliasTheSchema(t *testing.T) {
	first, _ := loadEventHookBody(t, "")
	first.EventHookEvents[0] = "killed"
	second, _ := loadEventHookBody(t, "")
	if second.EventHookEvents[0] != "waiting" {
		t.Fatalf("second load saw the first load's edit: %v", second.EventHookEvents)
	}
}

func TestEventHookKeysParse(t *testing.T) {
	cfg, err := loadEventHookBody(t, strings.Join([]string{
		`event_hook = "/usr/local/bin/ping me"`,
		`event_hook_default = true`,
		`event_hook_events = ["started", "idle", "killed"]`,
		`event_hook_timeout = 9`,
	}, "\n")+"\n")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.EventHook != "/usr/local/bin/ping me" {
		t.Errorf("EventHook = %q", cfg.EventHook)
	}
	if !cfg.EventHookDefault {
		t.Error("EventHookDefault = false, want true")
	}
	if want := []string{"started", "idle", "killed"}; !reflect.DeepEqual(cfg.EventHookEvents, want) {
		t.Errorf("EventHookEvents = %v, want %v", cfg.EventHookEvents, want)
	}
	if cfg.EventHookTimeout != 9*time.Second {
		t.Errorf("EventHookTimeout = %s, want 9s", cfg.EventHookTimeout)
	}
}

func TestEventHookEventsEmptyListIsAValueNotADefault(t *testing.T) {
	cfg, err := loadEventHookBody(t, "event_hook_events = []\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.EventHookEvents) != 0 {
		t.Fatalf("EventHookEvents = %v, want the empty list", cfg.EventHookEvents)
	}
}

func TestEventHookTimeoutAcceptsADuration(t *testing.T) {
	cfg, err := loadEventHookBody(t, "event_hook_timeout = \"1m30s\"\n")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.EventHookTimeout != 90*time.Second {
		t.Fatalf("EventHookTimeout = %s, want 90s", cfg.EventHookTimeout)
	}
}

// TestEventHookEventsUnknownKindIsALoadErrorNamingTheKey: prompt, env and
// note are §4 kinds but are never offered (SPEC §10.1), so they are refused
// exactly like a typo.
func TestEventHookEventsUnknownKindIsALoadErrorNamingTheKey(t *testing.T) {
	for _, kind := range []string{"waitting", "prompt", "env", "note", ""} {
		_, err := loadEventHookBody(t, `event_hook_events = ["waiting", "`+kind+`"]`+"\n")
		if err == nil {
			t.Fatalf("kind %q: load succeeded, want an error", kind)
		}
		if !strings.Contains(err.Error(), "event_hook_events") || !strings.Contains(err.Error(), `"`+kind+`"`) {
			t.Errorf("kind %q: error %q does not name the key and the kind", kind, err)
		}
		if !strings.Contains(err.Error(), "config.toml:1") {
			t.Errorf("kind %q: error %q does not name the file and line", kind, err)
		}
	}
}

func TestEventHookEventsMalformedArrayIsALoadErrorNamingTheKey(t *testing.T) {
	for _, body := range []string{`"waiting"`, `[waiting]`, `["waiting"`, `["waiting" "error"]`} {
		_, err := loadEventHookBody(t, "event_hook_events = "+body+"\n")
		if err == nil || !strings.Contains(err.Error(), "event_hook_events") {
			t.Errorf("body %s: err = %v, want one naming event_hook_events", body, err)
		}
	}
}

func TestEventHookTimeoutNonPositiveIsALoadErrorNamingTheKey(t *testing.T) {
	for _, value := range []string{"0", "-3", `"0s"`, `"-2s"`} {
		_, err := loadEventHookBody(t, "event_hook_timeout = "+value+"\n")
		if err == nil || !strings.Contains(err.Error(), "event_hook_timeout") {
			t.Errorf("value %s: err = %v, want one naming event_hook_timeout", value, err)
		}
	}
	if _, err := loadEventHookBody(t, "event_hook_timeout = soon\n"); err == nil || !strings.Contains(err.Error(), "event_hook_timeout") {
		t.Errorf("non-numeric: err = %v, want one naming event_hook_timeout", err)
	}
}

func TestEventHookToggleAndScriptMalformedAreLoadErrorsNamingTheKey(t *testing.T) {
	for key, body := range map[string]string{
		"event_hook_default": "event_hook_default = maybe\n",
		"event_hook":         "event_hook = /bin/true\n",
	} {
		if _, err := loadEventHookBody(t, body); err == nil || !strings.Contains(err.Error(), key) {
			t.Errorf("%s: err = %v, want one naming the key", key, err)
		}
	}
}

// TestEventHookKeysWriteBackRoundTrip writes every non-default value through
// WriteConfigFile and re-reads them: the file carries the four keys and the
// re-read config equals what was written.
func TestEventHookKeysWriteBackRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	cfg, err := loadConfigFile(path)
	if err != nil {
		t.Fatal(err)
	}
	cfg.EventHook = "/opt/hooks/notify --quiet"
	cfg.EventHookDefault = true
	cfg.EventHookEvents = []string{"resumed", "ended"}
	cfg.EventHookTimeout = 12 * time.Second
	if err := WriteConfigFile(path, cfg); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`event_hook = "/opt/hooks/notify --quiet"`,
		`event_hook_default = true`,
		`event_hook_events = ["resumed", "ended"]`,
		`event_hook_timeout = 12`,
	} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("written config.toml lacks %q:\n%s", want, raw)
		}
	}
	reread, err := loadConfigFile(path)
	if err != nil {
		t.Fatalf("written file did not parse: %v", err)
	}
	if !reflect.DeepEqual(reread, cfg) {
		t.Fatalf("re-read = %+v, want %+v", reread, cfg)
	}
}

func TestEventHookEventsEmptyListWritesBackAsEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	cfg, _ := loadConfigFile(path)
	cfg.EventHookEvents = []string{}
	if err := WriteConfigFile(path, cfg); err != nil {
		t.Fatal(err)
	}
	reread, err := loadConfigFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(reread.EventHookEvents) != 0 {
		t.Fatalf("EventHookEvents = %v, want empty after write-back", reread.EventHookEvents)
	}
}

// TestEventHookSettingsCarryTheFileValues proves LoadFrom hands the four
// keys to the running Settings, with the §10 defaults when absent.
func TestEventHookSettingsCarryTheFileValues(t *testing.T) {
	absent, err := LoadFrom(environment(map[string]string{"DECK_HOME": t.TempDir()}), fakeHome)
	if err != nil {
		t.Fatal(err)
	}
	if absent.EventHook != "" || absent.EventHookDefault || absent.EventHookTimeout != DefaultEventHookTimeout ||
		!reflect.DeepEqual(absent.EventHookEvents, []string{"waiting", "error", "ended"}) {
		t.Fatalf("default Settings event-hook fields = %q %v %v %s", absent.EventHook, absent.EventHookDefault, absent.EventHookEvents, absent.EventHookTimeout)
	}
	dir := writeConfigFile(t, "event_hook = \"/x/y\"\nevent_hook_default = true\nevent_hook_events = [\"idle\"]\nevent_hook_timeout = 5\n")
	set, err := LoadFrom(environment(map[string]string{"DECK_HOME": dir}), fakeHome)
	if err != nil {
		t.Fatal(err)
	}
	if set.EventHook != "/x/y" || !set.EventHookDefault || set.EventHookTimeout != 5*time.Second ||
		!reflect.DeepEqual(set.EventHookEvents, []string{"idle"}) {
		t.Fatalf("Settings event-hook fields = %q %v %v %s", set.EventHook, set.EventHookDefault, set.EventHookEvents, set.EventHookTimeout)
	}
}

func TestCheckListElementsAcceptsOfferedKindsAndRefusesOthers(t *testing.T) {
	field, ok := FieldByFullKey("event_hook_events")
	if !ok {
		t.Fatal("event_hook_events is not in the schema")
	}
	if err := CheckListElements(field, EventHookKinds); err != nil {
		t.Fatalf("every offered kind should pass: %v", err)
	}
	if err := CheckListElements(field, []string{"waiting", "snooze"}); err == nil || !strings.Contains(err.Error(), "event_hook_events") {
		t.Fatalf("err = %v, want a refusal naming event_hook_events", err)
	}
	if err := CheckListElements(Field{Key: "free"}, []string{"anything"}); err != nil {
		t.Fatalf("a field with no ElementValues takes any element: %v", err)
	}
}
