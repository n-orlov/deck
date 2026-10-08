package notify

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestProbeScriptNamesEachProblem(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "hook.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	plain := filepath.Join(dir, "plain.sh")
	if err := os.WriteFile(plain, []byte("#!/bin/sh\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	cases := []struct {
		name    string
		command []string
		want    error
	}{
		{"unset", nil, ErrScriptNotSet},
		{"blank", []string{" "}, ErrScriptNotSet},
		{"executable path", []string{script, "--flag"}, nil},
		{"missing path", []string{filepath.Join(dir, "gone.sh")}, ErrScriptMissing},
		{"not executable", []string{plain}, ErrScriptNotExecutable},
		{"directory", []string{dir}, ErrScriptNotExecutable},
		{"bare name on PATH", []string{"hook.sh"}, nil},
		{"bare name not on PATH", []string{"nope.sh"}, ErrScriptMissing},
		{"bare name found but not executable", []string{"plain.sh"}, ErrScriptMissing},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ProbeScript(c.command); !errors.Is(got, c.want) {
				t.Fatalf("ProbeScript(%v) = %v, want %v", c.command, got, c.want)
			}
		})
	}
}
