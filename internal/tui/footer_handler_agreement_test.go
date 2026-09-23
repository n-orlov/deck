package tui

import (
	"context"
	"os/exec"
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/service"
	"github.com/n-orlov/deck/internal/store"
	"github.com/n-orlov/deck/internal/tmux"
)

// This file is task 808's own test (SPEC §11.3: "eligibility has exactly
// one definition per action, shared by the footer and the key handler").
// Tasks 806 and 807 each closed one instance of that requirement --
// canArchive for `A`, canKill for `x` -- with a dedicated pair of tests: a
// behavioural check (press the key, see it refuse) and a structural one
// (parse the `case` arm's own source, catch an inlined copy of the same
// logic before it can drift). Neither of those two is generic: nothing yet
// drives EVERY footer action key's real handler across EVERY row class the
// footer's own predicates distinguish and cross-checks the result against
// the footer's own rendering, key by key. This test is that generic check.
//
// It is deliberately behavioural, not structural (806/807 already own the
// source-parse idiom for the two keys review flagged by name): the ground
// truth on the footer side is Model.footerKeyLegend()'s actual rendered
// text -- never a second call to footerRowEligible, which would just be
// asking the same function twice -- and the ground truth on the handler
// side is what pressing the real key, through the real Update, actually
// does: a service call recorded by a fake, or a dialog flag the handler
// itself sets. A key handler that stops consulting the footer's shared
// predicate -- whether by inlining an equivalent copy (806's hazard) or by
// dropping the eligibility check outright (807's hazard, and the one this
// task's own mutation demonstrates for `R`) -- flips one side of the
// comparison without flipping the other, and this test is the one place
// that would notice for R, U and Y, which carry no dedicated eligibility
// test of their own today.

// footerAgreementRecorder is a set of call flags for every service
// function a footer action key can reach, shared across all ten key
// checks below so building one Model wires every handler at once: the row
// class fixtures below are shared across all ten keys, and a key that
// early-returns on a nil service function (m.kill == nil, m.resume == nil,
// m.attach == nil, ...) would silently read as "did not act" for the wrong
// reason if any one of these were left unwired.
type footerAgreementRecorder struct {
	killCalled, resumeCalled, restartCalled, unarchiveCalled, acknowledgeCalled, attachCalled bool
}

// footerAgreementInteractiveFrame is the 80x9 terminal frame requirement
// 48's own godog fixture uses (features/interactive_refusals.feature's
// @requirement-48 scenario, re-derived by
// TestEnterInteractiveRefusesBelowTheSevenRowFloorWithoutAnyTmuxCall):
// previewContentSize() resolves it to 41x6, below interactiveMinInnerRows,
// so `\u21b5` on an ELIGIBLE row runs canReachPane, gets past it, and stops
// at "Refusal case 1" -- the floor -- before enterInteractive makes any
// tmux call at all. That is what makes `\u21b5` observable here with no tmux
// server present: the floor refusal is reachable only past the eligibility
// gate, so its message (footerAgreementPastEligibilityGate) is a faithful
// "the handler acted on this row" signal, while a row canReachPane rejects
// stops earlier with the "resume it first" message instead.
const (
	footerAgreementFrameWidth  = 80
	footerAgreementFrameHeight = 9

	// footerAgreementPastEligibilityGate appears in BOTH of enterInteractive's
	// preview-size refusals (too-small width and the 7-row floor) and in
	// neither the eligibility refusal nor the nil-client degrade, so testing
	// for it does not pin which of the two size messages the frame produces.
	footerAgreementPastEligibilityGate = "press a to attach instead"
)

