package theme

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeThemeFile(t *testing.T, dir, filename, content string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	path := filepath.Join(dir, filename)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return path
}

const validUserTheme = `name = "midnight"
appearance = "dark"

[colors]
background        = "#000000"
surface            = "#111111"
border             = "#222222"
border_focus       = "#333333"
selection          = "#444444"
selection_idle     = "#555555"
title              = "#666666"
text               = "#777777"
dimmed             = "#888888"
hint               = "#999999"
key                = "#aaaaaa"
accent             = "#bbbbbb"
group              = "#cccccc"
search_match       = "#dddddd"
badge              = "#eeeeee"
badge_warn         = "#ffffff"
waiting            = "#010101"
running            = "#020202"
idle               = "#030303"
starting           = "#040404"
stopped            = "#050505"
error              = "#060606"
archived           = "#070707"
`

func TestDiscoverUserThemesMissingDirIsNotAnError(t *testing.T) {
	themes, errs := DiscoverUserThemes(filepath.Join(t.TempDir(), "does-not-exist"))
	if len(themes) != 0 || len(errs) != 0 {
		t.Fatalf("expected empty discovery for missing dir, got themes=%v errs=%v", themes, errs)
	}
}

func TestDiscoverUserThemesFindsValidTheme(t *testing.T) {
	dir := t.TempDir()
	writeThemeFile(t, dir, "midnight.toml", validUserTheme)

	themes, errs := DiscoverUserThemes(dir)
	if len(errs) != 0 {
		t.Fatalf("unexpected errs: %v", errs)
	}
	th, ok := themes["midnight"]
	if !ok {
		t.Fatalf("expected theme %q discovered, got %v", "midnight", themes)
	}
	if th.Appearance != "dark" {
		t.Fatalf("appearance = %q, want dark", th.Appearance)
	}
}

func TestDiscoverUserThemesReportsUnparseableFileNamingIt(t *testing.T) {
	dir := t.TempDir()
	broken := `name = "broken"
appearance = "dark"

[colors]
background = "#000000"
surface = "not-a-hex-color"
`
	path := writeThemeFile(t, dir, "broken.toml", broken)

	themes, errs := DiscoverUserThemes(dir)
	if _, ok := themes["broken"]; ok {
		t.Fatalf("broken theme should not have been discovered as valid")
	}
	if len(errs) != 1 {
		t.Fatalf("expected exactly one error, got %v", errs)
	}
	e := errs[0]
	if e.Path != path {
		t.Fatalf("error path = %q, want %q", e.Path, path)
	}
	if e.Name != "broken" {
		t.Fatalf("error name = %q, want %q (peeked from the name line before the failure)", e.Name, "broken")
	}
	if e.Err == nil {
		t.Fatalf("expected a non-nil underlying error")
	}
}

func TestDiscoverUserThemesUnrecoverableNameOnEarlyFailure(t *testing.T) {
	dir := t.TempDir()
	// The name line itself is malformed (unquoted), so peekDeclaredName
	// cannot recover any name at all.
	broken := `name = midnight
appearance = "dark"
`
	writeThemeFile(t, dir, "unnamed.toml", broken)

	_, errs := DiscoverUserThemes(dir)
	if len(errs) != 1 {
		t.Fatalf("expected exactly one error, got %v", errs)
	}
	if errs[0].Name != "" {
		t.Fatalf("expected no recoverable name, got %q", errs[0].Name)
	}
}

func TestDiscoverUserThemesDuplicateNameLaterFileWins(t *testing.T) {
	dir := t.TempDir()
	first := strings.Replace(validUserTheme, "#000000", "#101010", 1)
	second := strings.Replace(validUserTheme, "#000000", "#202020", 1)
	writeThemeFile(t, dir, "a-first.toml", first)
	writeThemeFile(t, dir, "b-second.toml", second)

	themes, errs := DiscoverUserThemes(dir)
	if len(errs) != 0 {
		t.Fatalf("unexpected errs: %v", errs)
	}
	th, ok := themes["midnight"]
	if !ok {
		t.Fatalf("expected theme %q discovered", "midnight")
	}
	if th.Colors[Background] != "#202020" {
		t.Fatalf("expected the alphabetically later file (b-second.toml) to win, got background=%s", th.Colors[Background])
	}
}

