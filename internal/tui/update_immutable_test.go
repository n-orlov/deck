package tui

import (
	"context"
	"strings"
	"testing"

	upd "github.com/n-orlov/deck/internal/update"
)

// TestCtrlUConfirmationStatesImmutablePlan: the confirmation carries, on a
// line `Immutable release: <plan>`, what the update will do about the
// release's immutability (#104), asked through the real upd.PlanImmutable.
// The fake release server is a mirror with no release API, so requiring
// immutability there is a certain refusal and Enter is not offered as an
// install.
func TestCtrlUConfirmationStatesImmutablePlan(t *testing.T) {
	for _, c := range []struct {
		name    string
		require string
		want    string
		hint    string
	}{
		{"reported", "", upd.ImmutableReported, "Enter installs · Esc cancels"},
		{"required on a mirror", "1", upd.ImmutableRefused, "Enter tries anyway and is refused · Esc cancels"},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv(upd.EnvRequireAttestation, "")
			t.Setenv(upd.EnvRequireImmutable, c.require)
			m, _, _, runs := ctrlUInstallModel(t, false)
			m, _ = ctrlUPress(t, m, ctrlUKey)
			if m.updateImmPlan != c.want {
				t.Fatalf("updateImmPlan = %q, want %q", m.updateImmPlan, c.want)
			}
			if runs.Load() != 0 {
				t.Fatal("the update ran before the confirmation was answered")
			}
			body := m.updateConfirmBody()
			if !strings.Contains(body, "\nImmutable release: "+c.want+"\n") || !strings.Contains(body, "\n"+c.hint+"\n") {
				t.Errorf("confirmation lacks %q or %q:\n%s", "Immutable release: "+c.want, c.hint, body)
			}
			if view := stripANSI(m.View()); !strings.Contains(view, "Immutable release: "+c.want) {
				t.Errorf("rendered dialog lacks the plan %q:\n%s", c.want, view)
			}
		})
	}
}

// TestCtrlUImmutablePlanInjected: the plan source is replaceable and asked
// each time the dialog opens; each plan has its own sentence.
func TestCtrlUImmutablePlanInjected(t *testing.T) {
	m, _, _, _ := ctrlUInstallModel(t, false)
	plan := upd.ImmutableRequired
	m = m.WithUpdateImmutablePlan(func() string { return plan })
	m, _ = ctrlUPress(t, m, ctrlUKey)
	body := m.updateConfirmBody()
	if m.updateImmPlan != upd.ImmutableRequired || !strings.Contains(body, "unless GitHub reports the release immutable") || !strings.Contains(body, "Enter installs") {
		t.Fatalf("first plan = %q, body:\n%s", m.updateImmPlan, body)
	}
	m, _ = ctrlUPress(t, m, key("esc"))
	plan = upd.ImmutableReported
	m, _ = ctrlUPress(t, m, ctrlUKey)
	if m.updateImmPlan != upd.ImmutableReported || !strings.Contains(m.updateConfirmBody(), "still installs, and the result says so") {
		t.Fatalf("second plan = %q, want a fresh answer", m.updateImmPlan)
	}
}

// TestInstalledIndicatorNamesImmutability: Result.Immutable reaches the
// indicator; a release not verified immutable (not-immutable or unknown) is
// named as such, an immutable one is not, and a checksum-only install names
// both caveats.
func TestInstalledIndicatorNamesImmutability(t *testing.T) {
	for _, c := range []struct {
		att, imm string
		want     string
	}{
		{upd.AttestationVerified, upd.ImmutableVerified, "update installed: v1.2.3 — restart deck to apply"},
		{upd.AttestationVerified, upd.ImmutableNot, "update installed: v1.2.3 (not verified immutable) — restart deck to apply"},
		{upd.AttestationVerified, upd.ImmutableUnknown, "update installed: v1.2.3 (not verified immutable) — restart deck to apply"},
		{upd.AttestationSkipped, upd.ImmutableUnknown, "update installed: v1.2.3 (checksum-only, attestation not verified; not verified immutable) — restart deck to apply"},
		{upd.AttestationSkipped, upd.ImmutableVerified, "update installed: v1.2.3 (checksum-only, attestation not verified) — restart deck to apply"},
	} {
		t.Run(c.att+"/"+c.imm, func(t *testing.T) {
			m := footerNoteFrameModel(240, 24, false)
			m = m.WithUpdateInstaller(ctrlUCurrent, func(context.Context) (upd.Result, error) {
				return upd.Result{Tag: indicatorTag, Attestation: c.att, Immutable: c.imm}, nil
			})
			m = checkedModel(t, m, true, upd.Status{Latest: indicatorTag, Newer: true}, nil)
			next, _ := m.Update(m.startUpdate()())
			m = next.(Model)
			if m.updateInd.state != updateInstalled || m.updateInd.immutable != c.imm {
				t.Fatalf("indicator = %+v, want installed with immutable %q", m.updateInd, c.imm)
			}
			if got := m.updateIndicatorText(); got != c.want {
				t.Errorf("indicator = %q, want %q", got, c.want)
			}
		})
	}
}

// TestRealInstallFromMirrorIsNotVerifiedImmutable: the real update against a
// mirror (no release API) installs and the indicator says the release is not
// verified immutable; with DECK_REQUIRE_IMMUTABLE=1 it fails and the binary
// is untouched.
func TestRealInstallFromMirrorIsNotVerifiedImmutable(t *testing.T) {
	for _, require := range []bool{false, true} {
		name := "reported"
		if require {
			name = "required"
		}
		t.Run(name, func(t *testing.T) {
			t.Setenv(upd.EnvRequireAttestation, "")
			if require {
				t.Setenv(upd.EnvRequireImmutable, "1")
			} else {
				t.Setenv(upd.EnvRequireImmutable, "")
			}
			m, exe, oldSum, _ := ctrlUInstallModel(t, false)
			m, _ = ctrlUPress(t, m, ctrlUKey)
			m, cmd := ctrlUPress(t, m, key("enter"))
			next, _ := m.Update(cmd())
			m = next.(Model)
			if !require {
				if m.updateInd.state != updateInstalled || m.updateInd.immutable != upd.ImmutableUnknown || !strings.Contains(m.updateIndicatorText(), "not verified immutable") {
					t.Fatalf("indicator = %+v %q, want installed, unknown, named", m.updateInd, m.updateIndicatorText())
				}
				return
			}
			if m.updateInd.state != updateFailed || !strings.Contains(m.updateIndicatorText(), upd.EnvRequireImmutable+"=1") {
				t.Fatalf("indicator = %+v %q, want failed naming the switch", m.updateInd, m.updateIndicatorText())
			}
			if got := ctrlUFileSum(t, exe); got != oldSum {
				t.Errorf("binary sha256 = %s, want %s unchanged", got, oldSum)
			}
		})
	}
}
