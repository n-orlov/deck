package main

import (
	"strings"
	"testing"
)

// TestLoadConfig_MissingFileIsUsageError proves a missing ci/quality.json
// is a usage error, not a silent "every gate off".
func TestLoadConfig_MissingFileIsUsageError(t *testing.T) {
	_, err := loadConfig("/does/not/exist/quality.json")
	if err == nil {
		t.Fatalf("loadConfig on a missing file: want an error, got nil")
	}
}

// TestLoadConfig_MalformedJSONIsUsageError proves malformed JSON is a
// usage error.
func TestLoadConfig_MalformedJSONIsUsageError(t *testing.T) {
	path := writeTempFile(t, "{not json")
	_, err := loadConfig(path)
	if err == nil {
		t.Fatalf("loadConfig on malformed JSON: want an error, got nil")
	}
}

// TestLoadConfig_ParsesEveryField proves every field R187's criterion
// (1) names round-trips through loadConfig.
func TestLoadConfig_ParsesEveryField(t *testing.T) {
	path := writeTempFile(t, `{
		"coverage": {"enabled": true, "total_floor": 85, "package_floor": 80, "fixture_floor": 50},
		"crap": {"enabled": true, "ceiling": 30, "fixture_ceiling": 30}
	}`)
	cfg, err := loadConfig(path)
	if err != nil {
		t.Fatalf("loadConfig: unexpected error: %v", err)
	}
	if !cfg.Coverage.Enabled || cfg.Coverage.TotalFloor != 85 || cfg.Coverage.PackageFloor != 80 || cfg.Coverage.FixtureFloor != 50 {
		t.Errorf("coverage config = %+v, want enabled total=85 package=80 fixture=50", cfg.Coverage)
	}
	if !cfg.Crap.Enabled || cfg.Crap.Ceiling != 30 || cfg.Crap.FixtureCeiling != 30 {
		t.Errorf("crap config = %+v, want enabled ceiling=30 fixture_ceiling=30", cfg.Crap)
	}
}

// TestRun_EveryGateOffPassesAndSaysNothingEnabled proves the checked-in
// shape (every gate off) exits 0 and names that nothing is enabled,
// rather than silently printing an empty report.
func TestRun_EveryGateOffPassesAndSaysNothingEnabled(t *testing.T) {
	path := writeTempFile(t, `{
		"coverage": {"enabled": false, "total_floor": 85, "package_floor": 80, "fixture_floor": 50},
		"crap": {"enabled": false, "ceiling": 30, "fixture_ceiling": 30}
	}`)
	report, exitCode, err := run(path, "")
	if err != nil {
		t.Fatalf("run: unexpected error: %v", err)
	}
	if exitCode != 0 {
		t.Fatalf("exitCode = %d, want 0 (every gate off must pass)", exitCode)
	}
	if !strings.Contains(report, "no gates are enabled") {
		t.Errorf("report does not say no gates are enabled:\n%s", report)
	}
}

// TestRun_MissingConfigPathIsUsageError proves an empty -config is
// caught before anything else runs.
func TestRun_MissingConfigPathIsUsageError(t *testing.T) {
	_, exitCode, err := run("", "")
	if err == nil {
		t.Fatalf("run with no -config: want an error, got nil")
	}
	if exitCode != 2 {
		t.Errorf("exitCode = %d, want 2 (usage error)", exitCode)
	}
}
