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
// in mouse.feature: a scenario that creates one or more shell sessions,
// settles them with only a GENERIC "screen contains "running"" wait (true
// the instant the FIRST of them promotes, SPEC §7's shell-only
// starting->running tmux-liveness rule), then captures a whole-screen frame
// ("captures its frame as") and later compares it byte-for-byte against a
// fresh one ("frame still matches the captured ... frame"). A shell session
// still "starting" at capture time and promoted to "running" by the time of
// the compare makes that comparison flake on the reconcile loop's own
// timing, not on anything the scenario is actually testing.
//
// Failure modes the scan must reject, found by review (cure-01-02):
//   - "screen contains "starting"" is itself a frame wait on the TRANSIENT
//     status word: a shell promotes out of it within one reconcile tick, so
//     the wait races that promotion and is rejected ON ITS OWN, capture or
//     no capture -- a row-scoped one whenever it names a shell, a generic
//     one whenever any displayed shell is not yet settled by name. It is
//     never a settle signal either: only a wait naming "running" can mark
//     a shell settled. Store-only assertions ("the state database ...
//     "starting"") name no screen and are never matched.
//   - Background steps run before every scenario, so the shells a
//     Background creates are displayed (and must be settled) in each one.
//   - a capture is at risk whether or not the scenario ever waited at all:
//     the scan checks every capture's own outstanding-shell set directly,
//     rather than only bothering to look once some earlier generic wait was
//     seen.
//   - a generic "running" wait settles the sole remaining unconfirmed shell
//     only when NO other shell in the scenario is already known-settled --
//     if one is, the wait can be satisfied by that already-running row's
//     text alone and proves nothing about the one still unconfirmed.
//   - the same holds when the "other" already-displayed session is not a
//     shell at all: a claude/codex/pi agent session created earlier in the
//     scenario can itself already be showing "running" (durably, once its
//     own hook reports, or transiently through a wait of its own) by the
//     time a brand-new shell's generic "running" wait runs. That wait's
//     pass proves nothing about the new shell's own starting->running
//     promotion, so a generic wait is trusted to settle the sole
//     unconfirmed shell only when NO other session -- shell OR agent --
//     has been created anywhere earlier in the scenario (Background
//     included), not merely when no other SHELL has been settled by name.
//
// The scan does not flag every generic "running" wait -- only the
// combination that actually renders a byte-exact frame twice: a capture
// step whose own session set (every distinct shell session created earlier
// in the SAME scenario) is not a subset of the sessions the scenario
// settled BY NAME (a "row "<name>" contains "running"" wait, or an
// unambiguous generic "running" wait -- the scenario's only shell, with no
// other shell already settled AND no other session of any kind, shell or
// agent, created anywhere earlier in the scenario) before that capture. Other frame-shaped
// comparisons this file's sibling scenarios use on purpose -- a settled
// quiescence-based capture ("captures its settled frame as"), or a
// comparison scoped to the tmux pane/window or the preview panel's own
// content rather than the whole screen -- name different step text and are
// never matched here.
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
		t.Fatalf("R150: %d shell frame waypoint(s) either wait on the transient \"starting\" word or capture-and-later-compare a whole-screen frame before every displayed shell is settled by name, so a shell's reconcile-timed starting->running promotion can flake them:\n%s",
			len(violations), strings.Join(violations, "\n"))
	}
}

var (
	r150ScenarioHeaderRe   = regexp.MustCompile(`^\s*Scenario(?: Outline)?:\s*(.+)$`)
	r150BackgroundHeaderRe = regexp.MustCompile(`^\s*Background:`)
	r150ShellCreateRe      = regexp.MustCompile(`creates (?:a )?(?:long-named |persistent )?shell session "([^"]+)"`)
	r150RowSettleRe        = regexp.MustCompile(`row "([^"]+)" contains "running"`)
	r150GenericRunningRe   = regexp.MustCompile(`screen contains "running"`)
	r150RowStartingRe      = regexp.MustCompile(`row "([^"]+)" contains "starting"`)
	r150GenericStartingRe  = regexp.MustCompile(`screen contains "starting"`)
	r150CaptureFrameRe     = regexp.MustCompile(`captures its frame as "([^"]+)"`)
	r150CompareFrameRe     = regexp.MustCompile(`frame still matches the captured "([^"]+)" frame`)
	r150AgentCreateRe      = regexp.MustCompile(`creates (?:claude|codex|pi) session "([^"]+)"`)
)

