package features

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cucumber/godog"
)

// registerNoLeakScanSteps wires task 011's (I-7 assertion half) no-leak scan
// instrument. It is deliberately general on two axes at once: it walks
// EVERY cell of the live emulator grid rather than substring-searching
// Frame()'s NormalizeFrame-trimmed string (the same blind spot CellAt's own
// doc comment names), and it walks EVERY regular file under a scenario's
// DECK_HOME rather than a fixed list of three named files -- a JSONL log, a
// launch audit file, a store, a crash tail column, a config file are all
// just files under that tree, and a future file deck starts writing is
// covered for free. Every step below takes only a VALUE, never a file name.
func registerNoLeakScanSteps(sc *godog.ScenarioContext) {
	sc.Step(`^the leak scan for "([^"]+)" against deck client "([^"]+)" finds it on screen$`, leakScanFindsItOnScreen)
	sc.Step(`^the leak scan for "([^"]+)" against deck client "([^"]+)" finds it in a home directory file$`, leakScanFindsItInHomeFile)
	sc.Step(`^deck client "([^"]+)" screen grid never contains "([^"]+)"$`, screenGridNeverContains)
	sc.Step(`^no file under deck client "([^"]+)" home directory, other than the state database, ever contains "([^"]+)"$`, homeFilesOtherThanStateDBNeverContain)
}

// gridText renders every cell of the emulator's live grid into one string,
// row by row, reading CellAt directly rather than Frame()'s
// NormalizeFrame-trimmed string. NormalizeFrame trims trailing blanks per
// row; a value that content pushed past where a plain substring search over
// Frame() happened to look would be invisible to that search but is still
// sitting in a real cell -- exactly the gap CellAt's own doc comment
// describes for a different bug shape. Reading every cell directly has no
// such blind spot.
func (d *ScreenDriver) gridText() string {
	cols, rows := d.GridSize()
	var b strings.Builder
	for y := 0; y < rows; y++ {
		for x := 0; x < cols; x++ {
			if cell := d.CellAt(x, y); cell != nil {
				b.WriteString(cell.Content)
			}
		}
		b.WriteByte('\n')
	}
	return b.String()
}

