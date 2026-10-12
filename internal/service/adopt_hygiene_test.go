package service

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/n-orlov/deck/internal/agent"
	"github.com/n-orlov/deck/internal/notify"
	"github.com/n-orlov/deck/internal/store"
)

// hostileTitle is a title an attacker controls: an escape, a glyph prefix and
// shell syntax.
const hostileTitle = "\x1b]0;pwn\x07✳ $(touch PWNED); `id` \"q\" 'q'\x1b[31m"

// TestAdoptNameSanitisedTitleNeverReachesShellOrArgv: adoption writes the
// cleaned title to the store name and nothing else. No pane keystroke is sent,
// the slug is untouched, and the launch argv for the now-'harness' row carries
// no name at all (R253, R250).
func TestAdoptNameSanitisedTitleNeverReachesShellOrArgv(t *testing.T) {
	svc, db, _, _ := newAgentTestService(t, nil, "adopt-hygiene")
	asked := 0
	svc.Agents.Register(adoptFakeAdapter{claims: true, name: hostileTitle, ok: true, asked: &asked})
	pane := &recordingPane{}
	svc.NamePane = pane
	row := adoptRow(t, db, "row-a", "swift-otter", "derived")

	if renamed, err := svc.AdoptName(context.Background(), row.ID); err != nil || !renamed {
		t.Fatalf("renamed=%v err=%v, want the cleaned title adopted", renamed, err)
	}
	got, err := db.GetSession(context.Background(), row.ID)
	if err != nil {
		t.Fatal(err)
	}
	if want := "$(touch PWNED); `id` \"q\" 'q'"; got.Name != want {
		t.Fatalf("adopted name = %q, want %q", got.Name, want)
	}
	if got.Slug != row.Slug {
		t.Fatalf("slug moved %q -> %q", row.Slug, got.Slug)
	}
	if len(pane.plans) != 0 {
		t.Fatalf("adoption sent keystrokes to the pane: %q", pane.plans)
	}
	if name := launchName(got); name != "" {
		t.Fatalf("launchName for a harness row = %q, want none", name)
	}
	argv, err := agent.Claude{}.Launch(agent.LaunchInput{CWD: got.CWD, ConversationID: got.ConversationID, Profile: "safe", Name: launchName(got)})
	if err != nil {
		t.Fatal(err)
	}
	for _, arg := range argv {
		if strings.Contains(arg, "touch") || strings.Contains(arg, got.Name) {
			t.Fatalf("launch argv carries the adopted title: %q", argv)
		}
	}
}

func TestAdoptNameAllJunkTitleIsNotAdopted(t *testing.T) {
	svc, db, _, _ := newAgentTestService(t, nil, "adopt-junk")
	asked := 0
	svc.Agents.Register(adoptFakeAdapter{claims: true, name: "\x1b[2J\x07 ✳ ", ok: true, asked: &asked})
	row := adoptRow(t, db, "row-a", "swift-otter", "derived")
	if renamed, err := svc.AdoptName(context.Background(), row.ID); err != nil || renamed {
		t.Fatalf("renamed=%v err=%v, want an all-junk title never adopted", renamed, err)
	}
	assertAdoptionEvent(t, db, row.ID, false)
}

// recordingPane records the keystroke plans a push sends.
type recordingPane struct{ plans [][]string }

func (p *recordingPane) Screen(context.Context, string) (string, error) { return "", nil }
func (p *recordingPane) SendPlan(_ context.Context, _ string, plan []string) error {
	p.plans = append(p.plans, plan)
	return nil
}

