package tui

import (
	"context"
	"errors"
	"strings"
	"testing"

	upd "github.com/n-orlov/deck/internal/update"
)

// controlCorpus is R330's seven inputs: the server-derived text and what a
// sink may show of it once its control characters are removed.
var controlCorpus = []struct{ name, raw, want string }{
	{"ESC", "ok\x1b[31mred", "ok[31mred"},
	{"BEL", "a\x07b", "ab"},
	{"C1 CSI byte", "a\u009b31mb", "a31mb"},
	{"OSC title sequence", "x\x1b]0;pwned\x07y", "x]0;pwnedy"},
	{"newline", "line1\nline2", "line1line2"},
	{"normal string", "503 Service Unavailable", "503 Service Unavailable"},
	{"multi-byte string", "ошибка 配信 ✓ é", "ошибка 配信 ✓ é"},
}

func hasControlChar(s string) bool {
	return strings.ContainsFunc(s, func(r rune) bool { return r <= 0x1f || (r >= 0x7f && r <= 0x9f) })
}

// TestUpdateFailedTextStripsControlCharacters: the Failed note and the footer
// line it lands in carry no C0, DEL or C1 character from the cause.
func TestUpdateFailedTextStripsControlCharacters(t *testing.T) {
	for _, c := range controlCorpus {
		t.Run(c.name, func(t *testing.T) {
			want := "update failed — see deck update (" + c.want + ")"
			if got := updateFailedText(c.raw); got != want {
				t.Fatalf("updateFailedText(%q) = %q, want %q", c.raw, got, want)
			}
			m := footerNoteFrameModel(240, 24, false)
			m.settings.UpdateCheck = true
			m = sendUpdateMsg(t, m, updateFailedMsg{cause: c.raw})
			if got := m.updateIndicatorText(); got != want {
				t.Fatalf("indicator text %q, want %q", got, want)
			}
			// The footer is styled, so look at the text between its escapes
			// only: the cause must sit in it exactly as stripped.
			if footer := footerOf(m.mainView()); !strings.Contains(footer, c.want) {
				t.Fatalf("footer %q lacks %q", footer, c.want)
			}
		})
	}
}

// TestUpdateFailedMsgFromInstallerStripsControlCharacters: a ctrl+u install
// that fails with server text holding control characters records the cause
// without them.
func TestUpdateFailedMsgFromInstallerStripsControlCharacters(t *testing.T) {
	for _, c := range controlCorpus {
		t.Run(c.name, func(t *testing.T) {
			m := footerNoteFrameModel(240, 24, false)
			m = m.WithUpdateInstaller("v1.0.0", func(context.Context) (upd.Result, error) {
				return upd.Result{}, errors.New("GET https://mirror.example/x: " + c.raw)
			})
			m = checkedModel(t, m, true, upd.Status{Latest: indicatorTag, Newer: true}, nil)
			cmd := m.startUpdate()
			msg, ok := cmd().(updateFailedMsg)
			if !ok {
				t.Fatalf("install answered %T, want updateFailedMsg", cmd())
			}
			if hasControlChar(msg.cause) || !strings.HasSuffix(msg.cause, c.want) {
				t.Fatalf("cause %q: want no control character and the text %q", msg.cause, c.want)
			}
		})
	}
}

// TestCtrlUDialogStripsControlCharacters: the confirmation shows the target
// and the current version, both server- or build-derived text, without
// control characters.
func TestCtrlUDialogStripsControlCharacters(t *testing.T) {
	for _, c := range controlCorpus {
		t.Run(c.name, func(t *testing.T) {
			m := footerNoteFrameModel(240, 24, false)
			m = m.WithUpdateInstaller("v1.0.0"+c.raw, func(context.Context) (upd.Result, error) { return upd.Result{}, nil })
			m = checkedModel(t, m, true, upd.Status{Latest: indicatorTag + c.raw, Newer: true}, nil)
			m, _ = ctrlUPress(t, m, ctrlUKey)
			if !m.updateConfirming {
				t.Fatal("ctrl+u opened no confirmation")
			}
			body := m.updateConfirmBody()
			for _, want := range []string{"Current version: v1.0.0" + c.want + "\n", "Target version:  " + indicatorTag + c.want + "\n"} {
				if !strings.Contains(body, want) {
					t.Errorf("dialog text lacks %q:\n%s", want, body)
				}
			}
			// The only control character left is the newline between lines.
			if hasControlChar(strings.ReplaceAll(body, "\n", "")) {
				t.Errorf("dialog text holds a control character: %q", body)
			}
			if view := stripANSI(m.View()); !strings.Contains(view, indicatorTag+c.want) {
				t.Errorf("rendered dialog lacks the target %q:\n%s", indicatorTag+c.want, view)
			}
		})
	}
}

// TestUpdateFailedTextRedactsSanitisedText: removing control characters does
// not unmask a credential: one split by a control character is still masked,
// and the host stays.
func TestUpdateFailedTextRedactsSanitisedText(t *testing.T) {
	for _, cause := range []string{
		"GET https://bob:" + ctrlUSentinel + "@mirror.example/x: 503",
		"GET https://bob:\x1b" + ctrlUSentinel + "\x07@mirror.example/x: 503",
		"GET https://mirror.example/x?tok\x1ben=" + ctrlUSentinel + "\n&a=b: 500",
		"GET https://\u009b" + ctrlUSentinel + "@mirror.example/x: 502",
	} {
		note := updateFailedText(cause)
		if strings.Contains(note, ctrlUSentinel) || hasControlChar(note) || !strings.Contains(note, "mirror.example") || !strings.Contains(note, "***") {
			t.Errorf("note %q for cause %q leaks, holds a control character or lost the host", note, cause)
		}
	}
}
