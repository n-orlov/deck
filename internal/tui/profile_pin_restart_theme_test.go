package tui

import (
	"context"
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
	"github.com/n-orlov/deck/internal/theme"
)

// This file is task 020's own test obligation, the same shape
// archive_delete_confirm_theme_test.go (019) already carries for the
// single-session archive/delete confirms: the `P` permission-profile
// switch, `p` pin/start-fresh and `R` restart-or-inject-instead dialogs
// must render in SPEC.md:1355's §11.6 tokens at render time, with the
// dialog's one left/right-cycle target ("New:"/"Choice:") carrying the
// same `selection` treatment a selected list row does.

// task020ProfileModel opens the `P` dialog on a claude session at 80x24
// with colour enabled.
func task020ProfileModel(t *testing.T) Model {
	t.Helper()
	m := NewWithShellCreatorAttacherKillerResumerAndProfileSwitcher(
		nil, config.Settings{Color: true}, "", nil, nil, nil, nil, nil,
		func(ctx context.Context, id, profile string) (store.Session, error) {
			return store.Session{ID: id, PermissionProfile: profile}, nil
		},
	)
	m.width, m.height = 80, 24
	m.sessions = []store.Session{{ID: "s1", Name: "alpha", Agent: "claude", Status: "running", PermissionProfile: "safe"}}
	m.selected = rowCursor(0)
	got, _ := m.Update(key("P"))
	return got.(Model)
}

// task020PinModel opens the `p` dialog on a claude session with a
// conversation id at 80x24 with colour enabled.
func task020PinModel(t *testing.T) Model {
	t.Helper()
	m := NewWithShellCreatorAttacherKillerResumerProfileSwitcherAndResumeModer(
		nil, config.Settings{Color: true}, "", nil, nil, nil, nil, nil, nil,
		func(ctx context.Context, id, mode string) (store.Session, error) {
			return store.Session{ID: id, ResumeState: mode}, nil
		},
	)
	m.width, m.height = 80, 24
	m.sessions = []store.Session{{ID: "s1", Name: "alpha", Agent: "claude", Status: "running", ConversationID: "conv-1", ResumeState: "auto"}}
	m.selected = rowCursor(0)
	got, _ := m.Update(key("p"))
	return got.(Model)
}

// task020RestartChoiceModel opens the `R` restart-or-inject choice on a
// shell session at 80x24 with colour enabled.
func task020RestartChoiceModel(t *testing.T) Model {
	t.Helper()
	m := New(nil, config.Settings{Color: true}, "")
	m.width, m.height = 80, 24
	m.sessions = []store.Session{{ID: "s1", Name: "alpha", Agent: "shell", Status: "running"}}
	m.selected = rowCursor(0)
	m.restartChoosing = true
	m.restartChoiceValue = "restart"
	m.restartChoiceNote = ""
	return m
}

// TestProfileSwitchStyledBodyMatchesPlainBodyOnceStripped proves the
// styled body task 020 adds is the same dialog as profileSwitchBody --
// what the existing substring tests in profile_switch_test.go assert
// against -- with only colour added: once every escape sequence is
// stripped back out of both sides, styledProfileSwitchBody is
// byte-identical to wrapDialogLines(profileSwitchBody()) line for line.
func TestProfileSwitchStyledBodyMatchesPlainBodyOnceStripped(t *testing.T) {
	for _, tc := range []struct {
		name string
		mut  func(m *Model)
	}{
		{"initial candidate", func(m *Model) {}},
		{"cycled candidate", func(m *Model) { m.profileSwitchValue = "yolo" }},
		{"with a failure note", func(m *Model) { m.profileSwitchNote = "Cannot change permission profile: boom" }},
	} {
		m := task020ProfileModel(t)
		tc.mut(&m)

		plain := m.wrapDialogLines(m.profileSwitchBody())
		styled := strings.Split(m.styledProfileSwitchBody(), "\n")
		if len(styled) != len(plain) {
			t.Fatalf("%s: styled body has %d physical lines, plain has %d:\nplain: %q\nstyled: %q", tc.name, len(styled), len(plain), plain, styled)
		}
		for i := range plain {
			if got, want := stripANSI(styled[i]), stripANSI(plain[i]); got != want {
				t.Fatalf("%s: line %d differs once escapes are stripped:\n styled: %q\n  plain: %q", tc.name, i, got, want)
			}
		}
	}
}

