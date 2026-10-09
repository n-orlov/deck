package notify

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

const basePath = "PATH=/usr/local/bin:/usr/bin:/bin"

// script writes an executable sh script into a temp dir and returns its path.
func script(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "hook.sh")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

// captureScript records argv (one per line), env, stdin and the cwd-free
// facts into files next to itself, then exits with status.
func captureScript(t *testing.T, extra string) (path, dir string) {
	t.Helper()
	dir = t.TempDir()
	body := `d=` + strconv.Quote(dir) + `
for a in "$@"; do printf '%s\n' "$a"; done > "$d/argv"
env > "$d/env"
cat > "$d/stdin"
` + extra
	return script(t, body), dir
}

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func envMap(raw string) map[string]string {
	out := map[string]string{}
	for _, line := range strings.Split(raw, "\n") {
		if k, v, ok := strings.Cut(line, "="); ok {
			out[k] = v
		}
	}
	return out
}

var at = time.Date(2026, 10, 8, 12, 30, 0, 0, time.UTC)

func baseRequest(command ...string) Request {
	return Request{
		Command: command,
		Session: Session{
			ID: "sess-1", Name: "api", Slug: "api", CWD: "/work/api", Agent: "claude",
			Group: "infra", PermissionProfile: "default", Status: "waiting", Reason: "permission_prompt",
			Important: true, LaunchKind: "resume",
		},
		Event:   Event{Kind: "waiting", Reason: "permission_prompt", Message: "needs approval", At: at},
		Deck:    Deck{Host: "box", Version: "v9.9.9"},
		BaseEnv: []string{basePath, "KEEP=yes", "DECK_SESSION_ID=stale", "DECK_EVENT_KIND=stale"},
		Timeout: 10 * time.Second,
	}
}

func TestSpawnPassesKindAsArgvEnvAndVersionedJSONOnStdin(t *testing.T) {
	path, dir := captureScript(t, "")
	req := baseRequest(path, "--fixed")
	res, err := Spawn(context.Background(), req)
	if err != nil || res.Failed() {
		t.Fatalf("Spawn = %+v, %v", res, err)
	}
	if got := read(t, filepath.Join(dir, "argv")); got != "waiting\n--fixed\n" {
		t.Errorf("argv = %q, want the kind as argv[1], then the configured fixed argument", got)
	}
	env := envMap(read(t, filepath.Join(dir, "env")))
	for k, want := range map[string]string{
		"DECK_SESSION_ID": "sess-1", "DECK_SESSION_NAME": "api", "DECK_SESSION_SLUG": "api",
		"DECK_SESSION_CWD": "/work/api", "DECK_SESSION_AGENT": "claude", "DECK_SESSION_GROUP": "infra",
		"DECK_SESSION_PROFILE": "default", "DECK_SESSION_LAUNCH_KIND": "resume",
		"DECK_SESSION_CONVERSATION_ID": "", "DECK_EVENT_KIND": "waiting",
		"DECK_EVENT_REASON": "permission_prompt", "DECK_EVENT_MESSAGE": "needs approval",
		"DECK_EVENT_AT": "2026-10-08T12:30:00Z", "KEEP": "yes",
	} {
		if got, ok := env[k]; !ok || got != want {
			t.Errorf("env %s = %q (present %v), want %q exported", k, got, ok, want)
		}
	}
	var p struct {
		Version int `json:"version"`
		Session struct {
			ID, Name, CWD, Agent, Status, Reason, Group string
			PermissionProfile                           string `json:"permission_profile"`
			Important                                   bool
		}
		Event struct{ Kind, At, Reason, Message string }
		Deck  struct{ Host, Version string }
	}
	if err := json.Unmarshal([]byte(read(t, filepath.Join(dir, "stdin"))), &p); err != nil {
		t.Fatal(err)
	}
	if p.Version != 1 || p.Session.ID != "sess-1" || p.Session.Name != "api" || p.Session.CWD != "/work/api" ||
		p.Session.Agent != "claude" || p.Session.Status != "waiting" || p.Session.Reason != "permission_prompt" ||
		p.Session.PermissionProfile != "default" || p.Session.Group != "infra" || !p.Session.Important {
		t.Errorf("payload session/version = %+v", p)
	}
	if p.Event.Kind != "waiting" || p.Event.At != "2026-10-08T12:30:00Z" || p.Event.Reason != "permission_prompt" ||
		p.Event.Message != "needs approval" || p.Deck.Host != "box" || p.Deck.Version != "v9.9.9" {
		t.Errorf("payload event/deck = %+v", p)
	}
}

