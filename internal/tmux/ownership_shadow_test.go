package tmux

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// setShadowOwnership writes OwnershipOption at a scope other than the window's
// on a private server: scopeArgs is the `set-option` flag set that picks it.
func setShadowOwnership(t *testing.T, socket string, scopeArgs []string, value string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	args := append([]string{"-L", socket, "set-option"}, scopeArgs...)
	args = append(args, OwnershipOption, value)
	if output, err := exec.CommandContext(ctx, "tmux", args...).CombinedOutput(); err != nil {
		t.Fatalf("tmux %v: %v: %s", args, err, output)
	}
}

func shadowTestSocket(name string) string {
	return fmt.Sprintf("priv-ownership-%s-%d-%d", name, os.Getpid(), time.Now().UnixNano())
}

// TestClaimDeadPaneShadowNeverOverwritesLiveWindowClaim: a pane-scope option
// holding a dead claim over a window option holding a live claim never lets
// ClaimWindowOwnership overwrite the live window value. The pane value is what
// the compare-and-set condition would read, so the claim is refused with an
// error naming the pane scope.
func TestClaimDeadPaneShadowNeverOverwritesLiveWindowClaim(t *testing.T) {
	socket := shadowTestSocket("deadpane")
	cleanup := newBareOwnershipSession(t, socket)
	defer cleanup()
	client := Client{Socket: socket, Timeout: 3 * time.Second}

	live := formatOwnershipClaim("livewindow", os.Getpid())
	setWindowOwnershipRaw(t, socket, live)
	setShadowOwnership(t, socket, []string{"-p", "-t", "s0"}, formatOwnershipClaim("deadpane", 999999999))

	ownership, owned, err := client.ClaimWindowOwnership(context.Background(), "s0")
	if owned || ownership != nil {
		t.Fatalf("owned = %v, ownership = %v; want no claim over a live window claim", owned, ownership)
	}
	if err == nil || !strings.Contains(err.Error(), "pane-scope") {
		t.Fatalf("err = %v; want an error naming the pane-scope shadow", err)
	}
	got, readErr := readTmuxOptionForOwnershipTest(t, socket)
	if readErr != nil {
		t.Fatalf("read window option: %v", readErr)
	}
	if got != live {
		t.Fatalf("window option = %q; want the live claim %q untouched", got, live)
	}
}

func assertClaimRefusesShadow(t *testing.T, name, wantScope string, scopeArgs []string) {
	t.Helper()
	socket := shadowTestSocket(name)
	cleanup := newBareOwnershipSession(t, socket)
	defer cleanup()
	client := Client{Socket: socket, Timeout: 3 * time.Second}

	setShadowOwnership(t, socket, scopeArgs, formatOwnershipClaim("shadowowner", 999999999))
	before, err := client.readWindowOwnership(context.Background(), "s0")
	if err != nil {
		t.Fatalf("read window option: %v", err)
	}
	if before.Set {
		t.Fatalf("window option is set (%q) before the claim; the shadow must come from another scope", before.Value)
	}

	started := time.Now()
	ownership, owned, claimErr := client.ClaimWindowOwnership(context.Background(), "s0")
	elapsed := time.Since(started)
	if owned || ownership != nil {
		t.Fatalf("owned = %v, ownership = %v; want a refusal", owned, ownership)
	}
	if claimErr == nil || !strings.Contains(claimErr.Error(), wantScope) {
		t.Fatalf("err = %v; want an error naming %s", claimErr, wantScope)
	}
	if strings.Contains(claimErr.Error(), "gave up after") {
		t.Fatalf("err = %v; the shadow must be reported, not retried to the attempt bound", claimErr)
	}
	// Refused on the first pass: far below what maxOwnershipClaimAttempts passes cost.
	if elapsed > 2*time.Second {
		t.Fatalf("claim took %v; want an immediate refusal", elapsed)
	}
	after, err := client.readWindowOwnership(context.Background(), "s0")
	if err != nil {
		t.Fatalf("read window option after: %v", err)
	}
	if after.Set {
		t.Fatalf("window option = %q after the refusal; want it still unset", after.Value)
	}
}

func TestClaimRefusesPaneScopeShadow(t *testing.T) {
	assertClaimRefusesShadow(t, "shadowpane", "pane-scope", []string{"-p", "-t", "s0"})
}

func TestClaimRefusesSessionScopeShadow(t *testing.T) {
	assertClaimRefusesShadow(t, "shadowsession", "session-scope", []string{"-t", "s0"})
}

func TestClaimRefusesGlobalScopeShadow(t *testing.T) {
	assertClaimRefusesShadow(t, "shadowglobal", "global-scope", []string{"-g"})
}

// TestClaimRefusesGlobalWindowScopeShadow covers `set-option -wg`, the window
// options the window inherits from.
func TestClaimRefusesGlobalWindowScopeShadow(t *testing.T) {
	assertClaimRefusesShadow(t, "shadowglobalwindow", "global-window-scope", []string{"-wg"})
}

// TestReleaseWithShadowLeavesWindowOption pins Release's choice: with a pane
// value shadowing the window's claim the compare-and-set cannot match, the
// window option is left as it was, and Release reports the shadow by name
// instead of returning nil while the claim stays behind.
func TestReleaseWithShadowLeavesWindowOption(t *testing.T) {
	socket := shadowTestSocket("releaseshadow")
	cleanup := newBareOwnershipSession(t, socket)
	defer cleanup()
	client := Client{Socket: socket, Timeout: 3 * time.Second}

	ownership, owned, err := client.ClaimWindowOwnership(context.Background(), "s0")
	if err != nil || !owned {
		t.Fatalf("claim: owned=%v err=%v", owned, err)
	}
	setShadowOwnership(t, socket, []string{"-p", "-t", "s0"}, formatOwnershipClaim("deadpane", 999999999))

	releaseErr := ownership.Release(context.Background())
	if releaseErr == nil || !strings.Contains(releaseErr.Error(), "pane-scope") {
		t.Fatalf("Release err = %v; want an error naming the pane-scope shadow", releaseErr)
	}
	got, err := readTmuxOptionForOwnershipTest(t, socket)
	if err != nil {
		t.Fatalf("read window option: %v", err)
	}
	if got != ownership.claim {
		t.Fatalf("window option = %q after Release; want it left as the claim %q", got, ownership.claim)
	}
}