func TestResolveEmptyNameIsDefaultWithNoReason(t *testing.T) {
	th, reason := Resolve(nil, nil, "")
	if th.Name != Default().Name {
		t.Fatalf("theme = %q, want default %q", th.Name, Default().Name)
	}
	if reason != "" {
		t.Fatalf("reason = %q, want empty (no fallback occurred)", reason)
	}
}

func TestResolveFindsUserTheme(t *testing.T) {
	dir := t.TempDir()
	writeThemeFile(t, dir, "midnight.toml", validUserTheme)
	userThemes, errs := DiscoverUserThemes(dir)
	if len(errs) != 0 {
		t.Fatalf("unexpected errs: %v", errs)
	}

	th, reason := Resolve(userThemes, errs, "midnight")
	if th.Name != "midnight" {
		t.Fatalf("theme = %q, want midnight", th.Name)
	}
	if reason != "" {
		t.Fatalf("reason = %q, want empty (theme found, no fallback)", reason)
	}
}

func TestResolveFindsBuiltinTheme(t *testing.T) {
	th, reason := Resolve(nil, nil, "daylight")
	if th.Name != "daylight" {
		t.Fatalf("theme = %q, want daylight", th.Name)
	}
	if reason != "" {
		t.Fatalf("reason = %q, want empty", reason)
	}
}

func TestResolveUserThemeShadowsBuiltinOfSameName(t *testing.T) {
	dir := t.TempDir()
	// A user theme file that declares the SAME name as an embedded
	// built-in ("empire"), but with visibly different colours.
	shadow := strings.Replace(validUserTheme, `name = "midnight"`, `name = "empire"`, 1)
	writeThemeFile(t, dir, "empire.toml", shadow)
	userThemes, errs := DiscoverUserThemes(dir)
	if len(errs) != 0 {
		t.Fatalf("unexpected errs: %v", errs)
	}

	th, reason := Resolve(userThemes, errs, "empire")
	if reason != "" {
		t.Fatalf("reason = %q, want empty", reason)
	}
	builtinEmpire, _ := Builtin("empire")
	if th.Colors[Background] == builtinEmpire.Colors[Background] {
		t.Fatalf("expected the user theme to shadow the built-in, got the built-in's background %s", th.Colors[Background])
	}
	if th.Colors[Background] != "#000000" {
		t.Fatalf("background = %q, want the user file's %q", th.Colors[Background], "#000000")
	}
}

func TestResolveUnknownNameFallsBackWithReason(t *testing.T) {
	th, reason := Resolve(nil, nil, "no-such-theme")
	if th.Name != Default().Name {
		t.Fatalf("theme = %q, want default %q", th.Name, Default().Name)
	}
	if reason == "" {
		t.Fatalf("expected a non-empty fallback reason")
	}
	if !strings.Contains(reason, "no-such-theme") {
		t.Fatalf("reason %q does not name the unknown theme", reason)
	}
}

func TestResolveUnparseableFileFallsBackNamingTheFile(t *testing.T) {
	dir := t.TempDir()
	broken := `name = "broken"
appearance = "dark"

[colors]
background = "#000000"
surface = "not-a-hex-color"
`
	path := writeThemeFile(t, dir, "broken.toml", broken)
	userThemes, errs := DiscoverUserThemes(dir)
	if _, ok := userThemes["broken"]; ok {
		t.Fatalf("broken theme should not have discovered as valid")
	}

	th, reason := Resolve(userThemes, errs, "broken")
	if th.Name != Default().Name {
		t.Fatalf("theme = %q, want default %q", th.Name, Default().Name)
	}
	if reason == "" {
		t.Fatalf("expected a non-empty fallback reason")
	}
	if !strings.Contains(reason, path) {
		t.Fatalf("reason %q does not name the file %q", reason, path)
	}
	if !strings.Contains(reason, "broken") {
		t.Fatalf("reason %q does not name the requested theme", reason)
	}
}

func TestThemesDirDerivesFromConfigFilePath(t *testing.T) {
	got := ThemesDir("/home/user/.config/deck/config.toml")
	want := filepath.Join("/home/user/.config/deck", "themes")
	if got != want {
		t.Fatalf("ThemesDir = %q, want %q", got, want)
	}
}
