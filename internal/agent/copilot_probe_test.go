package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// copilotPane reads one real copilot 1.0.93 capture (see
// testdata/probes/copilot-PROVENANCE.md).
func copilotPane(t *testing.T, file string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(copilotFixtureDir, file))
	if err != nil {
		t.Fatalf("read copilot fixture %s: %v", file, err)
	}
	return string(raw)
}

// copilotRuleByReason returns the copilot probe rule carrying reason, and its
// position among the copilot rules.
func copilotRuleByReason(t *testing.T, reason string) (probeRule, int) {
	t.Helper()
	for position, rule := range copilotProbeRules {
		if rule.reason == reason {
			return rule, position
		}
	}
	t.Fatalf("no copilot rule with reason %q", reason)
	return probeRule{}, 0
}

// The expectation is written from R219, not from probe.go: each rule 1-7 is
// classified on the real capture the requirement is fitted to.
func TestCopilotProbeClassifiesEachRuleOnItsRealFixture(t *testing.T) {
	cases := []struct {
		file, status, reason string
	}{
		{"trust.txt", "waiting", "folder trust"},
		{"permission.txt", "waiting", "permission prompt"},
		{"question.txt", "waiting", "question"},
		{"working.txt", "running", "working indicator"},
		{"working-edit-prompt.txt", "running", "working indicator"},
		{"error.txt", "error", "error line"},
		{"idle.txt", "idle", "ready"},
		{"idle-draft.txt", "idle", "ready"},
		{"allow-all-footer.txt", "idle", "ready"},
	}
	for _, tc := range cases {
		t.Run(tc.file, func(t *testing.T) {
			status, reason := (Copilot{}).Probe(copilotPane(t, tc.file))
			if status != tc.status || reason != tc.reason {
				t.Fatalf("%s: got (%q, %q), want (%q, %q)", tc.file, status, reason, tc.status, tc.reason)
			}
		})
	}
}

func TestCopilotProbeGivesNoVerdictForAnUnmatchedPane(t *testing.T) {
	panes := map[string]string{
		"blank":           "\n\n   \n",
		"unrelated text":  "$ ls\nfoo bar\n",
		"another kind":    copilotPaneFromKind(t, "claude", "running.txt"),
		"startup banner":  "  Copilot v1.0.93 uses AI.\n",
		"warning no foot": "! no plugin.json found\n",
	}
	for name, pane := range panes {
		if status, reason := (Copilot{}).Probe(pane); status != "" || reason != "" {
			t.Errorf("%s: got (%q, %q), want no verdict", name, status, reason)
		}
	}
}

func copilotPaneFromKind(t *testing.T, kind, file string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "probes", kind, file))
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// Each test builds a pane in which BOTH rules of an adjacent pair match,
// proves that on the rules themselves, and asserts the earlier one wins. The
// pane the second rule alone would classify differently, so swapping the two
// rules in probeRules changes the verdict and fails the test.
func assertAdjacentPrecedence(t *testing.T, pane, winnerReason, loserReason string) {
	t.Helper()
	winner, winnerAt := copilotRuleByReason(t, winnerReason)
	loser, loserAt := copilotRuleByReason(t, loserReason)
	tail := lastContentLine(pane)
	if !winner.matches(pane, tail) || !loser.matches(pane, tail) {
		t.Fatalf("pane must match both %q and %q for the pair to compete", winnerReason, loserReason)
	}
	if winnerAt >= loserAt {
		t.Errorf("rule %q (position %d) must come before %q (position %d)", winnerReason, winnerAt, loserReason, loserAt)
	}
	if _, reason := (Copilot{}).Probe(pane); reason != winnerReason {
		t.Errorf("verdict reason %q, want %q (it outranks %q)", reason, winnerReason, loserReason)
	}
}

func TestCopilotProbePrecedenceTrustBeatsPermission(t *testing.T) {
	// The trust dialog and the permission dialog share the navigation line;
	// a pane that carries both sentences is a trust prompt.
	pane := copilotPane(t, "trust.txt") + "\n Do you want to run this command?\n"
	assertAdjacentPrecedence(t, pane, "folder trust", "permission prompt")
}

