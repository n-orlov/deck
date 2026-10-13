package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// report builds a quality-report.txt with the named gate sections.
func report(sections map[string]bool) string {
	names := make([]string, 0, len(sections))
	for n := range sections {
		names = append(names, n)
	}
	sort.Strings(names)
	var b strings.Builder
	for _, n := range names {
		fmt.Fprintf(&b, "=== %s gate ===\nsome output\n", n)
		if sections[n] {
			b.WriteString("what to do: nothing -- all clear.\n")
		} else {
			b.WriteString("what to do: fix each finding at its root.\n")
		}
	}
	return b.String()
}

func junit(tests, failures, errs int) string {
	return fmt.Sprintf("<?xml version=\"1.0\"?>\n<testsuites tests=\"%d\" failures=\"%d\" errors=\"%d\"></testsuites>\n", tests, failures, errs)
}

// profile builds a coverprofile whose product statements are pct percent
// covered out of 100, plus a fully uncovered cmd/fake-* package that must
// not count.
func profile(covered int) string {
	return fmt.Sprintf("mode: set\nexample.com/deck/internal/a/a.go:1.1,2.2 %d 1\nexample.com/deck/internal/a/a.go:3.1,4.2 %d 0\nexample.com/deck/cmd/fake-x/x.go:1.1,2.2 50 0\n", covered, 100-covered)
}

const garbage = "\x00\x01 this is not a result file <<<\n"

// fixtures maps a gate name to the content that file would hold for a
// passing and a failing run.
var passing = map[string]string{
	"lint":            report(map[string]bool{"golangci": true}),
	"unit-tests":      junit(10, 0, 0),
	"features":        junit(4, 0, 0),
	"coverage":        profile(90),
	"vulnerabilities": report(map[string]bool{"govulncheck": true, "trivy": true}),
}

var failing = map[string]string{
	"lint":            report(map[string]bool{"golangci": false}),
	"unit-tests":      junit(10, 1, 0),
	"features":        junit(4, 0, 2),
	"coverage":        profile(60),
	"vulnerabilities": report(map[string]bool{"govulncheck": false, "trivy": true}),
}

func gateByName(t *testing.T, name string) gate {
	t.Helper()
	for _, g := range gates {
		if g.name == name {
			return g
		}
	}
	t.Fatalf("no gate named %q", name)
	return gate{}
}

