package tmux

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

// claimStealsStoredValue stores raw verbatim in the window option on a private
// server, claims the window and requires the claim to steal that exact value.
func claimStealsStoredValue(t *testing.T, name, raw string) {
	t.Helper()
	socket := fmt.Sprintf("priv-ownership-%s-%d-%d", name, os.Getpid(), time.Now().UnixNano())
	cleanup := newBareOwnershipSession(t, socket)
	defer cleanup()
	client := Client{Socket: socket, Timeout: 3 * time.Second}

	setWindowOwnershipRaw(t, socket, raw)
	stored, err := client.readWindowOwnership(context.Background(), "s0")
	if err != nil {
		t.Fatalf("read the stored value: %v", err)
	}
	if !stored.Set || stored.Value != raw {
		t.Fatalf("read = %+v, want the stored value %q verbatim", stored, raw)
	}
	ownership, owned, err := client.ClaimWindowOwnership(context.Background(), "s0")
	if err != nil || !owned || ownership == nil {
		t.Fatalf("claim over %q: owned=%v err=%v; want the stale value stolen", raw, owned, err)
	}
	got, err := readTmuxOptionForOwnershipTest(t, socket)
	if err != nil {
		t.Fatalf("read after steal: %v", err)
	}
	if got != ownership.claim {
		t.Fatalf("option = %q after steal, want %q", got, ownership.claim)
	}
}

func TestClaimStealsDeadClaimWithOneTrailingNewline(t *testing.T) {
	claimStealsStoredValue(t, "nl1", formatOwnershipClaim("deadowner", 999999999)+"\n")
}

func TestClaimStealsDeadClaimWithTwoTrailingNewlines(t *testing.T) {
	claimStealsStoredValue(t, "nl2", formatOwnershipClaim("deadowner", 999999999)+"\n\n")
}

func TestClaimStealsUnparseableValueEndingInNewline(t *testing.T) {
	claimStealsStoredValue(t, "nlgarbage", "x,y}z#{w\"q'r;s #,\n")
}

// TestReleaseUnsetsAClaimEndingInNewline: Release of a claim whose value ends
// in a newline still matches it exactly and unsets the window option.
func TestReleaseUnsetsAClaimEndingInNewline(t *testing.T) {
	socket := fmt.Sprintf("priv-ownership-relnl-%d-%d", os.Getpid(), time.Now().UnixNano())
	cleanup := newBareOwnershipSession(t, socket)
	defer cleanup()
	client := Client{Socket: socket, Timeout: 3 * time.Second}

	claim := formatOwnershipClaim("tagwithnl", os.Getpid()) + "\n"
	setWindowOwnershipRaw(t, socket, claim)
	ownership := &WindowOwnership{client: client, target: "s0", claim: claim}
	if err := ownership.Release(context.Background()); err != nil {
		t.Fatalf("release: %v", err)
	}
	got, err := client.readWindowOwnership(context.Background(), "s0")
	if err != nil {
		t.Fatalf("read after release: %v", err)
	}
	if got.Set {
		t.Fatalf("option still set to %q after releasing a claim ending in a newline; want unset", got.Value)
	}
}

// TestDecodeShowOptionsValueRoundTripsTmuxEscaping: every value tmux's
// `show-options -w` can print for a stored ASCII string decodes back to it
// (the non-ASCII case is TestDecodeRoundTripNonASCIIUnderNonUTF8Locale).
func TestDecodeShowOptionsValueRoundTripsTmuxEscaping(t *testing.T) {
	socket := fmt.Sprintf("priv-ownership-decode-%d-%d", os.Getpid(), time.Now().UnixNano())
	cleanup := newBareOwnershipSession(t, socket)
	defer cleanup()
	client := Client{Socket: socket, Timeout: 3 * time.Second}

	values := []string{
		"dead:1", "dead:1\n", "dead:1\n\n", "a b", "q'x", " lead", "\"", "'", "~", "~tilde", "$dollar", `back\slash`,
		`dead:1\n`, "tab\there", "cr\rx", "esc\x1bx", "x,y}z#{w\"q'r;s #,", "x,y}z#{w\"q'r;s #,\n", "100%",
		"two\nlines\n", "a \"b\" c", "\x01\x02", "#",
	}
	for _, raw := range values {
		setWindowOwnershipRaw(t, socket, raw)
		got, err := client.readWindowOwnership(context.Background(), "s0")
		if err != nil {
			t.Errorf("read %q: %v", raw, err)
			continue
		}
		if !got.Set || got.Value != raw {
			t.Errorf("read of %q = %+v, want the value verbatim", raw, got)
		}
		if strings.Contains(raw, "\n") && !strings.Contains(got.Value, "\n") {
			t.Errorf("read of %q lost its newline: %q", raw, got.Value)
		}
	}
}