// assertClaimRefusesShadowValue is assertClaimRefusesShadow for a shadow whose
// value is chosen by the caller (equal to the window's, empty, or holding
// control characters), with the window option optionally set first. The
// refusal comes from the scope tmux's lookup reads, not from the values
// differing, so no value may hide the shadow.
func assertClaimRefusesShadowValue(t *testing.T, name, wantScope string, scopeArgs []string, windowValue, shadowValue string) {
	t.Helper()
	socket := shadowTestSocket(name)
	cleanup := newBareOwnershipSession(t, socket)
	defer cleanup()
	client := Client{Socket: socket, Timeout: 3 * time.Second}

	if windowValue != "" {
		setWindowOwnershipRaw(t, socket, windowValue)
	}
	setShadowOwnership(t, socket, scopeArgs, shadowValue)

	started := time.Now()
	ownership, owned, claimErr := client.ClaimWindowOwnership(context.Background(), "s0")
	if owned || ownership != nil {
		t.Fatalf("owned = %v, ownership = %v; want a refusal", owned, ownership)
	}
	if claimErr == nil || !strings.Contains(claimErr.Error(), wantScope) {
		t.Fatalf("err = %v; want an error naming %s", claimErr, wantScope)
	}
	if strings.Contains(claimErr.Error(), "gave up after") || time.Since(started) > 2*time.Second {
		t.Fatalf("err = %v after %v; want an immediate refusal, not a retry", claimErr, time.Since(started))
	}
	after, err := client.readWindowOwnership(context.Background(), "s0")
	if err != nil {
		t.Fatalf("read window option after: %v", err)
	}
	if after.Value != windowValue {
		t.Fatalf("window option = %q after the refusal; want it untouched (%q)", after.Value, windowValue)
	}
}

// TestClaimRefusesPaneShadowEqualToWindowValue: a pane value equal to the
// window's dead claim is still a pane value, so the claim is refused.
func TestClaimRefusesPaneShadowEqualToWindowValue(t *testing.T) {
	dead := formatOwnershipClaim("samevalue", 999999999)
	assertClaimRefusesShadowValue(t, "paneequal", "pane-scope", []string{"-p", "-t", "s0"}, dead, dead)
}

// TestClaimRefusesPaneShadowDifferingOnlyByControlCharacter: tmux's display
// forms print the pane's newline as `_`, which equals the window's literal
// underscore, so a display comparison sees no difference; the scope is the same.
func TestClaimRefusesPaneShadowDifferingOnlyByControlCharacter(t *testing.T) {
	assertClaimRefusesShadowValue(t, "panectl", "pane-scope", []string{"-p", "-t", "s0"}, "dead:1_", "dead:1\n")
}

// TestClaimRefusesSessionShadowWithControlCharacters: the window option is
// unset and the session holds a value with a newline in it.
func TestClaimRefusesSessionShadowWithControlCharacters(t *testing.T) {
	assertClaimRefusesShadowValue(t, "sessionctl", "session-scope", []string{"-t", "s0"}, "", "dead:1\n\n")
}

// TestClaimRefusesEmptySessionShadow: an empty session value reads the same as
// an absent window option through a display, and still shadows.
func TestClaimRefusesEmptySessionShadow(t *testing.T) {
	assertClaimRefusesShadowValue(t, "sessionempty", "session-scope", []string{"-t", "s0"}, "", "")
}

func TestClaimRefusesEmptyGlobalShadow(t *testing.T) {
	assertClaimRefusesShadowValue(t, "globalempty", "global-scope", []string{"-g"}, "", "")
}

func TestClaimRefusesGlobalWindowShadowWithControlCharacters(t *testing.T) {
	assertClaimRefusesShadowValue(t, "globalwindowctl", "global-window-scope", []string{"-wg"}, "", "a\tb\nc")
}

// TestReleaseWithEqualPaneShadowLeavesWindowOption: the pane holds exactly the
// claim, so the compare-and-set would match and unset the window option; the
// release refuses by scope and writes nothing.
func TestReleaseWithEqualPaneShadowLeavesWindowOption(t *testing.T) {
	socket := shadowTestSocket("releaseequal")
	cleanup := newBareOwnershipSession(t, socket)
	defer cleanup()
	client := Client{Socket: socket, Timeout: 3 * time.Second}

	ownership, owned, err := client.ClaimWindowOwnership(context.Background(), "s0")
	if err != nil || !owned {
		t.Fatalf("claim: owned=%v err=%v", owned, err)
	}
	setShadowOwnership(t, socket, []string{"-p", "-t", "s0"}, ownership.claim)

	releaseErr := ownership.Release(context.Background())
	if releaseErr == nil || !strings.Contains(releaseErr.Error(), "pane-scope") {
		t.Fatalf("Release err = %v; want an error naming the pane-scope shadow", releaseErr)
	}
	got, err := readTmuxOptionForOwnershipTest(t, socket)
	if err != nil {
		t.Fatalf("read window option: %v", err)
	}
	if got != ownership.claim {
		t.Fatalf("window option = %q after Release; want it left as the claim %q", got, ownership.claim)
	}
}
