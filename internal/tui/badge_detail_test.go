package tui

import (
	"strings"
	"testing"

	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
)

// TestListShowsProfileBadgeForAgentsAndNoneForShell proves the list badges a
// permission profile for agent rows (SPEC §5: "needs a badge visible in the
// list") and shows none for a shell row, which has no notion of a permission
// profile at all.
func TestListShowsProfileBadgeForAgentsAndNoneForShell(t *testing.T) {
	model := New(nil, config.Settings{}, "")
	model.sessions = []store.Session{
		{Name: "yolo-claude", Agent: "claude", Status: "starting", PermissionProfile: "yolo"},
		{Name: "plain-shell", Agent: "shell", Status: "running", PermissionProfile: "safe"},
	}
	view := model.View()
	if !strings.Contains(view, "[yolo]") {
		t.Fatalf("list view missing yolo badge:\n%s", view)
	}
	for _, line := range strings.Split(view, "\n") {
		if strings.Contains(line, "plain-shell") && strings.Contains(line, "[") {
			t.Fatalf("shell row rendered a profile badge:\n%s", line)
		}
	}
}

// TestDetailViewShowsProfileAndDegradation proves the detail pane states the
// resolved profile and, when the adapter degraded it, an explicit
// degradation sentence (SPEC §5: pi + plan -> safe).
func TestDetailViewShowsProfileAndDegradation(t *testing.T) {
	model := New(nil, config.Settings{}, "")
	model.sessions = []store.Session{
		{
			Name: "pi-plan", Agent: "pi", Status: "starting",
			PermissionProfile:       "safe",
			PermissionProfileReason: `pi does not support permission profile "plan"; falling back to safe`,
		},
	}
	model.selected = 0
	model.detail = true
	view := model.View()
	if !strings.Contains(view, "Permission profile: safe") {
		t.Fatalf("detail view missing resolved profile:\n%s", view)
	}
	if !strings.Contains(view, "degraded") || !strings.Contains(view, "falling back to safe") {
		t.Fatalf("detail view missing explicit degradation sentence:\n%s", view)
	}
}

// TestDetailViewOmitsProfileForShell proves a shell row's detail states the
// profile notion does not apply, rather than showing a meaningless default.
func TestDetailViewOmitsProfileForShell(t *testing.T) {
	model := New(nil, config.Settings{}, "")
	model.sessions = []store.Session{{Name: "plain-shell", Agent: "shell", Status: "running", PermissionProfile: "safe"}}
	model.selected = 0
	model.detail = true
	view := model.View()
	if !strings.Contains(view, "n/a") {
		t.Fatalf("shell detail view did not state the profile notion does not apply:\n%s", view)
	}
	if strings.Contains(view, "Permission profile: safe") {
		t.Fatalf("shell detail view showed a meaningless profile value:\n%s", view)
	}
}

// TestListBadgesStatusSourceQuality proves the status badge reports evidence
// quality rather than agent kind: hooks are live, probes (including Pi) are
// sampled, and tmux/user verdicts make no agent-quality claim.
func TestListBadgesStatusSourceQuality(t *testing.T) {
	model := New(nil, config.Settings{}, "")
	model.sessions = []store.Session{
		{Name: "hook-claude", Agent: "claude", Status: "waiting", StatusSource: "hook"},
		{Name: "probe-claude", Agent: "claude", Status: "running", StatusSource: "probe"},
		{Name: "probe-pi", Agent: "pi", Status: "idle", StatusSource: "probe"},
		{Name: "tmux-shell", Agent: "shell", Status: "running", StatusSource: "tmux"},
		{Name: "user-claude", Agent: "claude", Status: "stopped", StatusSource: "user"},
	}
	view := model.View()
	for _, want := range []struct{ name, badge string }{
		{"hook-claude", "live"},
		{"probe-claude", "sampled"},
		{"probe-pi", "sampled"},
	} {
		line := lineContaining(view, want.name)
		if !strings.Contains(line, want.badge) {
			t.Fatalf("row %q missing %q source badge:\n%s", want.name, want.badge, view)
		}
	}
	for _, name := range []string{"tmux-shell", "user-claude"} {
		line := lineContaining(view, name)
		if strings.Contains(line, "live") || strings.Contains(line, "sampled") {
			t.Fatalf("row %q falsely claims agent source quality: %q", name, line)
		}
	}
}