// nonUTF8Locale makes this process, and so the private tmux server and every
// client the test starts, run under a locale that is not UTF-8: tmux then
// renders non-ASCII bytes of its output as `_` unless the client says its
// terminal is UTF-8.
func nonUTF8Locale(t *testing.T) {
	t.Helper()
	t.Setenv("LC_ALL", "C")
	t.Setenv("LC_CTYPE", "C")
	t.Setenv("LANG", "C")
}

// TestClaimStealsNonASCIIUnparseableOwnerEndingInNewlineUnderNonUTF8Locale:
// R406 -- the value is read exactly, non-ASCII text and the stored trailing
// newline included, and the compare-and-set steals exactly that string.
func TestClaimStealsNonASCIIUnparseableOwnerEndingInNewlineUnderNonUTF8Locale(t *testing.T) {
	nonUTF8Locale(t)
	claimStealsStoredValue(t, "u8nl", "héllo ☃ wörld, 日本語 #{x}\n")
}

func TestClaimStealsNonASCIIDeadClaimUnderNonUTF8Locale(t *testing.T) {
	nonUTF8Locale(t)
	claimStealsStoredValue(t, "u8dead", formatOwnershipClaim("tagé☃", 999999999))
}

func TestClaimStealsFourByteRuneOwnerWithTwoTrailingNewlinesUnderNonUTF8Locale(t *testing.T) {
	nonUTF8Locale(t)
	claimStealsStoredValue(t, "u8emoji", "owner-🚀\n\n")
}

func TestClaimStealsNonASCIIOwnerWithEmbeddedNewlineAndQuotesUnderNonUTF8Locale(t *testing.T) {
	nonUTF8Locale(t)
	claimStealsStoredValue(t, "u8mix", "ñ\"'é\nü;\\ \n")
}

// TestReleaseUnsetsANonASCIIClaimEndingInNewlineUnderNonUTF8Locale: the
// release compare-and-set uses the exact stored string too.
func TestReleaseUnsetsANonASCIIClaimEndingInNewlineUnderNonUTF8Locale(t *testing.T) {
	nonUTF8Locale(t)
	socket := fmt.Sprintf("priv-ownership-relu8-%d-%d", os.Getpid(), time.Now().UnixNano())
	cleanup := newBareOwnershipSession(t, socket)
	defer cleanup()
	client := Client{Socket: socket, Timeout: 3 * time.Second}

	claim := formatOwnershipClaim("tagé☃", os.Getpid()) + "\n"
	setWindowOwnershipRaw(t, socket, claim)
	ownership := &WindowOwnership{client: client, target: "s0", claim: claim}
	if err := ownership.Release(context.Background()); err != nil {
		t.Fatalf("release: %v", err)
	}
	got, err := client.readWindowOwnership(context.Background(), "s0")
	if err != nil {
		t.Fatalf("read after release: %v", err)
	}
	if got.Set {
		t.Fatalf("option still set to %q after releasing a non-ASCII claim; want unset", got.Value)
	}
}

// TestDecodeRoundTripNonASCIIUnderNonUTF8Locale: non-ASCII values read back
// verbatim through the decode, whatever else the value holds.
func TestDecodeRoundTripNonASCIIUnderNonUTF8Locale(t *testing.T) {
	nonUTF8Locale(t)
	socket := fmt.Sprintf("priv-ownership-dec8-%d-%d", os.Getpid(), time.Now().UnixNano())
	cleanup := newBareOwnershipSession(t, socket)
	defer cleanup()
	client := Client{Socket: socket, Timeout: 3 * time.Second}
	for _, raw := range []string{"é", "é\n", "日本 語", "~é", "a\tü\r\n", "ÿ\\n"} {
		setWindowOwnershipRaw(t, socket, raw)
		got, err := client.readWindowOwnership(context.Background(), "s0")
		if err != nil || !got.Set || got.Value != raw {
			t.Errorf("read of %q = %+v, %v; want it verbatim", raw, got, err)
		}
	}
}
