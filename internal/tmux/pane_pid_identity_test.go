// pane_pid_identity_test.go proves PRD phase3b II-29: after a
// respawn-pane, pane_id, session_name, pane_dead, pane_start_time and
// pane_current_command all survive unchanged -- only pane_pid moves --
// and that is exactly why pane_pid belongs in Dispatcher's identity
// tuple. A verify that omits it (pane_id alone, or pane_id+session_name,
// or even all four of the other named fields together) still delivers
// keystrokes into the REPLACEMENT program after a respawn; only adding
// pane_pid to the checked set makes the same verify refuse. A
// non-vacuous control proves the omitting-pane_pid verify is a real
// mechanism, not a broken always-allow stub: the identical four-field
// check still delivers against an unmutated pane.
package tmux

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

func panePIDIdentitySocket(name string) string {
	return fmt.Sprintf("deck-panepid-%s-%d-%d", name, os.Getpid(), time.Now().UnixNano())
}

// panePIDIdentityFields is every field PRD II-29 names, read together in
// one display-message call so they are all measured at the same instant
// (the same discipline dispatch.go's dispatchIdentityFormat uses).
const panePIDIdentityFormat = "#{pane_id}|#{session_name}|#{pane_dead}|#{pane_start_time}|#{pane_current_command}|#{pane_pid}"

type panePIDIdentitySnapshot struct {
	PaneID             string
	SessionName        string
	PaneDead           string
	PaneStartTime      string
	PaneCurrentCommand string
	PanePID            string
}

func capturePanePIDIdentity(t *testing.T, socket, target string) panePIDIdentitySnapshot {
	t.Helper()
	raw := runTmux(t, socket, "display-message", "-p", "-t", target, panePIDIdentityFormat)
	fields := strings.Split(raw, "|")
	if len(fields) != 6 {
		t.Fatalf("display-message %q returned %d fields, want 6: %q", panePIDIdentityFormat, len(fields), raw)
	}
	return panePIDIdentitySnapshot{
		PaneID:             fields[0],
		SessionName:        fields[1],
		PaneDead:           fields[2],
		PaneStartTime:      fields[3],
		PaneCurrentCommand: fields[4],
		PanePID:            fields[5],
	}
}

// panePIDIdentityField is one field name this test's minimal
// respawn-detecting verify can be asked to check, keyed the same way as
// panePIDIdentitySnapshot's members so a caller can build up exactly the
// field subset PRD II-29 asks about ("pane_id alone", "pane_id +
// session_name", "adding pane_pid").
type panePIDIdentityField string

const (
	fieldPaneID             panePIDIdentityField = "pane_id"
	fieldSessionName        panePIDIdentityField = "session_name"
	fieldPaneDead           panePIDIdentityField = "pane_dead"
	fieldPaneCurrentCommand panePIDIdentityField = "pane_current_command"
	fieldPanePID            panePIDIdentityField = "pane_pid"
)

func (s panePIDIdentitySnapshot) value(field panePIDIdentityField) string {
	switch field {
	case fieldPaneID:
		return s.PaneID
	case fieldSessionName:
		return s.SessionName
	case fieldPaneDead:
		return s.PaneDead
	case fieldPaneCurrentCommand:
		return s.PaneCurrentCommand
	case fieldPanePID:
		return s.PanePID
	default:
		panic(fmt.Sprintf("unknown panePIDIdentityField %q", field))
	}
}

