package service

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/n-orlov/deck/internal/store"
)

// addEnvSession adds session s2 carrying env to the fixture's store.
func addEnvSession(t *testing.T, f hookDispatchFixture, status string, env map[string]string) {
	t.Helper()
	if _, err := f.db.CreateSession(context.Background(), store.CreateSessionInput{
		ID: "s2", Name: "web", CWD: f.out, Agent: "claude", CapturedPath: "/bin",
		Status: status, StatusAt: 1, CreatedAt: 1, ConversationID: "c2", Env: env,
	}); err != nil {
		t.Fatal(err)
	}
}

// SPEC §6.4, §10.1: a detached (session-end) spawn whose payload file cannot
// be created, because the temp dir `deck _hook` inherited is a session env
// value, records a not-started error in deck.jsonl without that value.
func TestDetachedEventHookAuditErrorCarriesNoSessionEnvValue(t *testing.T) {
	const secret = "/nonexistent-session-tmp-4417"
	f := newHookDispatchFixture(t, "stopped")
	logger := withHookAudit(t, &f)
	addEnvSession(t, f, "stopped", map[string]string{"TMPDIR": secret})
	t.Setenv("TMPDIR", secret)
	out := f.d.Dispatch(context.Background(), HookEvent{SessionID: "s2", StoredKind: "session_end", Reason: "logout", At: time.Now()}, true)
	if out.Spawned || out.Err == nil {
		t.Fatalf("detached dispatch = %+v, want a not-started error", out)
	}
	if strings.Contains(out.Err.Error(), secret) {
		t.Errorf("outcome error %q carries the session env value", out.Err)
	}
	lines := eventHookLogLines(t, logger)
	if len(lines) != 1 || lines[0]["error"] == nil {
		t.Fatalf("event_hook lines = %v, want one line with an error", lines)
	}
	if text, _ := lines[0]["error"].(string); strings.Contains(text, secret) {
		t.Errorf("audit error %q carries the session env value", text)
	}
}

// The attached spawn from the service never hands the script a session env
// value under an inherited alias, nor records one in the stored result.
func TestAttachedEventHookDropsAnInheritedAliasOfASessionEnvValue(t *testing.T) {
	const secret = "tok-alias-secret-9921"
	f := newHookDispatchFixture(t, "waiting")
	addEnvSession(t, f, "waiting", map[string]string{"API_TOKEN": secret})
	dump := filepath.Join(f.out, "env")
	script := filepath.Join(f.out, "dump.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nenv > "+dump+"\necho \"seen $AGENT_AUTH_COPY\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	f.d.Policy.Command = []string{script}
	f.d.BaseEnv = append(f.d.BaseEnv, "API_TOKEN="+secret, "AGENT_AUTH_COPY="+secret)
	out := f.d.Dispatch(context.Background(), HookEvent{SessionID: "s2", StoredKind: "notification", Reason: "permission_prompt", At: time.Now()}, false)
	if !out.Spawned || out.Err != nil {
		t.Fatalf("attached dispatch = %+v", out)
	}
	raw, err := os.ReadFile(dump)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), secret) || strings.Contains(out.Result.Output, secret) {
		t.Errorf("the session env value reached the script or its record: env %q output %q", raw, out.Result.Output)
	}
	if !strings.Contains(string(raw), "PATH=/usr/bin:/bin") {
		t.Errorf("unrelated inherited PATH was dropped: %q", raw)
	}
}