// TestPinStyledBodyMatchesPlainBodyOnceStripped is the same proof for the
// pin/start-fresh dialog.
func TestPinStyledBodyMatchesPlainBodyOnceStripped(t *testing.T) {
	for _, tc := range []struct {
		name string
		mut  func(m *Model)
	}{
		{"initial candidate", func(m *Model) {}},
		{"cycled candidate", func(m *Model) { m.pinValue = "fresh-once" }},
		{"with a failure note", func(m *Model) { m.pinNote = "Cannot change resume mode: boom" }},
	} {
		m := task020PinModel(t)
		tc.mut(&m)

		plain := m.wrapDialogLines(m.pinBody())
		styled := strings.Split(m.styledPinBody(), "\n")
		if len(styled) != len(plain) {
			t.Fatalf("%s: styled body has %d physical lines, plain has %d:\nplain: %q\nstyled: %q", tc.name, len(styled), len(plain), plain, styled)
		}
		for i := range plain {
			if got, want := stripANSI(styled[i]), stripANSI(plain[i]); got != want {
				t.Fatalf("%s: line %d differs once escapes are stripped:\n styled: %q\n  plain: %q", tc.name, i, got, want)
			}
		}
	}
}

// TestRestartChoiceStyledBodyMatchesPlainBodyOnceStripped is the same
// proof for the restart-or-inject-instead choice.
func TestRestartChoiceStyledBodyMatchesPlainBodyOnceStripped(t *testing.T) {
	for _, tc := range []struct {
		name string
		mut  func(m *Model)
	}{
		{"initial candidate", func(m *Model) {}},
		{"cycled candidate", func(m *Model) { m.restartChoiceValue = "inject" }},
		{"with a failure note", func(m *Model) { m.restartChoiceNote = "injecting the environment is unavailable" }},
	} {
		m := task020RestartChoiceModel(t)
		tc.mut(&m)

		plain := m.wrapDialogLines(m.restartChoiceBody())
		styled := strings.Split(m.styledRestartChoiceBody(), "\n")
		if len(styled) != len(plain) {
			t.Fatalf("%s: styled body has %d physical lines, plain has %d:\nplain: %q\nstyled: %q", tc.name, len(styled), len(plain), plain, styled)
		}
		for i := range plain {
			if got, want := stripANSI(styled[i]), stripANSI(plain[i]); got != want {
				t.Fatalf("%s: line %d differs once escapes are stripped:\n styled: %q\n  plain: %q", tc.name, i, got, want)
			}
		}
	}
}

// TestProfileSwitchTokensMatchSpec proves SPEC.md:1355's token mapping for
// the `P` dialog: the title in `title`, the restart-to-apply sentence in
// `dimmed`, the footer legend's keys in `key` with the surrounding prose
// in `hint`, and the "New:" row's own label/value pair carrying the
// `selection` background (the dialog's one left/right-cycle target) while
// the "Current:" row does not.
func TestProfileSwitchTokensMatchSpec(t *testing.T) {
	m := task020ProfileModel(t)
	titleHex := tokenHex(t, m, theme.Title)
	dimmedHex := tokenHex(t, m, theme.Dimmed)
	keyHex := tokenHex(t, m, theme.Key)
	hintHex := tokenHex(t, m, theme.Hint)
	selectionHex := tokenHex(t, m, theme.Selection)
	backgroundHex := tokenHex(t, m, theme.Background)

	view := m.View()
	term := renderSettingsToEmulator(t, view, m.width, m.height)

	titleRow := findRowContaining(t, term, "Change permission profile for alpha")
	titleCol := findCol(t, term, titleRow, "Change")
	if fg, ok := cellFgHex(t, term, titleCol, titleRow); !ok || fg != titleHex {
		t.Fatalf("title foreground = %q ok=%v, want title token %s", fg, ok, titleHex)
	}

	dimRow := findRowContaining(t, term, "This applies on the session's next launch/restart")
	dimCol := findCol(t, term, dimRow, "This")
	if fg, ok := cellFgHex(t, term, dimCol, dimRow); !ok || fg != dimmedHex {
		t.Fatalf("explanation foreground = %q ok=%v, want dimmed token %s", fg, ok, dimmedHex)
	}

	legendRow := findRowContaining(t, term, "Enter confirms")
	enterCol := findCol(t, term, legendRow, "Enter")
	if fg, ok := cellFgHex(t, term, enterCol, legendRow); !ok || fg != keyHex {
		t.Fatalf("legend Enter foreground = %q ok=%v, want key token %s", fg, ok, keyHex)
	}

	newRow := findRowContaining(t, term, "New:")
	newCol := findCol(t, term, newRow, "New:")
	if bg, ok := cellBgHex(t, term, newCol, newRow); !ok || bg != selectionHex {
		t.Fatalf("focused New row background = %q ok=%v, want selection token %s", bg, ok, selectionHex)
	}

	curRow := findRowContaining(t, term, "Current:")
	curCol := findCol(t, term, curRow, "Current:")
	// Task 004/R118: an unfocused dialog row carries no per-row token, but
	// every dialog row still composes through fullBoxContentLine's own
	// "" -> theme.Background fallback (canvasBackground), so it is never
	// left to the terminal's own background the way it was before this
	// task -- it now carries the theme's plain canvas colour instead of
	// none at all.
	if bg, ok := cellBgHex(t, term, curCol, curRow); !ok || bg != backgroundHex {
		t.Fatalf("unfocused Current row background = %q ok=%v, want theme.Background %s", bg, ok, backgroundHex)
	}

	if keyHex == hintHex {
		t.Skip("this theme's key and hint tokens share a colour; the prose-vs-key distinctness assertion below would be vacuous")
	}
	proseCol := findCol(t, term, legendRow, "confirms")
	if fg, ok := cellFgHex(t, term, proseCol, legendRow); !ok || fg != hintHex {
		t.Fatalf("legend prose foreground = %q ok=%v, want hint token %s", fg, ok, hintHex)
	}
}

