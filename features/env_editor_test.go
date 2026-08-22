package features

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/cucumber/godog"
)

// registerEnvEditorSteps exposes task 020's `e` env editor (SPEC
// §6.1/§6.3): opening it for a named session, and asserting a session's
// durable env map directly against the store, independent of anything the
// screen shows.
func registerEnvEditorSteps(sc *godog.ScenarioContext) {
	sc.Step(`^deck client "([^"]+)" opens the env editor for session "([^"]+)"$`, clientOpensEnvEditorForSession)
	sc.Step(`^the state database session "([^"]+)" has no env key "([^"]+)"$`, sessionHasNoEnvKey)
}

// clientOpensEnvEditorForSession selects the named row and sends `e` (SPEC
// §6.1/§6.3), the only way envView opens.
func clientOpensEnvEditorForSession(ctx context.Context, clientName, sessionName string) error {
	if err := clientSelectsSessionByName(ctx, clientName, sessionName); err != nil {
		return err
	}
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	client, err := h.Client(clientName)
	if err != nil {
		return err
	}
	if err := client.Send("e"); err != nil {
		return err
	}
	return client.WaitForFrame(ctx, false, "Environment for "+sessionName)
}

// sessionHasNoEnvKey reads the sessions.env JSON column directly (never
// Go internals) and asserts the named key is absent, proving an env
// editor that never writes anything (task 020 is read-only; task 021
// adds the write path) left the session's own env map untouched.
func sessionHasNoEnvKey(ctx context.Context, name, key string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	db, err := openObservedDatabase(h)
	if err != nil {
		return err
	}
	defer db.Close()
	var envJSON string
	if err := db.QueryRowContext(ctx, `SELECT env FROM sessions WHERE name = ?`, name).Scan(&envJSON); err != nil {
		return fmt.Errorf("observe session %q env: %w", name, err)
	}
	var env map[string]string
	if err := json.Unmarshal([]byte(envJSON), &env); err != nil {
		return fmt.Errorf("parse session %q env JSON %q: %w", name, envJSON, err)
	}
	if _, present := env[key]; present {
		return fmt.Errorf("session %q env unexpectedly has key %q (value %q) after an env-editor-only interaction", name, key, env[key])
	}
	return nil
}
