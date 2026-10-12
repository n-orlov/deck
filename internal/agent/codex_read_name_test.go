package agent

import (
	"bytes"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const codexTestID = "019e0000-aaaa-bbbb-cccc-000000000001"

// codexFixture builds a CODEX_HOME holding a state_5.sqlite created with
// ddl and the given statements. It returns the home and the database path.
func codexFixture(t *testing.T, ddl string, stmts ...string) (string, string) {
	t.Helper()
	home := t.TempDir()
	path := filepath.Join(home, codexStateFile)
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	for _, stmt := range append([]string{ddl}, stmts...) {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("fixture %q: %v", stmt, err)
		}
	}
	return home, path
}

const codexThreadsDDL = `CREATE TABLE threads (id TEXT PRIMARY KEY, name TEXT, rollout_path TEXT)`

func codexProbe(home, title string) NameProbeInput {
	return NameProbeInput{
		Env: map[string]string{"CODEX_HOME": home}, CWD: "/work/api", ConversationID: codexTestID,
		PaneTitle: func() string { return title },
	}
}

func captureDegrades(t *testing.T) *[]string {
	t.Helper()
	var logged []string
	SetNameDegradeLog(func(msg string) { logged = append(logged, msg) })
	t.Cleanup(func() { SetNameDegradeLog(nil) })
	return &logged
}

func TestCodexReadNameFromThreadsTable(t *testing.T) {
	logged := captureDegrades(t)
	home, _ := codexFixture(t, codexThreadsDDL,
		`INSERT INTO threads VALUES ('`+codexTestID+`', 'Write flaky test haiku', '/r.jsonl')`,
		`INSERT INTO threads VALUES ('other', 'Another thread', '/o.jsonl')`)
	name, explicit, ok := NewCodex().ReadName(codexProbe(home, "never read"))
	if name != "Write flaky test haiku" || explicit || !ok {
		t.Fatalf("ReadName = (%q, %v, %v), want the generated threads.name", name, explicit, ok)
	}
	if len(*logged) != 0 {
		t.Fatalf("a healthy database logged %v", *logged)
	}
}

func TestCodexReadNameNullNameMeansNoneYet(t *testing.T) {
	logged := captureDegrades(t)
	home, _ := codexFixture(t, codexThreadsDDL, `INSERT INTO threads VALUES ('`+codexTestID+`', NULL, '/r')`)
	name, _, ok := NewCodex().ReadName(codexProbe(home, "Pane Name | api"))
	if ok || name != "" {
		t.Fatalf("NULL name = (%q, %v), want no name (and no pane-title fallback)", name, ok)
	}
	if len(*logged) != 0 {
		t.Fatalf("a NULL name is not a degrade, logged %v", *logged)
	}
}

func TestCodexReadNameUnknownThreadIsNoName(t *testing.T) {
	home, _ := codexFixture(t, codexThreadsDDL, `INSERT INTO threads VALUES ('other', 'x', '/r')`)
	if name, _, ok := NewCodex().ReadName(codexProbe(home, "")); ok || name != "" {
		t.Fatalf("a thread without a row = (%q, %v), want no name", name, ok)
	}
}

// The three broken shapes degrade to the pane title, log once, and (with no
// usable pane title) keep the dummy name.
func TestCodexReadNameDegradesToPaneTitle(t *testing.T) {
	shapes := map[string]func(t *testing.T) string{
		"missing database": func(t *testing.T) string { return t.TempDir() },
		"missing threads table": func(t *testing.T) string {
			home, _ := codexFixture(t, `CREATE TABLE other (id TEXT)`)
			return home
		},
		"changed column shape": func(t *testing.T) string {
			home, _ := codexFixture(t, `CREATE TABLE threads (thread_id TEXT, title TEXT)`)
			return home
		},
	}
	for shape, build := range shapes {
		t.Run(shape, func(t *testing.T) {
			logged := captureDegrades(t)
			home := build(t)
			adapter := NewCodex()
			name, explicit, ok := adapter.ReadName(codexProbe(home, "⠋ Fix the parser | api"))
			if name != "Fix the parser" || explicit || !ok {
				t.Fatalf("pane-title fallback = (%q, %v, %v), want Fix the parser", name, explicit, ok)
			}
			if name, _, ok := adapter.ReadName(codexProbe(home, "host-name")); ok || name != "" {
				t.Fatalf("an unusable pane title = (%q, %v), want no name (dummy name kept)", name, ok)
			}
			if len(*logged) != 1 || !strings.HasPrefix((*logged)[0], "codex name: ") {
				t.Fatalf("degrade logged %q, want exactly one line over three reads", *logged)
			}
		})
	}
}

