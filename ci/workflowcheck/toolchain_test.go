// toolchain_test.go is task 012's probe (R197, #61): go.mod's `go 1.25.0`
// line only names the language floor, and actions/setup-go reads the
// version it installs from go.mod -- so without a `toolchain go1.25.N`
// line, CI and release builds resolve to 1.25.0 and miss every later
// 1.25.x security fix. The probe fails the moment the toolchain line is
// removed (or names 1.25.0 / an unparsable version), and the moment a
// ci.yml setup-go step could select 1.25.0 anyway.
//
// setup-go (v7 parseGoVersionFile) prefers go.mod's `toolchain` directive
// over its `go` directive unless GOTOOLCHAIN=local is set in the step's
// environment; probeSetupGoVersion mirrors that rule, so a ci.yml that
// pins `go-version: 1.25.0`, points go-version-file somewhere without the
// directive, or exports GOTOOLCHAIN=local is caught.
package workflowcheck

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

var (
	toolchainLineRE = regexp.MustCompile(`(?m)^toolchain go(1\.\d+(?:\.\d+|rc\d+)?)`)
	goLineRE        = regexp.MustCompile(`(?m)^go (\d+(?:\.\d+)*)`)
	patchVersionRE  = regexp.MustCompile(`^1\.(\d+)\.(\d+)$`)
)

// toolchainVersion extracts the version a go.mod's toolchain line names
// ("" when absent) the way setup-go does.
func toolchainVersion(gomod string) string {
	if m := toolchainLineRE.FindStringSubmatch(gomod); m != nil {
		return m[1]
	}
	return ""
}

// probeSetupGoVersion returns the Go version setup-go would install for
// one step's `with:` inputs and environment, given go.mod's contents.
// versionFiles maps a go-version-file path to its contents.
func probeSetupGoVersion(with, env map[string]string, versionFiles map[string]string) string {
	if v := with["go-version"]; v != "" {
		return v
	}
	contents := versionFiles[with["go-version-file"]]
	if env["GOTOOLCHAIN"] != "local" {
		if v := toolchainVersion(contents); v != "" {
			return v
		}
	}
	if m := goLineRE.FindStringSubmatch(contents); m != nil {
		return m[1]
	}
	return ""
}

// isFloorRelease reports whether a resolved version is a bare
// 1.25 line (an unpatched "1.25" resolves to the latest, but "1.25.0"
// and a missing patch from the go directive are the floor the task
// exists to avoid).
func isFloorRelease(v string) bool {
	m := patchVersionRE.FindStringSubmatch(v)
	if m == nil {
		return false
	}
	patch, _ := strconv.Atoi(m[2])
	return patch == 0
}

type setupGoStep struct {
	Name string            `yaml:"name"`
	Uses string            `yaml:"uses"`
	With map[string]string `yaml:"with"`
	Env  map[string]string `yaml:"env"`
}

type setupGoJob struct {
	Env   map[string]string `yaml:"env"`
	Steps []setupGoStep     `yaml:"steps"`
}

type setupGoWorkflow struct {
	Env  map[string]string `yaml:"env"`
	Jobs map[string]setupGoJob
}

func readRepoFile(t *testing.T, rel ...string) string {
	t.Helper()
	root, err := repositoryRoot()
	if err != nil {
		t.Fatalf("repositoryRoot: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(append([]string{root}, rel...)...))
	if err != nil {
		t.Fatalf("read %v: %v", rel, err)
	}
	return string(raw)
}

func TestGoModCarriesPatchedToolchainLine(t *testing.T) {
	gomod := readRepoFile(t, "go.mod")
	v := toolchainVersion(gomod)
	if v == "" {
		t.Fatalf("go.mod has no `toolchain go1.25.N` line: setup-go falls back to the `go` directive (1.25.0)")
	}
	if !strings.HasPrefix(v, "1.25.") || !patchVersionRE.MatchString(v) {
		t.Fatalf("go.mod toolchain line names %q, want a 1.25.N patch release", v)
	}
	if isFloorRelease(v) {
		t.Fatalf("go.mod toolchain line names %q: the floor release misses every later 1.25.x fix", v)
	}
}

func TestCISetupGoCannotSelectFloorRelease(t *testing.T) {
	gomod := readRepoFile(t, "go.mod")
	var wf setupGoWorkflow
	if err := yaml.Unmarshal([]byte(readRepoFile(t, ".github", "workflows", "ci.yml")), &wf); err != nil {
		t.Fatalf("parse ci.yml: %v", err)
	}
	seen := 0
	for jobName, job := range wf.Jobs {
		for _, s := range job.Steps {
			if !strings.HasPrefix(s.Uses, "actions/setup-go@") {
				continue
			}
			seen++
			env := map[string]string{}
			for _, layer := range []map[string]string{wf.Env, job.Env, s.Env} {
				for k, v := range layer {
					env[k] = v
				}
			}
			got := probeSetupGoVersion(s.With, env, map[string]string{"go.mod": gomod})
			if got == "" || isFloorRelease(got) || !patchVersionRE.MatchString(got) {
				t.Errorf("ci.yml job %q setup-go step resolves to %q, want a patched 1.25.N release (not 1.25.0)", jobName, got)
			}
		}
	}
	if seen == 0 {
		t.Fatalf("ci.yml has no actions/setup-go step: the probe would pass vacuously")
	}
}

// TestProbeSetupGoVersionRejectsFloorSelections pins the probe itself:
// each way a step could land on 1.25.0 must be reported as one.
func TestProbeSetupGoVersionRejectsFloorSelections(t *testing.T) {
	withTC := "module m\n\ngo 1.25.0\n\ntoolchain go1.25.14\n"
	noTC := "module m\n\ngo 1.25.0\n"
	files := func(c string) map[string]string { return map[string]string{"go.mod": c} }
	file := map[string]string{"go-version-file": "go.mod"}

	if got := probeSetupGoVersion(file, nil, files(withTC)); got != "1.25.14" {
		t.Errorf("toolchain line honoured: got %q, want 1.25.14", got)
	}
	cases := []struct {
		name string
		with map[string]string
		env  map[string]string
		mod  string
	}{
		{"toolchain line removed", file, nil, noTC},
		{"GOTOOLCHAIN=local ignores the line", file, map[string]string{"GOTOOLCHAIN": "local"}, withTC},
		{"explicit go-version 1.25.0", map[string]string{"go-version": "1.25.0"}, nil, withTC},
	}
	for _, c := range cases {
		got := probeSetupGoVersion(c.with, c.env, files(c.mod))
		if !isFloorRelease(got) {
			t.Errorf("%s: resolved %q, want it flagged as the 1.25.0 floor", c.name, got)
		}
	}
}
