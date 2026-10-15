package tui

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	upd "github.com/n-orlov/deck/internal/update"
)

// attestationModes are the three things the ctrl+u confirmation can promise
// about the build attestation (R331). Each is reached through the real
// detection (upd.PlanAttestation), by what is on PATH and in the environment.
var attestationModes = []struct {
	name    string
	gh      bool // an authenticated gh is on PATH
	require bool // DECK_REQUIRE_ATTESTATION=1
	want    string
}{
	{"attested", true, false, upd.AttestationVerified},
	{"checksum-only", false, false, upd.AttestationSkipped},
	{"require-variable-with-no-gh", false, true, upd.AttestationRefused},
}

func attestationPath(t *testing.T, gh bool) {
	t.Helper()
	dir := t.TempDir()
	if gh {
		// `gh auth status` succeeds; nothing else is ever called here.
		if err := os.WriteFile(filepath.Join(dir, "gh"), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", dir)
}

// TestCtrlUConfirmationStatesAttestationPlan: before the update runs the
// confirmation carries the attestation state (the field, and the token on its
// own line), and the update has not started.
func TestCtrlUConfirmationStatesAttestationPlan(t *testing.T) {
	for _, c := range attestationModes {
		t.Run(c.name, func(t *testing.T) {
			attestationPath(t, c.gh)
			if c.require {
				t.Setenv(upd.EnvRequireAttestation, "1")
			} else {
				t.Setenv(upd.EnvRequireAttestation, "")
			}
			m, _, _, runs := ctrlUInstallModel(t, false)
			attestationPath(t, c.gh) // ctrlUInstallModel empties PATH
			m, _ = ctrlUPress(t, m, ctrlUKey)
			if !m.updateConfirming {
				t.Fatal("ctrl+u opened no confirmation")
			}
			if m.updateAttPlan != c.want {
				t.Fatalf("updateAttPlan = %q, want %q", m.updateAttPlan, c.want)
			}
			if runs.Load() != 0 {
				t.Fatal("the update ran before the confirmation was answered")
			}
			body := m.updateConfirmBody()
			if !strings.Contains(body, "\nAttestation: "+c.want+"\n") {
				t.Errorf("confirmation lacks the line %q:\n%s", "Attestation: "+c.want, body)
			}
			for _, other := range attestationModes {
				if other.want != c.want && strings.Contains(body, "Attestation: "+other.want+"\n") {
					t.Errorf("confirmation also carries the token of %q:\n%s", other.want, body)
				}
			}
			if view := stripANSI(m.View()); !strings.Contains(view, "Attestation: "+c.want) {
				t.Errorf("rendered dialog lacks the state %q:\n%s", c.want, view)
			}
		})
	}
}

// TestCtrlUAttestationPlanInjected: the plan source is replaceable and is
// asked each time the dialog opens, not once.
func TestCtrlUAttestationPlanInjected(t *testing.T) {
	m, _, _, _ := ctrlUInstallModel(t, false)
	plan := upd.AttestationVerified
	m = m.WithUpdateAttestationPlan(func() string { return plan })
	m, _ = ctrlUPress(t, m, ctrlUKey)
	if m.updateAttPlan != upd.AttestationVerified {
		t.Fatalf("first plan = %q", m.updateAttPlan)
	}
	m, _ = ctrlUPress(t, m, key("esc"))
	plan = upd.AttestationSkipped
	m, _ = ctrlUPress(t, m, ctrlUKey)
	if m.updateAttPlan != upd.AttestationSkipped {
		t.Fatalf("second plan = %q, want a fresh answer", m.updateAttPlan)
	}
}

// TestInstalledIndicatorKeepsAttestation: Result.Attestation reaches the
// indicator state; only a checksum-only install is named as such.
func TestInstalledIndicatorKeepsAttestation(t *testing.T) {
	for _, c := range []struct {
		att      string
		wantName bool
	}{
		{upd.AttestationVerified, false},
		{upd.AttestationSkipped, true},
	} {
		t.Run(c.att, func(t *testing.T) {
			m := footerNoteFrameModel(240, 24, false)
			m = m.WithUpdateInstaller(ctrlUCurrent, func(context.Context) (upd.Result, error) {
				return upd.Result{Tag: indicatorTag, Attestation: c.att}, nil
			})
			m = checkedModel(t, m, true, upd.Status{Latest: indicatorTag, Newer: true}, nil)
			msg := m.startUpdate()()
			next, _ := m.Update(msg)
			m = next.(Model)
			if m.updateInd.state != updateInstalled || m.updateInd.attestation != c.att {
				t.Fatalf("indicator = %+v, want installed with attestation %q", m.updateInd, c.att)
			}
			text := m.updateIndicatorText()
			if got := strings.Contains(text, upd.AttestationSkipped); got != c.wantName {
				t.Errorf("indicator %q names checksum-only = %v, want %v", text, got, c.wantName)
			}
		})
	}
}

// TestRealInstallWithoutGhIsMarkedChecksumOnly: the real update with no gh on
// PATH ends in the Installed state with the checksum-only attestation.
func TestRealInstallWithoutGhIsMarkedChecksumOnly(t *testing.T) {
	m, _, _, runs := ctrlUInstallModel(t, false)
	m, _ = ctrlUPress(t, m, ctrlUKey)
	if m.updateAttPlan != upd.AttestationSkipped {
		t.Fatalf("plan = %q, want checksum-only with no gh", m.updateAttPlan)
	}
	m, cmd := ctrlUPress(t, m, key("enter"))
	next, _ := m.Update(cmd())
	m = next.(Model)
	if runs.Load() != 1 || m.updateInd.state != updateInstalled || m.updateInd.attestation != upd.AttestationSkipped {
		t.Fatalf("runs=%d indicator=%+v, want one install, installed, checksum-only", runs.Load(), m.updateInd)
	}
}
