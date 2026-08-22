package features

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/cucumber/godog"
)

// defaultTags keeps expensive or environment-dependent scenarios out of the
// ordinary Go test command. They are deliberately opt-in CI jobs.
const defaultTags = "~@real-agents && ~@nightly"

func TestFeatures(t *testing.T) {
	// DECK_GODOG_TAGS lets an operator opt into scenarios excluded from the
	// default run (currently only @real-agents; see task 029 and
	// docs/reports/phase1.md), without changing defaultTags itself, which
	// stays the one authority for what the ordinary `go test` invocation
	// covers.
	tags := defaultTags
	if override := os.Getenv("DECK_GODOG_TAGS"); override != "" {
		tags = override
	}
	suite := godog.TestSuite{
		ScenarioInitializer: initializeScenario,
		Options: &godog.Options{
			Format:   "pretty",
			Paths:    []string{"."},
			Tags:     tags,
			Strict:   true,
			TestingT: t,
		},
	}
	if suite.Run() != 0 {
		t.Fatal("godog feature suite failed")
	}
}

// initializeScenario wires black-box harness steps. Godog rejects any feature
// step that has not been registered, rather than silently accepting it.
func initializeScenario(sc *godog.ScenarioContext) {
	registerScenarioLifecycle(sc)
	registerEmulatorPlacementSteps(sc)
	registerResizeSteps(sc)
	registerMouseSynthesisSteps(sc)
	registerMouseControlSteps(sc)
	registerMouseBindingSteps(sc)
	registerSidebarWidthSteps(sc)
	registerLayoutModeSteps(sc)
	registerPreviewSteps(sc)
	registerBlackBoxAssertionSteps(sc)
	registerCellAttributeSteps(sc)
	registerFrameBudgetSteps(sc)
	registerConfigFileSteps(sc)
	registerDeterminismSteps(sc)
	registerStoreFeatureSteps(sc)
	registerCreateTildeCWDSteps(sc)
	registerThemePinningSteps(sc)
	registerColorDepthSteps(sc)
	registerThemeFrameGeometrySteps(sc)
	registerFingerprintSteps(sc)
	registerCreateSessionCWDPrefillSteps(sc)
	registerCreateCWDGhostSteps(sc)
	registerCreateCWDTabSteps(sc)
	registerCreateBlankNameSteps(sc)
	registerCreateValidationSteps(sc)
	sc.Step(`^the Godog harness is available$`, func() error { return nil })
	sc.Step(`^the private tmux server is killed$`, func(ctx context.Context) error {
		harness, err := scenarioHarness(ctx)
		if err != nil {
			return err
		}
		return harness.KillTMuxServer(ctx)
	})
	registerFakeAgentDriftSteps(sc)
	registerRealAgentHookSteps(sc)
	registerFakeAgentFeatureSteps(sc)
	registerFakeAgentSizeSteps(sc)
	registerAgentSessionSteps(sc)
	registerHookContractSteps(sc)
	registerClaudeHookStatusSteps(sc)
	registerProbeStatusSteps(sc)
	registerCrashStatusSteps(sc)
	registerAttentionSortSteps(sc)
	registerStatusRecoverySteps(sc)
	registerSettingsSteps(sc)
	registerDialogsSteps(sc)
	registerEnvEditorSteps(sc)
}

func TestGodogRejectsUndefinedAndFailedSteps(t *testing.T) {
	for name, feature := range map[string]string{
		"undefined": "Feature: undefined step\n  Scenario: no binding\n    Given an unregistered step\n",
		"failed":    "Feature: failed step\n  Scenario: error binding\n    Given a failing step\n",
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "failure.feature")
			if err := os.WriteFile(path, []byte(feature), 0o600); err != nil {
				t.Fatal(err)
			}

			suite := godog.TestSuite{
				ScenarioInitializer: func(sc *godog.ScenarioContext) {
					if name == "failed" {
						sc.Step(`^a failing step$`, func() error { return errors.New("deliberate step failure") })
					}
				},
				Options: &godog.Options{Format: "progress", Paths: []string{path}, Strict: true},
			}
			if status := suite.Run(); status == 0 {
				t.Fatal("godog accepted a scenario that must fail")
			}
		})
	}
}
