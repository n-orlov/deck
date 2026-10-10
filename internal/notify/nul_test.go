package notify

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// nulCase sets one env-bound field of the request to a value with a NUL in
// the middle and names the variable the capture script must then see.
type nulCase struct {
	variable string
	set      func(*Request, string)
	want     string
}

func nulCases() []nulCase {
	return []nulCase{
		{"DECK_SESSION_ID", func(r *Request, v string) { r.Session.ID = v }, "sess1"},
		{"DECK_SESSION_NAME", func(r *Request, v string) { r.Session.Name = v }, "apiname"},
		{"DECK_SESSION_SLUG", func(r *Request, v string) { r.Session.Slug = v }, "apislug"},
		{"DECK_SESSION_CWD", func(r *Request, v string) { r.Session.CWD = v }, "/work/api"},
		{"DECK_SESSION_AGENT", func(r *Request, v string) { r.Session.Agent = v }, "claude"},
		{"DECK_SESSION_GROUP", func(r *Request, v string) { r.Session.Group = v }, "infra"},
		{"DECK_SESSION_PROFILE", func(r *Request, v string) { r.Session.PermissionProfile = v }, "default"},
		{"DECK_SESSION_CONVERSATION_ID", func(r *Request, v string) { r.Session.ConversationID = v }, "conv9"},
		{"DECK_SESSION_LAUNCH_KIND", func(r *Request, v string) { r.Session.LaunchKind = v }, "resume"},
		{"DECK_EVENT_KIND", func(r *Request, v string) { r.Event.Kind = v }, "waiting"},
		{"DECK_EVENT_REASON", func(r *Request, v string) { r.Event.Reason = v }, "permissionprompt"},
		{"DECK_EVENT_MESSAGE", func(r *Request, v string) { r.Event.Message = v }, "needsapproval"},
	}
}

// withNUL splits want in the middle around a NUL byte.
func withNUL(want string) string {
	mid := len(want) / 2
	return want[:mid] + "\x00" + want[mid:]
}

// R241 (SPEC §10.1): a NUL in any env-bound string is stripped, so the spawn
// is not refused with "environment variable contains NUL" and the script runs
// and sees the event.
func TestSpawnStripsNULFromEveryEnvBoundField(t *testing.T) {
	for _, c := range nulCases() {
		t.Run(c.variable, func(t *testing.T) {
			path, dir := captureScript(t, "")
			req := baseRequest(path)
			c.set(&req, withNUL(c.want))
			res, err := Spawn(context.Background(), req)
			if err != nil || res.Failed() {
				t.Fatalf("Spawn with a NUL in %s = %+v, %v", c.variable, res, err)
			}
			raw := read(t, filepath.Join(dir, "env"))
			if got := envMap(raw)[c.variable]; got != c.want {
				t.Errorf("%s = %q, want %q (NUL stripped, the rest intact)", c.variable, got, c.want)
			}
			if got := read(t, filepath.Join(dir, "stdin")); strings.Contains(got, `\u0000`) || strings.Contains(got, "\x00") {
				t.Errorf("payload still carries a NUL: %q", got)
			}
			if c.variable == "DECK_EVENT_KIND" {
				if got := read(t, filepath.Join(dir, "argv")); got != "waiting\n" {
					t.Errorf("argv = %q, want the NUL-free kind", got)
				}
			}
		})
	}
}

// DECK_EVENT_AT is formatted from a time, so it can never carry a NUL; the
// row pins that it is exported, RFC 3339, beside NUL-carrying siblings.
func TestSpawnExportsEventAtBesideNULCarryingFields(t *testing.T) {
	path, dir := captureScript(t, "")
	req := baseRequest(path)
	req.Event.Message, req.Session.Name = "a\x00b", "x\x00y"
	if res, err := Spawn(context.Background(), req); err != nil || res.Failed() {
		t.Fatalf("Spawn = %+v, %v", res, err)
	}
	if got := envMap(read(t, filepath.Join(dir, "env")))["DECK_EVENT_AT"]; got != "2026-10-08T12:30:00Z" {
		t.Errorf("DECK_EVENT_AT = %q", got)
	}
}

// Every other NUL-carrying string the payload holds (the deck host and
// version, the session status and reason) is stripped too.
func TestSpawnStripsNULFromTheRemainingPayloadStrings(t *testing.T) {
	path, dir := captureScript(t, "")
	req := baseRequest(path)
	req.Deck.Host, req.Deck.Version = "bo\x00x", "v9\x00.9"
	req.Session.Status, req.Session.Reason = "wait\x00ing", "perm\x00ission"
	if res, err := Spawn(context.Background(), req); err != nil || res.Failed() {
		t.Fatalf("Spawn = %+v, %v", res, err)
	}
	stdin := read(t, filepath.Join(dir, "stdin"))
	for _, want := range []string{`"host":"box"`, `"version":"v9.9"`, `"status":"waiting"`, `"reason":"permission"`} {
		if !strings.Contains(stdin, want) {
			t.Errorf("payload lacks %s: %s", want, stdin)
		}
	}
}

// A secret split by a NUL is still removed: NUL is stripped before the
// by-value scrub, so the pieces cannot slip past it.
func TestSpawnScrubsASessionEnvValueSplitByNUL(t *testing.T) {
	path, dir := captureScript(t, "")
	req := baseRequest(path)
	req.SessionEnv = map[string]string{"API_TOKEN": "tok-secret-7731"} //nolint:gosec // G101: a fixture value
	req.Event.Message = "leaked tok-sec\x00ret-7731 here"
	if res, err := Spawn(context.Background(), req); err != nil || res.Failed() {
		t.Fatalf("Spawn = %+v, %v", res, err)
	}
	env := read(t, filepath.Join(dir, "env"))
	if strings.Contains(env, "tok-sec") {
		t.Errorf("a NUL-split session env value reached the script: %s", env)
	}
}

// The detached (session-end) spawn strips NUL on the same footing.
func TestStartStripsNULFromEveryEnvBoundField(t *testing.T) {
	path, dir := captureScript(t, `touch "$d/done"`)
	req := baseRequest(path)
	for _, c := range nulCases() {
		c.set(&req, withNUL(c.want))
	}
	if err := Start(req); err != nil {
		t.Fatalf("Start with NUL in every field: %v", err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		if _, err := os.Stat(filepath.Join(dir, "done")); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the detached script never finished")
		}
		time.Sleep(20 * time.Millisecond)
	}
	env := envMap(read(t, filepath.Join(dir, "env")))
	for _, c := range nulCases() {
		if env[c.variable] != c.want {
			t.Errorf("%s = %q, want %q", c.variable, env[c.variable], c.want)
		}
	}
}
