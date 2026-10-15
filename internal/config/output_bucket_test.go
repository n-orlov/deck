package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestOutputBucketDefaultFileAndEnvOverride pins R306's key: ui.output_bucket
// is 10 s when absent, takes a bare integer (seconds) or a quoted duration
// from the file, and is outranked by DECK_OUTPUT_BUCKET.
func TestOutputBucketDefaultFileAndEnvOverride(t *testing.T) {
	field, ok := FieldByFullKey("ui.output_bucket")
	if !ok || field.Kind != KindInteger || field.Default != 10 || field.Unit != "seconds" || field.IntBounds.Min != 1 {
		t.Fatalf("schema entry for ui.output_bucket = %+v, %v; want a seconds integer, default 10, minimum 1", field, ok)
	}
	if field.Scope != ScopeGlobal {
		t.Fatalf("ui.output_bucket scope = %q, want global (applies live)", field.Scope)
	}
	def, err := LoadFrom(environment(map[string]string{"DECK_HOME": t.TempDir()}), fakeHome)
	if err != nil || def.OutputBucket != 10*time.Second || def.File.OutputBucket != 10*time.Second {
		t.Fatalf("default OutputBucket = %v / %v, %v; want 10s", def.OutputBucket, def.File.OutputBucket, err)
	}
	if _, overridden := def.EnvOverrides["ui.output_bucket"]; overridden {
		t.Fatal("ui.output_bucket reported as env-overridden with no DECK_OUTPUT_BUCKET set")
	}
	dir := writeConfigFile(t, "[ui]\noutput_bucket = 4\n")
	fromFile, err := LoadFrom(environment(map[string]string{"DECK_HOME": dir}), fakeHome)
	if err != nil || fromFile.OutputBucket != 4*time.Second {
		t.Fatalf("output_bucket = 4 in the file gave %v, %v", fromFile.OutputBucket, err)
	}
	dirDur := writeConfigFile(t, "[ui]\noutput_bucket = \"1m30s\"\n")
	fromDur, err := LoadFrom(environment(map[string]string{"DECK_HOME": dirDur}), fakeHome)
	if err != nil || fromDur.OutputBucket != 90*time.Second {
		t.Fatalf("output_bucket = \"1m30s\" gave %v, %v; want 90s", fromDur.OutputBucket, err)
	}
	over, err := LoadFrom(environment(map[string]string{"DECK_HOME": dir, "DECK_OUTPUT_BUCKET": "7"}), fakeHome)
	if err != nil || over.OutputBucket != 7*time.Second || over.File.OutputBucket != 4*time.Second {
		t.Fatalf("DECK_OUTPUT_BUCKET=7 over the file gave %v (file %v), %v; want 7s over 4s", over.OutputBucket, over.File.OutputBucket, err)
	}
	if got := over.EnvOverrides["ui.output_bucket"]; got != "DECK_OUTPUT_BUCKET" {
		t.Fatalf("EnvOverrides[ui.output_bucket] = %q, want DECK_OUTPUT_BUCKET", got)
	}
	for _, bad := range []string{"0", "-3", "soon"} {
		if _, err := LoadFrom(environment(map[string]string{"DECK_HOME": dir, "DECK_OUTPUT_BUCKET": bad}), fakeHome); err == nil || !strings.Contains(err.Error(), "DECK_OUTPUT_BUCKET") {
			t.Errorf("DECK_OUTPUT_BUCKET=%s: err = %v, want one naming the variable", bad, err)
		}
	}
}

// TestOutputBucketBelowOneSecondIsALoadErrorNamingTheKey pins the minimum:
// 0, a negative value and a sub-second duration are all refused.
func TestOutputBucketBelowOneSecondIsALoadErrorNamingTheKey(t *testing.T) {
	for _, value := range []string{"0", "-5", "\"500ms\""} {
		_, err := loadEventHookBody(t, "[ui]\noutput_bucket = "+value+"\n")
		if err == nil || !strings.Contains(err.Error(), "ui.output_bucket must be at least 1") {
			t.Errorf("value %s: err = %v, want \"ui.output_bucket must be at least 1\"", value, err)
		}
	}
}

// TestOutputBucketRoundTripsThroughWriteConfigFile writes a non-default
// bucket and reads it back, keeping the keys around it.
func TestOutputBucketRoundTripsThroughWriteConfigFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte("some_future_key = \"x\"\n\n[ui]\noutput_bucket = 3\nsome_future_ui_key = 7\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := loadConfigFile(path)
	if err != nil || cfg.OutputBucket != 3*time.Second {
		t.Fatalf("loaded OutputBucket = %v, %v; want 3s", cfg.OutputBucket, err)
	}
	cfg.OutputBucket = 25 * time.Second
	if err := WriteConfigFile(path, cfg); err != nil {
		t.Fatal(err)
	}
	written, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"output_bucket = 25", "some_future_key = \"x\"", "some_future_ui_key = 7"} {
		if !strings.Contains(string(written), want) {
			t.Errorf("written config.toml lacks %q:\n%s", want, written)
		}
	}
	reread, err := loadConfigFile(path)
	if err != nil || reread.OutputBucket != 25*time.Second {
		t.Fatalf("re-read OutputBucket = %v, %v; want 25s", reread.OutputBucket, err)
	}
}

// TestReloadingAnOutputBucketChangeWritesNothing proves a config reload that
// picks up a new output_bucket leaves config.toml byte- and mtime-identical.
func TestReloadingAnOutputBucketChangeWritesNothing(t *testing.T) {
	f := newReloadFixture(t)
	f.write(f.config, "[ui]\noutput_bucket = 5\n")
	f.poll()
	f.write(f.config, "[ui]\noutput_bucket = 20\n")
	before, err := os.ReadFile(f.config)
	if err != nil {
		t.Fatal(err)
	}
	beforeInfo, _ := os.Stat(f.config)
	added, settings, changed := f.poll()
	if added != 1 || !changed || settings.OutputBucket != 20*time.Second {
		t.Fatalf("reload: loads=%d changed=%v OutputBucket=%v; want one reload to 20s", added, changed, settings.OutputBucket)
	}
	after, _ := os.ReadFile(f.config)
	afterInfo, _ := os.Stat(f.config)
	if string(before) != string(after) || !afterInfo.ModTime().Equal(beforeInfo.ModTime()) {
		t.Fatal("a reload that applied output_bucket rewrote config.toml")
	}
}
