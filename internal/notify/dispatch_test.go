package notify

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/config"
)

// fakeRow is a session row as the store keeps it: the three event-hook
// columns plus the notify_epoch they are scoped to. advance mirrors the
// store's rule that hook_fired is cleared whenever notify_epoch advances.
type fakeRow struct {
	enabled *bool
	events  []string
	fired   []Fired
	epoch   int
	reads   int
}

func (r *fakeRow) EventHookEnabled() *bool   { r.reads++; return r.enabled }
func (r *fakeRow) EventHookEvents() []string { r.reads++; return r.events }
func (r *fakeRow) HookFired() []Fired        { r.reads++; return r.fired }

func (r *fakeRow) advance() { r.epoch++; r.fired = nil }

func (r *fakeRow) fire(p Policy, kind, reason string) bool {
	d := Dispatch(p, r, kind, reason)
	if d.Spawn {
		r.fired = d.Fired
	}
	return d.Spawn
}

func boolp(b bool) *bool { return &b }

func policy(def bool, events ...string) Policy {
	return Policy{Command: []string{"/bin/hook"}, Default: def, Events: events}
}

func TestDispatchInertWithoutScriptReadsNoSessionField(t *testing.T) {
	for _, command := range [][]string{nil, {}, {""}} {
		row := &fakeRow{enabled: boolp(true), events: config.EventHookKinds}
		d := Dispatch(Policy{Command: command, Default: true, Events: config.EventHookKinds}, row, "waiting", "x")
		if d.Spawn || d.Skip != SkipInert {
			t.Fatalf("command %q: decision %+v, want inert", command, d)
		}
		if row.reads != 0 {
			t.Fatalf("command %q: inert dispatch read %d per-session fields", command, row.reads)
		}
	}
}

func TestDispatchNotOfferedKindsNeverSpawnAndReadNoSessionField(t *testing.T) {
	for _, kind := range []string{"prompt", "env", "note", "", "stop", "session_end"} {
		row := &fakeRow{enabled: boolp(true), events: []string{kind}}
		p := policy(true, kind)
		d := Dispatch(p, row, kind, "")
		if d.Spawn || d.Skip != SkipNotOffer {
			t.Errorf("kind %q: decision %+v, want not offered", kind, d)
		}
		if row.reads != 0 {
			t.Errorf("kind %q: read %d per-session fields before the offered check", kind, row.reads)
		}
	}
}

func TestDispatchEveryOfferedKindCanSpawn(t *testing.T) {
	for _, kind := range config.EventHookKinds {
		row := &fakeRow{}
		if !row.fire(policy(true, kind), kind, "") {
			t.Errorf("offered kind %q did not spawn", kind)
		}
	}
}

func TestDispatchEnabledFlagTable(t *testing.T) {
	cases := []struct {
		name    string
		session *bool
		def     bool
		spawn   bool
	}{
		{"inherit/default off", nil, false, false},
		{"inherit/default on", nil, true, true},
		{"on/default off", boolp(true), false, true},
		{"on/default on", boolp(true), true, true},
		{"off/default off", boolp(false), false, false},
		{"off/default on", boolp(false), true, false},
	}
	if len(cases) != 6 {
		t.Fatalf("the enabled table must hold all 6 combinations, has %d", len(cases))
	}
	for _, c := range cases {
		row := &fakeRow{enabled: c.session}
		d := Dispatch(policy(c.def, "waiting"), row, "waiting", "r")
		if d.Spawn != c.spawn {
			t.Errorf("%s: spawn = %v, want %v (%+v)", c.name, d.Spawn, c.spawn, d)
		}
		if !c.spawn && d.Skip != SkipDisabled {
			t.Errorf("%s: skip = %q, want %q", c.name, d.Skip, SkipDisabled)
		}
	}
}