// TestEventHookRenamedCarriesHarnessSource: a rename adopted from a harness
// reaches the event hook as `renamed` whose reason (the source) is
// harness:<kind>; a rename typed in deck carries "user" (SPEC §10.1).
func TestEventHookRenamedCarriesHarnessSource(t *testing.T) {
	svc, db, _, _ := newAgentTestService(t, nil, "hook-renamed")
	out := t.TempDir()
	script := filepath.Join(out, "hook.sh")
	body := "#!/bin/sh\nprintf '%s|%s|%s\\n' \"$1\" \"$DECK_EVENT_REASON\" \"$DECK_SESSION_NAME\" >> " + filepath.Join(out, "runs") + "\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	svc.EventHook = func() EventHookDispatcher {
		return EventHookDispatcher{
			Policy:  notify.Policy{Command: []string{script}, Default: true, Events: []string{"renamed"}},
			Timeout: 3 * time.Second, Deck: notify.Deck{Host: "box", Version: "test"}, BaseEnv: []string{"PATH=/usr/bin:/bin"},
		}
	}
	asked := 0
	svc.Agents.Register(adoptFakeAdapter{claims: true, name: "Fix the parser", ok: true, asked: &asked})
	row := adoptRow(t, db, "row-a", "swift-otter", "derived")

	if _, err := svc.AdoptName(context.Background(), row.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Rename(context.Background(), row.ID, "typed by hand"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Rename(context.Background(), row.ID, "typed again"); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(filepath.Join(out, "runs"))
	want := "renamed|harness:claude|Fix the parser\nrenamed|user|typed by hand\nrenamed|user|typed again\n"
	if string(raw) != want {
		t.Fatalf("hook runs:\n%s\nwant:\n%s", raw, want)
	}
}

// TestAdoptNameNeverReachesArgvWhateverTheProvenance covers R253's boundary for
// both kinds of adoption: a generated title (name_source=harness) and one a
// human chose in the harness (explicit, frozen as name_source=user, with or
// without collision suffixing). After each, the launch and resume argv carry no
// part of the title; names typed in deck still do.
func TestAdoptNameNeverReachesArgvWhateverTheProvenance(t *testing.T) {
	const wantClean = "$(touch PWNED); `id` \"q\" 'q'"
	for _, tc := range []struct {
		name, rowSource, holder string
		explicit                bool
		wantSource              string
	}{
		{name: "generated over derived", rowSource: "derived", wantSource: "harness"},
		{name: "generated over harness", rowSource: "harness", wantSource: "harness"},
		{name: "explicit over derived", rowSource: "derived", explicit: true, wantSource: "user"},
		{name: "explicit over harness", rowSource: "harness", explicit: true, wantSource: "user"},
		{name: "explicit with a collision suffix", rowSource: "derived", holder: wantClean, explicit: true, wantSource: "user"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, db, _, _ := newAgentTestService(t, nil, "adopt-hygiene-prov")
			asked := 0
			svc.Agents.Register(adoptFakeAdapter{claims: true, name: hostileTitle, explicit: tc.explicit, ok: true, asked: &asked})
			if tc.holder != "" {
				adoptRow(t, db, "other", tc.holder, "user")
			}
			row := adoptRow(t, db, "row-a", "swift-otter", tc.rowSource)
			if _, err := svc.AdoptName(context.Background(), row.ID); err != nil {
				t.Fatal(err)
			}
			got, err := db.GetSession(context.Background(), row.ID)
			if err != nil {
				t.Fatal(err)
			}
			if got.NameSource != tc.wantSource || !got.NameAdopted {
				t.Fatalf("name_source=%q adopted=%v, want %q adopted", got.NameSource, got.NameAdopted, tc.wantSource)
			}
			assertNoNameInArgv(t, got)
		})
	}
}

// assertNoNameInArgv builds the Claude launch and resume argv from the row the
// way the service does and fails when either carries any piece of its name.
func assertNoNameInArgv(t *testing.T, got store.Session) {
	t.Helper()
	if name := launchName(got); name != "" {
		t.Fatalf("launchName = %q, want none for an adopted name", name)
	}
	launch, err := agent.Claude{}.Launch(agent.LaunchInput{CWD: got.CWD, ConversationID: got.ConversationID, Profile: "safe", Name: launchName(got)})
	if err != nil {
		t.Fatal(err)
	}
	resume, err := agent.Claude{}.Resume(agent.ResumeInput{CWD: got.CWD, ConversationID: got.ConversationID, Profile: "safe", Name: launchName(got)})
	if err != nil {
		t.Fatal(err)
	}
	for _, argv := range [][]string{launch, resume} {
		for _, arg := range argv {
			if strings.Contains(arg, "touch") || strings.Contains(arg, got.Name) || arg == "--name" {
				t.Fatalf("argv carries the adopted title: %q", argv)
			}
		}
	}
}

// TestAdoptNameFrozenRowUnfreezesOnlyByATypedRename: a row frozen on an
// explicit harness name launches without a name, and a name the operator then
// types in deck is a user-entered name again, so it is passed as before.
func TestAdoptNameFrozenRowUnfreezesOnlyByATypedRename(t *testing.T) {
	svc, db, _, _ := newAgentTestService(t, nil, "adopt-hygiene-retype")
	asked := 0
	svc.Agents.Register(adoptFakeAdapter{claims: true, name: "Chosen in harness", explicit: true, ok: true, asked: &asked})
	row := adoptRow(t, db, "row-a", "swift-otter", "derived")
	if _, err := svc.AdoptName(context.Background(), row.ID); err != nil {
		t.Fatal(err)
	}
	got, _ := db.GetSession(context.Background(), row.ID)
	if launchName(got) != "" {
		t.Fatalf("frozen adopted row launchName = %q, want none", launchName(got))
	}
	if _, err := svc.Rename(context.Background(), row.ID, "typed in deck"); err != nil {
		t.Fatal(err)
	}
	got, _ = db.GetSession(context.Background(), row.ID)
	if got.NameAdopted || got.NameSource != "user" || launchName(got) != "typed in deck" {
		t.Fatalf("after a typed rename adopted=%v source=%q launchName=%q", got.NameAdopted, got.NameSource, launchName(got))
	}
}