func TestCopilotProbePrecedencePermissionBeatsWorking(t *testing.T) {
	pane := copilotPane(t, "permission.txt") + "\n" + copilotPane(t, "working.txt")
	assertAdjacentPrecedence(t, pane, "permission prompt", "working indicator")
}

func TestCopilotProbePrecedenceWorkingBeatsError(t *testing.T) {
	pane := strings.Replace(copilotPane(t, "working-edit-prompt.txt"), "\n /tmp/work\n", "\n ✗ Failed to get response from the AI model\n /tmp/work\n", 1)
	if !strings.Contains(pane, "✗ Failed") {
		t.Fatal("fixture no longer has a context line to insert the error above")
	}
	assertAdjacentPrecedence(t, pane, "working indicator", "error line")
}

func TestCopilotProbeQuestionSitsBetweenPermissionAndWorking(t *testing.T) {
	pane := copilotPane(t, "question.txt") + "\n" + copilotPane(t, "working.txt")
	if _, reason := (Copilot{}).Probe(pane); reason != "question" {
		t.Errorf("question with a working footer: reason %q, want question", reason)
	}
	pane = copilotPane(t, "permission.txt") + "\n" + copilotPane(t, "question.txt")
	if _, reason := (Copilot{}).Probe(pane); reason != "permission prompt" {
		t.Errorf("permission with a question: reason %q, want permission prompt", reason)
	}
}

func TestCopilotProbeErrorBeatsIdleFooter(t *testing.T) {
	assertAdjacentPrecedence(t, copilotPane(t, "error.txt"), "error line", "ready")
}

// An 80-column pane wraps the footer, splitting "Manual Approval"; the idle
// rule matches a substring that survives the wrap.
func TestCopilotProbeClassifiesTheWrappedFooterAt80Columns(t *testing.T) {
	pane := copilotPane(t, "idle-wrapped-80.txt")
	if strings.Contains(pane, "Manual Approval") {
		t.Fatal("fixture is not wrapped: Manual Approval is intact")
	}
	if status, reason := (Copilot{}).Probe(pane); status != "idle" || reason != "ready" {
		t.Fatalf("wrapped footer: got (%q, %q), want (idle, ready)", status, reason)
	}
}

func TestCopilotProbeTreatsABangLineAsAWarningNotAnError(t *testing.T) {
	pane := copilotPane(t, "warning.txt")
	if !strings.Contains(pane, "\n ! ") {
		t.Fatal("fixture has no `! ` warning line")
	}
	status, reason := (Copilot{}).Probe(pane)
	if status == "error" || status != "idle" || reason != "ready" {
		t.Fatalf("warning pane: got (%q, %q), want (idle, ready), never error", status, reason)
	}
}

func TestCopilotErrorLineIsOnlyAnErrorAfterTheLastTurnAboveTheContextLine(t *testing.T) {
	rule := "─────────"
	frame := func(transcript string) string {
		return transcript + "\n /tmp/work\n" + rule + "\n❯\n" + rule + "\n ← open sidebar · Interactive · / commands\n"
	}
	cases := []struct {
		name, transcript string
		want             bool
	}{
		{"error after the last turn", " ❯ hi\n ✗ Failed to get response", true},
		{"error before a newer turn", " ✗ Failed to get response\n ❯ hi again\n ● ok", false},
		{"error with no turn at all", " ✗ Failed to change directory", true},
		{"warning only", " ❯ hi\n ! no plugin.json", false},
		{"indented ✗ glyph mid-line", " ❯ hi\n ● the tool printed ✗ in prose", false},
	}
	for _, tc := range cases {
		if got := copilotErrorLine(frame(tc.transcript)); got != tc.want {
			t.Errorf("%s: copilotErrorLine = %v, want %v", tc.name, got, tc.want)
		}
	}
	// A pane with no composer separators still has its last line as context.
	if !copilotErrorLine(" ❯ hi\n ✗ boom\n /tmp/work\n") {
		t.Error("separator-less pane: error line above the context line not seen")
	}
	if copilotErrorLine("") {
		t.Error("empty pane reported an error")
	}
}

