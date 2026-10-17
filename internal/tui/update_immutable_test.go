package tui

import (
	"context"
	"os"
	"strings"
	"testing"

	upd "github.com/n-orlov/deck/internal/update"
)

// immutableUnset stands for "DECK_REQUIRE_IMMUTABLE is not in the
// environment" (the default) in setRequireImmutable.
const immutableUnset = "unset"

// setRequireImmutable sets DECK_REQUIRE_IMMUTABLE for one test, or removes
// it for immutableUnset; either is restored afterwards. The package
// baseline (TestMain) is the "0" opt-out.
func setRequireImmutable(t *testing.T, value string) {
	t.Helper()
	t.Setenv(upd.EnvRequireImmutable, value)
	if value == immutableUnset {
		_ = os.Unsetenv(upd.EnvRequireImmutable)
	}
}

// TestCtrlUConfirmationStatesImmutablePlan: the confirmation carries, on a
// line `Immutable release: <plan>`, what the update will do about the
// release's immutability (#104), asked through the real upd.PlanImmutable.
// The fake release server is a mirror with no release API, so requiring
// immutability there -- the default -- is a certain refusal and Enter is not
// offered as an install; DECK_REQUIRE_IMMUTABLE=0 opts out.
func TestCtrlUConfirmationStatesImmutablePlan(t *testing.T) {
	for _, c := range []struct {
		name    string
		require string
		want    string
		hint    string
	}{
		{"default on a mirror", immutableUnset, upd.ImmutableRefused, "Enter tries anyway and is refused · Esc cancels"},
		{"opted out", "0", upd.ImmutableReported, "Enter installs · Esc cancels"},
		{"required on a mirror", "1", upd.ImmutableRefused, "Enter tries anyway and is refused · Esc cancels"},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv(upd.EnvRequireAttestation, "")
			setRequireImmutable(t, c.require)
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
	if m.updateImmPlan != upd.ImmutableRequired || !strings.Contains(body, "unless GitHub reports the release\nimmutable") || !strings.Contains(body, "(DECK_REQUIRE_IMMUTABLE=0 opts out)") || !strings.Contains(body, "Enter installs") {
		t.Fatalf("first plan = %q, body:\n%s", m.updateImmPlan, body)
	}
	m, _ = ctrlUPress(t, m, key("esc"))
	plan = upd.ImmutableReported
	m, _ = ctrlUPress(t, m, ctrlUKey)
	if body := m.updateConfirmBody(); m.updateImmPlan != upd.ImmutableReported || !strings.Contains(body, "DECK_REQUIRE_IMMUTABLE=0 is set") || !strings.Contains(body, "still installs, and the\nresult says so") {
		t.Fatalf("second plan = %q, want a fresh answer:\n%s", m.updateImmPlan, body)
	}
	m, _ = ctrlUPress(t, m, key("esc"))
	plan = upd.ImmutableRefused
	m, _ = ctrlUPress(t, m, ctrlUKey)
	if body := m.updateConfirmBody(); !strings.Contains(body, "mirror with no GitHub release API") || !strings.Contains(body, "(DECK_REQUIRE_IMMUTABLE=0 opts out)") || !strings.Contains(body, "Enter tries anyway and is refused") {
		t.Fatalf("third plan = %q:\n%s", m.updateImmPlan, body)
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
// mirror (no release API). With DECK_REQUIRE_IMMUTABLE=0 it installs and the
// indicator says the release is not verified immutable; by default, and with
// DECK_REQUIRE_IMMUTABLE=1, it is refused: the indicator reads a calm "not
// installed" (not "update failed"), the cause names the opt-out, and the
// binary is untouched.
func TestRealInstallFromMirrorIsNotVerifiedImmutable(t *testing.T) {
	for _, require := range []string{"0", immutableUnset, "1"} {
		t.Run(require, func(t *testing.T) {
			t.Setenv(upd.EnvRequireAttestation, "")
			setRequireImmutable(t, require)
			m, exe, oldSum, _ := ctrlUInstallModel(t, false)
			m, _ = ctrlUPress(t, m, ctrlUKey)
			m, cmd := ctrlUPress(t, m, key("enter"))
			next, _ := m.Update(cmd())
			m = next.(Model)
			if require == "0" {
				if m.updateInd.state != updateInstalled || m.updateInd.immutable != upd.ImmutableUnknown || !strings.Contains(m.updateIndicatorText(), "not verified immutable") {
					t.Fatalf("indicator = %+v %q, want installed, unknown, named", m.updateInd, m.updateIndicatorText())
				}
				return
			}
			const want = "update not installed: v1.2.3 is not verified immutable — see deck update"
			if m.updateInd.state != updateFailed || !m.updateInd.notImmutable || m.updateIndicatorText() != want {
				t.Fatalf("indicator = %+v %q, want %q", m.updateInd, m.updateIndicatorText(), want)
			}
			if !strings.Contains(m.updateInd.reason, "mirror with no GitHub release API") || !strings.Contains(m.updateInd.reason, upd.EnvRequireImmutable+"=0") {
				t.Errorf("cause %q does not name the mirror and the opt-out", m.updateInd.reason)
			}
			if got := ctrlUFileSum(t, exe); got != oldSum {
				t.Errorf("binary sha256 = %s, want %s unchanged", got, oldSum)
			}
		})
	}
}

// TestOtherFailureIsNotHeld: a failure that is not the immutability refusal
// keeps the "update failed" note and is not held against a later check.
func TestOtherFailureIsNotHeld(t *testing.T) {
	msg := updateFailure(&upd.Refusal{Reason: upd.RefusedUnwritable, Message: "cannot write"})
	if msg.notImmutable || msg.cause != "cannot write" {
		t.Fatalf("updateFailure = %+v, want a plain failure", msg)
	}
	ind := updateIndicator{state: updateFailed, tag: indicatorTag, reason: msg.cause}
	if got := updateFailedNote(ind); got != "update failed — see deck update (cannot write)" {
		t.Errorf("note = %q", got)
	}
	m := Model{updateInd: ind}
	if m.heldNotImmutable(upd.Status{Latest: indicatorTag, Newer: true}) {
		t.Error("a plain failure held the release")
	}
}

// TestAutoUpdateRefusedNotImmutableIsNotRetried: with auto_update on and the
// default requirement, a mirror's release is refused once, with no download;
// a later check offering the same release neither re-offers nor re-installs
// it while deck runs, and a newer release is offered again.
func TestAutoUpdateRefusedNotImmutableIsNotRetried(t *testing.T) {
	setRequireImmutable(t, immutableUnset)
	srv := newAutoServer(t, false)
	exe, oldSum := autoBinary(t)
	m := autoRun(t, srv, exe, ctrlUCurrent, true)
	const want = "update not installed: v1.2.3 is not verified immutable — see deck update"
	if got := m.footerNote(); got != want {
		t.Fatalf("indicator = %q, want %q", got, want)
	}
	if srv.archives.Load() != 0 || ctrlUFileSum(t, exe) != oldSum {
		t.Fatal("the refused release was downloaded or installed")
	}
	next, cmd := m.Update(updateChecked{status: upd.Status{Latest: indicatorTag, Newer: true}})
	m = next.(Model)
	if cmd != nil || m.footerNote() != want {
		t.Fatalf("a re-check of the same release retried it (cmd %v) or changed the note to %q", cmd != nil, m.footerNote())
	}
	next, cmd = m.Update(updateChecked{status: upd.Status{Latest: "v1.2.4", Newer: true}})
	m = next.(Model)
	if cmd == nil || m.updateInd.state != updateInstalling || m.updateInd.tag != "v1.2.4" {
		t.Fatalf("a newer release was not taken up: %+v", m.updateInd)
	}
}
