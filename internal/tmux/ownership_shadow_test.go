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

// displayOwnershipForShadowTest reads `#{@deck_isize_owner}` through tmux's
// own format lookup, the scope chain the compare-and-set condition uses.
func displayOwnershipForShadowTest(t *testing.T, socket string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, "tmux", "-L", socket, "display-message", "-p", "-t", "s0", "#{"+OwnershipOption+"}").CombinedOutput()
	if err != nil {
		t.Fatalf("display-message: %v: %s", err, output)
	}
	return strings.TrimSuffix(string(output), "\n")
}

// assertReleaseRefusesDisplacedShadow: the claim was acquired and then
// displaced on the window (replaced by windowAfter, or unset when it is
// empty), and shadowArgs puts the ORIGINAL claim at another scope, where the
// compare-and-set condition would read it and match. Release refuses naming
// wantScope and the window option stays exactly as the displacement left it.
func assertReleaseRefusesDisplacedShadow(t *testing.T, name, wantScope string, shadowArgs []string, windowAfter string) {
	t.Helper()
	socket := shadowTestSocket(name)
	cleanup := newBareOwnershipSession(t, socket)
	defer cleanup()
	client := Client{Socket: socket, Timeout: 3 * time.Second}

	ownership, owned, err := client.ClaimWindowOwnership(context.Background(), "s0")
	if err != nil || !owned {
		t.Fatalf("claim: owned=%v err=%v", owned, err)
	}
	if windowAfter != "" {
		setWindowOwnershipRaw(t, socket, windowAfter)
	} else if output, err := exec.Command("tmux", "-L", socket, "set-option", "-w", "-u", "-t", "s0", OwnershipOption).CombinedOutput(); err != nil {
		t.Fatalf("unset window option: %v: %s", err, output)
	}
	setShadowOwnership(t, socket, shadowArgs, ownership.claim)

	releaseErr := ownership.Release(context.Background())
	if releaseErr == nil || !strings.Contains(releaseErr.Error(), wantScope) {
		t.Fatalf("Release err = %v; want an error naming %s", releaseErr, wantScope)
	}
	after, err := client.readWindowOwnership(context.Background(), "s0")
	if err != nil {
		t.Fatalf("read window option: %v", err)
	}
	if after.Value != windowAfter || after.Present != (windowAfter != "") {
		t.Fatalf("window option = %+v after Release; want it left as %q", after, windowAfter)
	}
}

// TestReleaseDisplacedWindowWithOriginalClaimInPaneRefuses: the window now
// holds another live claim and the pane holds the original one. The pane is
// what the compare-and-set reads, so without the scope check it would match
// and unset the replacement claim.
func TestReleaseDisplacedWindowWithOriginalClaimInPaneRefuses(t *testing.T) {
	assertReleaseRefusesDisplacedShadow(t, "releasedisplacedpane", "pane-scope", []string{"-p", "-t", "s0"},
		formatOwnershipClaim("replacement", os.Getpid()))
}

// TestReleaseDisplacedWindowWithOriginalClaimInPaneControlCharacterRefuses:
// the replacement differs from the original only where a display form would
// print `_`; the scope still decides.
func TestReleaseDisplacedWindowWithOriginalClaimInPaneControlCharacterRefuses(t *testing.T) {
	assertReleaseRefusesDisplacedShadow(t, "releasedisplacedctl", "pane-scope", []string{"-p", "-t", "s0"},
		formatOwnershipClaim("replace\nment", os.Getpid()))
}

// TestReleaseUnsetWindowWithOriginalClaimInSessionRefuses: the window option
// is absent, so the lookup falls back to the session, which holds the claim.
func TestReleaseUnsetWindowWithOriginalClaimInSessionRefuses(t *testing.T) {
	assertReleaseRefusesDisplacedShadow(t, "releaseunsetsession", "session-scope", []string{"-t", "s0"}, "")
}

// TestReleaseUnsetWindowWithOriginalClaimInGlobalWindowRefuses: the same
// fallback through the global window options.
func TestReleaseUnsetWindowWithOriginalClaimInGlobalWindowRefuses(t *testing.T) {
	assertReleaseRefusesDisplacedShadow(t, "releaseunsetglobalwindow", "global-window-scope", []string{"-wg"}, "")
}

