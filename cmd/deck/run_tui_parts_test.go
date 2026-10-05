package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/config"
)

// TestAbsoluteExecutableIsTheAbsolutePathOfTheRunningBinary pins what the
// service is handed as DeckExecutable: an absolute path naming this very
// process's executable.
func TestAbsoluteExecutableIsTheAbsolutePathOfTheRunningBinary(t *testing.T) {
	got, err := absoluteExecutable()
	if err != nil {
		t.Fatal(err)
	}
	if !filepath.IsAbs(got) {
		t.Fatalf("absoluteExecutable() = %q, want an absolute path", got)
	}
	raw, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Clean(raw); filepath.Clean(got) != want {
		t.Fatalf("absoluteExecutable() = %q, want %q", got, want)
	}
}

// TestTUIProgramOptionsAddMouseReportingOnlyWhenEnabled pins requirement 3's
// gate: the alternate screen is always requested and mouse reporting adds
// exactly one more option, only when settings.Mouse is on.
func TestTUIProgramOptionsAddMouseReportingOnlyWhenEnabled(t *testing.T) {
	extra := len(rawByteCountingProgramOptions())
	if got, want := len(tuiProgramOptions(config.Settings{Mouse: false})), 1+extra; got != want {
		t.Fatalf("mouse off: %d options, want %d", got, want)
	}
	if got, want := len(tuiProgramOptions(config.Settings{Mouse: true})), 2+extra; got != want {
		t.Fatalf("mouse on: %d options, want %d", got, want)
	}
}

// TestTouchLastUsedBestEffortReportsButNeverStopsTheLaunch pins SPEC §3.4's
// marker behaviour at launch: a writable data root gets its last_used
// marker and says nothing, an unwritable one is reported on stderr.
func TestTouchLastUsedBestEffortReportsButNeverStopsTheLaunch(t *testing.T) {
	root := t.TempDir()
	good := config.Settings{Paths: config.Paths{DataDir: filepath.Join(root, "data")}}
	var quiet bytes.Buffer
	touchLastUsedBestEffort(good, &quiet)
	if quiet.Len() != 0 {
		t.Fatalf("writable data root reported %q, want silence", quiet.String())
	}
	if entries, err := os.ReadDir(good.Paths.DataDir); err != nil || len(entries) != 1 {
		t.Fatalf("data dir after touch = %v (err %v), want exactly the marker", entries, err)
	}

	blocker := filepath.Join(root, "file")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	bad := config.Settings{Paths: config.Paths{DataDir: filepath.Join(blocker, "data")}}
	var loud bytes.Buffer
	touchLastUsedBestEffort(bad, &loud)
	if !strings.HasPrefix(loud.String(), "deck last used: ") {
		t.Fatalf("unwritable data root reported %q, want a \"deck last used:\" line", loud.String())
	}
}

// TestReclaimLeakedPipesBestEffortReportsButNeverStopsTheLaunch pins the
// startup reclaim's best-effort shape: an empty temp root reclaims nothing
// and says nothing, a temp root that cannot be scanned (here a regular
// file) is reported on stderr.
func TestReclaimLeakedPipesBestEffortReportsButNeverStopsTheLaunch(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	var quiet bytes.Buffer
	reclaimLeakedPipesBestEffort(&quiet)
	if quiet.Len() != 0 {
		t.Fatalf("empty temp root reported %q, want silence", quiet.String())
	}

	file := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TMPDIR", file)
	var loud bytes.Buffer
	reclaimLeakedPipesBestEffort(&loud)
	if !strings.HasPrefix(loud.String(), "deck interactive pipe reclaim: ") {
		t.Fatalf("unscannable temp root reported %q, want a \"deck interactive pipe reclaim:\" line", loud.String())
	}
}