// TestProfileSwitchNoteIsError mirrors TestArchiveConfirmNoteIsError for
// the `P` dialog's own failed-submit note.
func TestProfileSwitchNoteIsError(t *testing.T) {
	m := task020ProfileModel(t)
	m.profileSwitchNote = "Cannot change permission profile: boom"
	errorHex := tokenHex(t, m, theme.Error)

	view := m.View()
	term := renderSettingsToEmulator(t, view, m.width, m.height)

	row := findRowContaining(t, term, "Cannot change permission profile")
	col := findCol(t, term, row, "Cannot")
	if fg, ok := cellFgHex(t, term, col, row); !ok || fg != errorHex {
		t.Fatalf("profileSwitchNote foreground = %q ok=%v, want error token %s", fg, ok, errorHex)
	}
}

// TestPinTokensMatchSpec is TestProfileSwitchTokensMatchSpec's counterpart
// for the `p` dialog.
func TestPinTokensMatchSpec(t *testing.T) {
	m := task020PinModel(t)
	titleHex := tokenHex(t, m, theme.Title)
	dimmedHex := tokenHex(t, m, theme.Dimmed)
	keyHex := tokenHex(t, m, theme.Key)
	selectionHex := tokenHex(t, m, theme.Selection)
	backgroundHex := tokenHex(t, m, theme.Background)

	view := m.View()
	term := renderSettingsToEmulator(t, view, m.width, m.height)

	titleRow := findRowContaining(t, term, "Change resume mode for alpha")
	titleCol := findCol(t, term, titleRow, "Change")
	if fg, ok := cellFgHex(t, term, titleCol, titleRow); !ok || fg != titleHex {
		t.Fatalf("title foreground = %q ok=%v, want title token %s", fg, ok, titleHex)
	}

	dimRow := findRowContaining(t, term, "pinned always resumes")
	dimCol := findCol(t, term, dimRow, "pinned")
	if fg, ok := cellFgHex(t, term, dimCol, dimRow); !ok || fg != dimmedHex {
		t.Fatalf("explanation foreground = %q ok=%v, want dimmed token %s", fg, ok, dimmedHex)
	}

	legendRow := findRowContaining(t, term, "Enter confirms")
	enterCol := findCol(t, term, legendRow, "Enter")
	if fg, ok := cellFgHex(t, term, enterCol, legendRow); !ok || fg != keyHex {
		t.Fatalf("legend Enter foreground = %q ok=%v, want key token %s", fg, ok, keyHex)
	}

	newRow := findRowContaining(t, term, "New:")
	newCol := findCol(t, term, newRow, "New:")
	if bg, ok := cellBgHex(t, term, newCol, newRow); !ok || bg != selectionHex {
		t.Fatalf("focused New row background = %q ok=%v, want selection token %s", bg, ok, selectionHex)
	}

	curRow := findRowContaining(t, term, "Current:")
	curCol := findCol(t, term, curRow, "Current:")
	// Task 004/R118: see TestProfileSwitchTokensMatchSpec's own comment --
	// an unfocused dialog row now carries theme.Background rather than no
	// background at all.
	if bg, ok := cellBgHex(t, term, curCol, curRow); !ok || bg != backgroundHex {
		t.Fatalf("unfocused Current row background = %q ok=%v, want theme.Background %s", bg, ok, backgroundHex)
	}
}