func TestDispatchSessionListReplacesRatherThanMerges(t *testing.T) {
	global := policy(true, "waiting", "error", "ended")
	cases := []struct {
		name  string
		own   []string
		kind  string
		spawn bool
	}{
		{"inherit uses the global list", nil, "waiting", true},
		{"inherit, kind outside the global list", nil, "idle", false},
		{"own list, kind only in own", []string{"idle"}, "idle", true},
		{"own list drops a global kind", []string{"idle"}, "waiting", false},
		{"own list drops every global kind", []string{"idle"}, "ended", false},
		{"empty own list offers nothing", []string{}, "waiting", false},
	}
	for _, c := range cases {
		row := &fakeRow{events: c.own}
		d := Dispatch(global, row, c.kind, "")
		if d.Spawn != c.spawn {
			t.Errorf("%s: spawn = %v, want %v", c.name, d.Spawn, c.spawn)
		}
		if !c.spawn && d.Skip != SkipNotListed {
			t.Errorf("%s: skip = %q", c.name, d.Skip)
		}
	}
}

func TestDispatchDisabledSessionIsNotConsultedForItsList(t *testing.T) {
	row := &fakeRow{enabled: boolp(false)}
	if d := Dispatch(policy(true, "waiting"), row, "waiting", ""); d.Skip != SkipDisabled {
		t.Fatalf("decision %+v", d)
	}
	if row.reads != 1 {
		t.Fatalf("a disabled session read %d fields, want only its flag", row.reads)
	}
}

func TestDispatchDedupesByEpoch(t *testing.T) {
	p := policy(true, "waiting", "error")
	row := &fakeRow{}
	if !row.fire(p, "waiting", "permission_prompt") {
		t.Fatal("first (kind, reason) did not spawn")
	}
	if row.fire(p, "waiting", "permission_prompt") {
		t.Fatal("the same (kind, reason) spawned twice in one epoch")
	}
	if d := Dispatch(p, row, "waiting", "permission_prompt"); d.Skip != SkipDeduped {
		t.Fatalf("skip = %q, want %q", d.Skip, SkipDeduped)
	}
	if !row.fire(p, "waiting", "idle_prompt") {
		t.Fatal("a different reason in the same epoch did not spawn")
	}
	if !row.fire(p, "error", "permission_prompt") {
		t.Fatal("a different kind with the same reason did not spawn")
	}
	row.advance()
	if !row.fire(p, "waiting", "permission_prompt") {
		t.Fatal("the same (kind, reason) did not spawn again after the epoch advanced")
	}
	if row.fire(p, "waiting", "permission_prompt") {
		t.Fatal("the new epoch spawned the pair twice")
	}
}

func TestDispatchDoesNotAliasThePreviousFiredSet(t *testing.T) {
	prior := make([]Fired, 1, 8)
	prior[0] = Fired{Kind: "idle", Reason: "a"}
	row := &fakeRow{fired: prior}
	d := Dispatch(policy(true, "waiting"), row, "waiting", "b")
	if !d.Spawn || len(d.Fired) != 2 || d.Fired[1] != (Fired{Kind: "waiting", Reason: "b"}) {
		t.Fatalf("decision %+v", d)
	}
	if len(row.fired) != 1 || &d.Fired[0] == &prior[0] {
		t.Fatal("Dispatch wrote through the caller's fired slice")
	}
}

func TestFiredRoundTripAndDamagedColumn(t *testing.T) {
	if got := EncodeFired(nil); got != "" {
		t.Fatalf("empty set encodes to %q", got)
	}
	in := []Fired{{Kind: "waiting", Reason: "a b"}, {Kind: "error", Reason: ""}}
	if got := DecodeFired(EncodeFired(in)); !slices.Equal(got, in) {
		t.Fatalf("round trip %v", got)
	}
	for _, text := range []string{"", "  ", "not json", `{"kind":1}`} {
		if got := DecodeFired(text); got != nil {
			t.Errorf("DecodeFired(%q) = %v, want nil", text, got)
		}
	}
}