func TestCopilotWorkingFooterNeedsAGlyphWorkingAndAnEscHint(t *testing.T) {
	rule := "─────────"
	// frame draws a composer under a transcript, with footer as the line below it.
	frame := func(footer string) string {
		return " ❯ hi\n /tmp/work\n" + rule + "\n❯\n" + rule + "\n" + footer
	}
	cases := []struct {
		name, pane string
		want       bool
	}{
		{"interrupt", frame(" ◉ Working · 130 B esc interrupt    x\n"), true},
		{"edit prompt", frame(" ● Working esc edit prompt\n"), true},
		{"other glyphs", frame(" ○ Working esc interrupt\n\n ◎ Working esc interrupt\n"), true},
		{"no esc hint", frame(" ◉ Working\n"), false},
		{"no glyph", frame(" Working esc interrupt\n"), false},
		{"no composer, so no footer", " ◉ Working · 130 B esc interrupt\n", false},
	}
	for _, tc := range cases {
		if got := copilotWorkingFooter(tc.pane); got != tc.want {
			t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
		}
	}
}

// F2/R219: the Working verdict is read from the live footer only. A transcript
// line that quotes the busy footer, however close above the context line,
// leaves an idle pane idle.
func TestCopilotProbeIgnoresAWorkingLineInTheTranscript(t *testing.T) {
	idle := copilotPane(t, "idle.txt")
	const context = "\n /tmp/work\n"
	if !strings.Contains(idle, context) {
		t.Fatal("idle fixture has no context line to insert above")
	}
	for _, quoted := range []string{
		" ◉ Working · 130 B esc interrupt",
		" ● Working · esc edit prompt",
		" ○ Working esc interrupt",
	} {
		pane := strings.Replace(idle, context, "\n"+quoted+context, 1)
		if status, reason := (Copilot{}).Probe(pane); status != "idle" || reason != "ready" {
			t.Errorf("transcript %q: got (%q, %q), want (idle, ready)", quoted, status, reason)
		}
	}
	for _, file := range []string{"working.txt", "working-edit-prompt.txt"} {
		if status, reason := (Copilot{}).Probe(copilotPane(t, file)); status != "running" || reason != "working indicator" {
			t.Errorf("%s: got (%q, %q), want (running, working indicator)", file, status, reason)
		}
	}
}

func TestCopilotAuditProfileReadsOnlyTheFooterBelowTheComposer(t *testing.T) {
	allowAll := copilotPane(t, "allow-all-footer.txt")
	manual := copilotPane(t, "idle.txt")
	copilot := NewCopilot()
	for _, profile := range []string{"safe", "edits"} {
		if copilot.AuditProfile(profile, allowAll) != CopilotElevatedReason {
			t.Fatalf("%s with the Allow All footer not reported", profile)
		}
		if got := copilot.AuditProfile(profile, manual); got != "" {
			t.Fatalf("%s with Manual Approval reported: %q", profile, got)
		}
	}
	if got := copilot.AuditProfile("yolo", allowAll); got != "" {
		t.Fatalf("yolo reported: %q", got)
	}
	// "Allow All" quoted in the transcript, or a pane with no composer, is no claim.
	if got := copilot.AuditProfile("safe", "Interactive · Allow All quoted\n"+manual); got != "" {
		t.Fatalf("a transcript line was read as the footer: %q", got)
	}
	if got := copilot.AuditProfile("safe", "← Interactive · Allow All"); got != "" {
		t.Fatalf("a pane without a composer was read as a footer: %q", got)
	}
	// An 80-column wrap splits "Allow All"; the stable prefix still matches.
	wrapped := strings.Replace(copilotPane(t, "idle-wrapped-80.txt"), "Manual", "Allow ", 1)
	if got := copilot.AuditProfile("edits", wrapped); got != CopilotElevatedReason {
		t.Fatalf("wrapped Allow All footer not reported: %q", got)
	}
}
