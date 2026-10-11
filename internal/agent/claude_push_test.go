package agent

import (
	"reflect"
	"strings"
	"testing"
)

// recordingSender stands in for the tmux key-plan sender: it records each
// element as one delivery, exactly as SendKeyPlan makes one send-keys call per
// element.
type recordingSender struct{ deliveries []string }

func (r *recordingSender) send(plan []string) {
	r.deliveries = append(r.deliveries, plan...)
}

func TestClaudePushPlan(t *testing.T) {
	if got := NewClaude().Capabilities().Name.PushRename; got != NamePushSendKeys {
		t.Fatalf("PushRename = %q, want sendkeys", got)
	}
	plan, ok := PushName(NewClaude(), NamePushInput{Name: "fix the login bug"})
	if !ok {
		t.Fatal("PushName declined")
	}
	var rec recordingSender
	rec.send(plan)
	want := []string{"\x15", "/rename fix the login bug", "Enter"}
	if !reflect.DeepEqual(rec.deliveries, want) {
		t.Fatalf("deliveries = %q, want %q", rec.deliveries, want)
	}
	// Enter must be its own delivery, never part of the text.
	if strings.Contains(rec.deliveries[1], "Enter") || strings.ContainsAny(rec.deliveries[1], "\r\n") {
		t.Fatalf("the text delivery carries the Enter: %q", rec.deliveries[1])
	}
	if rec.deliveries[2] != "Enter" {
		t.Fatalf("last delivery = %q, want a lone Enter", rec.deliveries[2])
	}
}

func TestClaudePushNameIsSanitised(t *testing.T) {
	long := strings.Repeat("é", 250)
	plan, ok := NewClaude().PushName(NamePushInput{Name: "a\x1b[31mb\r\nEnter\x00c" + long})
	if !ok || len(plan) != 3 {
		t.Fatalf("plan = %q ok=%v", plan, ok)
	}
	text := plan[1]
	if strings.ContainsAny(text, "\x00\x1b\r\n") {
		t.Errorf("control characters survive in %q", text)
	}
	name := strings.TrimPrefix(text, "/rename ")
	if n := len([]rune(name)); n != 200 {
		t.Errorf("name is %d runes, want the 200-rune cap", n)
	}
	if _, ok := NewClaude().PushName(NamePushInput{Name: "\x00\x1b[0m \n"}); ok {
		t.Error("a name with nothing left was not declined")
	}
}
