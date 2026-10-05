package tmux

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
)

// optionFake is a tmux whose window option OwnershipOption lives in a file:
// show-options prints it (or fails "invalid option" when unset), set-option
// writes the value argument through writeBody ($VALUE holds it) and logs the
// call to "$0.sent".
func optionFake(t *testing.T, initial, writeBody string) Client {
	t.Helper()
	setup := `:`
	if initial != "" {
		setup = fmt.Sprintf(`[ -e "$0.state" ] || printf '%%s\n' %q > "$0.state"`, initial)
	}
	return fakeTmuxClient(t, setup+`
case "$3" in
show-options)
  if [ -s "$0.state" ]; then cat "$0.state"; else echo "invalid option: @deck_isize_owner" >&2; exit 1; fi;;
set-option)
  echo "$*" >> "$0.sent"
  VALUE="$8"
  `+writeBody+`;;
esac`)
}

func deadPID(t *testing.T) int {
	t.Helper()
	cmd := exec.Command("true")
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
	return cmd.Process.Pid
}

func TestClaimWindowOwnershipStandsDownForALiveOwnerWithoutWriting(t *testing.T) {
	client := optionFake(t, fmt.Sprintf("abcd:%d", os.Getpid()), `printf '%s\n' "$VALUE" > "$0.state"`)
	ownership, owned, err := client.ClaimWindowOwnership(context.Background(), "s0")
	if err != nil || owned || ownership != nil {
		t.Fatalf("claim over a live owner = %v, %v, %v; want a quiet stand-down", ownership, owned, err)
	}
	if _, err := os.Stat(client.Binary + ".sent"); !os.IsNotExist(err) {
		t.Fatalf("a live owner was written over (stat err = %v)", err)
	}
}

func TestClaimWindowOwnershipStealsFromADeadOrUnparseableOwner(t *testing.T) {
	for name, initial := range map[string]string{
		"unset":       "",
		"dead owner":  fmt.Sprintf("abcd:%d", deadPID(t)),
		"unparseable": "not-a-claim",
	} {
		client := optionFake(t, initial, `printf '%s\n' "$VALUE" > "$0.state"`)
		ownership, owned, err := client.ClaimWindowOwnership(context.Background(), "s0")
		if err != nil || !owned || ownership == nil || ownership.Target() != "s0" {
			t.Errorf("%s: claim = %v, %v, %v; want it acquired", name, ownership, owned, err)
			continue
		}
		if sent := readFakeSent(t, client); !strings.Contains(sent, fmt.Sprintf(":%d", os.Getpid())) {
			t.Errorf("%s: set-option calls %q lack this process's claim", name, sent)
		}
	}
}

func TestClaimWindowOwnershipLosingTheConfirmReadLoopsToAFreshRead(t *testing.T) {
	// The first write is overwritten by a live competitor: the next pass must
	// read it and stand down instead of writing again.
	client := optionFake(t, "", fmt.Sprintf(`printf 'rival:%d\n' > "$0.state"`, os.Getpid()))
	ownership, owned, err := client.ClaimWindowOwnership(context.Background(), "s0")
	if err != nil || owned || ownership != nil {
		t.Fatalf("claim against a live rival = %v, %v, %v; want a stand-down", ownership, owned, err)
	}
	if lines := strings.Count(readFakeSent(t, client), "\n"); lines != 1 {
		t.Fatalf("wrote %d times, want exactly one write before standing down", lines)
	}
}

func TestClaimWindowOwnershipGivesUpAfterTheBoundedAttempts(t *testing.T) {
	client := optionFake(t, "", fmt.Sprintf(`printf 'rival:%d\n' > "$0.state"`, deadPID(t)))
	_, owned, err := client.ClaimWindowOwnership(context.Background(), "s0")
	if owned || err == nil || !strings.Contains(err.Error(), "gave up after 3 attempts") {
		t.Fatalf("claim against an endless dead-rival race = %v, %v; want the give-up error", owned, err)
	}
	if lines := strings.Count(readFakeSent(t, client), "\n"); lines != maxOwnershipClaimAttempts {
		t.Fatalf("wrote %d times, want %d", lines, maxOwnershipClaimAttempts)
	}
}

func TestClaimWindowOwnershipReportsTransportFailures(t *testing.T) {
	ctx := context.Background()
	readFails := fakeTmuxClient(t, `echo "protocol error" >&2; exit 1`)
	if _, owned, err := readFails.ClaimWindowOwnership(ctx, "s0"); owned || err == nil || !strings.Contains(err.Error(), "show-options") {
		t.Fatalf("failing first read = %v, %v", owned, err)
	}
	writeFails := optionFake(t, "", `echo "read-only" >&2; exit 1`)
	if _, owned, err := writeFails.ClaimWindowOwnership(ctx, "s0"); owned || err == nil || !strings.Contains(err.Error(), "claim @deck_isize_owner") {
		t.Fatalf("failing write = %v, %v", owned, err)
	}
	confirmFails := fakeTmuxClient(t, `case "$3" in
show-options)
  if [ -e "$0.read" ]; then echo "protocol error" >&2; exit 1; fi
  : > "$0.read"; echo "invalid option: @deck_isize_owner" >&2; exit 1;;
esac`)
	if _, owned, err := confirmFails.ClaimWindowOwnership(ctx, "s0"); owned || err == nil || !strings.Contains(err.Error(), "show-options") {
		t.Fatalf("failing confirm-read = %v, %v", owned, err)
	}
}

func TestHoldsLiveClaimNeedsASetParseableLiveValue(t *testing.T) {
	live := windowOwnershipState{Set: true, Value: fmt.Sprintf("t:%d", os.Getpid())}
	dead := windowOwnershipState{Set: true, Value: fmt.Sprintf("t:%d", deadPID(t))}
	for name, tc := range map[string]struct {
		state windowOwnershipState
		want  bool
	}{
		"live":        {live, true},
		"dead":        {dead, false},
		"unparseable": {windowOwnershipState{Set: true, Value: "junk"}, false},
		"unset":       {windowOwnershipState{Value: live.Value}, false},
	} {
		if got := holdsLiveClaim(tc.state); got != tc.want {
			t.Errorf("%s: holdsLiveClaim = %v, want %v", name, got, tc.want)
		}
	}
}