// buildFooterAgreementModel wires a fresh Model with every footer-action
// service function recorded by a fresh footerAgreementRecorder, and
// populates m.sessions/m.selected/m.marked from the given rows. An empty
// selected.ID (used only by the "empty list" row class) leaves m.sessions
// nil -- New's own zero state -- rather than a one-element slice holding a
// zero-value Session, so canKill/canResume/... are never asked about a row
// that isn't really there.
func buildFooterAgreementModel(selected store.Session, extra []store.Session, marked map[string]bool) (Model, *footerAgreementRecorder) {
	rec := &footerAgreementRecorder{}
	m := New(nil, config.Settings{}, "")
	// A non-empty Socket that resolves to nothing real: enough to clear
	// enterInteractive's zero-client degrade path without a tmux server
	// existing (the 203/311/313 idiom), and the frame below keeps every
	// eligible press stopping at the floor refusal before any tmux call.
	m = m.WithTmuxClient(tmux.Client{Socket: "no-such-tmux-server-808"})
	m.width, m.height = footerAgreementFrameWidth, footerAgreementFrameHeight
	if selected.ID != "" {
		m.sessions = append([]store.Session{selected}, extra...)
	}
	m.selected = rowCursor(0)
	m.marked = marked
	m.kill = func(context.Context, store.Session) error {
		rec.killCalled = true
		return nil
	}
	m.resume = func(context.Context, string) (store.Session, service.ResumeOutcome, error) {
		rec.resumeCalled = true
		return store.Session{}, service.ResumeStarted, nil
	}
	m.restart = func(context.Context, string) (store.Session, service.ResumeOutcome, error) {
		rec.restartCalled = true
		return store.Session{}, service.ResumeStarted, nil
	}
	m.archiveSvc = func(context.Context, store.Session) error { return nil }
	m.unarchiveSvc = func(context.Context, string) (store.Session, error) {
		rec.unarchiveCalled = true
		return store.Session{}, nil
	}
	m.acknowledge = func(context.Context, string) error {
		rec.acknowledgeCalled = true
		return nil
	}
	// `a`'s handler (attachSelected) reaches the terminal-handover service
	// only past canReachPane; the command it returns is never run here
	// (tea.ExecProcess only wraps it into a message), so `true` stands in
	// for a real attach without launching anything.
	m.attach = func(context.Context, string) (*exec.Cmd, error) {
		rec.attachCalled = true
		return exec.Command("true"), nil
	}
	return m, rec
}

// footerAgreementRowClass is one of the six row classes the task requires:
// a Model/recorder pair built fresh for every (row class, key) pair below,
// so no key's press can leak state into another key's check of the same
// row class.
type footerAgreementRowClass struct {
	name  string
	build func() (Model, *footerAgreementRecorder)
}

func footerAgreementRowClasses() []footerAgreementRowClass {
	return []footerAgreementRowClass{
		{
			name: "empty list",
			build: func() (Model, *footerAgreementRecorder) {
				return buildFooterAgreementModel(store.Session{}, nil, nil)
			},
		},
		{
			name: "live/running",
			build: func() (Model, *footerAgreementRecorder) {
				return buildFooterAgreementModel(
					store.Session{ID: "s1", Name: "alpha", Slug: "alpha", Agent: "claude", Status: "running"}, nil, nil)
			},
		},
		{
			name: "stopped",
			build: func() (Model, *footerAgreementRecorder) {
				return buildFooterAgreementModel(
					store.Session{ID: "s1", Name: "alpha", Slug: "alpha", Agent: "claude", Status: "stopped"}, nil, nil)
			},
		},
		{
			name: "archived",
			build: func() (Model, *footerAgreementRecorder) {
				return buildFooterAgreementModel(
					store.Session{ID: "s1", Name: "alpha", Slug: "alpha", Agent: "claude", Status: "stopped", ArchivedAt: 100}, nil, nil)
			},
		},
		{
			name: "attention-pending",
			build: func() (Model, *footerAgreementRecorder) {
				return buildFooterAgreementModel(
					store.Session{ID: "s1", Name: "alpha", Slug: "alpha", Agent: "claude", Status: "waiting", Acknowledged: false}, nil, nil)
			},
		},
		{
			// Selected row (s1) is live/killable/restartable/archivable on
			// its own; the marked set (s2) is not. Y/r/R/A/U (and ↵/a/i,
			// added in the same spirit) all pass batch=false in footerLegend
			// and their own handlers never consult m.marked either, so they
			// must act on s1 (the selection) here, ignoring s2 entirely.
			// x/dd DO switch to the marked set once it is non-empty, so they
			// must NOT act here: s2 is the only marked row and it refuses both.
			name: "non-empty mark set: selected row differs from the marked row",
			build: func() (Model, *footerAgreementRecorder) {
				m, rec := buildFooterAgreementModel(
					store.Session{ID: "s1", Name: "alpha", Slug: "alpha", Agent: "claude", Status: "running"},
					[]store.Session{{ID: "s2", Name: "beta", Slug: "beta", Agent: "claude", Status: "stopped"}},
					map[string]bool{"s2": true},
				)
				return m, rec
			},
		},
	}
}

