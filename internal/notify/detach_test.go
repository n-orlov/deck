package notify

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/n-orlov/deck/internal/config"
)

func waitFile(t *testing.T, path string) string {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if raw, err := os.ReadFile(path); err == nil && len(raw) > 0 {
			return string(raw)
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("%s never appeared", path)
	return ""
}

func TestStartRunsTheScriptDetachedWithTheSamePayloadAsSpawn(t *testing.T) {
	path, dir := captureScript(t, "sleep 1\necho finished > \"$d/finished\"")
	started := time.Now()
	if err := Start(baseRequest(path, "--fixed")); err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(started); elapsed >= time.Second {
		t.Fatalf("Start took %s: it waited for the script", elapsed)
	}
	var p struct {
		Version int
		Event   struct{ Kind string }
	}
	if err := json.Unmarshal([]byte(waitFile(t, filepath.Join(dir, "stdin"))), &p); err != nil || p.Version != 1 || p.Event.Kind != "waiting" {
		t.Fatalf("stdin = %+v, %v", p, err)
	}
	if got := read(t, filepath.Join(dir, "argv")); got != "waiting\n--fixed\n" {
		t.Errorf("argv = %q", got)
	}
	if env := envMap(read(t, filepath.Join(dir, "env"))); env["DECK_EVENT_KIND"] != "waiting" || env["DECK_SESSION_ID"] != "sess-1" {
		t.Errorf("env kind/session = %q/%q", env["DECK_EVENT_KIND"], env["DECK_SESSION_ID"])
	}
	waitFile(t, filepath.Join(dir, "finished"))
}

// Start returns inside its one-second bound whatever the script does with its
// inputs and outputs afterwards: one that never reads its stdin, one that
// outlives the request's timeout (a detached script has none) and one that
// writes far more than a pipe holds to stdout and stderr.
func TestStartReturnsWithinTheBoundWhateverTheScriptDoesAfterwards(t *testing.T) {
	cases := map[string]struct {
		body    string
		timeout time.Duration
	}{
		"never reads stdin":        {"sleep 1\necho finished > \"$d/finished\"", 0},
		"outlives the timeout":     {"sleep 1\necho finished > \"$d/finished\"", time.Millisecond},
		"floods stdout and stderr": {"head -c 2000000 /dev/zero | tr '\\0' x\nhead -c 2000000 /dev/zero | tr '\\0' y >&2\necho finished > \"$d/finished\"", 0},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			path := script(t, "d="+strconv.Quote(dir)+"\n"+tc.body)
			req := baseRequest(path)
			if tc.timeout > 0 {
				req.Timeout = tc.timeout
			}
			started := time.Now()
			if err := Start(req); err != nil {
				t.Fatal(err)
			}
			if elapsed := time.Since(started); elapsed >= time.Second {
				t.Fatalf("Start took %s: it waited for the script", elapsed)
			}
			if _, err := os.Stat(filepath.Join(dir, "finished")); err == nil && name != "floods stdout and stderr" {
				t.Fatal("the script had already finished when Start returned")
			}
			waitFile(t, filepath.Join(dir, "finished"))
		})
	}
}

// Start does not outlive-wait a script that is still running when it
// returns: repeated starts of a slow script each return inside the bound.
func TestStartOfSeveralSlowScriptsNeverAccumulatesTheirWait(t *testing.T) {
	path, dir := captureScript(t, "sleep 1\necho finished > \"$d/finished\"")
	started := time.Now()
	for i := 0; i < 4; i++ {
		if err := Start(baseRequest(path)); err != nil {
			t.Fatal(err)
		}
	}
	if elapsed := time.Since(started); elapsed >= time.Second {
		t.Fatalf("four Starts took %s together: they waited for the scripts", elapsed)
	}
	waitFile(t, filepath.Join(dir, "finished"))
}

// The detached spawn gives short session env values and inherited session
// env entries the same protection as the attached one (SPEC §6.4, §10.1).
func TestStartRemovesShortAndInheritedSessionEnvValues(t *testing.T) {
	path, dir := captureScript(t, "echo finished > \"$d/finished\"")
	req := baseRequest(path)
	req.SessionEnv = map[string]string{"DB_PASSWORD": "Q", "PLAIN": "zj", "MODE": "9"}
	req.BaseEnv = append(req.BaseEnv, "MODE=9")
	seedShort(&req)
	if err := Start(req); err != nil {
		t.Fatal(err)
	}
	waitFile(t, filepath.Join(dir, "finished"))
	env := read(t, filepath.Join(dir, "env"))
	for name, haystack := range map[string]string{"env": env, "payload": read(t, filepath.Join(dir, "stdin"))} {
		for _, value := range []string{"Q", "zj"} {
			if strings.Contains(haystack, value) {
				t.Errorf("%s contains the short session env value %q", name, value)
			}
		}
	}
	if _, ok := envMap(env)["MODE"]; ok {
		t.Error("an inherited session env entry reached the detached script")
	}
}