// renderOne writes content (when non-nil) as the gate's input file in a
// fresh directory and returns the gate's badge.
func renderOne(t *testing.T, name string, content *string) badge {
	t.Helper()
	g := gateByName(t, name)
	dir := t.TempDir()
	if content != nil {
		p := filepath.Join(dir, filepath.FromSlash(g.input))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(*content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return g.render(dir)
}

func TestBadgeGenTable(t *testing.T) {
	type row struct {
		name    string
		content map[string]string // per gate; nil = file missing
		colors  string
		message string // "" = not checked
	}
	rows := []row{
		{"gate passed", passing, colorGreen, ""},
		{"gate failed", failing, colorRed, ""},
		{"result file missing", nil, colorGrey, "unknown"},
		{"result file unparsable", map[string]string{
			"lint": garbage, "unit-tests": garbage, "features": garbage, "coverage": garbage, "vulnerabilities": garbage,
		}, colorGrey, "unknown"},
	}
	for _, r := range rows {
		for _, g := range gates {
			t.Run(r.name+"/"+g.name, func(t *testing.T) {
				var content *string
				if c, ok := r.content[g.name]; ok {
					content = &c
				}
				b := renderOne(t, g.name, content)
				if b.Color != r.colors {
					t.Fatalf("color = %q, want %q (message %q)", b.Color, r.colors, b.Message)
				}
				if r.message != "" && b.Message != r.message {
					t.Fatalf("message = %q, want %q", b.Message, r.message)
				}
			})
		}
	}

	// Coverage one point below and one point above each threshold constant.
	cov := []struct {
		name  string
		pct   float64
		color string
	}{
		{"one point below the green threshold", coverageGreenMin - 1, colorYellow},
		{"one point above the green threshold", coverageGreenMin + 1, colorGreen},
		{"one point below the yellow threshold", coverageYellowMin - 1, colorRed},
		{"one point above the yellow threshold", coverageYellowMin + 1, colorYellow},
	}
	for _, c := range cov {
		t.Run("coverage "+c.name, func(t *testing.T) {
			content := profile(int(c.pct))
			b := renderOne(t, "coverage", &content)
			if b.Color != c.color {
				t.Fatalf("coverage %.0f%%: color = %q, want %q", c.pct, b.Color, c.color)
			}
			if want := fmt.Sprintf("%.1f%%", c.pct); b.Message != want {
				t.Fatalf("message = %q, want %q", b.Message, want)
			}
		})
	}
}

func TestBadgeNeverGreenOnBadInput(t *testing.T) {
	bad := map[string]map[string]string{
		"missing":    nil,
		"unparsable": {"lint": garbage, "unit-tests": garbage, "features": garbage, "coverage": garbage, "vulnerabilities": garbage},
		"failed":     failing,
		"empty":      {"lint": "", "unit-tests": "", "features": "", "coverage": "", "vulnerabilities": ""},
		"truncated": {
			"lint": "=== golangci gate ===\n", "unit-tests": "<testsuites tests=\"3\"", "features": "<testsuites",
			"coverage": "mode: set\n", "vulnerabilities": "=== govulncheck gate ===\nwhat to do: nothing -- x\n",
		},
		"no tests": {"unit-tests": junit(0, 0, 0), "features": junit(0, 0, 0)},
	}
	for kind, files := range bad {
		for _, g := range gates {
			t.Run(kind+"/"+g.name, func(t *testing.T) {
				var content *string
				if c, ok := files[g.name]; ok {
					content = &c
				} else if kind != "missing" {
					t.Skip("no fixture of this kind for this gate")
				}
				if b := renderOne(t, g.name, content); b.Color == colorGreen {
					t.Fatalf("%s input rendered green: %+v", kind, b)
				}
			})
		}
	}
	// One failed vulnerability section is red even if the other is missing.
	content := report(map[string]bool{"govulncheck": false})
	if b := renderOne(t, "vulnerabilities", &content); b.Color != colorRed {
		t.Fatalf("failed govulncheck with no trivy section: %+v", b)
	}
	// And one passing section alone is not enough for green.
	content = report(map[string]bool{"govulncheck": true})
	if b := renderOne(t, "vulnerabilities", &content); b.Color == colorGreen {
		t.Fatalf("govulncheck alone rendered green: %+v", b)
	}
}

func TestBadgeValuesFollowInput(t *testing.T) {
	small := map[string]string{
		"lint": passing["lint"], "unit-tests": junit(10, 0, 0), "features": junit(4, 0, 0),
		"coverage": profile(90), "vulnerabilities": passing["vulnerabilities"],
	}
	large := map[string]string{
		"lint": failing["lint"], "unit-tests": junit(777, 0, 0), "features": junit(55, 0, 0),
		"coverage": profile(73), "vulnerabilities": failing["vulnerabilities"],
	}
	for _, g := range gates {
		a, b := small[g.name], large[g.name]
		first, second := renderOne(t, g.name, &a), renderOne(t, g.name, &b)
		if first.Message == second.Message {
			t.Errorf("%s: two different inputs gave the same message %q", g.name, first.Message)
		}
	}
	// The colour follows the input too: 90% and 73% sit on different sides
	// of coverageGreenMin.
	hi, lo := small["coverage"], large["coverage"]
	if renderOne(t, "coverage", &hi).Color == renderOne(t, "coverage", &lo).Color {
		t.Error("coverage: 90% and 73% rendered the same colour")
	}
}

func TestGenerateWritesExactlyFourKeysPerGate(t *testing.T) {
	in, out := t.TempDir(), filepath.Join(t.TempDir(), "badges")
	for _, g := range gates {
		p := filepath.Join(in, filepath.FromSlash(g.input))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		// Both quality-report gates share one file; write the passing one.
		content := passing[g.name]
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := generate(in, out); err != nil {
		t.Fatal(err)
	}
	want := []string{"color", "label", "message", "schemaVersion"}
	for _, name := range []string{"lint", "unit-tests", "features", "coverage", "vulnerabilities"} {
		data, err := os.ReadFile(filepath.Join(out, name+".json"))
		if err != nil {
			t.Fatal(err)
		}
		var m map[string]any
		if err := json.Unmarshal(data, &m); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		var keys []string
		for k := range m {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		if strings.Join(keys, ",") != strings.Join(want, ",") {
			t.Errorf("%s: keys = %v, want %v", name, keys, want)
		}
		if m["schemaVersion"] != float64(1) {
			t.Errorf("%s: schemaVersion = %v", name, m["schemaVersion"])
		}
	}
}

func TestGenerateReportsAnUnwritableOutputDir(t *testing.T) {
	file := filepath.Join(t.TempDir(), "afile")
	if err := os.WriteFile(file, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := generate(t.TempDir(), filepath.Join(file, "badges")); err == nil {
		t.Fatal("generate into a path below a regular file succeeded")
	}
}

func TestParseBlockRejectsMalformedLines(t *testing.T) {
	for _, line := range []string{"", "a.go:1.1,2.2 x 1", "a.go:1.1,2.2 1 y", "nocolon 1 1", "a.go:1.1,2.2 -1 1", "a b"} {
		if _, _, _, ok := parseBlock(line); ok {
			t.Errorf("parseBlock(%q) accepted", line)
		}
	}
}
