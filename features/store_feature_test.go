package features

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/cucumber/godog"
	_ "modernc.org/sqlite"
)

// registerStoreFeatureSteps constructs fixtures and observes the released
// binary through SQLite and its stderr; it deliberately does not import deck's
// store package or schema constants.
func registerStoreFeatureSteps(sc *godog.ScenarioContext) {
	sc.Step(`^the scenario has an older supported database fixture$`, olderDatabaseFixture)
	sc.Step(`^the scenario has a newer unsupported database fixture$`, newerDatabaseFixture)
	sc.Step(`^the released deck binary opens the newer database$`, releasedBinaryOpensNewerDatabase)
	sc.Step(`^it clearly refuses the newer database$`, clearlyRefusesNewerDatabase)
	sc.Step(`^the newer database fixture remains unchanged$`, newerDatabaseRemainsUnchanged)
}

func writeDatabaseFixture(h *ScenarioHarness, version int) error {
	path := filepath.Join(h.Home, "state.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return fmt.Errorf("open external schema fixture: %w", err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE meta (key TEXT PRIMARY KEY, version INTEGER NOT NULL); INSERT INTO meta VALUES ('schema_version', ?);`, version); err != nil {
		return fmt.Errorf("create external schema v%d fixture: %w", version, err)
	}
	return nil
}

func olderDatabaseFixture(ctx context.Context) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	return writeDatabaseFixture(h, 0)
}

func newerDatabaseFixture(ctx context.Context) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	if err := writeDatabaseFixture(h, 2); err != nil {
		return err
	}
	h.databaseFixture, err = os.ReadFile(filepath.Join(h.Home, "state.db"))
	if err != nil {
		return fmt.Errorf("snapshot newer external fixture: %w", err)
	}
	return nil
}

func releasedBinaryOpensNewerDatabase(ctx context.Context) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, h.Binary)
	cmd.Env = append(os.Environ(), h.Environment()...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("run released binary against newer fixture: %w: %s", err, strings.TrimSpace(string(output)))
	}
	h.newerRefusal = string(output)
	return nil
}

func clearlyRefusesNewerDatabase(ctx context.Context) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	if !strings.Contains(h.newerRefusal, "newer than supported") {
		return fmt.Errorf("released binary did not clearly refuse newer database: %q", h.newerRefusal)
	}
	return nil
}

func newerDatabaseRemainsUnchanged(ctx context.Context) error {
	h, err := assertionHarness(ctx)
	if err != nil {
		return err
	}
	got, err := os.ReadFile(filepath.Join(h.Home, "state.db"))
	if err != nil {
		return fmt.Errorf("read newer fixture after refusal: %w", err)
	}
	if string(got) != string(h.databaseFixture) {
		return fmt.Errorf("newer database fixture was modified by refusal")
	}
	return nil
}