// A payload larger than any pipe buffer (64 KiB by default, 1 MiB at most
// on Linux) must not stall Start: the handoff may never wait on a reader, and
// the script must still read the whole payload after Start returned. Nothing
// is left behind in the temporary directory.
func TestStartHandsOverAPayloadLargerThanAPipeWithoutWaiting(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp)
	path, dir := captureScript(t, "sleep 1\necho finished > \"$d/finished\"")
	req := baseRequest(path)
	req.Session.Reason = strings.Repeat("r", 3<<20/2)
	done := make(chan error, 1)
	started := time.Now()
	go func() { done <- Start(req) }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Start blocked handing over a payload larger than a pipe")
	}
	if elapsed := time.Since(started); elapsed >= time.Second {
		t.Fatalf("Start took %s: it waited for the script", elapsed)
	}
	if entries, err := os.ReadDir(tmp); err != nil || len(entries) != 0 {
		t.Errorf("TMPDIR after Start = %v, %v; want the payload file already unlinked", entries, err)
	}
	waitFile(t, filepath.Join(dir, "finished"))
	var p struct{ Session struct{ Reason string } }
	if err := json.Unmarshal([]byte(read(t, filepath.Join(dir, "stdin"))), &p); err != nil || p.Session.Reason != req.Session.Reason {
		t.Fatalf("stdin decoded to a %d-byte reason (%v), want all %d bytes", len(p.Session.Reason), err, len(req.Session.Reason))
	}
}

func TestStartReportsAnUnwritablePayloadDirectoryAsNotStarted(t *testing.T) {
	t.Setenv("TMPDIR", filepath.Join(t.TempDir(), "missing"))
	path, dir := captureScript(t, "")
	if err := Start(baseRequest(path)); err == nil || !strings.Contains(err.Error(), "event hook: create payload file") {
		t.Fatalf("Start with no usable temporary directory = %v, want a not-started error", err)
	}
	time.Sleep(200 * time.Millisecond)
	if _, err := os.Stat(filepath.Join(dir, "argv")); err == nil {
		t.Error("the script ran although its payload could not be handed over")
	}
}

func TestStartScrubsSessionEnvValuesAndRefusesAnUnstartableScript(t *testing.T) {
	req := baseRequest(filepath.Join(t.TempDir(), "gone"))
	req.SessionEnv = map[string]string{"API_TOKEN": "s3cret-value"}
	req.Session.Name = "tok-s3cret-value"
	err := Start(req)
	if err == nil || strings.Contains(err.Error(), "s3cret-value") {
		t.Fatalf("Start of a missing script = %v, want a scrubbed error", err)
	}
	req.Event.Kind = "prompt"
	if err := Start(req); err == nil {
		t.Error("Start accepted a kind outside the offered set")
	}
	req = baseRequest(path0(t))
	req.Command = []string{filepath.Dir(path0(t))}
	if err := Start(req); err == nil {
		t.Error("Start accepted a directory")
	}
}

func path0(t *testing.T) string {
	t.Helper()
	return script(t, "exit 0")
}

func TestPolicyFromSettingsSplitsTheCommandOnWhitespace(t *testing.T) {
	got := PolicyFromSettings(config.Settings{EventHook: "  /bin/hook  --a   b ", EventHookDefault: true, EventHookEvents: []string{"idle"}})
	if len(got.Command) != 3 || got.Command[0] != "/bin/hook" || got.Command[2] != "b" || !got.Default || got.Events[0] != "idle" {
		t.Fatalf("policy = %+v", got)
	}
	if len(PolicyFromSettings(config.Settings{}).Command) != 0 {
		t.Error("an empty event_hook must be an inert policy")
	}
}

// A reason that is all KEY=VALUE pairs and separators is the costliest text
// for the free-text masking Start runs before handing the payload over; the
// detach bound must hold for it too, and the secrets must still be masked.
func TestStartHandsOverAReasonFullOfAssignmentsWithoutWaiting(t *testing.T) {
	path, dir := captureScript(t, "sleep 1\necho finished > \"$d/finished\"")
	req := baseRequest(path)
	req.Session.Reason = strings.Repeat("a=b API_TOKEN=hunter2 :: = ", 3<<20/2/26)
	started := time.Now()
	if err := Start(req); err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(started); elapsed >= time.Second {
		t.Fatalf("Start took %s: it waited for the script", elapsed)
	}
	waitFile(t, filepath.Join(dir, "finished"))
	if raw := read(t, filepath.Join(dir, "stdin")); strings.Contains(raw, "hunter2") || !strings.Contains(raw, "API_TOKEN="+MaskedPlaceholder) {
		t.Fatalf("payload did not mask the secret pairs (%d bytes)", len(raw))
	}
}
