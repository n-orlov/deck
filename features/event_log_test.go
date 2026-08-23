package features

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/cucumber/godog"
)

// registerEventLogSteps backs features/event_log.feature (task 124, I-9,
// SPEC \u00a712/requirement 32): opening the `E` event log, asserting screen
// text ordering (newest-first), and seeding a raw events-table row this
// package's own product code has no path to write today (SPEC's "env
// values never enter events" rule means no real write path carries a
// secret-shaped value), so the view's defensive masking is checked against
// the SHAPE of payload it must handle regardless of which writer produces
// it, the same way task 011's own scan is pointed at a real secret-shaped
// value rather than only a synthetic string.
func registerEventLogSteps(sc *godog.ScenarioContext) {
	sc.Step(`^deck client "([^"]+)" opens the event log$`, clientOpensEventLog)
	sc.Step(`^deck client "([^"]+)" screen text "([^"]+)" appears above screen text "([^"]+)"$`, clientScreenTextAppearsAboveText)
	sc.Step(`^the state database has a raw event with kind "([^"]+)" reason "([^"]+)", secret-shaped key "([^"]+)" value "([^"]+)", and ordinary key "([^"]+)" value "([^"]+)"$`, stateDatabaseHasRawEventWithSecretShapedAndOrdinaryKeys)
}

// clientOpensEventLog sends `E` (SPEC \u00a712/requirement 32, task 124), the
// only way eventLogView opens.
func clientOpensEventLog(ctx context.Context, clientName string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	client, err := h.Client(clientName)
	if err != nil {
		return err
	}
	if err := client.Send("E"); err != nil {
		return err
	}
	return client.WaitForFrame(ctx, false, "Event log")
}

// clientScreenTextAppearsAboveText locates both strings via FindText --
// the same real-grid cell scan task 011's own no-leak instrument uses,
// never a substring search over the NormalizeFrame-trimmed Frame() string
// -- and asserts upper's row is strictly above lower's, proving an actual
// rendered ORDER rather than merely that both strings are present
// somewhere on screen.
func clientScreenTextAppearsAboveText(ctx context.Context, clientName, upper, lower string) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	client, err := h.Client(clientName)
	if err != nil {
		return err
	}
	upperRow, _, err := client.FindText(upper)
	if err != nil {
		return fmt.Errorf("locate %q on deck client %q's screen: %w", upper, clientName, err)
	}
	lowerRow, _, err := client.FindText(lower)
	if err != nil {
		return fmt.Errorf("locate %q on deck client %q's screen: %w", lower, clientName, err)
	}
	if upperRow >= lowerRow {
		return fmt.Errorf("deck client %q: %q (row %d) is not above %q (row %d)", clientName, upper, upperRow, lower, lowerRow)
	}
	return nil
}

// stateDatabaseHasRawEventWithSecretShapedAndOrdinaryKeys inserts a row
// directly into the events table (session_id NULL, an orphan-shaped row --
// the event log renders these identically to a session-scoped row, since
// it never joins against sessions) whose payload is a JSON object built
// here (never hand-quoted in the .feature file, where embedded double
// quotes would break Gherkin's own step-argument matching) carrying one
// secret-shaped key's value and one ordinary key's value side by side.
// This is a fixture, not a call through Store.RecordOrphanEvent, so the
// scenario can plant a payload SHAPE (a JSON object carrying a
// secret-shaped key's value) that no real write path in this codebase
// produces today -- SPEC's "env values never enter events" rule means
// SetSessionEnvValue's own recorded payload is a key NAME only -- proving
// the view's masking holds for the payload it is handed rather than only
// for the shapes deck itself happens to write.
func stateDatabaseHasRawEventWithSecretShapedAndOrdinaryKeys(ctx context.Context, kind, reason, secretKey, secretValue, ordinaryKey, ordinaryValue string) error {
	h, err := scenarioHarness(ctx)
	if err != nil {
		return err
	}
	db, err := openObservedDatabase(h)
	if err != nil {
		return err
	}
	defer db.Close()
	payload, err := json.Marshal(map[string]string{secretKey: secretValue, ordinaryKey: ordinaryValue})
	if err != nil {
		return fmt.Errorf("encode raw event fixture payload: %w", err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO events (session_id, at, kind, reason, payload) VALUES (NULL, ?, ?, ?, ?)`,
		time.Now().UnixMilli(), kind, reason, string(payload)); err != nil {
		return fmt.Errorf("insert raw event fixture: %w", err)
	}
	return nil
}
