package main

import (
	"os"
	"path/filepath"
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
