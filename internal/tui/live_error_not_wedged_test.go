package tui

import (
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/store"
)

// This file is task 1001's own test. Approach 10 (tasks 901-906) narrowed
// internal/service's live-pane repair so a hook- or probe-sourced `error`
// row with no PaneExitStatus is left alone (finding F40): the tree
// deliberately does NOT overwrite that verdict with a repair back to a
// non-terminal status. The R76 defence for that decision, recorded in
// docs/phase3g.md, rests on a specific factual claim: such a row is not a
// SPEC-sense "wedge" ("a row that every action refuses", SPEC.md:559-575),
// because canKill, canReachPane and canRestart all still accept it -- only
// canResume (which only ever accepts a "stopped" row) declines. This test
// pins that claim directly against the four eligibility predicates
// themselves, and then against the real footer legend and the real
// single-row key handlers for every key those four predicates gate, using
// exactly the same behavioural idiom
// footer_handler_agreement_test.go's TestFooterHandlerAgreementAcrossEveryRowClass
// already uses for its six existing row classes: press the real key
// through the real Update and see whether it acted, compare that against
// what Model.footerKeyLegend() actually advertises for the same row.
//
// It is a NEW file, not an edit to footer_handler_agreement_test.go,
// because task 806 previously breached "no pre-existing test file edited"
// scope by touching that file for an unrelated fix; this task's own
// success criteria repeat that constraint by name. Everything it needs
// from that file -- buildFooterAgreementModel, footerAgreementKeys,
// pressAndRun, parseFooterLegendSource -- is unexported but same-package,
// so this file calls it directly rather than duplicating it.

// liveErrorNotWedgedRow is the row this test pins: Status "error",
// StatusSource "hook" (a hook-sourced verdict, per finding F40 and
// status_claude_hooks.feature) and no PaneExitStatus at all -- the exact
// shape SPEC §7's transition table allows an error to reach with no pane
// death, and the one repairTerminalRowWithLivePane deliberately leaves
// alone (internal/service/reconcile.go:83-104).
var liveErrorNotWedgedRow = store.Session{
	ID: "s1", Name: "alpha", Slug: "alpha", Agent: "claude",
	Status: "error", StatusSource: "hook",
}

// TestLiveErrorRowIsNotWedgedByEligibilityPredicates asserts the four
// predicate calls directly: canKill, canReachPane and canRestart must all
// accept liveErrorNotWedgedRow, and canResume must decline it (a resume is
// for a stopped row; this one never stopped). If SPEC §11.3's "eligibility
// has exactly one definition per action" sentence is a real guarantee,
// this is the smallest possible statement of "not every action refuses
// this row" -- the wedge definition R76 rests on.
func TestLiveErrorRowIsNotWedgedByEligibilityPredicates(t *testing.T) {
	row := liveErrorNotWedgedRow
	if row.PaneExitStatus != nil {
		t.Fatalf("fixture row carries a PaneExitStatus; the claim under test is specifically about a hook/probe error with none")
	}
	if !canKill(row) {
		t.Errorf("canKill refused a hook-sourced error row with no PaneExitStatus -- that would make the row wedged, contradicting finding F40")
	}
	if !canReachPane(row) {
		t.Errorf("canReachPane refused a hook-sourced error row with no PaneExitStatus -- that would make the row wedged, contradicting finding F40")
	}
	if !canRestart(row) {
		t.Errorf("canRestart refused a hook-sourced error row with no PaneExitStatus -- that would make the row wedged, contradicting finding F40")
	}
	if canResume(row) {
		t.Errorf("canResume accepted a hook-sourced error row -- canResume is defined for a stopped row only, and this row's Status is \"error\", not \"stopped\"")
	}
}

// TestLiveErrorRowFooterAndHandlersAgree extends
// footer_handler_agreement_test.go's own per-row-class agreement check
// (TestFooterHandlerAgreementAcrossEveryRowClass) to a seventh row class it
// does not carry: this same liveErrorNotWedgedRow, run through every one of
// that file's footerAgreementKeys (Y/x/r/R/A/U/dd/\u21b5/a/i). For each key,
// the real rendered footer legend (Model.footerKeyLegend()) and the real
// single-row key handler (driven through Model.Update, exactly as
// pressAndRun does for the six existing row classes) must agree key by
// key: the footer advertises the key if and only if pressing it actually
// acts. x, \u21b5 and R (gated by canKill/canReachPane/canRestart
// respectively) are expected advertised+acted; r (gated by canResume) is
// expected neither-advertised-nor-acted. The other keys are gated by
// predicates this test does not concern itself with (canAcknowledge,
// canArchive/canUnarchive, canDelete, canShowDetail are all row-shape
// independent or archive-state dependent, not error-state dependent), so
// this loop simply requires agreement for every one of them too, exactly
// like the existing row classes' own check does.
func TestLiveErrorRowFooterAndHandlersAgree(t *testing.T) {
	entries := parseFooterLegendSource(t)
	matchText := map[string]string{}
	for _, e := range entries {
		matchText[e.unicodeKey] = strings.TrimSpace(e.unicodeKey + " " + e.hint)
	}

	for _, k := range footerAgreementKeys() {
		t.Run(k.glyph, func(t *testing.T) {
			before, rec := buildFooterAgreementModel(liveErrorNotWedgedRow, nil, nil)
			text, ok := matchText[k.glyph]
			if !ok {
				t.Fatalf("no footerLegend entry parsed for glyph %q -- extraction is broken, not the source", k.glyph)
			}
			legend := before.footerKeyLegend()
			advertised := strings.Contains(legend, text)

			after := k.press(before)
			acted := k.acted(after, rec)

			if advertised != acted {
				t.Errorf("live-error row, key %q: footer advertised=%v (legend=%q) but the handler acted=%v -- the footer and the key handler must share exactly one eligibility definition (SPEC \u00a711.3)",
					k.glyph, advertised, legend, acted)
			}
		})
	}
}