// TestReleaseDisplacedWindowIgnoresFallbackScopes: a window option that is
// present ends the lookup, so session and global-window values holding the
// original claim are not read; Release is a no-op that leaves the
// replacement claim in place and reports nothing.
func TestReleaseDisplacedWindowIgnoresFallbackScopes(t *testing.T) {
	socket := shadowTestSocket("releasedisplacedfallback")
	cleanup := newBareOwnershipSession(t, socket)
	defer cleanup()
	client := Client{Socket: socket, Timeout: 3 * time.Second}

	ownership, owned, err := client.ClaimWindowOwnership(context.Background(), "s0")
	if err != nil || !owned {
		t.Fatalf("claim: owned=%v err=%v", owned, err)
	}
	replacement := formatOwnershipClaim("replacement", os.Getpid())
	setWindowOwnershipRaw(t, socket, replacement)
	setShadowOwnership(t, socket, []string{"-t", "s0"}, ownership.claim)
	setShadowOwnership(t, socket, []string{"-wg"}, ownership.claim)

	if err := ownership.Release(context.Background()); err != nil {
		t.Fatalf("Release: %v; want a no-op (the window option ends the lookup)", err)
	}
	got, err := readTmuxOptionForOwnershipTest(t, socket)
	if err != nil {
		t.Fatalf("read window option: %v", err)
	}
	if got != replacement {
		t.Fatalf("window option = %q after Release; want the replacement %q untouched", got, replacement)
	}
}

// assertClaimOverEmptyWindowIgnoresFallback: the window option is present
// but explicitly empty, and fallbackArgs puts a value at a scope tmux's
// lookup only reaches when the window option is absent. tmux itself reads
// the empty window value, so the claim is acquired and the fallback value is
// neither reported as a shadow nor touched.
func assertClaimOverEmptyWindowIgnoresFallback(t *testing.T, name string, fallbackArgs []string, fallbackValue string) {
	t.Helper()
	socket := shadowTestSocket(name)
	cleanup := newBareOwnershipSession(t, socket)
	defer cleanup()
	client := Client{Socket: socket, Timeout: 3 * time.Second}

	setWindowOwnershipRaw(t, socket, "")
	setShadowOwnership(t, socket, fallbackArgs, fallbackValue)
	if looked := displayOwnershipForShadowTest(t, socket); looked != "" {
		t.Fatalf("tmux's lookup reads %q; want the empty window value (the fixture is wrong)", looked)
	}
	before, err := client.readWindowOwnership(context.Background(), "s0")
	if err != nil || !before.Present || before.Set {
		t.Fatalf("window read = %+v, %v; want present and empty", before, err)
	}

	ownership, owned, err := client.ClaimWindowOwnership(context.Background(), "s0")
	if err != nil || !owned || ownership == nil {
		t.Fatalf("claim: owned=%v err=%v; want acquired over an empty window option", owned, err)
	}
	if got, _ := readTmuxOptionForOwnershipTest(t, socket); got != ownership.claim {
		t.Fatalf("window option = %q; want the claim %q", got, ownership.claim)
	}
	if looked := displayOwnershipForShadowTest(t, socket); looked != ownership.claim {
		t.Fatalf("tmux's lookup reads %q after the claim; want %q", looked, ownership.claim)
	}
	if err := ownership.Release(context.Background()); err != nil {
		t.Fatalf("Release: %v", err)
	}
}

// TestClaimEmptyWindowDoesNotFallBackToSession: a present empty window option
// is not an absent one; the session value behind it is not a shadow.
func TestClaimEmptyWindowDoesNotFallBackToSession(t *testing.T) {
	assertClaimOverEmptyWindowIgnoresFallback(t, "emptywindowsession", []string{"-t", "s0"}, formatOwnershipClaim("sessionowner", os.Getpid()))
}

func TestClaimEmptyWindowDoesNotFallBackToGlobalWindow(t *testing.T) {
	assertClaimOverEmptyWindowIgnoresFallback(t, "emptywindowglobalwindow", []string{"-wg"}, "dead:1\n")
}

func TestClaimEmptyWindowDoesNotFallBackToGlobal(t *testing.T) {
	assertClaimOverEmptyWindowIgnoresFallback(t, "emptywindowglobal", []string{"-g"}, "")
}

// TestClaimRefusesPaneShadowOverEmptyWindowOption: an empty window option
// stops the fallback, never the pane, which the lookup reads first.
func TestClaimRefusesPaneShadowOverEmptyWindowOption(t *testing.T) {
	socket := shadowTestSocket("emptywindowpane")
	cleanup := newBareOwnershipSession(t, socket)
	defer cleanup()
	client := Client{Socket: socket, Timeout: 3 * time.Second}

	setWindowOwnershipRaw(t, socket, "")
	setShadowOwnership(t, socket, []string{"-p", "-t", "s0"}, "")

	ownership, owned, err := client.ClaimWindowOwnership(context.Background(), "s0")
	if owned || ownership != nil || err == nil || !strings.Contains(err.Error(), "pane-scope") {
		t.Fatalf("claim: owned=%v err=%v; want a refusal naming pane-scope", owned, err)
	}
	after, readErr := client.readWindowOwnership(context.Background(), "s0")
	if readErr != nil || !after.Present || after.Set {
		t.Fatalf("window read = %+v, %v after the refusal; want it still present and empty", after, readErr)
	}
}
