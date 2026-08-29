package tui

import (
	"context"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
)

// This file is task 806's own test (review finding 2/R80): the footer's
// curated A/U slot and the `A` key handler (case "A" in tui.go) must share
// exactly one eligibility definition -- canArchive -- rather than the
// footer consulting a footer-named predicate of its own while the handler
// consulted a second, always-true `canArchive`. Before this task the footer
// already refused to show A for an archived row
// (TestFooterKeyLegendReflectsEligibility in footer_legend_test.go), but
// pressing A on that same row still opened the confirm dialog: a
// display-only refusal with no matching keypress refusal.
//
// Two different failures have to be caught, so there are two tests:
//
//   - behaviour: pressing A on an archived row must write nothing, open no
//     confirm and name U -- TestArchiveKeyOnAnArchivedRowRefusesAndNamesU;
//   - shared definition: the handler must reach that behaviour by calling
//     the footer's own predicate, not by a second copy of its logic that
//     could later drift from it -- TestArchiveKeyHandlerCallsTheFooterPredicateByName,
//     which re-parses tui.go's own source the way footer_bindings_parity_test.go
//     and help_keymap_parity_test.go already do. An inlined `ArchivedAt`
//     comparison inside case "A" is behaviourally identical today, so only
//     the source parse can fail on it -- and it must, because "identical
//     today" is exactly the state finding 2 says must not be re-created.

// TestArchiveKeyOnAnArchivedRowRefusesAndNamesU presses A on a row that is
// already archived and asserts all three of the requirement's parts: no
// command is issued (nothing is written), the confirm dialog does not open,
// and the surfaced message names U as the route back.
func TestArchiveKeyOnAnArchivedRowRefusesAndNamesU(t *testing.T) {
	archiver := &archiveRecorder{}
	model := New(nil, config.Settings{}, "")
	model.sessions = []store.Session{
		{ID: "s-archived", Name: "gone", Agent: "shell", Status: "stopped", ArchivedAt: 100},
	}
	model.selected = 0
	model.archiveSvc = archiver.archive
	model.attach = func(context.Context, string) (*exec.Cmd, error) { return exec.Command("true"), nil }

	got, cmd := model.Update(key("A"))
	model = got.(Model)

	if cmd != nil {
		t.Fatalf("A on an archived row issued a command (%T) -- it must write nothing", cmd())
	}
	if len(archiver.calls) != 0 {
		t.Fatalf("A on an archived row reached the archive service: %+v", archiver.calls)
	}
	if model.archiveConfirming {
		t.Fatal("A on an archived row opened the archive confirm dialog")
	}
	if !strings.Contains(model.attachError, "already archived") {
		t.Fatalf("A on an archived row said %q, want it to state the row is already archived", model.attachError)
	}
	if !strings.Contains(model.attachError, "U") {
		t.Fatalf("A on an archived row said %q, want it to name U as the route back", model.attachError)
	}
}

// listModeCaseBlock returns the source text of one `case "<key>":` arm of
// tui.go's top-level list-mode key switch, with its `//` comments stripped,
// so an assertion about what the arm CALLS cannot be satisfied by a comment
// that merely mentions the name. It is anchored exactly the way
// help_keymap_parity_test.go's listModeBoundKeys anchors the same switch
// (first `switch msg.String() {`, closed by the following
// `case tea.MouseMsg:`), so the two never disagree about which switch is
// being read.
func listModeCaseBlock(t *testing.T, key string) string {
	t.Helper()
	data, err := os.ReadFile("tui.go")
	if err != nil {
		t.Fatalf("ReadFile(tui.go): %v", err)
	}
	src := string(data)

	start := strings.Index(src, "switch msg.String() {")
	if start < 0 {
		t.Fatalf("could not find the list-mode key switch (`switch msg.String() {`) in tui.go -- extraction is broken, not the source")
	}
	relEnd := strings.Index(src[start:], "\n\tcase tea.MouseMsg:")
	if relEnd < 0 {
		t.Fatalf("could not find the end of the list-mode key switch (`case tea.MouseMsg:`) in tui.go -- extraction is broken, not the source")
	}
	block := src[start : start+relEnd]

	caseStart := strings.Index(block, "\n\t\tcase \""+key+"\":")
	if caseStart < 0 {
		t.Fatalf("could not find `case %q:` in tui.go's list-mode key switch -- extraction is broken, not the source", key)
	}
	arm := block[caseStart+1:]
	if relNext := strings.Index(arm[1:], "\n\t\tcase "); relNext >= 0 {
		arm = arm[:relNext+1]
	}

	var code []string
	for _, line := range strings.Split(arm, "\n") {
		if idx := strings.Index(line, "//"); idx >= 0 {
			line = line[:idx]
		}
		code = append(code, line)
	}
	return strings.Join(code, "\n")
}

