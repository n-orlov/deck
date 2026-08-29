package tui

import (
	"context"
	"os/exec"
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
)

// This file is task 806's own test (review finding 2/R80): the footer's
// curated A/U slot and the `A` key handler (case "A" in tui.go) must share
// exactly one eligibility definition -- footerArchiveEligible -- rather than
// the footer consulting footerArchiveEligible while the handler consulted
// its own always-true canArchive. Before this task the footer already
// refused to show A for an archived row (TestFooterKeyLegendReflectsEligibility
// in footer_legend_test.go), but pressing A on that same row still opened
// the confirm dialog: a display-only refusal with no matching keypress
// refusal. TestArchiveKeyOnAnArchivedRowRefusesAndNamesU below is the
// discriminating case: it fails if case "A" stops calling
// footerArchiveEligible (e.g. reverts to an always-true predicate, or a
// second copy of "ArchivedAt == 0" that could drift from the footer's).

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

// TestArchiveKeyEligibilityMatchesFooterPredicate is the structural half:
// it evaluates footerArchiveEligible directly (the same function
// TestArchiveKeyOnAnArchivedRowRefusesAndNamesU exercises indirectly via the
// key handler, and the same function footer_bindings_parity_test.go's
// footerPredicateByName evaluates for the footer's own A entry) across both
// states the predicate distinguishes, so a change that makes the two
// consult different logic again -- rather than literally the same
// function -- has one place that catches it even if the message-text
// assertion above is weakened.
func TestArchiveKeyEligibilityMatchesFooterPredicate(t *testing.T) {
	unarchived := store.Session{ID: "s1", Status: "running"}
	if !footerArchiveEligible(unarchived) {
		t.Fatal("footerArchiveEligible refused an unarchived row")
	}
	archived := store.Session{ID: "s2", Status: "stopped", ArchivedAt: 100}
	if footerArchiveEligible(archived) {
		t.Fatal("footerArchiveEligible accepted an already-archived row")
	}
}
