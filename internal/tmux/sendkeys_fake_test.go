package tmux

import (
	"context"
	"strings"
	"testing"
)

// sendKeysFake is a tmux that lists one live pane for deck_a, answers the
// dispatch identity read, and runs sendKeysBody for send-keys.
func sendKeysFake(t *testing.T, listBody, identityBody, sendKeysBody string) Client {
	t.Helper()
	return fakeTmuxClient(t, `case "$3" in
list-panes) `+listBody+`;;
display-message) `+identityBody+`;;
send-keys) `+sendKeysBody+`;;
esac`)
}

const (
	livePaneList  = `echo "deck_a|%1|42|0|||80|24"`
	liveIdentity  = `echo "/tmp/s|7|%1|42|deck_a|0"`
	sendKeysAllOK = `exit 0`
)

func TestSendKeysSendsTheLiteralThenEnterToTheResolvedPane(t *testing.T) {
	client := sendKeysFake(t, livePaneList, liveIdentity, `echo "$*" >> "$0.sent"`)
	if err := client.SendKeys(context.Background(), "a", "export K=v"); err != nil {
		t.Fatal(err)
	}
	sent := readFakeSent(t, client)
	if want := "-L fake-socket send-keys -t %1 -l -- export K=v\n-L fake-socket send-keys -t %1 Enter\n"; sent != want {
		t.Fatalf("send-keys calls = %q, want %q", sent, want)
	}
}

func TestSendKeysNamesTheStepThatFailed(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name   string
		client Client
		want   string
	}{
		{"no socket", Client{}, "socket name is required"},
		{"list fails", sendKeysFake(t, `echo "protocol error" >&2; exit 1`, liveIdentity, sendKeysAllOK), `resolve pane for session "a"`},
		{"no live pane", sendKeysFake(t, `exit 0`, liveIdentity, sendKeysAllOK), "no live pane"},
		{"dead pane", sendKeysFake(t, `echo "deck_a|%1|42|1|0||80|24"`, liveIdentity, sendKeysAllOK), "no live pane"},
		{"identity unreadable", sendKeysFake(t, livePaneList, `echo "can't find pane" >&2; exit 1`, sendKeysAllOK), `send keys to session "a"`},
		{"literal refused", sendKeysFake(t, livePaneList, liveIdentity, `case "$*" in *Enter*) exit 0;; esac; echo boom >&2; exit 1`), `send keys to session "a"`},
		{"enter refused", sendKeysFake(t, livePaneList, liveIdentity, `case "$*" in *Enter*) echo boom >&2; exit 1;; esac`), `send Enter to session "a"`},
	} {
		err := tc.client.SendKeys(ctx, "a", "x")
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: SendKeys error = %v, want it to contain %q", tc.name, err, tc.want)
		}
	}
}
