package config

import (
	"testing"

	"github.com/n-orlov/deck/internal/theme"
)

// TestResolveProfileNamePrecedence pins SPEC §3.4's stated precedence:
// positional argument, then DECK_PROFILE, then DefaultProfile -- with an
// empty DECK_PROFILE treated as unset, never as an (invalid) empty name.
func TestResolveProfileNamePrecedence(t *testing.T) {
	cases := []struct {
		name       string
		positional string
		env        map[string]string
		want       string
	}{
		{name: "nothing set", positional: "", env: map[string]string{}, want: DefaultProfile},
		{name: "empty DECK_PROFILE is unset", positional: "", env: map[string]string{"DECK_PROFILE": ""}, want: DefaultProfile},
		{name: "DECK_PROFILE alone", positional: "", env: map[string]string{"DECK_PROFILE": "work"}, want: "work"},
		{name: "positional alone", positional: "home", env: map[string]string{}, want: "home"},
		{name: "positional wins over DECK_PROFILE", positional: "home", env: map[string]string{"DECK_PROFILE": "work"}, want: "home"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ResolveProfileName(tc.positional, environment(tc.env))
			if got != tc.want {
				t.Fatalf("ResolveProfileName(%q, %v) = %q, want %q", tc.positional, tc.env, got, tc.want)
			}
		})
	}
}

// TestNamedProfilePathsInsertProfilesSegment pins that resolvePaths (via
// LoadFromProfile) inserts "profiles/<name>/" for a named profile only, in
// both XDG and DECK_HOME modes, per SPEC §3.4's table -- and that the
// default profile's own layout is completely unaffected (still the flat,
// byte-for-byte paths the existing oracle table pins).
func TestNamedProfilePathsInsertProfilesSegment(t *testing.T) {
	const injectedHome = "/oracle-home"
	fixedHome := func() (string, error) { return injectedHome, nil }

	t.Run("xdg named profile", func(t *testing.T) {
		settings, err := LoadFromProfile(environment(map[string]string{}), fixedHome, "work")
		if err != nil {
			t.Fatal(err)
		}
		wantData := "/oracle-home/.local/share/deck/profiles/work"
		wantConfig := "/oracle-home/.config/deck/profiles/work/config.toml"
		wantLog := "/oracle-home/.local/state/deck/profiles/work/log"
		wantStateDB := "/oracle-home/.local/share/deck/profiles/work/state.db"
		if settings.Paths.Home != wantData || settings.Paths.DataDir != wantData {
			t.Errorf("Home/DataDir = %q/%q, want %q", settings.Paths.Home, settings.Paths.DataDir, wantData)
		}
		if settings.Paths.ConfigFile != wantConfig {
			t.Errorf("ConfigFile = %q, want %q", settings.Paths.ConfigFile, wantConfig)
		}
		if settings.Paths.LogDir != wantLog {
			t.Errorf("LogDir = %q, want %q", settings.Paths.LogDir, wantLog)
		}
		if settings.Paths.StateDB != wantStateDB {
			t.Errorf("StateDB = %q, want %q", settings.Paths.StateDB, wantStateDB)
		}
	})

	t.Run("deck home named profile", func(t *testing.T) {
		settings, err := LoadFromProfile(environment(map[string]string{"DECK_HOME": "/oracle-deck-home"}), failIfCalled(t), "work")
		if err != nil {
			t.Fatal(err)
		}
		wantRoot := "/oracle-deck-home/profiles/work"
		if settings.Paths.Home != wantRoot || settings.Paths.DataDir != wantRoot {
			t.Errorf("Home/DataDir = %q/%q, want %q", settings.Paths.Home, settings.Paths.DataDir, wantRoot)
		}
		if settings.Paths.ConfigFile != wantRoot+"/config.toml" {
			t.Errorf("ConfigFile = %q, want %q", settings.Paths.ConfigFile, wantRoot+"/config.toml")
		}
		if settings.Paths.LogDir != wantRoot+"/log" {
			t.Errorf("LogDir = %q, want %q", settings.Paths.LogDir, wantRoot+"/log")
		}
		if settings.Paths.StateDB != wantRoot+"/state.db" {
			t.Errorf("StateDB = %q, want %q", settings.Paths.StateDB, wantRoot+"/state.db")
		}
	})

	t.Run("default profile is unaffected in both modes", func(t *testing.T) {
		xdg, err := LoadFromProfile(environment(map[string]string{}), fixedHome, "default")
		if err != nil {
			t.Fatal(err)
		}
		if xdg.Paths.Home != "/oracle-home/.local/share/deck" {
			t.Errorf("default XDG Home = %q, want flat layout unchanged", xdg.Paths.Home)
		}
		deckHome, err := LoadFromProfile(environment(map[string]string{"DECK_HOME": "/oracle-deck-home"}), failIfCalled(t), "")
		if err != nil {
			t.Fatal(err)
		}
		if deckHome.Paths.Home != "/oracle-deck-home" {
			t.Errorf("default DECK_HOME Home = %q, want flat layout unchanged", deckHome.Paths.Home)
		}
	})
}

