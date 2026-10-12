package config

import (
	"strings"
	"testing"
	"time"
)

// TestNoteTimeoutDefaultFileAndEnvOverride pins R283's key: ui.note_timeout
// is whole seconds, 10 s when absent, takes the file's value (a bare integer
// or a duration string), and is outranked by DECK_NOTE_TIMEOUT.
func TestNoteTimeoutDefaultFileAndEnvOverride(t *testing.T) {
	field, ok := FieldByFullKey("ui.note_timeout")
	if !ok || field.Kind != KindInteger || field.Default != 10 || field.Unit != "seconds" {
		t.Fatalf("schema entry for ui.note_timeout = %+v, %v; want a seconds integer defaulting to 10", field, ok)
	}
	def, err := LoadFrom(environment(map[string]string{"DECK_HOME": t.TempDir()}), fakeHome)
	if err != nil || def.NoteTimeout != 10*time.Second {
		t.Fatalf("default Settings.NoteTimeout = %v, %v; want 10s", def.NoteTimeout, err)
	}
	if _, overridden := def.EnvOverrides["ui.note_timeout"]; overridden {
		t.Fatal("ui.note_timeout reported as env-overridden with no DECK_NOTE_TIMEOUT set")
	}
	dir := writeConfigFile(t, "[ui]\nnote_timeout = 4\n")
	fromFile, err := LoadFrom(environment(map[string]string{"DECK_HOME": dir}), fakeHome)
	if err != nil || fromFile.NoteTimeout != 4*time.Second || fromFile.File.NoteTimeout != 4*time.Second {
		t.Fatalf("note_timeout = 4 in the file gave %v / %v, %v", fromFile.NoteTimeout, fromFile.File.NoteTimeout, err)
	}
	dirDur := writeConfigFile(t, "[ui]\nnote_timeout = \"1m30s\"\n")
	fromDur, err := LoadFrom(environment(map[string]string{"DECK_HOME": dirDur}), fakeHome)
	if err != nil || fromDur.NoteTimeout != 90*time.Second {
		t.Fatalf("note_timeout = \"1m30s\" gave %v, %v; want 90s", fromDur.NoteTimeout, err)
	}
	over, err := LoadFrom(environment(map[string]string{"DECK_HOME": dir, "DECK_NOTE_TIMEOUT": "7"}), fakeHome)
	if err != nil || over.NoteTimeout != 7*time.Second || over.File.NoteTimeout != 4*time.Second {
		t.Fatalf("DECK_NOTE_TIMEOUT=7 over the file gave %v (file %v), %v; want 7s over 4s", over.NoteTimeout, over.File.NoteTimeout, err)
	}
	if got := over.EnvOverrides["ui.note_timeout"]; got != "DECK_NOTE_TIMEOUT" {
		t.Fatalf("EnvOverrides[ui.note_timeout] = %q, want DECK_NOTE_TIMEOUT", got)
	}
	for _, bad := range []string{"0", "-3", "soon"} {
		if _, err := LoadFrom(environment(map[string]string{"DECK_HOME": dir, "DECK_NOTE_TIMEOUT": bad}), fakeHome); err == nil || !strings.Contains(err.Error(), "DECK_NOTE_TIMEOUT") {
			t.Errorf("DECK_NOTE_TIMEOUT=%s: err = %v, want one naming the variable", bad, err)
		}
	}
}

// TestNoteTimeoutNonPositiveIsALoadErrorNamingTheKey pins the schema's usual
// bound error for a zero or negative value.
func TestNoteTimeoutNonPositiveIsALoadErrorNamingTheKey(t *testing.T) {
	for _, value := range []string{"0", "-5"} {
		_, err := loadEventHookBody(t, "[ui]\nnote_timeout = "+value+"\n")
		if err == nil || !strings.Contains(err.Error(), "ui.note_timeout must be at least 1") {
			t.Errorf("value %s: err = %v, want \"ui.note_timeout must be at least 1\"", value, err)
		}
	}
}
