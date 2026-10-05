package tmux

import (
	"strings"
	"testing"
)

func TestParsePaneFactsNamesTheFieldThatDoesNotParse(t *testing.T) {
	for _, tc := range []struct {
		line string
		want string
	}{
		{"%1|x|0|||80|24", "parse pane PID"},
		{"%1|1|0|||wide|24", "parse pane width"},
		{"%1|1|0|||80|tall", "parse pane height"},
		{"%1|1|1|x||80|24", "parse pane exit status"},
		{"%1|1|1||x|80|24", "parse pane death signal"},
	} {
		pane, err := parsePaneFacts("deck_a", tc.line)
		if err == nil || !strings.Contains(err.Error(), tc.want) || !strings.Contains(err.Error(), `"deck_a"`) {
			t.Errorf("parsePaneFacts(%q) = %+v, %v; want an error naming %q", tc.line, pane, err, tc.want)
		}
	}
}

func TestParsePaneFactsReadsExitStatusAndLiveness(t *testing.T) {
	live, err := parsePaneFacts("deck_a", "%3|77|0|||100|40")
	if err != nil {
		t.Fatal(err)
	}
	if live.Dead || live.DeadStatus != nil || live.PID != 77 || live.Width != 100 || live.Height != 40 {
		t.Fatalf("live pane = %+v, want alive with no exit status at 100x40", live)
	}
	exited, err := parsePaneFacts("deck_a", "%3|77|1|3||100|40")
	if err != nil {
		t.Fatal(err)
	}
	if !exited.Dead || exited.DeadStatus == nil || *exited.DeadStatus != 3 {
		t.Fatalf("exited pane = %+v, want dead with exit status 3", exited)
	}
}
