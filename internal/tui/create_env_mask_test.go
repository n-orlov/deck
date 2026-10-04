package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// SPEC §6.4: "Values whose key matches *TOKEN*|*SECRET*|*KEY*|*PASSWORD*|
// *CREDENTIAL* are masked in every view ... reveal is a per-view explicit
// toggle." The create modal's Env field is one such view.

const createEnvSecretValue = "hunter2-do-not-draw"

func createEnvModal(t *testing.T, text string) Model {
	t.Helper()
	m := createEditOpen(t, t.TempDir())
	m.createField = createFieldEnv
	m.setCreateText(createFieldEnv, text)
	return m
}

func createEnvCtrlR(t *testing.T, m Model) Model {
	t.Helper()
	updated, _ := m.Update(tea.KeyMsg(tea.Key{Type: tea.KeyCtrlR}))
	return updated.(Model)
}

func TestCreateEnvFieldDoesNotDrawAPrefilledSecretValue(t *testing.T) {
	m := createEnvModal(t, "AUDIT_TOKEN="+createEnvSecretValue+",PLAIN=visible")
	for _, view := range []string{m.createBody(), stripANSI(m.createView()), m.styledCreateBody()} {
		if strings.Contains(view, createEnvSecretValue) {
			t.Fatalf("a secret-shaped env value is drawn in clear in the create modal:\n%s", view)
		}
	}
	row := createEditRow(t, m, "Env (key=value, comma-separated)")
	if !strings.Contains(row, "AUDIT_TOKEN="+m.maskedSecretPlaceholder()) || !strings.Contains(row, "PLAIN=visible") {
		t.Fatalf("Env row = %q, want the secret masked by the placeholder and the plain value shown", row)
	}
	if got := m.createText(createFieldEnv); !strings.Contains(got, createEnvSecretValue) {
		t.Fatalf("masking changed the stored text: %q", got)
	}
}

func TestCreateEnvFieldMasksWhileTypingAndRevealTogglesPerView(t *testing.T) {
	m := createEnvModal(t, "")
	m = createEditType(t, m, "DB_PASSWORD="+createEnvSecretValue)
	if strings.Contains(m.createBody(), createEnvSecretValue) {
		t.Fatalf("typed secret is drawn in clear:\n%s", m.createBody())
	}
	revealed := createEnvCtrlR(t, m)
	if !strings.Contains(revealed.createBody(), createEnvSecretValue) {
		t.Fatalf("Ctrl+R on the Env field did not reveal the value:\n%s", revealed.createBody())
	}
	if again := createEnvCtrlR(t, revealed); strings.Contains(again.createBody(), createEnvSecretValue) {
		t.Fatal("a second Ctrl+R did not mask the value again")
	}
	updated, _ := revealed.Update(key("esc"))
	closed := updated.(Model)
	updated, _ = closed.Update(key("n"))
	if reopened := updated.(Model); reopened.createEnvReveal {
		t.Fatal("reopening the create modal kept the previous view's reveal")
	}
}

func TestCreateEnvRevealKeyOnlyActsOnTheEnvField(t *testing.T) {
	m := createEnvModal(t, "API_KEY=x")
	m.createField = createFieldName
	if createEnvCtrlR(t, m).createEnvReveal {
		t.Fatal("Ctrl+R toggled the Env reveal while another field was focused")
	}
}

func TestCreateEnvMaskedValueIsNotCopied(t *testing.T) {
	m := createEnvModal(t, "API_KEY="+createEnvSecretValue)
	if !m.createEnvMasked() {
		t.Fatal("a secret-shaped value is not reported masked")
	}
	if createEnvModal(t, "PLAIN=1").createEnvMasked() {
		t.Fatal("a plain env value is reported masked")
	}
}
