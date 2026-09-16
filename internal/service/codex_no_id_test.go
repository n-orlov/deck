package service

import (
	"context"
	"strings"
	"testing"
)

// TestKillingAndRecreatingAnIDlessCodexRowIsAFreshLaunchNotAResume proves
// PRD R125's last bullet: codex mints its own conversation id on the
// agent's first prompt, never at launch, so a freshly created codex row
// carries no id yet (Caps.AssignsConversationID false). Killing that row
// before it ever got one, then creating a brand-new session (never
// resuming the killed one -- R125's own "r/R refuse" bullet is proven at
// the internal/tui layer), is an ordinary fresh launch: the new row's own
// launch argv is codex's Launch shape, never Resume's, and carries no
// trace of the killed row's identity.
func TestKillingAndRecreatingAnIDlessCodexRowIsAFreshLaunchNotAResume(t *testing.T) {
	cwd := t.TempDir()
	stubExecutableOnPath(t, "codex")
	service, _, logger, _ := newAgentTestService(t, nil, "codex-no-id-recreate")

	created, err := service.CreateAgent(context.Background(), AgentCreateInput{
		Name: "Codex: no id yet", CWD: cwd, Agent: "codex", PermissionProfile: "safe",
	})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	if created.ConversationID != "" {
		t.Fatalf("codex row minted a conversation id at create time, want none yet (R125): %q", created.ConversationID)
	}

	// Kill it before codex's own first prompt would ever fire SessionStart:
	// the row is stopped with no conversation to resume from.
	if err := service.Kill(context.Background(), created); err != nil {
		t.Fatalf("kill: %v", err)
	}

	// Re-create rather than resume the killed row: a brand-new session, same
	// agent kind and directory. This never touches the killed row's id --
	// there is nothing to resume, so nothing here even reads it.
	recreated, err := service.CreateAgent(context.Background(), AgentCreateInput{
		Name: "Codex: no id yet (again)", CWD: cwd, Agent: "codex", PermissionProfile: "safe",
	})
	if err != nil {
		t.Fatalf("re-create agent: %v", err)
	}
	if recreated.ID == created.ID {
		t.Fatal("re-created session reused the killed row's durable id")
	}
	if recreated.ConversationID != "" {
		t.Fatalf("re-created codex row carries a conversation id from nowhere: %q", recreated.ConversationID)
	}

	records := auditRecords(t, logger.Path())
	launchArgvs := map[string][]string{}
	for _, record := range records {
		if record["event"] == "launch" {
			sessionID, _ := record["session_id"].(string)
			launchArgvs[sessionID] = jsonStrings(record["argv"])
		}
	}
	recreatedArgv, ok := launchArgvs[recreated.ID]
	if !ok {
		t.Fatalf("no launch record for the re-created session; records = %#v", records)
	}
	if len(recreatedArgv) == 0 || recreatedArgv[0] != "codex" {
		t.Fatalf("re-created session launch argv = %#v, want codex's Launch shape", recreatedArgv)
	}
	for _, token := range recreatedArgv {
		if token == "resume" {
			t.Fatalf("re-created session's argv is codex's Resume shape, not a fresh Launch: %#v", recreatedArgv)
		}
		if strings.Contains(token, created.ID) {
			t.Fatalf("re-created session's argv %#v carries the killed row's own id %q", recreatedArgv, created.ID)
		}
	}
}
