package tui

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/n-orlov/deck/internal/agent"
	"github.com/n-orlov/deck/internal/config"
	"github.com/n-orlov/deck/internal/store"
)

// TestTranscriptPathForCodexPrefersSessionEnvOverConfigAndAmbient is task
// 007's regression, exercised through the production caller
// (m.transcriptPathFor -- the same function the `dd` purge-choice path at
// tui.go:2871 calls, never agent.Codex.TranscriptPaths directly) with all
// three CODEX_HOME layers SPEC §6.1 recognises in play at once and set to
// three DIFFERENT fixture trees, each containing a rollout file for the
// SAME conversation id:
//
//   - the ambient process environment (this test process's own $CODEX_HOME,
//     set with t.Setenv -- the "server env" layer resolveEnvKey falls back
//     to only when neither of the layers below supplies a value);
//   - the config [env] layer (config.Settings.Env["CODEX_HOME"]);
//   - the session's own Env["CODEX_HOME"] -- the layer that must win.
//
// Because every tree holds a same-id rollout file, a caller that
// consulted the wrong layer would still resolve *some* path rather than
// erroring, which is exactly why this guards something a single-tree test
// cannot: it proves the winning path is the session tree's own file, and
// explicitly that it is never the ambient tree's file (nor the config
// tree's).
//
// A second case then points the session's own CODEX_HOME at a tree that
// has rollout files but none for the conversation id being looked up,
// proving the miss still degrades to not-ok end to end through the same
// caller, rather than falling through to the ambient or config trees.
func TestTranscriptPathForCodexPrefersSessionEnvOverConfigAndAmbient(t *testing.T) {
	const id = "43ac9425-9b54-4c5d-8063-ac52768d0cdb"

	// writeRollout drops a well-formed rollout-*.jsonl file for id under
	// root/sessions/<yyyy>/<mm>/<dd>/, mirroring the codex-cli 0.154.0
	// spike convention (docs/reports/codex-cli-0.154.0-spike.md), and
	// returns its path.
	writeRollout := func(root string) string {
		dir := filepath.Join(root, "sessions", "2026", "01", "05")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(dir, "rollout-2026-01-05T09-00-00-"+id+".jsonl")
		if err := os.WriteFile(path, []byte("{}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		return path
	}

	ambientHome := t.TempDir()
	ambientPath := writeRollout(ambientHome)
	configHome := t.TempDir()
	configPath := writeRollout(configHome)
	sessionHome := t.TempDir()
	sessionPath := writeRollout(sessionHome)

	// The ambient process environment layer -- resolveEnvKey's lowest-
	// priority "server env" fallback. t.Setenv restores the previous
	// value (or absence) automatically at test end.
	t.Setenv("CODEX_HOME", ambientHome)

	registry := agent.NewRegistry()
	registry.Register(agent.NewCodex())

	settings := config.Settings{Env: map[string]string{"CODEX_HOME": configHome}}
	m := NewWithShellCreatorAttacherKillerResumerProfileSwitcherResumeModerAgentCreatorAndRegistry(
		nil, settings, "", nil, nil, nil, nil, nil, nil, nil, nil, registry,
	)

	session := store.Session{
		Name:           "codex-three-layers",
		Agent:          "codex",
		ConversationID: id,
		Env:            map[string]string{"CODEX_HOME": sessionHome},
	}

	got, ok := m.transcriptPathFor(session)
	if !ok {
		t.Fatalf("transcriptPathFor(%+v) declined; want it to resolve the session's own CODEX_HOME tree", session)
	}
	if got != sessionPath {
		t.Fatalf("transcriptPathFor(%+v) = %q, want the session-env tree's file %q", session, got, sessionPath)
	}
	if got == ambientPath {
		t.Fatalf("transcriptPathFor resolved the AMBIENT process-env fixture tree's file (%q); the session's own CODEX_HOME must win over the ambient environment", ambientPath)
	}
	if got == configPath {
		t.Fatalf("transcriptPathFor resolved the config [env] fixture tree's file (%q); the session's own CODEX_HOME must win over config [env]", configPath)
	}

	// Same three competing layers, but the session's own tree has no
	// rollout file for THIS conversation id -- the miss must still
	// degrade to not-ok through the production caller, never silently
	// falling through to the config or ambient trees (which do have a
	// same-id file, so a fallthrough bug would resolve one of those
	// instead of declining).
	noMatch := session
	noMatch.ConversationID = "no-such-conversation-id"
	if path, ok := m.transcriptPathFor(noMatch); ok {
		t.Fatalf("transcriptPathFor(%+v) = (%q, true), want (\"\", false): no rollout file for this id exists under the session's own CODEX_HOME tree", noMatch, path)
	}
}