// offerCases is the mapping table the SPEC §10.4 table documents.
var offerCases = []struct {
	stored, reason, want string
}{
	{"session_start", "resume", "resumed"},
	{"session_start", "compact", ""},
	{"session_start", "startup", "started"},
	{"session_start", "clear", "started"},
	{"session_start", "", "started"},
	{"notification", "permission_prompt", "waiting"},
	{"permission_request", "Bash", "waiting"},
	{"stop", "end_turn", "idle"},
	{"stop_failure", "rate_limit", "error"},
	{"session_end", "logout", "ended"},
	{"probe.waiting", "", "waiting"},
	{"probe.idle", "", "idle"},
	{"probe.error", "", "error"},
	{"tmux.pane_dead", "tmux pane exited with status 1", "error"},
	{"killed", "", "killed"},
	// Nothing below offers the hook.
	{"user_prompt_submitted", "prompt", ""},
	{"error_occurred", "", ""},
	{"probe.running", "", ""},
	{"probe.stopped", "", ""},
	{"launch.ready", "", ""},
	{"launch.failed", "boom", ""},
	{"restart", "", ""},
	{"tmux.session_gone", "", ""},
	{"tmux.shell_live", "", ""},
	{"stop.superseded", "declined", ""},
	{"session_end.identity_mismatch", "", ""},
	{"running", "", ""},
	{"", "", ""},
}

func TestOfferedKindTable(t *testing.T) {
	for _, c := range offerCases {
		got, ok := OfferedKind(c.stored, c.reason)
		if got != c.want || ok != (c.want != "") {
			t.Errorf("OfferedKind(%q, %q) = %q, %v; want %q", c.stored, c.reason, got, ok, c.want)
		}
		if ok && !slices.Contains(config.EventHookKinds, got) {
			t.Errorf("OfferedKind(%q) returned %q outside the offered set", c.stored, got)
		}
	}
}

func TestOfferedKindMappedKindsFeedDispatch(t *testing.T) {
	row := &fakeRow{}
	kind, ok := OfferedKind("notification", "permission_prompt")
	if !ok || !row.fire(policy(true, "waiting"), kind, "permission_prompt") {
		t.Fatalf("a notification event did not reach a spawn (kind %q)", kind)
	}
}

// TestSpecTablesTheOfferedKindMapping pins SPEC §10.4's table to the table
// OfferedKind implements, row for row.
func TestSpecTablesTheOfferedKindMapping(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "SPEC.md"))
	if err != nil {
		t.Fatal(err)
	}
	start := strings.Index(string(data), "**Which kind a recorded change offers.**")
	if start < 0 {
		t.Fatal("SPEC.md has no mapping section in §10.4")
	}
	row := regexp.MustCompile("^\\| `([^`]+)`(?:, reason `([^`]+)`)?(?:, any other reason)? \\| [^|]+ \\| (`([a-z]+)`|none[^|]*) \\|$")
	seen := map[string]string{}
	for _, line := range strings.Split(string(data)[start:], "\n") {
		if strings.HasPrefix(line, "Every other stored kind") {
			break
		}
		m := row.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		reason := m[2]
		if strings.Contains(line, "any other reason") {
			reason = "startup"
		}
		key := m[1] + "/" + reason
		seen[key] = m[4]
		got, _ := OfferedKind(m[1], reason)
		if got != m[4] {
			t.Errorf("SPEC row %q says %q, OfferedKind says %q", key, m[4], got)
		}
	}
	for _, c := range offerCases {
		if c.want != "" && !specHasKind(seen, c.stored) {
			t.Errorf("SPEC §10.4 does not table %q", c.stored)
		}
	}
	if len(seen) != 13 {
		t.Errorf("SPEC table has %d rows, want 13: %v", len(seen), seen)
	}
}

func specHasKind(seen map[string]string, stored string) bool {
	for key := range seen {
		if strings.HasPrefix(key, stored+"/") {
			return true
		}
	}
	return false
}
