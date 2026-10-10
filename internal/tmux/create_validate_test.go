package tmux

import (
	"context"
	"reflect"
	"strings"
	"testing"
)

func TestValidateLaunchRefusesInTheOrderCreateAlwaysChecked(t *testing.T) {
	good := Launch{Slug: "a", CWD: "/work", Command: []string{"sh"}}
	with := func(edit func(*Launch)) Launch {
		launch := good
		edit(&launch)
		return launch
	}
	for _, tc := range []struct {
		name   string
		client Client
		launch Launch
		want   string
	}{
		{"socket first", Client{}, with(func(l *Launch) { l.Slug = "Bad Slug" }), "socket name is required"},
		{"slug", Client{Socket: "s"}, with(func(l *Launch) { l.Slug = "Bad Slug" }), "invalid session slug"},
		{"cwd", Client{Socket: "s"}, with(func(l *Launch) { l.CWD = "" }), "working directory is required"},
		{"no command", Client{Socket: "s"}, with(func(l *Launch) { l.Command = nil }), "command is required"},
		{"empty command", Client{Socket: "s"}, with(func(l *Launch) { l.Command = []string{""} }), "command is required"},
		{"bad env", Client{Socket: "s"}, with(func(l *Launch) { l.Env = map[string]string{"A=B": "x"} }), "invalid environment variable"},
	} {
		if _, _, err := tc.client.validateLaunch(tc.launch); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: validateLaunch error = %v, want it to contain %q", tc.name, err, tc.want)
		}
	}
	name, env, err := (Client{Socket: "s"}).validateLaunch(with(func(l *Launch) { l.Env = map[string]string{"B": "2", "A": "1"} }))
	if err != nil || name != "deck_a" || !reflect.DeepEqual(env, []string{"A", "1", "B", "2"}) {
		t.Fatalf("validateLaunch = %q, %q, %v; want deck_a with the sorted environment pairs", name, env, err)
	}
}

func TestNewSessionArgsMirrorsEnvironmentThenRunsTheCommandUnderEnv(t *testing.T) {
	got := newSessionArgs("deck_a", []string{"A", "1", "B", "2"}, Launch{CWD: "/work", Command: []string{"sh", "-c", "true"}})
	want := []string{"new-session", "-d", "-P", "-F", paneFactsFormat, "-s", "deck_a", "-e", "A=1", "-e", "B=2", "-c", "/work", "--", "env", "A=1", "B=2", "sh", "-c", "true"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("newSessionArgs = %q, want %q", got, want)
	}
	if got := newSessionArgs("deck_a", nil, Launch{CWD: "/work", Command: []string{"sh"}}); !reflect.DeepEqual(got, []string{"new-session", "-d", "-P", "-F", paneFactsFormat, "-s", "deck_a", "-c", "/work", "--", "env", "sh"}) {
		t.Fatalf("newSessionArgs without environment = %q", got)
	}
}

func TestCreateThroughAFakeTmuxReportsEachFailingStep(t *testing.T) {
	ctx := context.Background()
	launch := Launch{Slug: "a", CWD: "/work", Command: []string{"sh"}}
	ok := fakeTmuxClient(t, `case "$3" in new-session) echo "%1|42|0|||80|24";; list-panes) echo "list-panes after new-session" >&2; exit 1;; esac`)
	session, err := ok.Create(ctx, launch)
	if err != nil || session.Name != "deck_a" || len(session.Panes) != 1 || session.Panes[0].ID != "%1" {
		t.Fatalf("Create = %+v, %v; want deck_a with its one pane", session, err)
	}
	if _, err := (Client{}).Create(ctx, launch); err == nil || !strings.Contains(err.Error(), "socket name is required") {
		t.Fatalf("Create without a socket = %v", err)
	}
	noBootstrap := fakeTmuxClient(t, `echo "no tmux today" >&2; exit 1`)
	if _, err := noBootstrap.Create(ctx, launch); err == nil || !strings.Contains(err.Error(), "bootstrap tmux server") {
		t.Fatalf("Create with a failing bootstrap = %v", err)
	}
	noNewSession := fakeTmuxClient(t, `case "$3" in new-session) echo "duplicate session" >&2; exit 1;; esac`)
	if _, err := noNewSession.Create(ctx, launch); err == nil || !strings.Contains(err.Error(), `create session "deck_a"`) || !strings.Contains(err.Error(), "duplicate session") {
		t.Fatalf("Create with a failing new-session = %v", err)
	}
	badFacts := fakeTmuxClient(t, `case "$3" in new-session) echo "not pane facts";; esac`)
	if _, err := badFacts.Create(ctx, launch); err == nil || !strings.Contains(err.Error(), `parse pane facts for session "deck_a"`) {
		t.Fatalf("Create with unparseable new-session pane facts = %v", err)
	}
}