// TestNamedProfileSocket pins the derived socket for a named profile
// (deck-<name>) unless DECK_TMUX_SOCKET is set, in which case it always
// wins outright -- even for the default profile, matching the existing
// oracle table's own DECK_TMUX_SOCKET case.
func TestNamedProfileSocket(t *testing.T) {
	fixedHome := func() (string, error) { return "/oracle-home", nil }

	settings, err := LoadFromProfile(environment(map[string]string{}), fixedHome, "work")
	if err != nil {
		t.Fatal(err)
	}
	if settings.Socket != "deck-work" {
		t.Fatalf("Socket = %q, want %q", settings.Socket, "deck-work")
	}

	overridden, err := LoadFromProfile(environment(map[string]string{"DECK_TMUX_SOCKET": "custom"}), fixedHome, "work")
	if err != nil {
		t.Fatal(err)
	}
	if overridden.Socket != "custom" {
		t.Fatalf("Socket with DECK_TMUX_SOCKET override = %q, want %q", overridden.Socket, "custom")
	}
}

// TestNamedProfileSharedClockPathFollowsProfileRoot pins that the frozen
// clock's shared path (used to keep concurrent processes on one profile
// agreeing on the same frozen instant) is rooted under that profile's own
// data directory, never the default profile's, so two profiles never
// share a clock (SPEC §3.4).
func TestNamedProfileSharedClockPathFollowsProfileRoot(t *testing.T) {
	fixedHome := func() (string, error) { return "/oracle-home", nil }

	settings, err := LoadFromProfile(environment(map[string]string{}), fixedHome, "work")
	if err != nil {
		t.Fatal(err)
	}
	want := "/oracle-home/.local/share/deck/profiles/work/clock.now"
	if settings.Clock.sharedPath != want {
		t.Fatalf("Clock.sharedPath = %q, want %q", settings.Clock.sharedPath, want)
	}
	if settings.Clock.sharedPath == settings.Paths.Home+"/../clock.now" {
		t.Fatalf("Clock.sharedPath unexpectedly matches a non-profile path")
	}
}

// TestSettingsExposesResolvedProfileName pins that Settings.Profile carries
// whichever name ResolveProfileName picked, for both the default and a
// named profile, through every entry point (LoadFrom, LoadFromProfile).
func TestSettingsExposesResolvedProfileName(t *testing.T) {
	fixedHome := func() (string, error) { return "/oracle-home", nil }

	viaLoadFrom, err := LoadFrom(environment(map[string]string{}), fixedHome)
	if err != nil {
		t.Fatal(err)
	}
	if viaLoadFrom.Profile != DefaultProfile {
		t.Fatalf("LoadFrom Profile = %q, want %q", viaLoadFrom.Profile, DefaultProfile)
	}

	viaEnv, err := LoadFrom(environment(map[string]string{"DECK_PROFILE": "work"}), fixedHome)
	if err != nil {
		t.Fatal(err)
	}
	if viaEnv.Profile != "work" {
		t.Fatalf("LoadFrom with DECK_PROFILE Profile = %q, want %q", viaEnv.Profile, "work")
	}

	viaPositional, err := LoadFromProfile(environment(map[string]string{"DECK_PROFILE": "work"}), fixedHome, "home")
	if err != nil {
		t.Fatal(err)
	}
	if viaPositional.Profile != "home" {
		t.Fatalf("LoadFromProfile positional Profile = %q, want %q", viaPositional.Profile, "home")
	}
}

