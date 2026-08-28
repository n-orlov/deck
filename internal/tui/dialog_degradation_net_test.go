package tui

import (
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/theme"
)

// This file is task 107's own success criterion (R82): net EVERY dialog
// themed across this phase against NO_COLOR and DECK_ASCII set TOGETHER --
// config.Load resolves the two independently (config.go: DECK_ASCII never
// touches Settings.Color, NO_COLOR/DECK_COLOR never touch Settings.ASCII),
// so exercising both at once is the one scenario that is not already a
// strict subset of "NO_COLOR alone" for these bodies -- and proving, for
// each dialog, that the degraded body carries no SGR bytes at all, that
// its text is byte-identical to the themed body once ANSI is stripped back
// out (no label or value lost to either the colour cut or the ASCII glyph
// fallback), and that wrapDialogLines/dialogContentBudget -- the functions
// every scroll/page keystroke measures against -- agree between the themed
// body and its own ANSI-stripped twin, so a theme change or DECK_ASCII
// toggling on can never move where a page boundary falls.
//
// Every case below reuses the SAME model-building helper the dialog's own
// theming task already built and already exercises with Color: true
// (task016CreateTestModel, task017EnvTestModel, task018BulkDeleteModel,
// task019ArchiveModel/task019DeleteModel, task020ProfileModel/
// task020PinModel/task020RestartChoiceModel, task021RenameModel,
// task021EventLogModel) rather than a fresh, potentially-drifting
// construction, so this file cannot silently diverge from what those
// dialogs' own theme tests already assert is a themed render.
type dialogDegradationMutation struct {
	name string
	mut  func(m *Model)
}

type dialogDegradationCase struct {
	name string
	// build returns a colour-enabled (Color: true) model for the dialog,
	// open and ready to render -- ASCII is left at its zero value (false)
	// here so degraded (below) is the only variant that ever sets it.
	build func(t *testing.T) Model
	// plain/styled are the dialog's own body pair: plain is what
	// wrapDialogLines/dialogMaxScroll/PgUp/PgDn measure, styled is the
	// coloured render body task 107 is netting against NO_COLOR/DECK_ASCII.
	plain  func(m Model) string
	styled func(m Model) string
	// mutations exercises more than the dialog's bare initial state --
	// principally the failure-note branch every §11.4 dialog carries in
	// `error` -- so "no label or value lost" is checked against more than
	// one shape of body per dialog.
	mutations []dialogDegradationMutation
}

