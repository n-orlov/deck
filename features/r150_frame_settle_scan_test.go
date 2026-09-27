package features

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// TestR150EveryShellFrameCaptureIsSettled (R150) is a static scan over every
// features/*.feature file, guarding against the exact defect task 004 fixed
// in mouse.feature: a scenario that creates two or more shell sessions,
// settles them with only a GENERIC "screen contains "running"" wait (true
// the instant the FIRST of them promotes, SPEC §7's shell-only
// starting->running tmux-liveness rule), then captures a whole-screen frame
// ("captures its frame as") and later compares it byte-for-byte against a
// fresh one ("frame still matches the captured ... frame"). A shell session
// still "starting" at capture time and promoted to "running" by the time of
// the compare makes that comparison flake on the reconcile loop's own
// timing, not on anything the scenario is actually testing.
//
// The scan does not flag every generic "running" wait -- only the
// combination that actually renders a byte-exact frame twice: a capture
// step whose own session set (every distinct shell session created earlier
// in the SAME scenario) is not a subset of the sessions the scenario
// settled BY NAME (a "row "<name>" contains "running""/"starting" wait, or
// a generic wait that ran while only ONE shell session was still
// unsettled -- unambiguous, since nothing else could have satisfied it)
// before that capture. Other frame-shaped comparisons this file's sibling
// scenarios use on purpose -- a settled quiescence-based capture ("captures
// its settled frame as"), or a comparison scoped to the tmux pane/window or
// the preview panel's own content rather than the whole screen -- name
// different step text and are never matched here.
func TestR150EveryShellFrameCaptureIsSettled(t *testing.T) {
	dir := "."
	files, err := filepath.Glob(filepath.Join(dir, "*.feature"))
	if err != nil {
		t.Fatalf("glob *.feature: %v", err)
	}
	if len(files) == 0 {
		t.Fatalf("no *.feature files found under %s -- glob pattern is wrong", dir)
	}

	var violations []string
	for _, path := range files {
		vs, err := scanFeatureFileForUnsettledFrameCapture(path)
		if err != nil {
			t.Fatalf("scan %s: %v", path, err)
		}
		violations = append(violations, vs...)
	}

	if len(violations) > 0 {
		sort.Strings(violations)
		t.Fatalf("R150: %d scenario(s) capture-and-later-compare a whole-screen frame after only a generic (not per-session) settle wait, so a shell session still \"starting\" at capture time can promote to \"running\" before the compare and flake the byte-exact match:\n%s",
			len(violations), strings.Join(violations, "\n"))
	}
}

var (
	r150ScenarioHeaderRe = regexp.MustCompile(`^\s*Scenario(?: Outline)?:\s*(.+)$`)
	r150ShellCreateRe    = regexp.MustCompile(`creates (?:a )?(?:long-named )?shell session "([^"]+)"`)
	r150RowSettleRe      = regexp.MustCompile(`row "([^"]+)" contains "(?:running|starting)"`)
	r150GenericWaitRe    = regexp.MustCompile(`screen contains "(?:running|starting)"`)
	r150CaptureFrameRe   = regexp.MustCompile(`captures its frame as "([^"]+)"`)
	r150CompareFrameRe   = regexp.MustCompile(`frame still matches the captured "([^"]+)" frame`)
)

// scanFeatureFileForUnsettledFrameCapture applies the rule documented on
// TestR150EveryShellFrameCaptureIsSettled to one feature file, scenario by
// scenario, and returns one human-readable "<file>:<line>: <detail>" entry
// per violation.
func scanFeatureFileForUnsettledFrameCapture(path string) ([]string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	lines := strings.Split(string(raw), "\n")

	type scenario struct {
		title     string
		startLine int
		lines     []string
		lineNos   []int
	}
	var scenarios []scenario
	for i, l := range lines {
		if m := r150ScenarioHeaderRe.FindStringSubmatch(l); m != nil {
			scenarios = append(scenarios, scenario{title: m[1], startLine: i + 1})
		}
	}
	for si := range scenarios {
		start := scenarios[si].startLine // 1-indexed line of the header itself
		end := len(lines)
		if si+1 < len(scenarios) {
			end = scenarios[si+1].startLine - 1
		}
		for ln := start; ln <= end && ln <= len(lines); ln++ {
			scenarios[si].lines = append(scenarios[si].lines, lines[ln-1])
			scenarios[si].lineNos = append(scenarios[si].lineNos, ln)
		}
	}

	var out []string
	for _, sc := range scenarios {
		created := map[string]bool{}
		settled := map[string]bool{}
		genericWaitSeen := false
		capturedAt := map[string]map[string]bool{} // label -> snapshot of created-but-unsettled shells at capture time
		for i, l := range sc.lines {
			if m := r150ShellCreateRe.FindStringSubmatch(l); m != nil {
				created[m[1]] = true
			}
			if m := r150RowSettleRe.FindStringSubmatch(l); m != nil {
				settled[m[1]] = true
			}
			if r150GenericWaitRe.MatchString(l) {
				genericWaitSeen = true
				// A generic wait is unambiguous, and so settles, exactly
				// when there is only one still-unsettled shell session to
				// have satisfied it.
				var unsettled []string
				for name := range created {
					if !settled[name] {
						unsettled = append(unsettled, name)
					}
				}
				if len(unsettled) == 1 {
					settled[unsettled[0]] = true
				}
			}
			// A capture is only at risk once the scenario has actually
			// relied on a generic wait to mean "settled" -- a capture
			// that never went through that ambiguous mechanism at all
			// (e.g. a dialog/takeover already covering the sidebar row,
			// or an attached raw-pane capture) shows no status word to
			// race in the first place.
			if m := r150CaptureFrameRe.FindStringSubmatch(l); m != nil && genericWaitSeen {
				unsettled := map[string]bool{}
				for name := range created {
					if !settled[name] {
						unsettled[name] = true
					}
				}
				capturedAt[m[1]] = unsettled
			}
			if m := r150CompareFrameRe.FindStringSubmatch(l); m != nil {
				unsettled, ok := capturedAt[m[1]]
				if !ok || len(unsettled) == 0 {
					continue
				}
				names := make([]string, 0, len(unsettled))
				for name := range unsettled {
					names = append(names, name)
				}
				sort.Strings(names)
				out = append(out, fmt.Sprintf("%s:%d: scenario %q compares frame %q captured while shell session(s) %s were only generically (not by-name) settled",
					path, sc.lineNos[i], sc.title, m[1], strings.Join(names, ", ")))
			}
		}
	}
	return out, nil
}
