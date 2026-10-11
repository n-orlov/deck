package tmux

import (
	"context"
	"strings"
	"testing"
)

// TestSendKeyPlanSendsOneCallPerElementInOrder: Enter goes as a key name, the
// control byte and the text as literals, each as its own send-keys call to
// the resolved pane id.
func TestSendKeyPlanSendsOneCallPerElementInOrder(t *testing.T) {
	client := sendKeysFake(t, livePaneList, liveIdentity, `echo "$*" >> "$0.sent"`)
	if err := client.SendKeyPlan(context.Background(), "a", []string{"\x15", "/rename X", "Enter"}); err != nil {
		t.Fatal(err)
	}
	want := "-L fake-socket send-keys -t %1 -l -- \x15\n-L fake-socket send-keys -t %1 -l -- /rename X\n-L fake-socket send-keys -t %1 Enter\n"
	if sent := readFakeSent(t, client); sent != want {
		t.Fatalf("send-keys calls = %q, want %q", sent, want)
	}
}

// TestSendKeyPlanStopsAtTheFirstFailureAndNamesIt: a refused element ends the
// plan, so nothing after it is sent.
func TestSendKeyPlanStopsAtTheFirstFailureAndNamesIt(t *testing.T) {
	ctx := context.Background()
	client := sendKeysFake(t, livePaneList, liveIdentity, `echo "$*" >> "$0.sent"; case "$*" in *rename*) echo boom >&2; exit 1;; esac`)
	err := client.SendKeyPlan(ctx, "a", []string{"/rename X", "Enter"})
	if err == nil || !strings.Contains(err.Error(), `send key plan to session "a"`) {
		t.Fatalf("error = %v", err)
	}
	if sent := readFakeSent(t, client); strings.Contains(sent, "Enter") {
		t.Fatalf("Enter was sent after a failed element: %q", sent)
	}
	for name, c := range map[string]Client{
		"no socket": {},
		"no pane":   sendKeysFake(t, `exit 0`, liveIdentity, sendKeysAllOK),
	} {
		if err := c.SendKeyPlan(ctx, "a", []string{"x"}); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
	if err := client.SendKeyPlan(ctx, "a", nil); err == nil {
		t.Error("an empty plan must be refused")
	}
}
