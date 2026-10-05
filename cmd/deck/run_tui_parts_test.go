package main

import (
	"os"
	"path/filepath"
	"testing"

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