// archivedAtRe matches any direct reference to the ArchivedAt field --
// i.e. a second, local copy of what the shared predicate already decides.
var archivedAtRe = regexp.MustCompile(`\bArchivedAt\b`)

// TestArchiveKeyHandlerCallsTheFooterPredicateByName is the structural half
// of review finding 2/R80 for `A`: it reads the predicate name out of
// footerLegend's own `A` entry (via footer_bindings_parity_test.go's
// parseFooterLegendSource, the same live extraction the footer's parity test
// trusts) and then asserts that the `case "A":` arm of the list-mode key
// switch CALLS that very function, and contains no ArchivedAt comparison of
// its own.
//
// This is the test the mutation demo under
// docs/reports/phase3g-806-archive-eligibility/ turns red: replacing
// `canArchive(m.sessions[m.selected])` in case "A" with an equivalent inline
// `m.sessions[m.selected].ArchivedAt != 0` keeps every behavioural
// assertion green (the two definitions agree today -- that is the whole
// hazard) and fails here, because the footer and the handler would once
// again be two definitions that merely happen to match.
//
// It also holds the predicate's own doc comment to the finding: the shared
// definition must not describe itself as belonging to the footer.
func TestArchiveKeyHandlerCallsTheFooterPredicateByName(t *testing.T) {
	var predicateName string
	for _, e := range parseFooterLegendSource(t) {
		if e.unicodeKey == "A" {
			if !e.hasPredicate {
				t.Fatal("footerLegend's A entry carries no eligibility predicate at all -- A's eligibility must have exactly one definition, shared with case \"A\"")
			}
			predicateName = e.predicateName
		}
	}
	if predicateName == "" {
		t.Fatal("footerLegend has no A entry -- extraction is broken, or SPEC \u00a711.3's A/U slot was removed")
	}
	if strings.Contains(strings.ToLower(predicateName), "footer") {
		t.Errorf("A's shared eligibility predicate is named %q: the key handler consults it too, so its name must not claim it belongs to the footer", predicateName)
	}
	if _, ok := footerPredicateByName[predicateName]; !ok {
		t.Fatalf("footerLegend's A entry names predicate %q, which footerPredicateByName does not resolve -- add it there so both parity tests can evaluate the real function", predicateName)
	}

	arm := listModeCaseBlock(t, "A")
	if !strings.Contains(arm, predicateName+"(") {
		t.Errorf("case \"A\" in tui.go's list-mode key switch does not call %s(), the predicate footerLegend's own A entry names: the footer and the key handler must consult one definition of A's eligibility (review finding 2/R80, SPEC \u00a711.3), never two that merely agree.\ncase \"A\" arm (comments stripped):\n%s", predicateName, arm)
	}
	if loc := archivedAtRe.FindString(arm); loc != "" {
		t.Errorf("case \"A\" in tui.go's list-mode key switch reads ArchivedAt itself: that is a second copy of %s()'s logic, which is exactly the parallel definition review finding 2/R80 rejects -- call %s() instead.\ncase \"A\" arm (comments stripped):\n%s", predicateName, predicateName, arm)
	}

	data, err := os.ReadFile("tui.go")
	if err != nil {
		t.Fatalf("ReadFile(tui.go): %v", err)
	}
	src := string(data)
	defIdx := strings.Index(src, "\nfunc "+predicateName+"(session store.Session) bool {")
	if defIdx < 0 {
		t.Fatalf("could not find `func %s(session store.Session) bool {` in tui.go -- extraction is broken, not the source", predicateName)
	}
	docStart := strings.LastIndex(src[:defIdx], "\n\n")
	if docStart < 0 {
		docStart = 0
	}
	doc := src[docStart:defIdx]
	if strings.Contains(strings.ToLower(doc), "footer-only") {
		t.Errorf("%s's doc comment still calls it footer-only, but case \"A\" consults it too:\n%s", predicateName, doc)
	}
}

// TestArchiveKeyEligibilityMatchesFooterPredicate is the value-level check:
// it evaluates the shared predicate directly (the same function
// TestArchiveKeyOnAnArchivedRowRefusesAndNamesU exercises indirectly via the
// key handler, and the same function footer_bindings_parity_test.go's
// footerPredicateByName evaluates for the footer's own A entry) across both
// states the predicate distinguishes, so a change that makes the predicate
// itself stop distinguishing them is caught here rather than only through
// the key handler.
func TestArchiveKeyEligibilityMatchesFooterPredicate(t *testing.T) {
	unarchived := store.Session{ID: "s1", Status: "running"}
	if !canArchive(unarchived) {
		t.Fatal("canArchive refused an unarchived row")
	}
	archived := store.Session{ID: "s2", Status: "stopped", ArchivedAt: 100}
	if canArchive(archived) {
		t.Fatal("canArchive accepted an already-archived row")
	}
}