// TestThemesDirSharedAcrossProfiles pins that a named profile's resolved
// Settings still discovers user themes in the default profile's themes/
// directory (SPEC §3.4: "a theme is colour data, not state"), never its
// own profiles/<name>/themes/.
func TestThemesDirSharedAcrossProfiles(t *testing.T) {
	fixedHome := func() (string, error) { return "/oracle-home", nil }

	named, err := LoadFromProfile(environment(map[string]string{}), fixedHome, "work")
	if err != nil {
		t.Fatal(err)
	}
	deft, err := LoadFromProfile(environment(map[string]string{}), fixedHome, "default")
	if err != nil {
		t.Fatal(err)
	}

	// The two profiles' config files differ (profiles/work/ is inserted
	// for the named one)...
	if named.Paths.ConfigFile == deft.Paths.ConfigFile {
		t.Fatalf("expected the two profiles' config files to differ, both were %q", named.Paths.ConfigFile)
	}
	// ... but theme.ThemesDir, given each profile's own resolved
	// ConfigFile, must still land on the exact same shared directory --
	// this is what LoadFromProfile actually feeds theme.DiscoverUserThemes
	// for every profile.
	if got, want := theme.ThemesDir(named.Paths.ConfigFile), theme.ThemesDir(deft.Paths.ConfigFile); got != want {
		t.Fatalf("ThemesDir(named profile's config) = %q, want %q (the default profile's own)", got, want)
	}
	if want := "/oracle-home/.config/deck/themes"; theme.ThemesDir(named.Paths.ConfigFile) != want {
		t.Fatalf("ThemesDir(named profile's config) = %q, want %q", theme.ThemesDir(named.Paths.ConfigFile), want)
	}
}

// TestDataRootIsTheSharedRootAndRoundTripsThroughAPaneEnvironment pins
// SPEC §6.1's "DECK_HOME and DECK_PROFILE name the data root and the
// profile a hook writes to": Settings.DataRoot is the root every profile
// nests under (never a named profile's own profiles/<name>/ directory),
// and a hook process whose environment carries exactly that pair --
// DECK_HOME=DataRoot, DECK_PROFILE=<name>, no positional -- resolves the
// very state.db the launching profile opened, in both modes, default
// included. Before this existed a named pane carried
// DECK_HOME=$root/profiles/<name> and its hook resolved
// $root/profiles/<name>/profiles/<name>/, a profile that does not exist.
func TestDataRootIsTheSharedRootAndRoundTripsThroughAPaneEnvironment(t *testing.T) {
	fixedHome := func() (string, error) { return "/oracle-home", nil }
	cases := []struct {
		name     string
		env      map[string]string
		profile  string
		wantRoot string
	}{
		{name: "deck home named", env: map[string]string{"DECK_HOME": "/oracle-deck-home"}, profile: "acme", wantRoot: "/oracle-deck-home"},
		{name: "deck home default", env: map[string]string{"DECK_HOME": "/oracle-deck-home"}, profile: "", wantRoot: "/oracle-deck-home"},
		{name: "xdg named", env: map[string]string{}, profile: "acme", wantRoot: "/oracle-home/.local/share/deck"},
		{name: "xdg default", env: map[string]string{}, profile: "default", wantRoot: "/oracle-home/.local/share/deck"},
		{name: "xdg data home named", env: map[string]string{"XDG_DATA_HOME": "/xdg-data"}, profile: "work", wantRoot: "/xdg-data/deck"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			launched, err := LoadFromProfile(environment(tc.env), fixedHome, tc.profile)
			if err != nil {
				t.Fatal(err)
			}
			if launched.DataRoot != tc.wantRoot {
				t.Fatalf("DataRoot = %q, want %q", launched.DataRoot, tc.wantRoot)
			}
			paneEnv := map[string]string{}
			for key, value := range tc.env {
				paneEnv[key] = value
			}
			paneEnv["DECK_HOME"] = launched.DataRoot
			paneEnv["DECK_PROFILE"] = launched.Profile
			hook, err := LoadFromProfile(environment(paneEnv), failIfCalled(t), "")
			if err != nil {
				t.Fatal(err)
			}
			if hook.Profile != launched.Profile {
				t.Fatalf("hook profile = %q, want %q", hook.Profile, launched.Profile)
			}
			if hook.Paths.StateDB != launched.Paths.StateDB {
				t.Fatalf("hook StateDB = %q, want the launching profile's own %q", hook.Paths.StateDB, launched.Paths.StateDB)
			}
		})
	}
}