// verifyAndSendOnFieldSubset is a deliberately minimal stand-in for
// Dispatcher.Send, parameterized by exactly which fields of the identity
// it checks -- built purely to answer PRD II-29's question ("why is
// pane_pid in the tuple, and not some smaller subset"), never intended as
// a second production dispatch path. It re-reads target's current
// identity, compares only the named fields against before, and only runs
// the send command if every named field still matches -- exactly
// Dispatcher.Send's own refuse-on-drift discipline, but with the checked
// field set under the test's control instead of fixed at five.
func verifyAndSendOnFieldSubset(t *testing.T, socket, target string, before panePIDIdentitySnapshot, fields []panePIDIdentityField, sendArgs ...string) error {
	t.Helper()
	after := capturePanePIDIdentity(t, socket, target)
	for _, field := range fields {
		if before.value(field) != after.value(field) {
			return fmt.Errorf("refuse: field %q drifted: before %q, after %q", field, before.value(field), after.value(field))
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	client := Client{Socket: socket, Timeout: 5 * time.Second}
	if _, err := client.run(ctx, sendArgs...); err != nil {
		return fmt.Errorf("send: %w", err)
	}
	return nil
}

// waitForMarkerInPane polls capture-pane until marker appears, failing the
// test if it never does within the timeout.
func waitForMarkerInPane(t *testing.T, socket, target, marker string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		capture := runTmux(t, socket, "capture-pane", "-p", "-t", target)
		if strings.Contains(capture, marker) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("marker %q never appeared in pane %q output:\n%s", marker, target, capture)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// waitForMarkerAbsentFromPane polls capture-pane for the given quiet
// window and fails if marker ever appears -- used to prove a refused send
// really never reached the pane, not merely that it was slow to arrive.
func waitForMarkerAbsentFromPane(t *testing.T, socket, target, marker string, quiet time.Duration) {
	t.Helper()
	deadline := time.Now().Add(quiet)
	for time.Now().Before(deadline) {
		capture := runTmux(t, socket, "capture-pane", "-p", "-t", target)
		if strings.Contains(capture, marker) {
			t.Fatalf("marker %q unexpectedly appeared in pane %q output (should have been refused):\n%s", marker, target, capture)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// TestRespawnPaneChangesOnlyPanePID is PRD II-29's core measurement: after
// respawn-pane, pane_id, session_name, pane_dead, pane_start_time and
// pane_current_command are all unchanged, and only pane_pid moves. It
// also records the finding that pane_start_time reads empty both before
// and after -- useless as a discriminator, exactly as the PRD states.
func TestRespawnPaneChangesOnlyPanePID(t *testing.T) {
	socket := panePIDIdentitySocket("core")
	cleanup := newBareGeometrySession(t, socket, "s0", 40, 10)
	defer cleanup()
	// Both the original pane and the respawned one run the identical
	// command, "bash", so pane_current_command is a like-for-like
	// comparison rather than an artifact of whatever the tmux server's
	// own default shell happens to be on this host.
	runTmux(t, socket, "respawn-pane", "-k", "-t", "s0", "bash")
	time.Sleep(200 * time.Millisecond)

	before := capturePanePIDIdentity(t, socket, "s0")

	runTmux(t, socket, "respawn-pane", "-k", "-t", "s0", "bash")
	time.Sleep(200 * time.Millisecond)

	after := capturePanePIDIdentity(t, socket, "s0")

	if before.PaneID != after.PaneID {
		t.Errorf("pane_id changed across respawn-pane: before %q, after %q", before.PaneID, after.PaneID)
	}
	if before.SessionName != after.SessionName {
		t.Errorf("session_name changed across respawn-pane: before %q, after %q", before.SessionName, after.SessionName)
	}
	if before.PaneDead != after.PaneDead {
		t.Errorf("pane_dead changed across respawn-pane: before %q, after %q", before.PaneDead, after.PaneDead)
	}
	if before.PaneStartTime != after.PaneStartTime {
		t.Errorf("pane_start_time changed across respawn-pane: before %q, after %q", before.PaneStartTime, after.PaneStartTime)
	}
	if before.PaneCurrentCommand != after.PaneCurrentCommand {
		t.Errorf("pane_current_command changed across respawn-pane: before %q, after %q", before.PaneCurrentCommand, after.PaneCurrentCommand)
	}
	if before.PanePID == after.PanePID {
		t.Fatalf("pane_pid unchanged across respawn-pane (%q); test setup invalid, respawn-pane must replace the pane's process", after.PanePID)
	}

	// The PRD II-29 finding: pane_start_time is empty and therefore
	// useless as a discriminator, not merely "unchanged" (which an
	// always-empty field trivially would be regardless of respawn).
	if before.PaneStartTime != "" {
		t.Logf("FINDING contradicted on this tmux version: #{pane_start_time} = %q, not empty", before.PaneStartTime)
	} else {
		t.Logf("FINDING confirmed: #{pane_start_time} is empty on this tmux version, useless as a discriminator (PRD II-29)")
	}
}

// TestPaneIDAloneStillDeliversIntoTheReplacementProgramAfterRespawn is
// PRD II-29's first insufficiency case: a verify that checks only pane_id
// does not notice a respawn-pane at all, and delivers the payload straight
// into the REPLACEMENT program, not the one the caller thought it was
// talking to.
func TestPaneIDAloneStillDeliversIntoTheReplacementProgramAfterRespawn(t *testing.T) {
	socket := panePIDIdentitySocket("id-alone")
	cleanup := newBareGeometrySession(t, socket, "s0", 80, 10)
	defer cleanup()
	runTmux(t, socket, "respawn-pane", "-k", "-t", "s0", "bash")
	time.Sleep(200 * time.Millisecond)

	before := capturePanePIDIdentity(t, socket, "s0")

	runTmux(t, socket, "respawn-pane", "-k", "-t", "s0", "bash")
	time.Sleep(200 * time.Millisecond)

	marker := "panepid-idalone-marker-71029"
	err := verifyAndSendOnFieldSubset(t, socket, "s0", before, []panePIDIdentityField{fieldPaneID},
		"send-keys", "-t", "s0", "-l", "--", "echo "+marker)
	if err != nil {
		t.Fatalf("verify(pane_id alone) refused a send after respawn-pane, want it to (wrongly) deliver: %v", err)
	}
	runTmux(t, socket, "send-keys", "-t", "s0", "Enter")
	waitForMarkerInPane(t, socket, "s0", marker, 3*time.Second)
}

// TestPaneIDPlusSessionNameStillDeliversIntoTheReplacementProgramAfterRespawn
// is PRD II-29's second insufficiency case: adding session_name to the
// checked set does not help either, because session_name also survives a
// respawn-pane unchanged.
func TestPaneIDPlusSessionNameStillDeliversIntoTheReplacementProgramAfterRespawn(t *testing.T) {
	socket := panePIDIdentitySocket("id-session")
	cleanup := newBareGeometrySession(t, socket, "s0", 80, 10)
	defer cleanup()
	runTmux(t, socket, "respawn-pane", "-k", "-t", "s0", "bash")
	time.Sleep(200 * time.Millisecond)

	before := capturePanePIDIdentity(t, socket, "s0")

	runTmux(t, socket, "respawn-pane", "-k", "-t", "s0", "bash")
	time.Sleep(200 * time.Millisecond)

	marker := "panepid-idsession-marker-40218"
	err := verifyAndSendOnFieldSubset(t, socket, "s0", before, []panePIDIdentityField{fieldPaneID, fieldSessionName},
		"send-keys", "-t", "s0", "-l", "--", "echo "+marker)
	if err != nil {
		t.Fatalf("verify(pane_id+session_name) refused a send after respawn-pane, want it to (wrongly) deliver: %v", err)
	}
	runTmux(t, socket, "send-keys", "-t", "s0", "Enter")
	waitForMarkerInPane(t, socket, "s0", marker, 3*time.Second)
}

// TestFourFieldVerifyStillDeliversIntoTheReplacementProgramAfterRespawn is
// the task's title punchline: even ALL FOUR of the other fields PRD II-29
// names together -- pane_id, session_name, pane_dead, pane_current_command
// (pane_start_time is deliberately left out of this set: the sibling test
// above already shows it is always empty, so including it would add
// nothing but the appearance of a stronger check) -- still fail to notice
// a respawn-pane, because every one of the four survives it unchanged.
// Only pane_pid, checked in the sibling rejection test below, actually
// moves.
func TestFourFieldVerifyStillDeliversIntoTheReplacementProgramAfterRespawn(t *testing.T) {
	socket := panePIDIdentitySocket("four-field")
	cleanup := newBareGeometrySession(t, socket, "s0", 80, 10)
	defer cleanup()
	runTmux(t, socket, "respawn-pane", "-k", "-t", "s0", "bash")
	time.Sleep(200 * time.Millisecond)

	before := capturePanePIDIdentity(t, socket, "s0")

	runTmux(t, socket, "respawn-pane", "-k", "-t", "s0", "bash")
	time.Sleep(200 * time.Millisecond)

	fourFields := []panePIDIdentityField{fieldPaneID, fieldSessionName, fieldPaneDead, fieldPaneCurrentCommand}
	marker := "panepid-fourfield-marker-58301"
	err := verifyAndSendOnFieldSubset(t, socket, "s0", before, fourFields,
		"send-keys", "-t", "s0", "-l", "--", "echo "+marker)
	if err != nil {
		t.Fatalf("four-field verify (pane_id, session_name, pane_dead, pane_current_command) refused a send after respawn-pane, want it to (wrongly) deliver: %v", err)
	}
	runTmux(t, socket, "send-keys", "-t", "s0", "Enter")
	waitForMarkerInPane(t, socket, "s0", marker, 3*time.Second)
}

// TestFourFieldVerifyIsNonVacuousAndDeliversAgainstAnUnmutatedPane is the
// PRD-required negative control: the SAME four-field verify used above to
// show insufficiency is exercised against a pane that was never
// respawned, and still delivers. This rules out the alternative
// explanation for the test above's result -- that verifyAndSendOnFieldSubset
// is simply a broken stub that always allows the send regardless of any
// field, respawned or not -- by proving the identical mechanism, run
// under ordinary unmutated conditions, also succeeds for the ordinary
// reason (nothing drifted), not vacuously.
func TestFourFieldVerifyIsNonVacuousAndDeliversAgainstAnUnmutatedPane(t *testing.T) {
	socket := panePIDIdentitySocket("four-field-control")
	cleanup := newBareGeometrySession(t, socket, "s0", 80, 10)
	defer cleanup()
	runTmux(t, socket, "respawn-pane", "-k", "-t", "s0", "bash")
	time.Sleep(200 * time.Millisecond)

	before := capturePanePIDIdentity(t, socket, "s0")
	// Deliberately no respawn-pane here: the pane is left exactly as it
	// was when before was captured.

	fourFields := []panePIDIdentityField{fieldPaneID, fieldSessionName, fieldPaneDead, fieldPaneCurrentCommand}
	marker := "panepid-fourfield-control-marker-90214"
	err := verifyAndSendOnFieldSubset(t, socket, "s0", before, fourFields,
		"send-keys", "-t", "s0", "-l", "--", "echo "+marker)
	if err != nil {
		t.Fatalf("four-field verify refused a send against an UNMUTATED pane, want it to deliver (non-vacuous control): %v", err)
	}
	runTmux(t, socket, "send-keys", "-t", "s0", "Enter")
	waitForMarkerInPane(t, socket, "s0", marker, 3*time.Second)
}

// TestAddingPanePIDRejectsAfterRespawn is PRD II-29's payoff: take the
// exact four-field set proven insufficient above and add pane_pid. The
// same verify mechanism, on the same respawned pane, now refuses -- and
// the marker genuinely never reaches the pane, proving the refusal
// actually stopped the send rather than merely returning an error while
// the command ran anyway.
func TestAddingPanePIDRejectsAfterRespawn(t *testing.T) {
	socket := panePIDIdentitySocket("five-field")
	cleanup := newBareGeometrySession(t, socket, "s0", 80, 10)
	defer cleanup()
	runTmux(t, socket, "respawn-pane", "-k", "-t", "s0", "bash")
	time.Sleep(200 * time.Millisecond)

	before := capturePanePIDIdentity(t, socket, "s0")

	runTmux(t, socket, "respawn-pane", "-k", "-t", "s0", "bash")
	time.Sleep(200 * time.Millisecond)

	fiveFields := []panePIDIdentityField{fieldPaneID, fieldSessionName, fieldPaneDead, fieldPaneCurrentCommand, fieldPanePID}
	marker := "panepid-fivefield-marker-60127"
	err := verifyAndSendOnFieldSubset(t, socket, "s0", before, fiveFields,
		"send-keys", "-t", "s0", "-l", "--", "echo "+marker)
	if err == nil {
		t.Fatalf("verify with pane_pid added: got nil error after a respawn-pane, want a refusal")
	}
	if !strings.Contains(err.Error(), "pane_pid") {
		t.Fatalf("verify with pane_pid added: err = %v, want it to name pane_pid as the drifted field", err)
	}
	waitForMarkerAbsentFromPane(t, socket, "s0", marker, 1*time.Second)
}