func dialogDegradationCases() []dialogDegradationCase {
	return []dialogDegradationCase{
		{
			name:   "create modal",
			build:  task016CreateTestModel,
			plain:  func(m Model) string { return m.createBody() },
			styled: func(m Model) string { return m.styledCreateBody() },
			mutations: []dialogDegradationMutation{
				{"initial state", func(m *Model) {}},
				{"with a validation error", func(m *Model) { m.createError = "working directory is required" }},
			},
		},
		{
			name:   "env editor",
			build:  task017EnvTestModel,
			plain:  func(m Model) string { return m.envBody() },
			styled: func(m Model) string { return m.styledEnvBody() },
			mutations: []dialogDegradationMutation{
				{"initial state", func(m *Model) {}},
				{"with a failure note", func(m *Model) { m.envNote = "Cannot edit environment: boom" }},
			},
		},
		{
			name:   "bulk delete confirm",
			build:  func(t *testing.T) Model { return task018BulkDeleteModel(t, 3) },
			plain:  func(m Model) string { return m.bulkDeleteConfirmBody() },
			styled: func(m Model) string { return m.styledBulkDeleteConfirmBody() },
			mutations: []dialogDegradationMutation{
				{"initial state", func(m *Model) {}},
				{"with a failure note", func(m *Model) { m.deleteNote = "Cannot delete marked-01: boom" }},
			},
		},
		{
			name:   "delete/purge confirm",
			build:  task019DeleteModel,
			plain:  func(m Model) string { return m.deleteConfirmBody() },
			styled: func(m Model) string { return m.styledDeleteConfirmBody() },
			mutations: []dialogDegradationMutation{
				{"keep chosen", func(m *Model) {}},
				{"purge chosen, path resolved", func(m *Model) {
					m.deletePurgeValue = "purge"
					m.deletePurgeOK = true
					m.deletePurgePath = "/home/user/.claude/projects/x/conv-1.jsonl"
				}},
				{"with a failure note", func(m *Model) { m.deleteNote = "Cannot delete alpha: boom" }},
			},
		},
		{
			name:   "archive confirm",
			build:  task019ArchiveModel,
			plain:  func(m Model) string { return m.archiveConfirmBody() },
			styled: func(m Model) string { return m.styledArchiveConfirmBody() },
			mutations: []dialogDegradationMutation{
				{"live row", func(m *Model) {}},
				{"with a failure note", func(m *Model) { m.archiveNote = "Cannot archive alpha: boom" }},
			},
		},
		{
			name:   "profile picker",
			build:  task020ProfileModel,
			plain:  func(m Model) string { return m.profileSwitchBody() },
			styled: func(m Model) string { return m.styledProfileSwitchBody() },
			mutations: []dialogDegradationMutation{
				{"initial candidate", func(m *Model) {}},
				{"with a failure note", func(m *Model) { m.profileSwitchNote = "Cannot change permission profile: boom" }},
			},
		},
		{
			name:   "pin conversation",
			build:  task020PinModel,
			plain:  func(m Model) string { return m.pinBody() },
			styled: func(m Model) string { return m.styledPinBody() },
			mutations: []dialogDegradationMutation{
				{"initial candidate", func(m *Model) {}},
				{"with a failure note", func(m *Model) { m.pinNote = "Cannot change resume mode: boom" }},
			},
		},
		{
			name:   "restart-or-inject",
			build:  task020RestartChoiceModel,
			plain:  func(m Model) string { return m.restartChoiceBody() },
			styled: func(m Model) string { return m.styledRestartChoiceBody() },
			mutations: []dialogDegradationMutation{
				{"initial candidate", func(m *Model) {}},
				{"with a failure note", func(m *Model) { m.restartChoiceNote = "injecting the environment is unavailable" }},
			},
		},
		{
			name:   "rename",
			build:  task021RenameModel,
			plain:  func(m Model) string { return m.renameBody() },
			styled: func(m Model) string { return m.styledRenameBody() },
			mutations: []dialogDegradationMutation{
				{"prefilled value", func(m *Model) {}},
				{"with a failure note", func(m *Model) { m.renameNote = `session name "b" already exists` }},
			},
		},
		{
			name:   "event log",
			build:  task021EventLogModel,
			plain:  func(m Model) string { return m.eventLogBody() },
			styled: func(m Model) string { return m.styledEventLogBody() },
			mutations: []dialogDegradationMutation{
				{"populated", func(m *Model) {}},
				{"read error", func(m *Model) { m.eventLogErr = errRenameCollisionForTest }},
			},
		},
	}
}