// TestDetailShowsSourceFrozenClockAgeAndStatusArtifacts proves stale verdicts
// are not presented as current and the status payloads needed for diagnosis
// remain visible. Advancing only the frozen clock changes the displayed age.
func TestDetailShowsSourceFrozenClockAgeAndStatusArtifacts(t *testing.T) {
	clock, err := config.NewClock("2025-01-02T03:04:05Z", "2m")
	if err != nil {
		t.Fatal(err)
	}
	exitStatus := 137
	at := clock.Now().UnixMilli()
	model := New(nil, config.Settings{Clock: clock}, "")
	model.sessions = []store.Session{{
		Name: "diagnostic", Agent: "claude", Status: "error", StatusSource: "hook", StatusAt: at,
		LastMessage: "the last assistant answer", PaneExitStatus: &exitStatus, CrashTail: "final output\nprocess killed",
	}}
	model.detail = true

	fresh := model.View()
	for _, want := range []string{
		"Verdict source:     hook (live)",
		"Verdict age:        just now",
	} {
		if !strings.Contains(fresh, want) {
			t.Fatalf("fresh detail missing %q:\n%s", want, fresh)
		}
	}
	if !consecutiveLinesContain(fresh, []string{"Last message:", "the last assistant answer"}) {
		t.Fatalf("fresh detail missing consecutive lines %q:\n%s", []string{"Last message:", "the last assistant answer"}, fresh)
	}
	if !consecutiveLinesContain(fresh, []string{"Crash tail (exit status 137):", "final output", "process killed"}) {
		t.Fatalf("fresh detail missing consecutive lines %q:\n%s", []string{"Crash tail (exit status 137):", "final output", "process killed"}, fresh)
	}
	clock.Advance()
	aged := model.View()
	if !strings.Contains(aged, "Verdict age:        2m") || strings.Contains(aged, "Verdict age:        just now") {
		t.Fatalf("detail did not expose frozen-clock-controlled verdict age:\n%s", aged)
	}

	// Probes keep their sampled label in detail too; tmux/user sources have no
	// parenthesized quality claim.
	model.sessions[0].StatusSource = "probe"
	if got := model.View(); !strings.Contains(got, "Verdict source:     probe (sampled)") {
		t.Fatalf("probe detail missing sampled quality:\n%s", got)
	}
	model.sessions[0].StatusSource = "tmux"
	if got := model.View(); !strings.Contains(lineContaining(got, "Verdict source:"), "tmux") || strings.Contains(got, "tmux (") {
		t.Fatalf("tmux detail falsely claims agent quality:\n%s", got)
	}

	model.sessions[0].CrashTail = strings.Join([]string{"first", "2", "3", "4", "5", "6", "7", "8", "9", "last"}, "\n")
	bounded := model.View()
	if !consecutiveLinesContain(bounded, []string{"first", "2", "3", "4", "… 2 lines omitted …", "7", "8", "9", "last"}) {
		t.Fatalf("long crash tail did not retain both ends in bounded detail:\n%s", bounded)
	}
}

func lineContaining(view, value string) string {
	for _, line := range strings.Split(view, "\n") {
		if strings.Contains(line, value) {
			return line
		}
	}
	return ""
}

// consecutiveLinesContain reports whether some run of len(wants) adjacent
// rendered lines each contain the corresponding wants[i] substring, in
// order. Since task 014 boxed every dialog (SPEC requirement 16), a
// multi-line value like a crash tail no longer survives as one literal
// "line one\nline two" substring — each of its lines now has its own
// border/padding wrapped around it — so assertions on multi-line content
// walk adjacent rendered lines instead of the raw joined text.
func consecutiveLinesContain(view string, wants []string) bool {
	lines := strings.Split(view, "\n")
	for start := 0; start+len(wants) <= len(lines); start++ {
		ok := true
		for i, want := range wants {
			if !strings.Contains(lines[start+i], want) {
				ok = false
				break
			}
		}
		if ok {
			return true
		}
	}
	return false
}

// TestDetailDistinguishesTotalProbeMissFromNoSignalYet proves a row whose
// stale sample matched no probe rule at all is distinguishable in the `i`
// detail dialog from one with no signal yet (SPEC requirement 38, task 009):
// the dialog states the pane was sampled and matched no rule, with the
// sample's age, and only while that miss is newer than the row's current
// verdict — no status value, status_source, or §7 transition changes.
func TestDetailDistinguishesTotalProbeMissFromNoSignalYet(t *testing.T) {
	clock, err := config.NewClock("2025-01-02T03:04:05Z", "2m")
	if err != nil {
		t.Fatal(err)
	}
	statusAt := clock.Now().UnixMilli()
	model := New(nil, config.Settings{Clock: clock}, "")
	model.sessions = []store.Session{{
		Name: "unclassifiable", Agent: "pi", Status: "running", StatusSource: "hook", StatusAt: statusAt,
	}}
	model.detail = true

	// No signal yet: the row has never been probe-sampled at all
	// (LastProbeAt is zero), so nothing extra renders.
	noSignal := model.View()
	if strings.Contains(noSignal, "no rule matched") {
		t.Fatalf("detail claimed a probe miss for a never-sampled row:\n%s", noSignal)
	}

	// Sampled, no rule matched: a miss strictly newer than the row's current
	// verdict is the freshest evidence deck has and must be surfaced with its
	// own age, without touching status/status_source/status_at.
	clock.Advance()
	missAt := clock.Now().UnixMilli()
	model.sessions[0].LastProbeAt = missAt
	sampled := model.View()
	if !strings.Contains(sampled, "Probe:              sampled, no rule matched (just now)") {
		t.Fatalf("detail missing probe-miss copy with sample age:\n%s", sampled)
	}
	if !strings.Contains(sampled, "Status:             running") || !strings.Contains(sampled, "Verdict source:     hook") {
		t.Fatalf("probe miss copy changed status or status_source:\n%s", sampled)
	}

	// Superseded: once a fresher verdict lands (StatusAt advances past the
	// stale miss), the miss copy stops rendering — a probe.miss is never
	// shown once fresher evidence exists.
	model.sessions[0].StatusAt = missAt + 1
	superseded := model.View()
	if strings.Contains(superseded, "no rule matched") {
		t.Fatalf("detail showed a stale probe miss superseded by a fresher verdict:\n%s", superseded)
	}
}

// TestDetailKeyTogglesAndEscCloses proves i opens and closes the detail
// pane, and Esc also closes it without disturbing the list.
func TestDetailKeyTogglesAndEscCloses(t *testing.T) {
	model := New(nil, config.Settings{}, "")
	model.sessions = []store.Session{{Name: "s", Agent: "shell", Status: "running"}}
	updated, _ := model.Update(key("i"))
	model = updated.(Model)
	if !model.detail {
		t.Fatal("i did not open detail")
	}
	updated, _ = model.Update(key("i"))
	model = updated.(Model)
	if model.detail {
		t.Fatal("i did not close detail")
	}
	model.detail = true
	updated, _ = model.Update(key("esc"))
	if updated.(Model).detail {
		t.Fatal("Esc did not close detail")
	}
}