// pressAndRun sends msg through the real Update and, if it returned a
// command, runs it immediately: every service call this test can observe
// (kill/resume/restart/unarchive/acknowledge) is dispatched through a
// tea.Cmd, never made synchronously inside Update itself, so a check that
// only inspected cmd != nil (true even for a stopped row's `x`, which
// still dispatches a command carrying nothing but a refusal message) would
// misread "acted" for "produced any command at all".
func pressAndRun(m Model, keyStr string) Model {
	got, cmd := m.Update(key(keyStr))
	m = got.(Model)
	if cmd != nil {
		cmd()
	}
	return m
}

// footerAgreementKey is one footer action key's own press-and-observe
// pair: press drives the real handler (a chord, for "dd", is two separate
// key(“d”) messages -- exactly what a real dd keypress sends, and exactly
// what m.pendingDelete's own intercept expects; key("dd") as one KeyMsg
// would not match either `case "d"` at all), and acted reports whether the
// handler actually did the thing the footer's hint names, reading it off
// the recorder or a dialog flag the handler itself set -- never off cmd's
// mere non-nilness.
type footerAgreementKey struct {
	glyph string
	press func(m Model) Model
	acted func(after Model, rec *footerAgreementRecorder) bool
}

func footerAgreementKeys() []footerAgreementKey {
	return []footerAgreementKey{
		{
			glyph: "Y",
			press: func(m Model) Model { return pressAndRun(m, "Y") },
			acted: func(_ Model, rec *footerAgreementRecorder) bool { return rec.acknowledgeCalled },
		},
		{
			glyph: "x",
			press: func(m Model) Model { return pressAndRun(m, "x") },
			acted: func(_ Model, rec *footerAgreementRecorder) bool { return rec.killCalled },
		},
		{
			glyph: "r",
			press: func(m Model) Model { return pressAndRun(m, "r") },
			acted: func(_ Model, rec *footerAgreementRecorder) bool { return rec.resumeCalled },
		},
		{
			glyph: "R",
			press: func(m Model) Model { return pressAndRun(m, "R") },
			acted: func(_ Model, rec *footerAgreementRecorder) bool { return rec.restartCalled },
		},
		{
			glyph: "A",
			// A writes nothing on the keypress itself (R72/issue #10): the
			// observable effect of an eligible press is that the confirm
			// dialog opens, not a service call.
			press: func(m Model) Model { return pressAndRun(m, "A") },
			acted: func(after Model, _ *footerAgreementRecorder) bool { return after.archiveConfirming },
		},
		{
			glyph: "U",
			press: func(m Model) Model { return pressAndRun(m, "U") },
			acted: func(_ Model, rec *footerAgreementRecorder) bool { return rec.unarchiveCalled },
		},
		{
			glyph: "dd",
			press: func(m Model) Model {
				m = pressAndRun(m, "d")
				return pressAndRun(m, "d")
			},
			acted: func(after Model, _ *footerAgreementRecorder) bool { return after.deleteConfirming },
		},
		{
			// `\u21b5` (enter interactive) shares canReachPane with `a`. Its
			// observable "acted" is reaching a refusal that lies PAST the
			// eligibility gate -- the preview-size floor, on this fixture's
			// 80x9 frame -- rather than a service call: enterInteractive's
			// success path needs a live tmux server, which no unit test in
			// this package has. A row canReachPane rejects stops at the
			// "resume it first" message instead and never reaches the floor,
			// so the two are distinguishable without any tmux call.
			glyph: "\u21b5",
			press: func(m Model) Model { return pressAndRun(m, "enter") },
			acted: func(after Model, _ *footerAgreementRecorder) bool {
				return strings.Contains(after.attachError, footerAgreementPastEligibilityGate)
			},
		},
		{
			glyph: "a",
			press: func(m Model) Model { return pressAndRun(m, "a") },
			acted: func(_ Model, rec *footerAgreementRecorder) bool { return rec.attachCalled },
		},
		{
			// `i` opens the detail dialog on the keypress itself (its one
			// store read is dispatched as a tea.Cmd and degrades to an empty
			// result with the nil store this fixture carries), so the dialog
			// flag the handler sets is the whole observable effect.
			glyph: "i",
			press: func(m Model) Model { return pressAndRun(m, "i") },
			acted: func(after Model, _ *footerAgreementRecorder) bool { return after.detail },
		},
	}
}