func TestSpawnArgumentWithShellMetacharactersArrivesVerbatim(t *testing.T) {
	path, dir := captureScript(t, "")
	marker := filepath.Join(t.TempDir(), "injected")
	nasty := "$(touch " + marker + "); `touch " + marker + "` | cat & \"q\" 'x' ; *"
	req := baseRequest(path, nasty)
	if res, err := Spawn(context.Background(), req); err != nil || res.Failed() {
		t.Fatalf("Spawn = %+v, %v", res, err)
	}
	if got, want := read(t, filepath.Join(dir, "argv")), "waiting\n"+nasty+"\n"; got != want {
		t.Errorf("argv = %q, want %q verbatim", got, want)
	}
	if _, err := os.Stat(marker); err == nil {
		t.Error("a shell interpreted the argument: the injected command ran")
	}
}

func alive(pid int) bool {
	stat, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return false
	}
	_, after, _ := strings.Cut(string(stat), ") ")
	return !strings.HasPrefix(after, "Z")
}

func TestSpawnTimeoutKillsTheWholeProcessGroup(t *testing.T) {
	pidFile := filepath.Join(t.TempDir(), "child.pid")
	path := script(t, "sleep 60 &\necho $! > "+strconv.Quote(pidFile)+"\nsleep 60")
	req := baseRequest(path)
	req.Timeout = 400 * time.Millisecond
	start := time.Now()
	res, err := Spawn(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if !res.TimedOut || !res.Failed() || res.ExitCode != -1 {
		t.Errorf("result = %+v, want a timeout fact", res)
	}
	if took := time.Since(start); took > 10*time.Second {
		t.Errorf("Spawn took %v: it waited on the forked child's pipe", took)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(read(t, pidFile)))
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for alive(pid) && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if alive(pid) {
		t.Errorf("the forked child %d survived the timeout: only the direct child was killed", pid)
	}
}

func TestSpawnCapsTheOutputTail(t *testing.T) {
	path := script(t, `i=0; while [ $i -lt 3000 ]; do echo "line-$i-padding-padding"; i=$((i+1)); done; echo "the-end" >&2`)
	req := baseRequest(path)
	req.OutputCap = 64
	res, err := Spawn(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Output) > 64 || !res.Truncated {
		t.Errorf("output len %d truncated %v, want <= 64 and truncated", len(res.Output), res.Truncated)
	}
	if !strings.HasSuffix(res.Output, "line-2999-padding-padding\nthe-end\n") && !strings.HasSuffix(res.Output, "the-end\n") {
		t.Errorf("output = %q, want the tail including the stderr end", res.Output)
	}
	if strings.Contains(res.Output, "line-0-") {
		t.Errorf("output kept the head: %q", res.Output)
	}
}

func TestSpawnMasksAndRedactsTheMessage(t *testing.T) {
	path, dir := captureScript(t, "")
	req := baseRequest(path)
	req.SessionEnv = map[string]string{"REGION": "eu-north-7"}
	req.Event.Message = "failed with API_TOKEN=abc123 and password: hunter2 in region eu-north-7; build=ok " + strings.Repeat("é", 40)
	req.MessageCap = 100
	if res, err := Spawn(context.Background(), req); err != nil || res.Failed() {
		t.Fatalf("Spawn = %+v, %v", res, err)
	}
	env := envMap(read(t, filepath.Join(dir, "env")))
	stdin := read(t, filepath.Join(dir, "stdin"))
	var p struct{ Event struct{ Message string } }
	if err := json.Unmarshal([]byte(stdin), &p); err != nil {
		t.Fatal(err)
	}
	for name, got := range map[string]string{"env": env["DECK_EVENT_MESSAGE"], "payload": p.Event.Message} {
		for _, leak := range []string{"abc123", "hunter2", "eu-north-7"} {
			if strings.Contains(got, leak) {
				t.Errorf("%s message leaks %q: %q", name, leak, got)
			}
		}
		if !strings.Contains(got, "API_TOKEN="+MaskedPlaceholder) || !strings.Contains(got, "build=ok") {
			t.Errorf("%s message = %q, want the secret value masked and the plain pair kept", name, got)
		}
		if len(got) > 100+len("…") || !strings.HasSuffix(got, "…") {
			t.Errorf("%s message len %d = %q, want truncated at the cap", name, len(got), got)
		}
	}
}

func TestSpawnWithholdsTheMessageForASensitiveSession(t *testing.T) {
	path, dir := captureScript(t, "")
	req := baseRequest(path)
	req.Session.Sensitive = true
	req.Event.Message = "the assistant said something private"
	if res, err := Spawn(context.Background(), req); err != nil || res.Failed() {
		t.Fatalf("Spawn = %+v, %v", res, err)
	}
	env := envMap(read(t, filepath.Join(dir, "env")))
	if got, ok := env["DECK_EVENT_MESSAGE"]; !ok || got != "" {
		t.Errorf("DECK_EVENT_MESSAGE = %q (present %v), want exported empty", got, ok)
	}
	if stdin := read(t, filepath.Join(dir, "stdin")); strings.Contains(stdin, "private") || !strings.Contains(stdin, `"message":""`) {
		t.Errorf("payload = %s, want the message withheld", stdin)
	}
}

func TestSpawnMissingOrNonExecutableScriptIsARecordableError(t *testing.T) {
	plain := filepath.Join(t.TempDir(), "plain.sh")
	if err := os.WriteFile(plain, []byte("#!/bin/sh\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cases := map[string]Request{
		"missing":         baseRequest(filepath.Join(t.TempDir(), "absent.sh")),
		"not executable":  baseRequest(plain),
		"directory":       baseRequest(t.TempDir()),
		"unknown on PATH": baseRequest("deck-no-such-hook-binary"),
		"no command":      baseRequest(),
		"empty path":      baseRequest(""),
		"unoffered kind":  func() Request { r := baseRequest(plain); r.Event.Kind = "prompt"; return r }(),
	}
	for name, req := range cases {
		t.Run(name, func(t *testing.T) {
			res, err := Spawn(context.Background(), req)
			if err == nil || !strings.HasPrefix(err.Error(), "event hook: ") {
				t.Fatalf("err = %v, want a recordable event hook error", err)
			}
			if res.ExitCode != -1 || !res.Failed() || res.Output != "" {
				t.Errorf("result = %+v, want a not-started fact", res)
			}
		})
	}
}

func TestNoSessionEnvValueReachesTheEnvPayloadOrRecord(t *testing.T) {
	const secret = "s3cr3t-value-0f9a"
	const plain = "plainvalue42"
	path, dir := captureScript(t, `echo "out: $DECK_EVENT_MESSAGE"; echo "err: leaked=`+secret+`" >&2`)
	req := baseRequest(path)
	req.SessionEnv = map[string]string{"DB_PASSWORD": secret, "PLAIN": plain}
	req.Event.Message = "tried " + secret + " then " + plain
	// Seed both values into every string the caller hands over that can
	// reach the script's environment or payload.
	seed := func(field *string) { *field += "-" + secret + "-" + plain }
	s := &req.Session
	for _, field := range []*string{
		&s.ID, &s.Name, &s.Slug, &s.CWD, &s.Agent, &s.Group, &s.PermissionProfile,
		&s.ConversationID, &s.LaunchKind, &s.Status, &s.Reason,
		&req.Event.Reason, &req.Deck.Host, &req.Deck.Version,
	} {
		seed(field)
	}
	res, err := Spawn(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	record, _ := json.Marshal(res)
	for name, haystack := range map[string]string{
		"env": read(t, filepath.Join(dir, "env")), "payload": read(t, filepath.Join(dir, "stdin")),
		"record": string(record) + res.Output,
	} {
		for _, value := range []string{secret, plain} {
			if strings.Contains(haystack, value) {
				t.Errorf("%s contains the session env value %q", name, value)
			}
		}
	}
	if !strings.Contains(res.Output, "out: tried "+MaskedPlaceholder+" then "+MaskedPlaceholder) {
		t.Errorf("output = %q, want the scrubbed message echoed", res.Output)
	}
	if env := envMap(read(t, filepath.Join(dir, "env"))); env["DECK_SESSION_NAME"] != "api-"+MaskedPlaceholder+"-"+MaskedPlaceholder {
		t.Errorf("DECK_SESSION_NAME = %q, want the rest of the name kept around the scrubbed values", env["DECK_SESSION_NAME"])
	}
}

func TestANotStartedErrorCarriesNoSessionEnvValue(t *testing.T) {
	const envValue = "zq-dir-value-77"
	req := baseRequest(filepath.Join(t.TempDir(), envValue, "hook.sh"))
	req.SessionEnv = map[string]string{"TOKEN": envValue}
	_, err := Spawn(context.Background(), req)
	if err == nil || strings.Contains(err.Error(), envValue) || !strings.HasPrefix(err.Error(), "event hook: ") {
		t.Fatalf("err = %v, want a recordable error without the session env value", err)
	}
	if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("err = %v, want the cause kept in the chain", err)
	}
}

func TestSpawnReportsANonZeroExitAsAFactNotAnError(t *testing.T) {
	path := script(t, "echo boom; exit 7")
	res, err := Spawn(context.Background(), baseRequest(path))
	if err != nil || res.ExitCode != 7 || !res.Failed() || res.TimedOut || !strings.Contains(res.Output, "boom") {
		t.Errorf("result = %+v, err = %v, want exit 7 with the output", res, err)
	}
	killed := script(t, "kill -9 $$")
	if res, err := Spawn(context.Background(), baseRequest(killed)); err != nil || res.ExitCode != -1 || !res.Failed() {
		t.Errorf("signal death result = %+v, err = %v", res, err)
	}
}

func TestSpawnDefaultsTheCapsAndInheritsNothingStale(t *testing.T) {
	path := script(t, `echo "$DECK_SESSION_ID $DECK_EVENT_KIND"`)
	req := baseRequest(path)
	req.Timeout, req.OutputCap, req.MessageCap = 0, 0, 0
	res, err := Spawn(context.Background(), req)
	if err != nil || strings.TrimSpace(res.Output) != "sess-1 waiting" {
		t.Errorf("result = %+v, err = %v, want the owned variables to win over the inherited ones", res, err)
	}
}

func TestSpawnPathLookupAndStartFailure(t *testing.T) {
	// A bare name resolves on PATH like an agent binary.
	res, err := Spawn(context.Background(), func() Request {
		r := baseRequest("true")
		return r
	}())
	if err != nil || res.Failed() {
		t.Errorf("true: %+v, %v", res, err)
	}
	// An interpreter that does not exist makes the start itself fail.
	bad := filepath.Join(t.TempDir(), "bad.sh")
	if err := os.WriteFile(bad, []byte("#!/no/such/interpreter\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := Spawn(context.Background(), baseRequest(bad)); err == nil || !strings.HasPrefix(err.Error(), "event hook: start ") {
		t.Errorf("err = %v, want a start failure", err)
	}
}

// shortEnv holds session env values below any plausible length floor, one
// secret-shaped key and one plain key (SPEC §6.4: env values never appear, with
// no length exception).
var shortEnv = map[string]string{"DB_PASSWORD": "Q", "PLAIN": "zj", "EMPTY": ""}

// seedShort puts every short env value into every string the caller hands over
// that can reach the script's environment, payload or record.
func seedShort(req *Request) {
	req.Event.Message = "tried Q and zj"
	s := &req.Session
	for _, field := range []*string{
		&s.ID, &s.Name, &s.Slug, &s.CWD, &s.Agent, &s.Group, &s.PermissionProfile,
		&s.ConversationID, &s.LaunchKind, &s.Status, &s.Reason,
		&req.Event.Reason, &req.Deck.Host, &req.Deck.Version,
	} {
		*field += "-Q-zj"
	}
}

func TestShortSessionEnvValuesNeverReachTheEnvPayloadOrRecord(t *testing.T) {
	path, dir := captureScript(t, `echo "out: $DECK_EVENT_MESSAGE"; echo "err: Q zj" >&2`)
	req := baseRequest(path)
	req.SessionEnv = shortEnv
	seedShort(&req)
	res, err := Spawn(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	record, _ := json.Marshal(res)
	for name, haystack := range map[string]string{
		"env": read(t, filepath.Join(dir, "env")), "payload": read(t, filepath.Join(dir, "stdin")),
		"record": string(record) + res.Output,
	} {
		for _, value := range []string{"Q", "zj"} {
			if strings.Contains(haystack, value) {
				t.Errorf("%s contains the short session env value %q: %q", name, value, haystack)
			}
		}
	}
	if env := envMap(read(t, filepath.Join(dir, "env"))); env["DECK_EVENT_MESSAGE"] != "tried "+MaskedPlaceholder+" and "+MaskedPlaceholder {
		t.Errorf("DECK_EVENT_MESSAGE = %q, want both short values removed", env["DECK_EVENT_MESSAGE"])
	}
}

func TestAnEnvValueInheritedFromTheAgentEnvironmentIsNotPassedOn(t *testing.T) {
	path, dir := captureScript(t, "")
	req := baseRequest(path)
	req.SessionEnv = map[string]string{"MODE": "7", "API_TOKEN": "long-inherited-secret-1"}
	req.BaseEnv = append(req.BaseEnv, "MODE=7", "API_TOKEN=long-inherited-secret-1", "MODEL=kept")
	if _, err := Spawn(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	env := envMap(read(t, filepath.Join(dir, "env")))
	for _, key := range []string{"MODE", "API_TOKEN"} {
		if v, ok := env[key]; ok {
			t.Errorf("inherited %s=%q reached the script's environment", key, v)
		}
	}
	if env["MODEL"] != "kept" || env["KEEP"] != "yes" {
		t.Errorf("unrelated inherited variables were dropped: %v", env)
	}
}

func TestANotStartedErrorCarriesNoShortSessionEnvValue(t *testing.T) {
	req := baseRequest(filepath.Join(t.TempDir(), "QQ-dir", "hook.sh"))
	req.SessionEnv = map[string]string{"TOKEN": "Q"}
	_, err := Spawn(context.Background(), req)
	if err == nil || strings.Contains(err.Error(), "Q") {
		t.Fatalf("err = %v, want a recordable error without the short session env value", err)
	}
	if err := Start(req); err == nil || strings.Contains(err.Error(), "Q") {
		t.Fatalf("Start err = %v, want the same protection on the detached path", err)
	}
}
