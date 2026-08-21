package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// TestWriteConfigFileRoundTripsUnknownKeysAndSections proves the writer
// (task 012) leaves everything it does not understand exactly as it found
// it: an unrecognised top-level key, an unrecognised [ui] key, and a whole
// unrecognised section family ([notify] and a repeated [[notify.rule]]
// table, SPEC §10) all survive a write that changes only a known field.
func TestWriteConfigFileRoundTripsUnknownKeysAndSections(t *testing.T) {
	original := "" +
		"allow_yolo = false\n" +
		"some_future_key = \"whatever\"\n" +
		"\n" +
		"[ui]\n" +
		"mouse = true\n" +
		"some_future_ui_key = 7\n" +
		"\n" +
		"[notify]\n" +
		"webhook = \"https://example.invalid/hook\"\n" +
		"\n" +
		"[[notify.rule]]\n" +
		"match = \"idle\"\n" +
		"\n" +
		"[[notify.rule]]\n" +
		"match = \"error\"\n"

	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := loadConfigFile(path)
	if err != nil {
		t.Fatal(err)
	}
	cfg.AllowYolo = true // the one field this test changes

	if err := WriteConfigFile(path, cfg); err != nil {
		t.Fatal(err)
	}

	written, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"some_future_key = \"whatever\"",
		"some_future_ui_key = 7",
		"[notify]",
		"webhook = \"https://example.invalid/hook\"",
		"[[notify.rule]]",
		"match = \"idle\"",
		"match = \"error\"",
	} {
		if !strings.Contains(string(written), want) {
			t.Fatalf("write dropped or altered unknown content %q; got:\n%s", want, written)
		}
	}
	if !strings.Contains(string(written), "allow_yolo = true") {
		t.Fatalf("write did not apply the changed allow_yolo field; got:\n%s", written)
	}

	// The two [[notify.rule]] occurrences must both still be present, not
	// merged or deduplicated by treating them as the same section.
	if n := strings.Count(string(written), "[[notify.rule]]"); n != 2 {
		t.Fatalf("[[notify.rule]] occurs %d times after write, want 2; got:\n%s", n, written)
	}
}

// TestWriteConfigFileParsedContentMatchesWrite proves a written file, when
// read back through this package's own reader, yields exactly what was
// written -- not just that some bytes landed on disk.
func TestWriteConfigFileParsedContentMatchesWrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")

	cfg, err := loadConfigFile(path) // file does not exist yet: schema defaults
	if err != nil {
		t.Fatal(err)
	}
	cfg.AllowYolo = true
	cfg.StaleAfter = 90 * time.Second
	cfg.CaptureMinInterval = 12 * time.Second
	cfg.ASCII = true
	cfg.Mouse = false
	cfg.RecentCwdLimit = 9
	cfg.Theme = "solarized"
	cfg.Env = map[string]string{"FOO": "bar", "BAZ": "qux"}

	if err := WriteConfigFile(path, cfg); err != nil {
		t.Fatal(err)
	}

	reread, err := loadConfigFile(path)
	if err != nil {
		t.Fatalf("written config.toml did not parse: %v", err)
	}
	if !reflect.DeepEqual(reread, cfg) {
		t.Fatalf("re-read config = %+v, want %+v", reread, cfg)
	}
}

// TestWriteConfigFileFailureLeavesPreviousFileIntact proves an atomic
// writer's whole point: a write that cannot complete never leaves
// config.toml half-written or unparseable, because the previous file is
// only ever replaced by an atomic rename of a fully-written temp file.
func TestWriteConfigFileFailureLeavesPreviousFileIntact(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	original := "allow_yolo = true\nstale_after = 30\n"
	if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := loadConfigFile(path)
	if err != nil {
		t.Fatal(err)
	}
	cfg.AllowYolo = false

	// Deny write access to the directory so the writer cannot create its
	// temp file: the failure must happen before any change reaches path.
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(dir, 0o700)

	if err := WriteConfigFile(path, cfg); err == nil {
		os.Chmod(dir, 0o700)
		t.Fatal("write into a read-only directory unexpectedly succeeded")
	}
	os.Chmod(dir, 0o700)

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != original {
		t.Fatalf("previous file changed after a failed write: got %q, want %q", after, original)
	}
	reread, err := loadConfigFile(path)
	if err != nil {
		t.Fatalf("previous file no longer parses after a failed write: %v", err)
	}
	if !reread.AllowYolo {
		t.Fatal("previous file's allow_yolo=true was lost after a failed write")
	}
}

// TestWriteConfigFileAddsMissingUISection proves that a field with no
// existing [ui] table at all still gets written: the writer creates the
// section rather than silently dropping the field.
func TestWriteConfigFileAddsMissingUISection(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte("allow_yolo = true\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := loadConfigFile(path)
	if err != nil {
		t.Fatal(err)
	}
	cfg.RecentCwdLimit = 20

	if err := WriteConfigFile(path, cfg); err != nil {
		t.Fatal(err)
	}
	reread, err := loadConfigFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if reread.RecentCwdLimit != 20 {
		t.Fatalf("RecentCwdLimit after adding a fresh [ui] section = %d, want 20", reread.RecentCwdLimit)
	}
	if !reread.AllowYolo {
		t.Fatal("pre-existing allow_yolo = true was lost when [ui] was created")
	}
}

// TestWriteConfigFileEnvDropsRemovedKeyAddsNewKey proves the [env] table
// is fully owned by cfg.Env: a key removed from the map disappears from
// the file, and a newly added key is written.
func TestWriteConfigFileEnvDropsRemovedKeyAddsNewKey(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	original := "[env]\nFOO = \"one\"\nGONE = \"two\"\n"
	if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := loadConfigFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Env["GONE"] != "two" {
		t.Fatalf("precondition: GONE should have loaded as %q, got %q", "two", cfg.Env["GONE"])
	}
	delete(cfg.Env, "GONE")
	cfg.Env["NEW"] = "three"

	if err := WriteConfigFile(path, cfg); err != nil {
		t.Fatal(err)
	}
	reread, err := loadConfigFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, present := reread.Env["GONE"]; present {
		t.Fatal("GONE should have been dropped from [env] after removal from cfg.Env")
	}
	if reread.Env["FOO"] != "one" {
		t.Fatalf("FOO = %q, want %q (untouched)", reread.Env["FOO"], "one")
	}
	if reread.Env["NEW"] != "three" {
		t.Fatalf("NEW = %q, want %q (newly added)", reread.Env["NEW"], "three")
	}
}