// TestThemedDialogsDegradeCleanlyUnderNoColorAndASCII is task 107's own
// success criterion, run against every dialog themed this phase.
func TestThemedDialogsDegradeCleanlyUnderNoColorAndASCII(t *testing.T) {
	for _, tc := range dialogDegradationCases() {
		t.Run(tc.name, func(t *testing.T) {
			for _, mtc := range tc.mutations {
				t.Run(mtc.name, func(t *testing.T) {
					m := tc.build(t)
					mtc.mut(&m)

					themed := tc.styled(m)
					if !strings.ContainsRune(themed, 0x1b) {
						t.Fatalf("themed styled body carries no escape at all with Color enabled -- this case is not exercising colour, so the degradation proof below would be vacuous:\n%q", themed)
					}

					// degraded simulates NO_COLOR (Color: false) and
					// DECK_ASCII (ASCII: true) set together -- the one
					// combined scenario config.Load can actually produce,
					// since the two env vars gate entirely independent
					// Settings fields.
					degraded := m
					degraded.settings.Color = false
					degraded.settings.ASCII = true
					degradedBody := tc.styled(degraded)

					if strings.ContainsRune(degradedBody, 0x1b) {
						t.Fatalf("styled body under NO_COLOR+DECK_ASCII still carries an escape byte:\n%q", degradedBody)
					}
					strippedThemed := stripANSI(themed)
					if degradedBody != strippedThemed {
						t.Fatalf("NO_COLOR+DECK_ASCII body differs from the themed body once ANSI is stripped -- a label or value was lost:\n  degraded: %q\n  themed (stripped): %q", degradedBody, strippedThemed)
					}

					// wrapDialogLines/dialogContentBudget parity: the
					// measurement PgUp/PgDn and dialogMaxScroll run must
					// never move because colour was present.
					themedLines := m.wrapDialogLines(themed)
					strippedLines := m.wrapDialogLines(strippedThemed)
					if len(themedLines) != len(strippedLines) {
						t.Fatalf("wrapDialogLines(themed body) has %d physical lines, wrapDialogLines(ANSI-stripped themed body) has %d -- colour is moving where a line wraps:\nthemed: %q\nstripped: %q", len(themedLines), len(strippedLines), themedLines, strippedLines)
					}
					for i := range themedLines {
						if got, want := stripANSI(themedLines[i]), strippedLines[i]; got != want {
							t.Fatalf("wrapped physical line %d differs once stripped:\n themed: %q\n stripped: %q", i, got, want)
						}
					}
					if got, want := m.dialogContentBudget(), degraded.dialogContentBudget(); got != want {
						t.Fatalf("dialogContentBudget() = %d under the themed model, %d under NO_COLOR+DECK_ASCII -- the budget must not depend on colour/ASCII", got, want)
					}

					// The degraded STYLED body's physical lines must match
					// wrapDialogLines(the dialog's own PLAIN body, same
					// degraded settings) line for line, byte for byte -- the
					// same "styled == wrapped plain" proof every per-dialog
					// theme test already runs under Color: true, now run
					// under NO_COLOR+DECK_ASCII too.
					plainLines := degraded.wrapDialogLines(tc.plain(degraded))
					degradedLines := strings.Split(degradedBody, "\n")
					if len(degradedLines) != len(plainLines) {
						t.Fatalf("styled body under NO_COLOR+DECK_ASCII has %d physical lines, wrapDialogLines(plain body) has %d:\n plain: %q\n styled: %q", len(degradedLines), len(plainLines), plainLines, degradedLines)
					}
					for i := range plainLines {
						if degradedLines[i] != plainLines[i] {
							t.Fatalf("styled body under NO_COLOR+DECK_ASCII line %d differs from the plain body's own wrapped line:\n styled: %q\n plain:  %q", i, degradedLines[i], plainLines[i])
						}
					}
				})
			}
		})
	}
}

// TestRenameFieldRowTruncationReemitsItsOwnReset is task 107's own "at
// least one dialog" truncation clause (SPEC §11.3): renderRenameFieldRow
// (rename.go, task 105) composes the rename dialog's one focused field
// under a single Selection BACKGROUND span opened once and closed with
// exactly one trailing "\x1b[0m" at the very end of the line -- precisely
// the shape truncateToWidth's own doc comment (panel.go) says a caller
// that cuts the line before reaching that reset must not leave open. This
// asserts it against a REAL themed dialog line (not a synthetic string
// like escape_background_test.go's own generic proof), truncated to a
// budget narrower than the line's full width, so a background span left
// open by a themed §11.4 dialog can never bleed into whatever the box
// frame concatenates next.
func TestRenameFieldRowTruncationReemitsItsOwnReset(t *testing.T) {
	m := task021RenameModel(t)
	line := m.renderRenameFieldRow()

	full := stringWidth(line)
	budget := full - 3
	if budget < 1 {
		t.Fatalf("rename field row is only %d columns wide -- this test needs room to truncate it", full)
	}

	bgOpen, ok := m.backgroundSGR(theme.Selection)
	if !ok {
		t.Fatal("theme has no selection token to open a background span with")
	}
	if !strings.Contains(line, bgOpen) {
		t.Fatalf("rename field row does not open a Selection background at all -- this test needs a real open span to be non-vacuous:\n%q", line)
	}
	if strings.HasSuffix(line, "\x1b[0m") == false {
		t.Fatalf("rename field row's own natural reset is missing -- this test needs the untruncated line to close its span, so the truncated case actually proves something:\n%q", line)
	}

	got := truncateToWidth(line, budget)
	if !strings.Contains(got, bgOpen) {
		t.Fatalf("truncateToWidth(line, %d) dropped the opening Selection background escape entirely:\n%q", budget, got)
	}
	if !strings.HasSuffix(got, "\x1b[0m") {
		t.Fatalf("truncateToWidth(line, %d) left the Selection background span open -- no reset re-emitted:\n%q", budget, got)
	}
	if w := stringWidth(got); w > budget {
		t.Fatalf("truncateToWidth(line, %d) visible width = %d, exceeds the budget", budget, w)
	}
}
