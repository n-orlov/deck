package features

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
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
			Format:   godogFormat(),
			Paths:    godogPaths(),
			Tags:     tags,
			Strict:   true,
			TestingT: t,
		},
	}
	if suite.Run() != 0 {
		t.Fatal("godog feature suite failed")
	}
}

// godogFormat returns the godog Format string TestFeatures uses. It is
// exactly "pretty" -- unchanged from before DECK_GODOG_JUNIT existed --
// unless DECK_GODOG_JUNIT names a path, in which case godog's own
// "pretty,junit:<path>" multi-formatter syntax is used: the terminal output
// stays pretty, and godog's built-in JUnit formatter additionally writes one
// <testcase> per scenario to <path> (task 014, R146). Local runs that never
// set the variable are byte-for-byte unaffected.
func godogFormat() string {
	path := strings.TrimSpace(os.Getenv("DECK_GODOG_JUNIT"))
	if path == "" {
		return "pretty"
	}
	return "pretty,junit:" + path
}

// godogPaths returns the feature paths the suite runs. It is the whole
// package directory unless DECK_GODOG_PATHS names specific feature files
// (comma separated, relative to this package), which is a diagnostic knob for
// running one feature targeted -- notably
// interactive_sigwinch_budget.feature, whose scenarios carry NO tags and so
// cannot be selected with DECK_GODOG_TAGS at all, which used to leave the
// entire ~5 minute suite as the only way to exercise them. Each comma-
// separated entry may also carry a trailing ":<line>" (godog's own path:line
// syntax, e.g. "mouse.feature:42") to select the single scenario at that
// line instead of every scenario in the file (task 014, R146).
//
// It never shrinks what the ordinary run covers: unset -- the case for
// `go test ./features/`, for CI and for every deliverable suite run -- means
// []string{"."}, exactly as before. defaultTags remains the one authority for
// which scenarios the ordinary invocation runs, and a targeted path list is
// evidence about those files only, never a substitute for a whole-suite run.
func godogPaths() []string {
	override := strings.TrimSpace(os.Getenv("DECK_GODOG_PATHS"))
	if override == "" {
		return []string{"."}
	}
	var paths []string
	for _, candidate := range strings.Split(override, ",") {
		if candidate = strings.TrimSpace(candidate); candidate != "" {
			paths = append(paths, candidate)
		}
	}
	if len(paths) == 0 {
		return []string{"."}
	}
	return paths
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
	registerCodexHooksSteps(sc)
	registerPermissionModesCodexSteps(sc)
	registerProbeStatusSteps(sc)
	registerCrashStatusSteps(sc)
	registerAttentionSortSteps(sc)
	registerSortOrderSteps(sc)
	registerNewSessionSelectionSteps(sc)
	registerStatusRecoverySteps(sc)
	registerTerminalRepairFieldRouteSteps(sc)
	registerSettingsSteps(sc)
	registerDialogsSteps(sc)
	registerEnvEditorSteps(sc)
	registerLaunchInputsEditorSteps(sc)
	registerKillDeleteUndoSteps(sc)
	registerKillDeleteUndoFingerprintSteps(sc)
	registerTeardownHooksSteps(sc)
	registerAttachScrollSteps(sc)
	registerAttachCropSteps(sc)
	registerNoLeakScanSteps(sc)
	registerEventLogSteps(sc)
	registerFilterSteps(sc)
	registerTmuxOptionScopeSteps(sc)
	registerPaneHistoryLimitSteps(sc)
	registerInteractiveGeometrySteps(sc)
	registerInteractiveSigwinchBudgetSteps(sc)
	registerInteractiveOptionTableSteps(sc)
	registerInteractiveFocusSteps(sc)
	registerInteractiveRefusalsSteps(sc)
	registerInteractiveScrollSteps(sc)
	registerInteractiveSelectionSteps(sc)
	registerInteractiveRetargetSteps(sc)
	registerInteractivePipeLeakSteps(sc)
	registerInteractiveForceAttachSteps(sc)
}

// TestGodogFormatBuildsJUnitOnlyWhenTheEnvVarIsSet pins the option builder
// behind DECK_GODOG_JUNIT: unset, TestFeatures' Format stays exactly "pretty"
// as before the variable existed; set, it becomes godog's own
// "pretty,junit:<path>" multi-formatter string, verbatim, whatever the path's
// own shape (task 014, R146).
func TestGodogFormatBuildsJUnitOnlyWhenTheEnvVarIsSet(t *testing.T) {
	t.Run("unset stays exactly pretty", func(t *testing.T) {
		t.Setenv("DECK_GODOG_JUNIT", "")
		if got := godogFormat(); got != "pretty" {
			t.Fatalf("godogFormat() = %q, want exactly %q", got, "pretty")
		}
	})
	t.Run("set adds the junit multi-formatter at that path", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "junit.xml")
		t.Setenv("DECK_GODOG_JUNIT", path)
		want := "pretty,junit:" + path
		if got := godogFormat(); got != want {
			t.Fatalf("godogFormat() = %q, want %q", got, want)
		}
	})
}

// TestGodogJUnitFormatterWritesOneTestcasePerScenario runs a tiny two-
// scenario suite through godog's own "pretty,junit:<path>" Format string --
// the exact shape godogFormat() produces -- and asserts the written file
// holds one <testcase> per scenario, both named, neither ever a place TUI
// implementation code participates (task 014, R146).
func TestGodogJUnitFormatterWritesOneTestcasePerScenario(t *testing.T) {
	dir := t.TempDir()
	featurePath := filepath.Join(dir, "two.feature")
	feature := "Feature: two scenarios\n" +
		"  Scenario: first scenario passes\n" +
		"    Given a passing step\n" +
		"  Scenario: second scenario passes\n" +
		"    Given a passing step\n"
	if err := os.WriteFile(featurePath, []byte(feature), 0o600); err != nil {
		t.Fatal(err)
	}
	junitPath := filepath.Join(dir, "junit.xml")

	suite := godog.TestSuite{
		ScenarioInitializer: func(sc *godog.ScenarioContext) {
			sc.Step(`^a passing step$`, func() error { return nil })
		},
		Options: &godog.Options{
			Format: "pretty,junit:" + junitPath,
			Paths:  []string{featurePath},
			Strict: true,
		},
	}
	if status := suite.Run(); status != 0 {
		t.Fatalf("suite.Run() = %d, want 0", status)
	}

	contents, err := os.ReadFile(junitPath)
	if err != nil {
		t.Fatalf("reading junit output: %v", err)
	}
	if got := strings.Count(string(contents), "<testcase "); got != 2 {
		t.Fatalf("junit output has %d <testcase> elements, want exactly 2:\n%s", got, contents)
	}
	if !strings.Contains(string(contents), `name="first scenario passes"`) {
		t.Fatalf("junit output missing the first scenario's own testcase name:\n%s", contents)
	}
	if !strings.Contains(string(contents), `name="second scenario passes"`) {
		t.Fatalf("junit output missing the second scenario's own testcase name:\n%s", contents)
	}
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
