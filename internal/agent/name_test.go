package agent

import (
	"reflect"
	"testing"
)

// nameCapsRows is the single place each adapter's declared NameCaps lives in a
// test: an adapter task that declares a name capability updates its own row
// here in the same commit. Until then every row is the zero value.
var nameCapsRows = map[string]NameCaps{
	"claude":  {LaunchName: NameLaunchFlag, AutoTitle: true, PushRename: NamePushSendKeys, ObserveName: []string{ObserveHook, ObserveTranscript, ObservePaneTitle}},
	"codex":   {LaunchName: NameLaunchSendKeysAfterReady, PushRename: NamePushSendKeys},
	"copilot": {LaunchName: NameLaunchFlagNewSessionOnly, AutoTitle: true, PushRename: NamePushSendKeys, ObserveName: []string{ObserveHook, ObserveYAML}},
	"pi":      {LaunchName: NameLaunchFlag, PushRename: NamePushSendKeys, ObserveName: []string{ObserveExtension}},
	"shell":   {},
}

func nameCapsAdapters() map[string]Adapter {
	return map[string]Adapter{
		"claude": NewClaude(), "codex": NewCodex(), "copilot": NewCopilot(), "pi": NewPi(), "shell": NewShell(),
	}
}

func TestNameCapsDeclaredPerAdapter(t *testing.T) {
	adapters := nameCapsAdapters()
	if len(adapters) != len(nameCapsRows) {
		t.Fatalf("%d adapters, %d NameCaps rows", len(adapters), len(nameCapsRows))
	}
	for kind, want := range nameCapsRows {
		got := adapters[kind].Capabilities().Name
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s NameCaps = %#v, want %#v", kind, got, want)
		}
	}
}

// TestNameCapsHasTheFourDeclaredFields pins the contract's shape: the four
// #81 fields exist with the declared types.
func TestNameCapsHasTheFourDeclaredFields(t *testing.T) {
	caps := NameCaps{LaunchName: NameLaunchFlag, AutoTitle: true, PushRename: NamePushSendKeys, ObserveName: []string{ObserveHook}}
	if !caps.AutoTitle || caps.LaunchName != "flag" || caps.PushRename != "sendkeys" || !caps.ClaimsRead() || !caps.ClaimsPush() {
		t.Fatalf("NameCaps fields do not round-trip: %#v", caps)
	}
	if (NameCaps{}).ClaimsRead() || (NameCaps{}).ClaimsPush() {
		t.Fatal("the zero NameCaps claims a name capability")
	}
}

// nameSpy implements ReadName and PushName and counts every call.
type nameSpy struct {
	Shell
	caps          NameCaps
	reads, pushes int
}

func (s *nameSpy) Capabilities() Caps { return Caps{Name: s.caps} }

func (s *nameSpy) ReadName(NameProbeInput) (string, bool, bool) {
	s.reads++
	return "spy", true, true
}

func (s *nameSpy) PushName(NamePushInput) ([]string, bool) {
	s.pushes++
	return []string{"/rename x"}, true
}

func TestNameDispatchNeverCallsAnUnclaimedMethod(t *testing.T) {
	for name, caps := range map[string]NameCaps{
		"zero":          {},
		"launch only":   {LaunchName: NameLaunchFlag, AutoTitle: true},
		"push none":     {PushRename: NamePushNone},
		"push ext":      {PushRename: NamePushExtension},
		"empty observe": {ObserveName: []string{}},
	} {
		spy := &nameSpy{caps: caps}
		if n, explicit, ok := ReadName(spy, NameProbeInput{ConversationID: "c"}); n != "" || explicit || ok {
			t.Errorf("%s: ReadName = %q %v %v, want none", name, n, explicit, ok)
		}
		if keys, ok := PushName(spy, NamePushInput{Name: "x"}); keys != nil || ok {
			t.Errorf("%s: PushName = %v %v, want none", name, keys, ok)
		}
		if spy.reads != 0 || spy.pushes != 0 {
			t.Errorf("%s: spy recorded %d reads and %d pushes, want 0 and 0", name, spy.reads, spy.pushes)
		}
	}
}

func TestNameDispatchCallsAClaimedMethod(t *testing.T) {
	spy := &nameSpy{caps: NameCaps{PushRename: NamePushSendKeys, ObserveName: []string{ObservePaneTitle}}}
	if n, explicit, ok := ReadName(spy, NameProbeInput{ConversationID: "c"}); n != "spy" || !explicit || !ok {
		t.Errorf("ReadName = %q %v %v", n, explicit, ok)
	}
	if keys, ok := PushName(spy, NamePushInput{Name: "x"}); !ok || len(keys) != 1 {
		t.Errorf("PushName = %v %v", keys, ok)
	}
	if spy.reads != 1 || spy.pushes != 1 {
		t.Errorf("spy recorded %d reads and %d pushes, want 1 and 1", spy.reads, spy.pushes)
	}
}

func TestNameDispatchIgnoresAnAdapterWithoutTheMethods(t *testing.T) {
	// the shell adapter implements neither optional method and claims neither
	// capability; the dispatcher must decline rather than assert.
	if _, _, ok := ReadName(NewShell(), NameProbeInput{}); ok {
		t.Error("ReadName answered for an adapter with no ReadName")
	}
	if _, ok := PushName(NewShell(), NamePushInput{Name: "x"}); ok {
		t.Error("PushName answered for an adapter with no PushName")
	}
}

const nameTestUUID = "7f1b2c3d-4e5f-4a6b-8c7d-9e0f1a2b3c4d"

func hasNameFlag(argv []string) bool {
	for _, a := range argv {
		if a == "--name" || a == "-n" || len(a) > 7 && a[:7] == "--name=" {
			return true
		}
	}
	return false
}

// TestDerivedNoName proves a row with an empty Name -- what a derived or
// harness row yields -- produces no name flag in any adapter's launch or resume
// argv, so the harness keeps its own auto-title (R250).
func TestDerivedNoNameArgvPerAdapter(t *testing.T) {
	for _, kind := range []string{"claude", "codex", "copilot", "pi"} {
		t.Run(kind, func(t *testing.T) { derivedNoNameCase(t, kind) })
	}
}

func derivedNoNameCase(t *testing.T, kind string) {
	{
		adapter := nameCapsAdapters()[kind]
		launch, err := adapter.Launch(LaunchInput{CWD: "/w", ConversationID: nameTestUUID, Profile: "safe", Name: ""})
		if err != nil {
			t.Fatalf("launch: %v", err)
		}
		resume, err := adapter.Resume(ResumeInput{CWD: "/w", ConversationID: nameTestUUID, Profile: "safe", Name: ""})
		if err != nil {
			t.Fatalf("resume: %v", err)
		}
		for what, argv := range map[string][]string{"launch": launch, "resume": resume} {
			if hasNameFlag(argv) {
				t.Errorf("%s argv carries a name flag with an empty Name: %v", what, argv)
			}
		}
	}
}