// TestFooterHandlerAgreementCoversEveryEligibilityGatedFooterKey is the
// completeness half of this file: the agreement matrix below is only "every
// footer action key" for as long as footerAgreementKeys keeps up with
// footerLegend itself, and a new eligibility-gated entry added to tui.go
// would otherwise slip in un-cross-checked and silently narrow the claim.
// The authority is footerLegend's own source text (parseFooterLegendSource,
// shared with task 021's parity tests): every entry that carries an
// eligible predicate at all must have a press/observe pair here, and every
// pair here must name a real entry. Entries with no predicate (up/down, n,
// ,, ?, q) are always advertised and have no eligibility to agree about.
func TestFooterHandlerAgreementCoversEveryEligibilityGatedFooterKey(t *testing.T) {
	covered := map[string]bool{}
	for _, k := range footerAgreementKeys() {
		if covered[k.glyph] {
			t.Errorf("footerAgreementKeys lists glyph %q twice", k.glyph)
		}
		covered[k.glyph] = true
	}

	gated := map[string]bool{}
	for _, e := range parseFooterLegendSource(t) {
		if !e.hasPredicate {
			continue
		}
		gated[e.unicodeKey] = true
		if !covered[e.unicodeKey] {
			t.Errorf("footerLegend entry %q (%s, gated by %s) has no press/observe pair in footerAgreementKeys -- the agreement matrix would silently skip it",
				e.unicodeKey, e.hint, e.predicateName)
		}
	}
	for glyph := range covered {
		if !gated[glyph] {
			t.Errorf("footerAgreementKeys covers glyph %q, which is not an eligibility-gated footerLegend entry -- the matrix compares against a footer entry that no longer exists or was never gated", glyph)
		}
	}
	if len(gated) < 10 {
		t.Fatalf("only %d eligibility-gated footerLegend entries parsed, expected at least 10 -- extraction is broken, not the source", len(gated))
	}
}

// TestFooterHandlerAgreementAcrossEveryRowClass is task 808's own test: for
// every footer action key this task names, in every row class the footer's
// predicates distinguish, the real key handler acts if and only if the
// real rendered footer legend advertises that key -- absence checked just
// as strictly as presence, since a handler that quietly acts on a row the
// footer refuses to name is exactly what SPEC §11.3's "eligibility has
// exactly one definition" sentence forbids, not only the opposite gap.
//
// The footer side of the comparison is Model.footerKeyLegend()'s actual
// rendered text (never footerRowEligible called a second time -- that
// would just repeat the function under test), matched by the exact
// key+hint text parseFooterLegendSource itself re-parses out of tui.go's
// footerLegend literal, so a hint-word rename can't silently desync the
// match text from what the footer really prints.
func TestFooterHandlerAgreementAcrossEveryRowClass(t *testing.T) {
	entries := parseFooterLegendSource(t)
	matchText := map[string]string{}
	for _, e := range entries {
		matchText[e.unicodeKey] = strings.TrimSpace(e.unicodeKey + " " + e.hint)
	}

	for _, rc := range footerAgreementRowClasses() {
		for _, k := range footerAgreementKeys() {
			t.Run(rc.name+"/"+k.glyph, func(t *testing.T) {
				before, rec := rc.build()
				text, ok := matchText[k.glyph]
				if !ok {
					t.Fatalf("no footerLegend entry parsed for glyph %q -- extraction is broken, not the source", k.glyph)
				}
				legend := before.footerKeyLegend()
				advertised := strings.Contains(legend, text)

				after := k.press(before)
				acted := k.acted(after, rec)

				if advertised != acted {
					t.Errorf("%s row, key %q: footer advertised=%v (legend=%q) but the handler acted=%v -- the footer and the key handler must share exactly one eligibility definition (SPEC \u00a711.3)",
						rc.name, k.glyph, advertised, legend, acted)
				}
			})
		}
	}
}