// homeFileLeaks walks every regular file under home and returns the
// home-relative path of every one whose raw bytes contain value, in walk
// order. It never special-cases which files exist: a future file deck
// starts writing is scanned the same as config.toml, log/deck.jsonl or
// state.db are today.
func homeFileLeaks(home, value string) ([]string, error) {
	var leaks []string
	err := filepath.WalkDir(home, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read %q: %w", path, err)
		}
		if strings.Contains(string(data), value) {
			rel, relErr := filepath.Rel(home, path)
			if relErr != nil {
				rel = path
			}
			leaks = append(leaks, filepath.ToSlash(rel))
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return leaks, nil
}

// leakScanFindsItOnScreen is the grid half of the scan's own positive
// control: it fails unless value is actually present somewhere in the live
// grid, proving the scan mechanism itself can find a value it is pointed
// at, so a later "never contains" assertion using the same mechanism means
// something rather than passing vacuously because the scan never looks
// anywhere real.
func leakScanFindsItOnScreen(ctx context.Context, value, clientName string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	client, err := h.Client(clientName)
	if err != nil {
		return err
	}
	if !strings.Contains(client.gridText(), value) {
		return fmt.Errorf("leak scan positive control failed: %q was not found anywhere in deck client %q's screen grid -- the scan instrument itself is not finding real, present values", value, clientName)
	}
	return nil
}

// leakScanFindsItInHomeFile is homeFileLeaks' own positive control: it
// fails unless value is actually present in at least one file under the
// scenario's DECK_HOME.
func leakScanFindsItInHomeFile(ctx context.Context, value, clientName string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	if _, err := h.Client(clientName); err != nil {
		return err
	}
	leaks, err := homeFileLeaks(h.Home, value)
	if err != nil {
		return err
	}
	if len(leaks) == 0 {
		return fmt.Errorf("leak scan positive control failed: %q was not found in any file under DECK_HOME %q -- the scan instrument itself is not finding real, present values", value, h.Home)
	}
	return nil
}

// screenGridNeverContains is the general "no leak on screen" negative
// assertion, built on the same gridText scan the positive control above
// proves works.
func screenGridNeverContains(ctx context.Context, clientName, value string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	client, err := h.Client(clientName)
	if err != nil {
		return err
	}
	if strings.Contains(client.gridText(), value) {
		return fmt.Errorf("deck client %q's screen grid unexpectedly contains %q", clientName, value)
	}
	return nil
}

// homeFilesOtherThanStateDBNeverContain is the general "no leak on disk"
// negative assertion. state.db (and, since SQLite's default journal mode is
// WAL, its state.db-wal/-shm/-journal siblings, all still holding the same
// not-yet-checkpointed row) is named as the one explicit, deliberate
// exception rather than silently skipped: a session's own env map is
// legitimately stored there in plaintext because deck needs the real value
// to relaunch the pane (SPEC §6.4 masking is a DISPLAY/audit-log contract,
// not an at-rest encryption one). Every other file the walk finds --
// including ones this scenario never names -- must not carry the value.
func homeFilesOtherThanStateDBNeverContain(ctx context.Context, clientName, value string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	if _, err := h.Client(clientName); err != nil {
		return err
	}
	leaks, err := homeFileLeaks(h.Home, value)
	if err != nil {
		return err
	}
	var unexpected []string
	for _, rel := range leaks {
		if rel == "state.db" || strings.HasPrefix(rel, "state.db-") {
			continue
		}
		unexpected = append(unexpected, rel)
	}
	if len(unexpected) > 0 {
		return fmt.Errorf("file(s) %v under DECK_HOME unexpectedly contain %q (only state.db and its WAL siblings may legitimately hold a session's own operational env value)", unexpected, value)
	}
	return nil
}

// TestHomeFileLeaksFindsAnUnmaskedControlValueAndIgnoresAnAbsentOne is the
// file-scan half of task 011's own positive control, in isolation from the
// deck binary: it proves homeFileLeaks actually reads bytes off disk and
// finds a value planted in a file nested under a subdirectory (not just a
// file directly in home), and that it reports nothing for a value that
// truly is not present anywhere in the tree. A file-scan helper that always
// returned "found" or always returned "not found" would pass a test that
// only checked one of these; this checks both.
func TestHomeFileLeaksFindsAnUnmaskedControlValueAndIgnoresAnAbsentOne(t *testing.T) {
	home := t.TempDir()
	nested := filepath.Join(home, "log")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatalf("mkdir nested dir: %v", err)
	}
	planted := filepath.Join(nested, "deck.jsonl")
	control := "no-leak-file-scan-control-71a2e9"
	if err := os.WriteFile(planted, []byte(`{"msg":"`+control+`"}`+"\n"), 0o644); err != nil {
		t.Fatalf("plant control value: %v", err)
	}
	clean := filepath.Join(home, "config.toml")
	if err := os.WriteFile(clean, []byte("allow_yolo = false\n"), 0o644); err != nil {
		t.Fatalf("write clean file: %v", err)
	}

	leaks, err := homeFileLeaks(home, control)
	if err != nil {
		t.Fatalf("homeFileLeaks: %v", err)
	}
	if len(leaks) != 1 || leaks[0] != "log/deck.jsonl" {
		t.Fatalf("homeFileLeaks(planted control) = %v, want exactly [\"log/deck.jsonl\"] -- the scan instrument is not finding a real, present value", leaks)
	}

	absent := "no-leak-file-scan-absent-3d84f1"
	leaks, err = homeFileLeaks(home, absent)
	if err != nil {
		t.Fatalf("homeFileLeaks: %v", err)
	}
	if len(leaks) != 0 {
		t.Fatalf("homeFileLeaks(absent value) = %v, want none -- the scan instrument is reporting a leak that is not there", leaks)
	}
}