// TestPinNoteIsError mirrors TestProfileSwitchNoteIsError for the `p`
// dialog's own failed-submit note.
func TestPinNoteIsError(t *testing.T) {
	m := task020PinModel(t)
	m.pinNote = "Cannot change resume mode: boom"
	errorHex := tokenHex(t, m, theme.Error)

	view := m.View()
	term := renderSettingsToEmulator(t, view, m.width, m.height)

	row := findRowContaining(t, term, "Cannot change resume mode")
	col := findCol(t, term, row, "Cannot")
	if fg, ok := cellFgHex(t, term, col, row); !ok || fg != errorHex {
		t.Fatalf("pinNote foreground = %q ok=%v, want error token %s", fg, ok, errorHex)
	}
}

// TestRestartChoiceTokensMatchSpec is TestProfileSwitchTokensMatchSpec's
// counterpart for the `R` restart-or-inject-instead choice: unlike the
// other two dialogs, there is only one field row ("Choice:"), which is
// always the focused cycle target and so always carries the `selection`
// background.
func TestRestartChoiceTokensMatchSpec(t *testing.T) {
	m := task020RestartChoiceModel(t)
	titleHex := tokenHex(t, m, theme.Title)
	dimmedHex := tokenHex(t, m, theme.Dimmed)
	keyHex := tokenHex(t, m, theme.Key)
	selectionHex := tokenHex(t, m, theme.Selection)

	view := m.View()
	term := renderSettingsToEmulator(t, view, m.width, m.height)

	titleRow := findRowContaining(t, term, "Restart or inject for alpha")
	titleCol := findCol(t, term, titleRow, "Restart")
	if fg, ok := cellFgHex(t, term, titleCol, titleRow); !ok || fg != titleHex {
		t.Fatalf("title foreground = %q ok=%v, want title token %s", fg, ok, titleHex)
	}

	dimRow := findRowContaining(t, term, "restart kills this session's pane")
	dimCol := findCol(t, term, dimRow, "restart")
	if fg, ok := cellFgHex(t, term, dimCol, dimRow); !ok || fg != dimmedHex {
		t.Fatalf("explanation foreground = %q ok=%v, want dimmed token %s", fg, ok, dimmedHex)
	}

	legendRow := findRowContaining(t, term, "Enter confirms")
	enterCol := findCol(t, term, legendRow, "Enter")
	if fg, ok := cellFgHex(t, term, enterCol, legendRow); !ok || fg != keyHex {
		t.Fatalf("legend Enter foreground = %q ok=%v, want key token %s", fg, ok, keyHex)
	}

	choiceRow := findRowContaining(t, term, "Choice:")
	choiceCol := findCol(t, term, choiceRow, "Choice:")
	if bg, ok := cellBgHex(t, term, choiceCol, choiceRow); !ok || bg != selectionHex {
		t.Fatalf("focused Choice row background = %q ok=%v, want selection token %s", bg, ok, selectionHex)
	}
}

// TestRestartChoiceNoteIsError mirrors TestProfileSwitchNoteIsError for
// the `R` choice's own failed-submit note.
func TestRestartChoiceNoteIsError(t *testing.T) {
	m := task020RestartChoiceModel(t)
	m.restartChoiceNote = "injecting the environment is unavailable"
	errorHex := tokenHex(t, m, theme.Error)

	view := m.View()
	term := renderSettingsToEmulator(t, view, m.width, m.height)

	row := findRowContaining(t, term, "injecting the environment is unavailable")
	col := findCol(t, term, row, "injecting")
	if fg, ok := cellFgHex(t, term, col, row); !ok || fg != errorHex {
		t.Fatalf("restartChoiceNote foreground = %q ok=%v, want error token %s", fg, ok, errorHex)
	}
}