// scanFeatureFileForUnsettledFrameCapture applies the rule documented on
// TestR150EveryShellFrameCaptureIsSettled to one feature file, scenario by
// scenario (each scenario's steps preceded by the file's Background steps,
// which godog runs before every scenario), and returns one human-readable
// "<file>:<line>: <detail>" entry per violation.
func scanFeatureFileForUnsettledFrameCapture(path string) ([]string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	lines := strings.Split(string(raw), "\n")

	type scenario struct {
		title   string
		lines   []string
		lineNos []int
	}
	// Split the file into sections: an optional Background and each
	// Scenario, each section running until the next header of either kind.
	var bgLines []string
	var bgLineNos []int
	var scenarios []scenario
	section := "" // "", "background" or "scenario"
	for i, l := range lines {
		if m := r150ScenarioHeaderRe.FindStringSubmatch(l); m != nil {
			scenarios = append(scenarios, scenario{title: m[1]})
			section = "scenario"
			continue
		}
		if r150BackgroundHeaderRe.MatchString(l) {
			section = "background"
			continue
		}
		switch section {
		case "background":
			bgLines = append(bgLines, l)
			bgLineNos = append(bgLineNos, i+1)
		case "scenario":
			sc := &scenarios[len(scenarios)-1]
			sc.lines = append(sc.lines, l)
			sc.lineNos = append(sc.lineNos, i+1)
		}
	}

	var out []string
	for _, sc := range scenarios {
		steps := append(append([]string{}, bgLines...), sc.lines...)
		stepLineNos := append(append([]int{}, bgLineNos...), sc.lineNos...)
		created := map[string]bool{}
		// otherSessions tracks every claude/codex/pi AGENT session created
		// anywhere earlier in the scenario (Background included). An agent
		// session can already be displaying "running" on screen -- durably
		// once its own hook reports, or through a wait of its own -- by the
		// time a brand-new shell's generic wait runs, so its mere presence
		// makes that generic wait ambiguous even though it names no shell
		// and is never itself required to be "settled by name".
		otherSessions := map[string]bool{}
		settled := map[string]bool{}
		unsettledNames := func() []string {
			var names []string
			for name := range created {
				if !settled[name] {
					names = append(names, name)
				}
			}
			sort.Strings(names)
			return names
		}
		capturedAt := map[string][]string{} // label -> created-but-unsettled shells at capture time
		for i, l := range steps {
			if strings.HasPrefix(strings.TrimSpace(l), "#") {
				continue
			}
			ln := stepLineNos[i]
			if m := r150ShellCreateRe.FindStringSubmatch(l); m != nil {
				created[m[1]] = true
			}
			if m := r150AgentCreateRe.FindStringSubmatch(l); m != nil {
				otherSessions[m[1]] = true
			}
			// A screen wait on the TRANSIENT "starting" word is a shell
			// frame waypoint on a state SPEC §7 promotes out of within one
			// reconcile tick: a row-scoped one naming a shell is always
			// such a waypoint, and a generic one is whenever any shell
			// displayed at that point is not yet durably settled (its own
			// transient "starting" can satisfy the wait, then vanish).
			// Store-only assertions ("the state database ... starting")
			// name no screen and never match either regex.
			if m := r150RowStartingRe.FindStringSubmatch(l); m != nil && created[m[1]] {
				out = append(out, fmt.Sprintf("%s:%d: scenario %q waits for shell session %q's row to show the transient \"starting\" status",
					path, ln, sc.title, m[1]))
			} else if r150GenericStartingRe.MatchString(l) {
				if names := unsettledNames(); len(names) > 0 {
					out = append(out, fmt.Sprintf("%s:%d: scenario %q waits for the screen to show the transient \"starting\" status while shell session(s) %s are not settled by name",
						path, ln, sc.title, strings.Join(names, ", ")))
				}
			}
			if m := r150RowSettleRe.FindStringSubmatch(l); m != nil {
				settled[m[1]] = true
			} else if r150GenericRunningRe.MatchString(l) {
				// A generic "running" wait settles the sole remaining
				// unconfirmed shell only when there is exactly one such
				// shell, no other shell in the scenario is already
				// known-settled, AND no other session -- shell or agent --
				// has been created anywhere earlier in the scenario. An
				// already-settled row, or an already-created agent session
				// that may already be displaying "running" on its own, can
				// each satisfy this wait by itself, on text unrelated to the
				// one shell still unconfirmed, proving nothing about it.
				if names := unsettledNames(); len(names) == 1 && len(settled) == 0 && len(otherSessions) == 0 {
					settled[names[0]] = true
				}
			}
			// Every capture snapshots its own outstanding-shell set directly,
			// whether or not the scenario ever waited on anything at all.
			if m := r150CaptureFrameRe.FindStringSubmatch(l); m != nil {
				capturedAt[m[1]] = unsettledNames()
			}
			if m := r150CompareFrameRe.FindStringSubmatch(l); m != nil {
				names, ok := capturedAt[m[1]]
				if !ok || len(names) == 0 {
					continue
				}
				out = append(out, fmt.Sprintf("%s:%d: scenario %q compares frame %q captured while shell session(s) %s were not settled by name",
					path, ln, sc.title, m[1], strings.Join(names, ", ")))
			}
		}
	}
	return out, nil
}
