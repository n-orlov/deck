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

func TestProbeScriptRefusesARelativePathBeforeLookingAtTheDisk(t *testing.T) {
	for _, command := range [][]string{{"./hook.sh"}, {"scripts/hook.sh"}, {"~/hook.sh"}, {"../x/hook.sh"}} {
		if got := ProbeScript(command); !errors.Is(got, ErrScriptRelative) {
			t.Errorf("ProbeScript(%v) = %v, want ErrScriptRelative", command, got)
		}
		if !RelativeScript(command) {
			t.Errorf("RelativeScript(%v) = false", command)
		}
	}
	for _, command := range [][]string{nil, {"/abs/hook.sh"}, {"hook.sh"}} {
		if RelativeScript(command) {
			t.Errorf("RelativeScript(%v) = true, want false (absolute, bare name or empty)", command)
		}
	}
}