func TestCodexReadNameNeverWritesTheDatabase(t *testing.T) {
	home, path := codexFixture(t, codexThreadsDDL, `INSERT INTO threads VALUES ('`+codexTestID+`', 'Named', '/r')`)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for range 3 {
		NewCodex().ReadName(codexProbe(home, ""))
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("state_5.sqlite changed after ReadName")
	}
	if entries, _ := filepath.Glob(path + "-*"); len(entries) != 0 {
		t.Fatalf("ReadName left sidecar files %v", entries)
	}
}

// A database that is not writable at all proves the open is read-only: a
// read-write open of a read-only file would fail, mode=ro must not.
func TestCodexReadNameOpensReadOnly(t *testing.T) {
	home, path := codexFixture(t, codexThreadsDDL, `INSERT INTO threads VALUES ('`+codexTestID+`', 'Named', '/r')`)
	if err := os.Chmod(path, 0o400); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(home, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(home, 0o700) })
	name, _, ok := NewCodex().ReadName(codexProbe(home, ""))
	if name != "Named" || !ok {
		t.Fatalf("read-only database = (%q, %v), want Named", name, ok)
	}
}

func TestCodexReadNameDSNIsReadOnlyWithShortBusyTimeout(t *testing.T) {
	dsn := codexDSN("/h/.codex/state_5.sqlite")
	for _, want := range []string{"file:///h/.codex/state_5.sqlite?", "mode=ro", "busy_timeout%28250%29"} {
		if !strings.Contains(dsn, want) {
			t.Errorf("dsn %q lacks %q", dsn, want)
		}
	}
	// A read-write open would create the missing file; mode=ro must not.
	missing := filepath.Join(t.TempDir(), codexStateFile)
	if _, _, reason := queryCodexThread(missing, codexTestID); reason == "" {
		t.Fatal("a missing database was reported healthy")
	}
	if _, err := os.Stat(missing); err == nil {
		t.Fatal("ReadName created the missing database")
	}
}

func TestCodexReadNameExplicitHeuristic(t *testing.T) {
	for _, tc := range []struct {
		name, adopted string
		explicit      bool
	}{
		{"Generated", "", false},
		{"Generated", "Generated", false},
		{"Generated", "Generated-2", false},
		{"my own", "Generated", true},
	} {
		home, _ := codexFixture(t, codexThreadsDDL, `INSERT INTO threads VALUES ('`+codexTestID+`', '`+tc.name+`', '/r')`)
		in := codexProbe(home, "")
		in.AdoptedName = tc.adopted
		if _, explicit, ok := NewCodex().ReadName(in); !ok || explicit != tc.explicit {
			t.Errorf("name %q adopted %q: explicit = %v, want %v", tc.name, tc.adopted, explicit, tc.explicit)
		}
	}
}

func TestCodexReadNameUnsafeIDIsNoName(t *testing.T) {
	home, _ := codexFixture(t, codexThreadsDDL)
	in := codexProbe(home, "")
	in.ConversationID = "../x"
	if _, _, ok := NewCodex().ReadName(in); ok {
		t.Fatal("an unsafe conversation id read a name")
	}
}

// TestCodexReadNameCapsAnOversizeThreadName: a threads.name far past the cap
// is cut in the query (substr) and comes back sanitised and cut to
// MaxHarnessNameRunes, with escapes and controls removed (SPEC §8, R273).
func TestCodexReadNameCapsAnOversizeThreadName(t *testing.T) {
	captureDegrades(t)
	huge := "\x1b[31m  Fix\tthe\nparser " + strings.Repeat("é", 1<<17)
	home, _ := codexFixture(t, codexThreadsDDL,
		`INSERT INTO threads VALUES ('`+codexTestID+`', '`+huge+`', '/r.jsonl')`)
	name, _, ok := NewCodex().ReadName(codexProbe(home, "never read"))
	want := "Fix the parser " + strings.Repeat("é", MaxHarnessNameRunes-len([]rune("Fix the parser ")))
	if !ok || name != want {
		t.Fatalf("ReadName = (%d runes, ok=%v), want the %d-rune sanitised cut", len([]rune(name)), ok, MaxHarnessNameRunes)
	}
}
